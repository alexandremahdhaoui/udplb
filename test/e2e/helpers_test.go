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
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	e2eutil "github.com/alexandremahdhaoui/udplb/pkg/test/e2e"
)

const (
	sshUser = "ubuntu"

	// bgpConvergenceTimeout is the maximum time to wait for all BGP peers to reach Established state.
	bgpConvergenceTimeout = 90 * time.Second

	// operationTimeout is the default timeout for individual SSH operations.
	operationTimeout = 30 * time.Second
)

// allLBIPs returns the IPs of all 3 load balancer VMs.
func allLBIPs(cfg *e2eutil.TestenvConfig) []string {
	return []string{cfg.LB0IP, cfg.LB1IP, cfg.LB2IP}
}

// allBEIPs returns the IPs of all 3 backend VMs.
func allBEIPs(cfg *e2eutil.TestenvConfig) []string {
	return []string{cfg.BE0IP, cfg.BE1IP, cfg.BE2IP}
}

// sshCmd is a shorthand for runSSHCommand with the testenv SSH key and ubuntu user.
func sshCmd(t *testing.T, ctx context.Context, cfg *e2eutil.TestenvConfig, host, command string) string {
	t.Helper()
	output, err := runSSHCommand(ctx, cfg.SSHKeyPath, sshUser, host, command)
	require.NoError(t, err, "SSH command failed on %s: %s", host, command)
	return output
}

// sshCmdIgnoreErr runs an SSH command and returns output, ignoring errors.
// Use for cleanup operations where failure is acceptable.
func sshCmdIgnoreErr(ctx context.Context, cfg *e2eutil.TestenvConfig, host, command string) string {
	output, _ := runSSHCommand(ctx, cfg.SSHKeyPath, sshUser, host, command)
	return output
}

// waitForBGPConvergence polls "vtysh -c 'show bgp summary'" on the router until all 3 LB peers
// show "Established" state. Fails the test if the timeout is reached.
func waitForBGPConvergence(t *testing.T, cfg *e2eutil.TestenvConfig, timeout time.Duration) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	lbIPs := allLBIPs(cfg)

	// Poll every 5 seconds until all peers are Established.
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// One last attempt to show the current state before failing.
			output := sshCmdIgnoreErr(ctx, cfg, cfg.RouterIP, "sudo vtysh -c 'show bgp summary'")
			require.Fail(t, "BGP convergence timeout",
				"timed out after %s waiting for all 3 LB peers to reach Established state.\nLast BGP summary:\n%s",
				timeout, output)
			return
		case <-ticker.C:
			output, err := runSSHCommand(ctx, cfg.SSHKeyPath, sshUser, cfg.RouterIP, "sudo vtysh -c 'show bgp summary'")
			if err != nil {
				t.Logf("BGP poll: SSH error (retrying): %v", err)
				continue
			}

			established := 0
			for _, ip := range lbIPs {
				// FRR "show bgp summary" output includes lines like:
				//   10.100.0.10  4 65001  ...  Established  ...
				// or with a state column showing a number (prefixRcvd count) when established.
				if peerIsEstablished(output, ip) {
					established++
				}
			}

			t.Logf("BGP convergence: %d/%d peers established", established, len(lbIPs))
			if established == len(lbIPs) {
				return
			}
		}
	}
}

// peerIsEstablished checks if a BGP peer IP appears in the summary with an Established state.
// In FRR's "show bgp summary", an established peer shows a numeric prefix count in the State/PfxRcd
// column (index 9). A non-established peer shows a state string like "Active", "Connect", "OpenSent".
// FRR 8.x+ includes additional PfxSnt and Desc columns after State/PfxRcd.
func peerIsEstablished(bgpSummary, peerIP string) bool {
	for _, line := range strings.Split(bgpSummary, "\n") {
		if !strings.Contains(line, peerIP) {
			continue
		}
		// Fields: Neighbor(0) V(1) AS(2) MsgRcvd(3) MsgSent(4) TblVer(5)
		//         InQ(6) OutQ(7) Up/Down(8) State/PfxRcd(9) [PfxSnt(10) Desc(11+)]
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		// State/PfxRcd is at index 9. A numeric value means the peer is established.
		matched, _ := regexp.MatchString(`^\d+$`, fields[9])
		if matched {
			return true
		}
	}
	return false
}

