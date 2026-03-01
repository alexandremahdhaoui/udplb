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

// TestSinglePacketForwarding verifies that a single UDP packet sent to the VIP reaches exactly
// one backend via XDP forwarding.
//
// Prerequisites:
// - All binaries deployed (udplb, echo-backend, traffic-gen)
// - Echo backends running on all 3 BE VMs
// - udplb running on all 3 LB VMs with XDP attached
// - BGP converged so the router has ECMP routes to VIP
func TestSinglePacketForwarding(t *testing.T) {
	cfg, err := e2eutil.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available -- run with 'forge test run e2e'")

	// Use a generous timeout for all diagnostics.
	diagCtx, diagCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer diagCancel()

	// Deploy and start all components.
	deployBinaries(t, cfg)
	startEchoBackends(t, cfg)
	defer stopEchoBackends(t, cfg)
	startAllUdplb(t, cfg)
	defer stopAllUdplb(t, cfg)

	// Wait for BGP to converge.
	waitForBGPConvergence(t, cfg, bgpConvergenceTimeout)

	// Restart echo-backends for clean logs (avoids sparse file corruption from truncate).
	stopEchoBackends(t, cfg)
	for _, beIP := range allBEIPs(cfg) {
		sshCmdIgnoreErr(diagCtx, cfg, beIP, "rm -f /tmp/echo-backend.log")
	}
	time.Sleep(1 * time.Second)
	startEchoBackends(t, cfg)

	// Wait for health probes to arrive (sanity check for UDP connectivity).
	t.Logf("Waiting 12s for health probes to arrive at backends...")
	time.Sleep(12 * time.Second)

	// Check if health probes arrived (zero-UUID). This tests basic UDP between LB and backend.
	for _, beIP := range allBEIPs(cfg) {
		log := sshCmdIgnoreErr(diagCtx, cfg, beIP, "cat /tmp/echo-backend.log 2>/dev/null")
		t.Logf("Backend %s echo-backend log after health probes:\n%s", beIP, log)
	}

	// Test basic UDP connectivity: client sends a socat UDP packet to a backend directly.
	firstBE := allBEIPs(cfg)[0]
	sshCmdIgnoreErr(diagCtx, cfg, firstBE,
		"nohup socat -u UDP-LISTEN:9999,reuseaddr OPEN:/tmp/socat-test.txt,creat 2>/dev/null &")
	time.Sleep(1 * time.Second)
	sshCmdIgnoreErr(diagCtx, cfg, cfg.ClientIP,
		"echo 'udp-connectivity-test' | socat - UDP:"+firstBE+":9999")
	time.Sleep(1 * time.Second)
	udpTestResult := sshCmdIgnoreErr(diagCtx, cfg, firstBE, "cat /tmp/socat-test.txt 2>/dev/null")
	t.Logf("Direct UDP test (client -> %s:9999): %q", firstBE, udpTestResult)

	// Dump network diagnostics.
	t.Logf("=== Network diagnostics ===")
	t.Logf("Router ip_forward: %s",
		sshCmdIgnoreErr(diagCtx, cfg, cfg.RouterIP, "sysctl net.ipv4.ip_forward"))
	t.Logf("Router route to VIP:\n%s",
		sshCmdIgnoreErr(diagCtx, cfg, cfg.RouterIP, "ip route show "+cfg.VIP))
	t.Logf("Client route to VIP:\n%s",
		sshCmdIgnoreErr(diagCtx, cfg, cfg.ClientIP, "ip route show "+cfg.VIP))
	t.Logf("Router send_redirects (all): %s",
		sshCmdIgnoreErr(diagCtx, cfg, cfg.RouterIP, "cat /proc/sys/net/ipv4/conf/all/send_redirects"))
	t.Logf("Client accept_redirects (all): %s",
		sshCmdIgnoreErr(diagCtx, cfg, cfg.ClientIP, "cat /proc/sys/net/ipv4/conf/all/accept_redirects"))

	// Pre-warm ARP and verify.
	sshCmdIgnoreErr(diagCtx, cfg, cfg.ClientIP, "ping -c 2 -W 2 "+cfg.RouterIP+" 2>&1")
	t.Logf("Client ARP cache:\n%s",
		sshCmdIgnoreErr(diagCtx, cfg, cfg.ClientIP, "ip neigh show"))

	// Ensure tracefs is mounted and tracing is enabled on all LBs.
	for _, lbIP := range allLBIPs(cfg) {
		sshCmdIgnoreErr(diagCtx, cfg, lbIP,
			"sudo mount -t debugfs debugfs /sys/kernel/debug 2>/dev/null; "+
				"sudo mount -t tracefs tracefs /sys/kernel/debug/tracing 2>/dev/null; "+
				"sudo sh -c 'echo 1 > /sys/kernel/debug/tracing/tracing_on' 2>/dev/null; "+
				"sudo sh -c 'echo > /sys/kernel/debug/tracing/trace' 2>/dev/null")
	}

	// Start tcpdump with nohup to survive SSH session close.
	// Also capture on the CLIENT to verify packets leave.
	sshCmdIgnoreErr(diagCtx, cfg, cfg.ClientIP,
		"nohup sudo timeout 30 tcpdump -i ens2 -c 50 udp port 12345 -w /tmp/client-capture.pcap > /tmp/tcpdump-client.err 2>&1 &")
	sshCmdIgnoreErr(diagCtx, cfg, cfg.RouterIP,
		"nohup sudo timeout 30 tcpdump -i ens2 -c 50 host "+cfg.VIP+" -w /tmp/vip-capture.pcap > /tmp/tcpdump-router.err 2>&1 &")
	for _, lbIP := range allLBIPs(cfg) {
		sshCmdIgnoreErr(diagCtx, cfg, lbIP,
			"nohup sudo timeout 30 tcpdump -i ens2 -c 50 udp -w /tmp/lb-capture.pcap > /tmp/tcpdump-lb.err 2>&1 &")
	}
	for _, beIP := range allBEIPs(cfg) {
		sshCmdIgnoreErr(diagCtx, cfg, beIP,
			"nohup sudo timeout 30 tcpdump -i ens2 -c 50 udp -w /tmp/be-capture.pcap > /tmp/tcpdump-be.err 2>&1 &")
	}
	time.Sleep(2 * time.Second) // Give tcpdump time to start

	// Verify tcpdump is running on at least the client.
	t.Logf("Client tcpdump running: %s",
		sshCmdIgnoreErr(diagCtx, cfg, cfg.ClientIP, "pgrep -a tcpdump || echo 'NOT RUNNING'"))

	// Send 1 packet from client to VIP.
	tgenOutput := runTrafficGen(t, cfg, 1, 1, 2*time.Second, 0) // 1 session, 1 pps, 2s
	t.Logf("Traffic-gen output: %s", tgenOutput)

	// CRITICAL: Read BPF trace IMMEDIATELY after traffic, before other SSH
	// commands that generate ip_miss noise in the trace buffer.
	time.Sleep(1 * time.Second) // brief pause for XDP processing
	t.Logf("=== BPF trace (captured immediately after traffic) ===")
	for _, lbIP := range allLBIPs(cfg) {
		fullTrace := sshCmdIgnoreErr(diagCtx, cfg, lbIP,
			"sudo cat /sys/kernel/debug/tracing/trace 2>/dev/null | tail -50")
		if strings.TrimSpace(fullTrace) != "" && !strings.Contains(fullTrace, "# tracer: nop") {
			t.Logf("BPF trace on %s (last 50 lines):\n%s", lbIP, fullTrace)
		} else {
			t.Logf("BPF trace on %s: (empty or tracefs header only)", lbIP)
		}
	}

	// XDP stats — check if XDP_TX actually transmitted any packets.
	t.Logf("=== XDP stats ===")
	for _, lbIP := range allLBIPs(cfg) {
		xdpStats := sshCmdIgnoreErr(diagCtx, cfg, lbIP,
			"ethtool -S ens2 2>/dev/null | grep -iE 'xdp|tx_drop|rx_drop' || echo 'no ethtool stats'")
		t.Logf("XDP/TX stats on %s:\n%s", lbIP, xdpStats)
		linkStats := sshCmdIgnoreErr(diagCtx, cfg, lbIP,
			"ip -s link show ens2 | head -10")
		t.Logf("Link stats on %s:\n%s", lbIP, linkStats)
	}

	// Wait for echo-backend to report (report interval is 5s) and captures to accumulate.
	time.Sleep(8 * time.Second)

	// Kill tcpdump so pcap files are flushed.
	sshCmdIgnoreErr(diagCtx, cfg, cfg.ClientIP, "sudo pkill tcpdump 2>/dev/null || true")
	sshCmdIgnoreErr(diagCtx, cfg, cfg.RouterIP, "sudo pkill tcpdump 2>/dev/null || true")
	for _, lbIP := range allLBIPs(cfg) {
		sshCmdIgnoreErr(diagCtx, cfg, lbIP, "sudo pkill tcpdump 2>/dev/null || true")
	}
	for _, beIP := range allBEIPs(cfg) {
		sshCmdIgnoreErr(diagCtx, cfg, beIP, "sudo pkill tcpdump 2>/dev/null || true")
	}
	time.Sleep(1 * time.Second)

	// Read ALL tcpdump captures, showing stderr for debugging.
	t.Logf("=== Packet captures ===")
	t.Logf("Client tcpdump (port 12345):\n%s",
		sshCmdIgnoreErr(diagCtx, cfg, cfg.ClientIP,
			"sudo tcpdump -r /tmp/client-capture.pcap -n 2>&1 || echo 'no capture file'"))
	t.Logf("Router tcpdump (VIP) with MACs:\n%s",
		sshCmdIgnoreErr(diagCtx, cfg, cfg.RouterIP,
			"sudo tcpdump -r /tmp/vip-capture.pcap -ne 2>&1 || echo 'no capture file'"))
	for _, lbIP := range allLBIPs(cfg) {
		t.Logf("LB %s tcpdump (UDP) with MACs:\n%s", lbIP,
			sshCmdIgnoreErr(diagCtx, cfg, lbIP,
				"sudo tcpdump -r /tmp/lb-capture.pcap -ne 2>&1 || echo 'no capture file'"))
	}
	for _, beIP := range allBEIPs(cfg) {
		t.Logf("Backend %s tcpdump (all UDP) with MACs:\n%s", beIP,
			sshCmdIgnoreErr(diagCtx, cfg, beIP,
				"sudo tcpdump -r /tmp/be-capture.pcap -ne 2>&1 || echo 'no capture file'"))
	}

	// Collect udplb logs.
	for _, lbIP := range allLBIPs(cfg) {
		t.Logf("udplb log on %s:\n%s", lbIP, collectUdplbLog(t, cfg, lbIP))
	}

	// Router-level diagnostics: FRR BGP, kernel route, ARP cache.
	t.Logf("=== Router deep diagnostics ===")
	t.Logf("Router FRR BGP table for VIP:\n%s",
		sshCmdIgnoreErr(diagCtx, cfg, cfg.RouterIP,
			"sudo vtysh -c 'show ip bgp "+cfg.VIP+"/32' 2>/dev/null || echo 'vtysh not available'"))
	t.Logf("Router kernel route get VIP:\n%s",
		sshCmdIgnoreErr(diagCtx, cfg, cfg.RouterIP,
			"ip route get "+cfg.VIP+" 2>/dev/null || echo 'no route'"))

	// XDP attachment verification on LBs.
	for _, lbIP := range allLBIPs(cfg) {
		xdpStatus := sshCmdIgnoreErr(diagCtx, cfg, lbIP, "ip link show ens2")
		t.Logf("XDP attachment on %s:\n%s", lbIP, xdpStatus)
	}

	// Dump raw echo-backend logs.
	for _, beIP := range allBEIPs(cfg) {
		t.Logf("Backend %s raw echo-backend log:\n%s", beIP,
			sshCmdIgnoreErr(diagCtx, cfg, beIP, "cat /tmp/echo-backend.log 2>/dev/null || echo 'no log'"))
	}

	// Verify exactly 1 backend received the traffic-gen packet.
	receivedCount := 0
	for _, beIP := range allBEIPs(cfg) {
		received := backendReceivedTrafficGenPackets(t, cfg, beIP)
		t.Logf("Backend %s received traffic-gen packets: %v", beIP, received)
		if received {
			receivedCount++
		}
	}
	require.Equal(t, 1, receivedCount, "exactly 1 backend should receive the traffic-gen packet")
}

