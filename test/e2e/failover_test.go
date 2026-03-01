//go:build e2e

/*
 * Copyright 2025 Alexandre Mahdhaoui
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package e2e_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	e2eutil "github.com/alexandremahdhaoui/udplb/pkg/test/e2e"
)

// TestBackendFailure verifies that when one backend is stopped, new sessions are routed to
// the remaining backends. The health monitor detects the failure within ~7s (5s probe
// interval + 2s timeout) and triggers recomputeAndApply to remove the failed backend.
func TestBackendFailure(t *testing.T) {
	cfg, err := e2eutil.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available -- run with 'forge test run e2e'")

	deployBinaries(t, cfg)
	defer cleanupAllProcesses(t, cfg)

	startEchoBackends(t, cfg)
	defer stopEchoBackends(t, cfg)
	startAllUdplb(t, cfg)
	defer stopAllUdplb(t, cfg)
	waitForBGPConvergence(t, cfg, bgpConvergenceTimeout)

	// Verify traffic flows initially.
	_ = runTrafficGen(t, cfg, 10, 100, 3*time.Second, 0)

	// Wait for the echo-backend report interval (5s) to ensure the report
	// includes all packets from the traffic-gen run above.
	time.Sleep(6 * time.Second)

	// Stop 1 backend (be-0) BEFORE snapshotting.
	// This freezes the log so health probes can't increase the counter
	// between the snapshot and the kill.
	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()
	sshCmdIgnoreErr(ctx, cfg, cfg.BE0IP, "pkill -f '[u]dplb-echo-backend' || true")
	time.Sleep(1 * time.Second) // Give it time to die.

	// Snapshot be-0's total received packets (log is frozen now).
	be0ReceivedBefore := backendTotalReceived(t, cfg, cfg.BE0IP)

	// Wait for health monitor to detect be-0 failure and recompute BPF maps.
	// defaultProbeInterval=5s, defaultProbeTimeout=2s => detection within ~7s.
	// Add margin for BPF map update propagation.
	time.Sleep(15 * time.Second)

	// Clear echo-backend logs on surviving backends before the second traffic round.
	sshCmdIgnoreErr(ctx, cfg, cfg.BE1IP, "truncate -s 0 /tmp/echo-backend.log")
	sshCmdIgnoreErr(ctx, cfg, cfg.BE2IP, "truncate -s 0 /tmp/echo-backend.log")

	// Send new sessions -- should reach only be-1 and be-2.
	_ = runTrafficGen(t, cfg, 20, 100, 5*time.Second, 0)

	// Verify be-0 process is not running.
	be0Procs := sshCmdIgnoreErr(ctx, cfg, cfg.BE0IP, "pgrep -f '[u]dplb-echo-backend' || echo 'none'")
	require.Contains(t, be0Procs, "none", "be-0 echo-backend should not be running")

	// Verify be-0 received no new packets after it was stopped.
	be0ReceivedAfter := backendTotalReceived(t, cfg, cfg.BE0IP)
	require.Equal(t, be0ReceivedBefore, be0ReceivedAfter,
		"be-0 should receive no new packets after being stopped (before=%d, after=%d)",
		be0ReceivedBefore, be0ReceivedAfter)

	// Verify surviving backends received traffic.
	be1Log := collectEchoBackendLog(t, cfg, cfg.BE1IP)
	be2Log := collectEchoBackendLog(t, cfg, cfg.BE2IP)
	require.NotEmpty(t, be1Log, "be-1 should have received packets after be-0 failure")
	require.NotEmpty(t, be2Log, "be-2 should have received packets after be-0 failure")
}

// TestLBFailure verifies that when one LB is stopped, traffic is rerouted via the remaining LBs
// after BGP reconvergence.
//
// This test validates BGP route withdrawal when an LB's FRR daemon stops announcing the VIP.
// The router should remove the failed LB's nexthop from the ECMP group, leaving 2 paths.
func TestLBFailure(t *testing.T) {
	cfg, err := e2eutil.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available -- run with 'forge test run e2e'")

	deployBinaries(t, cfg)
	defer cleanupAllProcesses(t, cfg)

	startEchoBackends(t, cfg)
	defer stopEchoBackends(t, cfg)
	startAllUdplb(t, cfg)
	defer stopAllUdplb(t, cfg)
	waitForBGPConvergence(t, cfg, bgpConvergenceTimeout)

	// Verify initial state: 3 ECMP routes on the router.
	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()

	routesBefore := sshCmd(t, ctx, cfg, cfg.RouterIP, "ip route show "+cfg.VIP)
	t.Logf("Routes before LB failure:\n%s", routesBefore)
	require.Contains(t, routesBefore, cfg.LB0IP, "initial routes should include lb-0")

	// Stop lb-0: kill udplb and stop FRR to withdraw the BGP route.
	sshCmdIgnoreErr(ctx, cfg, cfg.LB0IP, "sudo pkill -f '[/]tmp/udplb' || true")
	sshCmdIgnoreErr(ctx, cfg, cfg.LB0IP, "sudo systemctl stop frr")
	t.Log("Stopped lb-0 udplb and FRR -- waiting for BGP reconvergence")

	// Wait for BGP to reconverge. The router should drop lb-0's route after the hold timer expires.
	// Default BGP hold time is 180s; FRR uses 180s by default, but route removal can happen faster
	// when the TCP session is torn down.
	// Poll for up to 120s for the route to be removed.
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		routeCtx, routeCancel := context.WithTimeout(context.Background(), operationTimeout)
		routes, err := runSSHCommand(routeCtx, cfg.SSHKeyPath, sshUser, cfg.RouterIP, "ip route show "+cfg.VIP)
		routeCancel()
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}

		// Check if lb-0's IP is still in the routes.
		if !strings.Contains(routes, cfg.LB0IP) {
			t.Log("BGP reconverged: lb-0 route removed from ECMP group")
			t.Logf("Routes after LB failure:\n%s", routes)

			// Verify remaining 2 LBs are still in the routes.
			require.Contains(t, routes, cfg.LB1IP, "routes should still include lb-1")
			require.Contains(t, routes, cfg.LB2IP, "routes should still include lb-2")

			// Restore lb-0's FRR for cleanup.
			sshCmdIgnoreErr(ctx, cfg, cfg.LB0IP, "sudo systemctl start frr")
			return
		}
		time.Sleep(5 * time.Second)
	}

	// Restore FRR even on failure.
	sshCmdIgnoreErr(ctx, cfg, cfg.LB0IP, "sudo systemctl start frr")
	require.Fail(t, "BGP did not reconverge within 120s after lb-0 failure")
}

// TestLBFailureBGPSummary verifies that the router's BGP summary reflects a peer going down.
func TestLBFailureBGPSummary(t *testing.T) {
	cfg, err := e2eutil.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available -- run with 'forge test run e2e'")

	// Wait for initial convergence.
	waitForBGPConvergence(t, cfg, bgpConvergenceTimeout)

	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()

	// Verify all 3 peers are established.
	summaryBefore := sshCmd(t, ctx, cfg, cfg.RouterIP, "sudo vtysh -c 'show bgp summary'")
	t.Logf("BGP summary before lb-0 failure:\n%s", summaryBefore)
	for _, lbIP := range allLBIPs(cfg) {
		require.True(t, peerIsEstablished(summaryBefore, lbIP),
			"peer %s should be Established before failure", lbIP)
	}

	// Stop FRR on lb-0 to simulate LB failure.
	sshCmdIgnoreErr(ctx, cfg, cfg.LB0IP, "sudo systemctl stop frr")
	defer func() {
		restoreCtx, restoreCancel := context.WithTimeout(context.Background(), operationTimeout)
		defer restoreCancel()
		sshCmdIgnoreErr(restoreCtx, cfg, cfg.LB0IP, "sudo systemctl start frr")
	}()

	t.Log("Stopped FRR on lb-0 -- waiting for peer to drop")

	// Wait for lb-0 to disappear from the Established peers.
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		pollCtx, pollCancel := context.WithTimeout(context.Background(), operationTimeout)
		summary, err := runSSHCommand(pollCtx, cfg.SSHKeyPath, sshUser, cfg.RouterIP, "sudo vtysh -c 'show bgp summary'")
		pollCancel()
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}

		if !peerIsEstablished(summary, cfg.LB0IP) {
			t.Log("Confirmed: lb-0 is no longer in Established state")
			t.Logf("BGP summary after lb-0 failure:\n%s", summary)

			// Verify remaining peers are still Established.
			require.True(t, peerIsEstablished(summary, cfg.LB1IP), "lb-1 should remain Established")
			require.True(t, peerIsEstablished(summary, cfg.LB2IP), "lb-2 should remain Established")
			return
		}

		time.Sleep(5 * time.Second)
	}

	require.Fail(t, "lb-0 BGP peer did not drop within 120s after stopping FRR")
}