// deployBinaries copies udplb, traffic-gen, and echo-backend binaries to appropriate VMs.
func deployBinaries(t *testing.T, cfg *e2eutil.TestenvConfig) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Deploy udplb to all LB VMs.
	udplbPath, err := findBinary("udplb")
	require.NoError(t, err, "udplb binary must exist -- run 'forge build udplb'")

	for _, lbIP := range allLBIPs(cfg) {
		err := copyFileToVM(ctx, cfg.SSHKeyPath, sshUser, lbIP, udplbPath, "/tmp/udplb")
		require.NoError(t, err, "failed to copy udplb to %s", lbIP)
		sshCmd(t, ctx, cfg, lbIP, "chmod +x /tmp/udplb")
	}

	// Deploy echo-backend to all BE VMs.
	echoPath, err := findBinary("udplb-echo-backend")
	require.NoError(t, err, "udplb-echo-backend binary must exist -- run 'forge build udplb-echo-backend'")

	for _, beIP := range allBEIPs(cfg) {
		err := copyFileToVM(ctx, cfg.SSHKeyPath, sshUser, beIP, echoPath, "/tmp/udplb-echo-backend")
		require.NoError(t, err, "failed to copy udplb-echo-backend to %s", beIP)
		sshCmd(t, ctx, cfg, beIP, "chmod +x /tmp/udplb-echo-backend")
	}

	// Deploy traffic-gen to client VM.
	tgenPath, err := findBinary("udplb-traffic-gen")
	require.NoError(t, err, "udplb-traffic-gen binary must exist -- run 'forge build udplb-traffic-gen'")

	err = copyFileToVM(ctx, cfg.SSHKeyPath, sshUser, cfg.ClientIP, tgenPath, "/tmp/udplb-traffic-gen")
	require.NoError(t, err, "failed to copy udplb-traffic-gen to client")
	sshCmd(t, ctx, cfg, cfg.ClientIP, "chmod +x /tmp/udplb-traffic-gen")
}

// startEchoBackends starts udplb-echo-backend on all 3 backend VMs.
func startEchoBackends(t *testing.T, cfg *e2eutil.TestenvConfig) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()

	for _, beIP := range allBEIPs(cfg) {
		// Kill any existing instance first.
		sshCmdIgnoreErr(ctx, cfg, beIP, "pkill -f '[u]dplb-echo-backend' || true")
		time.Sleep(500 * time.Millisecond)

		cmd := fmt.Sprintf(
			"nohup /tmp/udplb-echo-backend --listen 0.0.0.0:%d --report-interval 5s > /tmp/echo-backend.log 2>&1 &",
			e2eutil.BackendPort,
		)
		sshCmd(t, ctx, cfg, beIP, cmd)
		t.Logf("Started echo-backend on %s:%d", beIP, e2eutil.BackendPort)
	}

	// Wait briefly for backends to bind.
	time.Sleep(2 * time.Second)
}

// stopEchoBackends stops udplb-echo-backend on all 3 backend VMs.
func stopEchoBackends(t *testing.T, cfg *e2eutil.TestenvConfig) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()

	for _, beIP := range allBEIPs(cfg) {
		sshCmdIgnoreErr(ctx, cfg, beIP, "pkill -f '[u]dplb-echo-backend' || true")
	}
}

// getBackendMAC returns the MAC address of the ens2 interface on the given VM.
func getBackendMAC(t *testing.T, cfg *e2eutil.TestenvConfig, vmIP string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()

	// Parse MAC from "ip link show ens2" output.
	// Example line: "    link/ether 52:54:00:xx:xx:xx brd ff:ff:ff:ff:ff:ff"
	output := sshCmd(t, ctx, cfg, vmIP, "ip link show ens2")

	re := regexp.MustCompile(`link/ether\s+([0-9a-f:]{17})`)
	matches := re.FindStringSubmatch(output)
	require.Len(t, matches, 2, "failed to parse MAC address from 'ip link show ens2' on %s:\n%s", vmIP, output)

	return matches[1]
}