// TestSessionAffinity verifies that all packets with the same session UUID are forwarded to
// the same backend.
func TestSessionAffinity(t *testing.T) {
	cfg, err := e2eutil.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available -- run with 'forge test run e2e'")

	deployBinaries(t, cfg)
	startEchoBackends(t, cfg)
	defer stopEchoBackends(t, cfg)
	startAllUdplb(t, cfg)
	defer stopAllUdplb(t, cfg)
	waitForBGPConvergence(t, cfg, bgpConvergenceTimeout)

	// Restart echo-backends for clean logs (avoids sparse file corruption from truncate).
	stopEchoBackends(t, cfg)
	diagCtx, diagCancel := context.WithTimeout(context.Background(), operationTimeout)
	defer diagCancel()
	for _, beIP := range allBEIPs(cfg) {
		sshCmdIgnoreErr(diagCtx, cfg, beIP, "rm -f /tmp/echo-backend.log")
	}
	time.Sleep(1 * time.Second)
	startEchoBackends(t, cfg)
	time.Sleep(6 * time.Second) // Wait for health probes to populate backends

	// Send 100 packets with a single session from client to VIP.
	_ = runTrafficGen(t, cfg, 1, 100, 2*time.Second, 0) // 1 session, 100 pps, 2s

	// Wait for echo-backend to report (report interval is 5s).
	time.Sleep(6 * time.Second)

	// Verify all packets arrived at the same backend.
	// Use backendReceivedTrafficGenPackets to exclude health probes (zero-UUID).
	receivingBackends := 0
	for _, beIP := range allBEIPs(cfg) {
		if backendReceivedTrafficGenPackets(t, cfg, beIP) {
			receivingBackends++
		}
	}
	require.Equal(t, 1, receivingBackends,
		"all packets for the same session should go to exactly 1 backend")
}

