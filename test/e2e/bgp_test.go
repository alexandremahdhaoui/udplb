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

	"github.com/stretchr/testify/require"

	e2eutil "github.com/alexandremahdhaoui/udplb/pkg/test/e2e"
)

// TestBGPConvergence verifies that the FRR router learns VIP routes from all 3 LBs.
// Each LB VM runs FRR with AS 65001, announcing 10.100.0.200/32 to the router (AS 65000).
// The cloud-init configuration in forge.yaml pre-configures FRR on boot.
func TestBGPConvergence(t *testing.T) {
	cfg, err := e2eutil.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available -- run with 'forge test run e2e'")

	t.Log("Waiting for BGP convergence: 3 LB peers must reach Established state on the router")
	waitForBGPConvergence(t, cfg, bgpConvergenceTimeout)
	t.Log("BGP convergence achieved: all 3 LB peers are Established")

	// Log the final BGP summary for visibility.
	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()

	summary := sshCmd(t, ctx, cfg, cfg.RouterIP, "sudo vtysh -c 'show bgp summary'")
	t.Logf("BGP summary:\n%s", summary)
}

// TestECMPRoutes verifies that after BGP convergence, the router has 3 equal-cost next-hops
// for the VIP address (10.100.0.200/32). This confirms ECMP is properly configured with
// "maximum-paths 3" in the router's FRR config.
func TestECMPRoutes(t *testing.T) {
	cfg, err := e2eutil.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available -- run with 'forge test run e2e'")

	// Wait for BGP to converge first.
	waitForBGPConvergence(t, cfg, bgpConvergenceTimeout)

	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()

	// Check the kernel routing table for ECMP routes to the VIP.
	output := sshCmd(t, ctx, cfg, cfg.RouterIP, "ip route show "+cfg.VIP)
	t.Logf("Routes to VIP:\n%s", output)

	// Verify all 3 LB IPs appear as nexthops.
	for _, lbIP := range allLBIPs(cfg) {
		require.Contains(t, output, lbIP,
			"VIP route should include nexthop via %s", lbIP)
	}

	// Count the number of "nexthop" entries. With ECMP, we expect 3.
	// The first route line uses "via" directly, subsequent nexthops appear on separate lines.
	nexthopCount := strings.Count(output, "nexthop") + strings.Count(output, "via")
	// "via" appears in each nexthop line plus potentially the first line.
	// With 3 ECMP paths, we expect at least 3 references to "via".
	t.Logf("Nexthop/via count in route output: %d", nexthopCount)
	require.GreaterOrEqual(t, nexthopCount, 3,
		"expected at least 3 next-hop entries for ECMP routing to VIP")
}

// TestBGPPeerDetail verifies detailed BGP peer information for each LB.
func TestBGPPeerDetail(t *testing.T) {
	cfg, err := e2eutil.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available -- run with 'forge test run e2e'")

	waitForBGPConvergence(t, cfg, bgpConvergenceTimeout)

	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()

	for _, lbIP := range allLBIPs(cfg) {
		t.Run("peer-"+lbIP, func(t *testing.T) {
			// Check that each LB peer is visible from the router.
			output := sshCmd(t, ctx, cfg, cfg.RouterIP,
				"sudo vtysh -c 'show bgp neighbor "+lbIP+"'")
			t.Logf("BGP neighbor %s:\n%s", lbIP, output)

			require.Contains(t, output, "BGP state = Established",
				"peer %s should be in Established state", lbIP)
			require.Contains(t, output, "65001",
				"peer %s should be in AS 65001", lbIP)
		})
	}
}