// generateUdplbConfig builds a YAML config string for the udplb binary.
// It resolves backend MACs at runtime via SSH.
func generateUdplbConfig(t *testing.T, cfg *e2eutil.TestenvConfig) string {
	t.Helper()

	beIPs := allBEIPs(cfg)
	var backendEntries []string
	for _, beIP := range beIPs {
		mac := getBackendMAC(t, cfg, beIP)
		entry := fmt.Sprintf(`  - enabled: true
    ip: %s
    mac: "%s"
    port: %d`, beIP, mac, e2eutil.BackendPort)
		backendEntries = append(backendEntries, entry)
	}

	config := fmt.Sprintf(`ifname: ens2
ip: %s
port: %d
backends:
%s
`, cfg.VIP, e2eutil.VIPPort, strings.Join(backendEntries, "\n"))

	return config
}

// startUdplb deploys the udplb config and starts udplb on a given LB VM.
// Requires CAP_BPF and CAP_NET_ADMIN, so runs with sudo.
func startUdplb(t *testing.T, cfg *e2eutil.TestenvConfig, lbIP, configYAML string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()

	// Kill any existing instance first.
	sshCmdIgnoreErr(ctx, cfg, lbIP, "sudo pkill -f '[/]tmp/udplb' || true")
	time.Sleep(500 * time.Millisecond)

	// Write config to the LB VM via a heredoc over SSH.
	writeConfigCmd := fmt.Sprintf("cat > /tmp/udplb-config.yaml << 'UDPLBEOF'\n%sUDPLBEOF", configYAML)
	sshCmd(t, ctx, cfg, lbIP, writeConfigCmd)

	// Start udplb with sudo (needs CAP_BPF + CAP_NET_ADMIN for XDP).
	startCmd := "sudo nohup /tmp/udplb /tmp/udplb-config.yaml > /tmp/udplb.log 2>&1 &"
	sshCmd(t, ctx, cfg, lbIP, startCmd)
	t.Logf("Started udplb on %s", lbIP)

	// Wait briefly for udplb to initialize.
	time.Sleep(2 * time.Second)
}

// startAllUdplb starts udplb on all 3 LB VMs with the same config.
func startAllUdplb(t *testing.T, cfg *e2eutil.TestenvConfig) {
	t.Helper()

	configYAML := generateUdplbConfig(t, cfg)
	for _, lbIP := range allLBIPs(cfg) {
		startUdplb(t, cfg, lbIP, configYAML)
	}
}

// stopUdplb stops udplb on a given LB VM.
func stopUdplb(t *testing.T, cfg *e2eutil.TestenvConfig, lbIP string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()

	sshCmdIgnoreErr(ctx, cfg, lbIP, "sudo pkill -f '[/]tmp/udplb' || true")
}

// stopAllUdplb stops udplb on all 3 LB VMs.
func stopAllUdplb(t *testing.T, cfg *e2eutil.TestenvConfig) {
	t.Helper()

	for _, lbIP := range allLBIPs(cfg) {
		stopUdplb(t, cfg, lbIP)
	}
}

// collectEchoBackendLog retrieves the echo-backend log from a backend VM.
func collectEchoBackendLog(t *testing.T, cfg *e2eutil.TestenvConfig, vmIP string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()

	output, err := runSSHCommand(ctx, cfg.SSHKeyPath, sshUser, vmIP, "cat /tmp/echo-backend.log 2>/dev/null || echo ''")
	if err != nil {
		t.Logf("Warning: failed to collect echo-backend log from %s: %v", vmIP, err)
		return ""
	}
	return output
}

// backendReceivedPackets returns true if the echo-backend on vmIP has received at least 1 packet.
// It parses the JSON reports in the echo-backend log to check totalReceived.
func backendReceivedPackets(t *testing.T, cfg *e2eutil.TestenvConfig, vmIP string) bool {
	t.Helper()
	log := collectEchoBackendLog(t, cfg, vmIP)
	for _, line := range strings.Split(log, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var report struct {
			TotalReceived int `json:"totalReceived"`
		}
		if err := json.Unmarshal([]byte(line), &report); err != nil {
			continue
		}
		if report.TotalReceived > 0 {
			return true
		}
	}
	return false
}