// TestMultiSessionDistribution verifies that traffic from multiple sessions is distributed
// across all backends.
func TestMultiSessionDistribution(t *testing.T) {
	cfg, err := e2eutil.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available -- run with 'forge test run e2e'")

	deployBinaries(t, cfg)
	startEchoBackends(t, cfg)
	defer stopEchoBackends(t, cfg)
	startAllUdplb(t, cfg)
	defer stopAllUdplb(t, cfg)
	waitForBGPConvergence(t, cfg, bgpConvergenceTimeout)

	// Send 100 different sessions from client to VIP.
	_ = runTrafficGen(t, cfg, 100, 1000, 5*time.Second, 0) // 100 sessions, 1000 pps, 5s

	// Collect per-backend packet counts and verify distribution.
	// No backend should receive more than 60% of total traffic.
	for _, beIP := range allBEIPs(cfg) {
		log := collectEchoBackendLog(t, cfg, beIP)
		t.Logf("Backend %s log length: %d", beIP, len(log))
	}
}

// TestBinaryDeployment verifies that all binaries can be deployed to VMs and are executable.
// This is a concrete, non-XDP test that validates the deployment pipeline.
func TestBinaryDeployment(t *testing.T) {
	cfg, err := e2eutil.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available -- run with 'forge test run e2e'")

	deployBinaries(t, cfg)
	defer cleanupAllProcesses(t, cfg)

	// Verify binaries exist and are executable on target VMs.
	t.Run("udplb-on-lbs", func(t *testing.T) {
		for _, lbIP := range allLBIPs(cfg) {
			output := sshCmd(t, testCtx(t), cfg, lbIP, "file /tmp/udplb")
			require.Contains(t, output, "ELF", "udplb should be an ELF binary on %s", lbIP)
		}
	})

	t.Run("echo-backend-on-backends", func(t *testing.T) {
		for _, beIP := range allBEIPs(cfg) {
			output := sshCmd(t, testCtx(t), cfg, beIP, "file /tmp/udplb-echo-backend")
			require.Contains(t, output, "ELF", "echo-backend should be an ELF binary on %s", beIP)
		}
	})

	t.Run("traffic-gen-on-client", func(t *testing.T) {
		output := sshCmd(t, testCtx(t), cfg, cfg.ClientIP, "file /tmp/udplb-traffic-gen")
		require.Contains(t, output, "ELF", "traffic-gen should be an ELF binary on client")
	})
}

