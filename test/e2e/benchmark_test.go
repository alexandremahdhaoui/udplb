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
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	e2eutil "github.com/alexandremahdhaoui/udplb/pkg/test/e2e"
)

// trafficGenReport matches the JSON output of udplb-traffic-gen.
type trafficGenReport struct {
	TotalSent int                `json:"totalSent"`
	Duration  string             `json:"duration"`
	Sessions  map[string]session `json:"sessions"`
}

type session struct {
	Sent int `json:"sent"`
}

// TestBenchmarkParametric runs parametric traffic benchmarks against the full stack:
// client -> router (ECMP) -> LBs (XDP) -> backends.
func TestBenchmarkParametric(t *testing.T) {
	cfg, err := e2eutil.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available -- run with 'forge test run e2e'")

	deployBinaries(t, cfg)
	defer cleanupAllProcesses(t, cfg)

	startEchoBackends(t, cfg)
	defer stopEchoBackends(t, cfg)
	startAllUdplb(t, cfg)
	defer stopAllUdplb(t, cfg)
	waitForBGPConvergence(t, cfg, bgpConvergenceTimeout)

	scenarios := []struct {
		name     string
		sessions int
		pps      int
		duration time.Duration
		backends int
	}{
		{"10sess-100pps-10s", 10, 100, 10 * time.Second, 3},
		{"100sess-1000pps-30s", 100, 1000, 30 * time.Second, 3},
		{"1000sess-10000pps-60s", 1000, 10000, 60 * time.Second, 3},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			// Run traffic generator on the client VM.
			output := runTrafficGen(t, cfg, sc.sessions, sc.pps, sc.duration, 0)

			// Parse the traffic-gen report.
			var report trafficGenReport
			err := json.Unmarshal([]byte(output), &report)
			require.NoError(t, err, "traffic-gen output should be valid JSON:\n%s", output)

			t.Logf("Traffic-gen report: sent=%d, duration=%s, sessions=%d",
				report.TotalSent, report.Duration, len(report.Sessions))

			// Verify the expected number of sessions were created.
			require.Len(t, report.Sessions, sc.sessions,
				"traffic-gen should create exactly %d sessions", sc.sessions)

			// Collect backend stats.
			for _, beIP := range allBEIPs(cfg) {
				log := collectEchoBackendLog(t, cfg, beIP)
				t.Logf("Backend %s log length: %d bytes", beIP, len(log))
			}

			// TODO: Once XDP forwarding is operational, add assertions:
			// - Total received across all backends should be close to report.TotalSent
			// - Packet loss should be below a threshold (e.g., < 1% for low-rate scenarios)
			// - No single backend should receive more than 60% of total traffic
		})
	}
}

// TestBenchmarkTrafficGenSmoke is a smoke test for the traffic generator tool itself.
// It verifies that the binary runs, sends packets, and produces valid JSON output.
// Unlike the parametric benchmark, this test does not require XDP forwarding -- it sends
// traffic to a socat listener on a backend VM directly.
func TestBenchmarkTrafficGenSmoke(t *testing.T) {
	cfg, err := e2eutil.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available -- run with 'forge test run e2e'")

	deployBinaries(t, cfg)
	defer cleanupAllProcesses(t, cfg)

	ctx := testCtx(t)

	// Start a socat UDP listener on be-0 to receive traffic directly (bypassing ECMP/XDP).
	sshCmdIgnoreErr(ctx, cfg, cfg.BE0IP, "pkill socat || true")
	time.Sleep(500 * time.Millisecond)

	listenCmd := "nohup socat -u UDP4-LISTEN:19999,reuseaddr,fork OPEN:/tmp/tgen-smoke.log,creat,append > /dev/null 2>&1 &"
	sshCmd(t, ctx, cfg, cfg.BE0IP, listenCmd)
	time.Sleep(1 * time.Second)

	defer func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), operationTimeout)
		defer cleanCancel()
		sshCmdIgnoreErr(cleanCtx, cfg, cfg.BE0IP, "pkill socat || true; rm -f /tmp/tgen-smoke.log")
	}()

	// Run traffic-gen from the client VM targeting be-0 directly on port 19999.
	tgenCtx, tgenCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer tgenCancel()

	tgenCmd := "/tmp/udplb-traffic-gen --target " + cfg.BE0IP + ":19999 --sessions 5 --rate 50 --duration 3s --payload-size 64"
	output, err := runSSHCommand(tgenCtx, cfg.SSHKeyPath, sshUser, cfg.ClientIP, tgenCmd)
	require.NoError(t, err, "traffic-gen should run without errors")

	t.Logf("Traffic-gen output:\n%s", output)

	// Parse the JSON report.
	var report trafficGenReport
	err = json.Unmarshal([]byte(output), &report)
	require.NoError(t, err, "traffic-gen output should be valid JSON")

	require.Greater(t, report.TotalSent, 0, "traffic-gen should have sent packets")
	require.Len(t, report.Sessions, 5, "traffic-gen should have created 5 sessions")
	require.NotEmpty(t, report.Duration, "duration field should be set")

	t.Logf("Smoke test passed: sent %d packets across %d sessions in %s",
		report.TotalSent, len(report.Sessions), report.Duration)
}