// healthProbeUUID is the zero-UUID used by health probes.
const healthProbeUUID = "00000000-0000-0000-0000-000000000000"

// backendReceivedTrafficGenPackets returns true if the echo-backend on vmIP has received
// at least 1 packet with a non-zero session UUID (i.e., a real traffic-gen packet, not
// a health probe which uses uuid.Nil).
func backendReceivedTrafficGenPackets(t *testing.T, cfg *e2eutil.TestenvConfig, vmIP string) bool {
	t.Helper()
	log := collectEchoBackendLog(t, cfg, vmIP)
	for _, line := range strings.Split(log, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var report struct {
			Sessions map[string]int `json:"sessions"`
		}
		if err := json.Unmarshal([]byte(line), &report); err != nil {
			continue
		}
		for sid, count := range report.Sessions {
			if sid != healthProbeUUID && count > 0 {
				return true
			}
		}
	}
	return false
}

// backendTotalReceived returns the highest totalReceived value reported by the echo-backend on vmIP.
// Returns 0 if no reports found or if totalReceived is 0 in all reports.
func backendTotalReceived(t *testing.T, cfg *e2eutil.TestenvConfig, vmIP string) int {
	t.Helper()
	log := collectEchoBackendLog(t, cfg, vmIP)
	maxReceived := 0
	for _, line := range strings.Split(log, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var report struct {
			TotalReceived int `json:"totalReceived"`
		}
		if err := json.Unmarshal([]byte(line), &report); err != nil {
			continue
		}
		if report.TotalReceived > maxReceived {
			maxReceived = report.TotalReceived
		}
	}
	return maxReceived
}

// collectUdplbLog retrieves the udplb log from an LB VM.
func collectUdplbLog(t *testing.T, cfg *e2eutil.TestenvConfig, lbIP string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()

	output, err := runSSHCommand(ctx, cfg.SSHKeyPath, sshUser, lbIP, "cat /tmp/udplb.log 2>/dev/null || echo ''")
	if err != nil {
		t.Logf("Warning: failed to collect udplb log from %s: %v", lbIP, err)
		return ""
	}
	return output
}

// runTrafficGen runs the traffic generator on the client VM and returns its JSON output.
// The output is cleaned to extract only the JSON portion, stripping any warning/error
// lines that traffic-gen writes to stderr (which get mixed in via CombinedOutput).
func runTrafficGen(t *testing.T, cfg *e2eutil.TestenvConfig, sessions, pps int, duration time.Duration, payloadSize int) string {
	t.Helper()

	// Use a generous context timeout: duration + 30s buffer for SSH overhead.
	ctx, cancel := context.WithTimeout(context.Background(), duration+30*time.Second)
	defer cancel()

	cmd := fmt.Sprintf(
		"/tmp/udplb-traffic-gen --target %s:%d --sessions %d --rate %d --duration %s --payload-size %d",
		cfg.VIP, e2eutil.VIPPort,
		sessions, pps,
		duration.String(), payloadSize,
	)

	output := sshCmd(t, ctx, cfg, cfg.ClientIP, cmd)

	// Strip non-JSON lines (e.g., "warning: send error: ..." from stderr).
	// Find the first '{' which starts the JSON report.
	if idx := strings.Index(output, "{"); idx > 0 {
		output = output[idx:]
	}

	return output
}

// testCtx returns a context with the standard operation timeout for use in test functions.
func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	t.Cleanup(cancel)
	return ctx
}

// waitMs sleeps for the given number of milliseconds.
func waitMs(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

// cleanupAllProcesses stops all udplb and echo-backend processes on all VMs.
// Use in defer blocks for test cleanup.
func cleanupAllProcesses(t *testing.T, cfg *e2eutil.TestenvConfig) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()

	for _, lbIP := range allLBIPs(cfg) {
		sshCmdIgnoreErr(ctx, cfg, lbIP, "sudo pkill -f '[/]tmp/udplb' || true")
	}
	for _, beIP := range allBEIPs(cfg) {
		sshCmdIgnoreErr(ctx, cfg, beIP, "pkill -f '[u]dplb-echo-backend' || true")
	}
	sshCmdIgnoreErr(ctx, cfg, cfg.ClientIP, "pkill -f '[u]dplb-traffic-gen' || true")
}