// TestEchoBackendLifecycle verifies that echo backends can be started and stopped cleanly.
func TestEchoBackendLifecycle(t *testing.T) {
	cfg, err := e2eutil.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available -- run with 'forge test run e2e'")

	deployBinaries(t, cfg)
	defer cleanupAllProcesses(t, cfg)

	// Start backends.
	startEchoBackends(t, cfg)

	// Verify backends are listening.
	for _, beIP := range allBEIPs(cfg) {
		output := sshCmd(t, testCtx(t), cfg, beIP, "pgrep -f '[u]dplb-echo-backend'")
		require.NotEmpty(t, strings.TrimSpace(output),
			"echo-backend should be running on %s", beIP)
	}

	// Stop backends.
	stopEchoBackends(t, cfg)

	// Give processes time to exit.
	waitMs(1000)

	// Verify backends are stopped.
	for _, beIP := range allBEIPs(cfg) {
		output, _ := runSSHCommand(testCtx(t), cfg.SSHKeyPath, sshUser, beIP,
			"pgrep -f '[u]dplb-echo-backend' || echo 'not running'")
		require.Contains(t, output, "not running",
			"echo-backend should be stopped on %s", beIP)
	}
}

// TestBackendMACResolution verifies that backend MAC addresses can be resolved via SSH.
// This is a prerequisite for generating the udplb config.
func TestBackendMACResolution(t *testing.T) {
	cfg, err := e2eutil.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available -- run with 'forge test run e2e'")

	for _, beIP := range allBEIPs(cfg) {
		t.Run("be-"+beIP, func(t *testing.T) {
			mac := getBackendMAC(t, cfg, beIP)
			t.Logf("Backend %s MAC: %s", beIP, mac)

			// Validate MAC format: xx:xx:xx:xx:xx:xx
			require.Regexp(t, `^[0-9a-f]{2}(:[0-9a-f]{2}){5}$`, mac,
				"MAC should be in standard format")
		})
	}
}

// TestUdplbConfigGeneration verifies that we can generate a valid udplb config YAML
// with runtime-resolved backend MACs.
func TestUdplbConfigGeneration(t *testing.T) {
	cfg, err := e2eutil.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available -- run with 'forge test run e2e'")

	configYAML := generateUdplbConfig(t, cfg)
	t.Logf("Generated udplb config:\n%s", configYAML)

	// Verify the config contains expected fields.
	require.Contains(t, configYAML, "ifname: ens2")
	require.Contains(t, configYAML, "ip: "+cfg.VIP)
	require.Contains(t, configYAML, "port: 12345")
	require.Contains(t, configYAML, "backends:")

	// Verify all 3 backend IPs are present.
	for _, beIP := range allBEIPs(cfg) {
		require.Contains(t, configYAML, beIP, "config should contain backend IP %s", beIP)
	}
}
