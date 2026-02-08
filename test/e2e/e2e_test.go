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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestenvConfig holds configuration for testenv-vm environment.
type TestenvConfig struct {
	// VM information - static IPs from forge.yaml cloud-init
	VM0IP string // vm0-lb: 192.168.200.10
	VM1IP string // vm1-lb: 192.168.200.11
	VM2IP string // vm2-lb: 192.168.200.12

	// SSH key information
	SSHKeyPath string

	// Network information
	BridgeIP string
}

// LoadTestenvConfig loads configuration from testenv-vm environment.
// Uses static IPs configured via cloud-init networkConfig in forge.yaml.
func LoadTestenvConfig() (*TestenvConfig, error) {
	cfg := &TestenvConfig{
		BridgeIP: "192.168.200.1",
		// Static IPs from forge.yaml cloud-init networkConfig
		VM0IP: "192.168.200.10",
		VM1IP: "192.168.200.11",
		VM2IP: "192.168.200.12",
	}

	// Try to find SSH key from testenv-vm
	uid := os.Getuid()
	keyDir := fmt.Sprintf("/tmp/testenv-vm-%d/keys", uid)
	keyPath := filepath.Join(keyDir, "VmSsh")

	if _, err := os.Stat(keyPath); err != nil {
		return nil, fmt.Errorf("SSH key not found at %s - is the test environment running? (forge test create-env e2e)", keyPath)
	}
	cfg.SSHKeyPath = keyPath

	return cfg, nil
}

// runSSHCommand executes a command on a remote VM via SSH.
func runSSHCommand(ctx context.Context, sshKeyPath, user, host, command string) (string, error) {
	args := []string{
		"-i", sshKeyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=10",
		fmt.Sprintf("%s@%s", user, host),
		command,
	}

	cmd := exec.CommandContext(ctx, "ssh", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("SSH command failed: %w\nOutput: %s", err, string(output))
	}

	return string(output), nil
}

// copyFileToVM copies a local file to a remote VM via SCP.
func copyFileToVM(ctx context.Context, sshKeyPath, user, host, localPath, remotePath string) error {
	args := []string{
		"-i", sshKeyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=10",
		localPath,
		fmt.Sprintf("%s@%s:%s", user, host, remotePath),
	}

	cmd := exec.CommandContext(ctx, "scp", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("SCP failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// findUdplbBinary finds the udplb binary in the build directory.
func findUdplbBinary() (string, error) {
	// Try common locations
	locations := []string{
		"./build/bin/udplb",
		"./udplb",
		"../../build/bin/udplb",
		"../../udplb",
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			absPath, err := filepath.Abs(loc)
			if err != nil {
				continue
			}
			return absPath, nil
		}
	}

	return "", errors.New("udplb binary not found - run 'forge build udplb' first")
}

// TestUDPLBE2E_VMsReachable tests that all VMs are reachable via SSH.
func TestUDPLBE2E_VMsReachable(t *testing.T) {
	cfg, err := LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available - run with 'forge test run e2e'")

	t.Logf("Using testenv configuration:")
	t.Logf("  VM0 IP: %s", cfg.VM0IP)
	t.Logf("  VM1 IP: %s", cfg.VM1IP)
	t.Logf("  VM2 IP: %s", cfg.VM2IP)
	t.Logf("  SSH Key: %s", cfg.SSHKeyPath)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	vms := []struct {
		name string
		ip   string
	}{
		{"vm0-lb", cfg.VM0IP},
		{"vm1-lb", cfg.VM1IP},
		{"vm2-lb", cfg.VM2IP},
	}

	for _, vm := range vms {
		t.Run(vm.name, func(t *testing.T) {
			output, err := runSSHCommand(ctx, cfg.SSHKeyPath, "ubuntu", vm.ip, "hostname")
			require.NoError(t, err, "failed to connect to %s", vm.name)
			t.Logf("Connected to %s, hostname: %s", vm.name, strings.TrimSpace(output))
		})
	}
}

// TestUDPLBE2E_CopyBinary tests copying the udplb binary to VMs.
func TestUDPLBE2E_CopyBinary(t *testing.T) {
	cfg, err := LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available - run with 'forge test run e2e'")

	binaryPath, err := findUdplbBinary()
	require.NoError(t, err, "udplb binary must exist")
	t.Logf("Found udplb binary at: %s", binaryPath)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	vms := []struct {
		name string
		ip   string
	}{
		{"vm0-lb", cfg.VM0IP},
		{"vm1-lb", cfg.VM1IP},
		{"vm2-lb", cfg.VM2IP},
	}

	for _, vm := range vms {
		t.Run(vm.name, func(t *testing.T) {
			// Copy the binary
			err := copyFileToVM(ctx, cfg.SSHKeyPath, "ubuntu", vm.ip, binaryPath, "/tmp/udplb")
			require.NoError(t, err, "failed to copy udplb binary to %s", vm.name)

			// Make it executable
			_, err = runSSHCommand(ctx, cfg.SSHKeyPath, "ubuntu", vm.ip, "chmod +x /tmp/udplb")
			require.NoError(t, err, "failed to make udplb executable on %s", vm.name)

			// Verify it's there
			output, err := runSSHCommand(ctx, cfg.SSHKeyPath, "ubuntu", vm.ip, "ls -la /tmp/udplb")
			require.NoError(t, err, "failed to verify udplb on %s", vm.name)
			t.Logf("udplb binary on %s: %s", vm.name, strings.TrimSpace(output))
		})
	}
}

// TestUDPLBE2E_UDPConnectivity tests basic UDP connectivity between VMs.
func TestUDPLBE2E_UDPConnectivity(t *testing.T) {
	cfg, err := LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available - run with 'forge test run e2e'")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Cleanup any leftover socat processes from previous test runs
	_, _ = runSSHCommand(ctx, cfg.SSHKeyPath, "ubuntu", cfg.VM1IP, "pkill socat; rm -f /tmp/udp_received.txt")
	time.Sleep(1 * time.Second)

	// Start a simple UDP listener on vm1-lb using socat
	t.Log("Starting UDP listener on vm1-lb...")
	listenerCmd := "nohup socat -u UDP4-LISTEN:12345,reuseaddr,fork OPEN:/tmp/udp_received.txt,creat,append > /dev/null 2>&1 &"
	_, err = runSSHCommand(ctx, cfg.SSHKeyPath, "ubuntu", cfg.VM1IP, listenerCmd)
	require.NoError(t, err, "failed to start UDP listener on vm1-lb")

	// Give the listener time to start
	time.Sleep(2 * time.Second)

	// Send a UDP packet from vm0-lb to vm1-lb
	t.Log("Sending UDP packet from vm0-lb to vm1-lb...")
	sendCmd := fmt.Sprintf("echo 'hello from vm0' | socat -u - UDP4-SENDTO:%s:12345", cfg.VM1IP)
	_, err = runSSHCommand(ctx, cfg.SSHKeyPath, "ubuntu", cfg.VM0IP, sendCmd)
	require.NoError(t, err, "failed to send UDP packet from vm0-lb")

	// Give time for packet to arrive
	time.Sleep(2 * time.Second)

	// Check if packet was received
	t.Log("Checking if packet was received on vm1-lb...")
	output, err := runSSHCommand(ctx, cfg.SSHKeyPath, "ubuntu", cfg.VM1IP, "cat /tmp/udp_received.txt")
	require.NoError(t, err, "failed to check received packets on vm1-lb")
	require.Contains(t, output, "hello from vm0", "UDP packet should have been received")

	t.Log("UDP connectivity verified between VMs")

	// Cleanup
	_, _ = runSSHCommand(ctx, cfg.SSHKeyPath, "ubuntu", cfg.VM1IP, "pkill socat; rm -f /tmp/udp_received.txt")
}

// TestUDPLBE2E_NetworkInterfaces verifies network interfaces on VMs.
func TestUDPLBE2E_NetworkInterfaces(t *testing.T) {
	cfg, err := LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available - run with 'forge test run e2e'")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	vms := []struct {
		name string
		ip   string
	}{
		{"vm0-lb", cfg.VM0IP},
		{"vm1-lb", cfg.VM1IP},
		{"vm2-lb", cfg.VM2IP},
	}

	for _, vm := range vms {
		t.Run(vm.name, func(t *testing.T) {
			// Check network interfaces - Alpine may use eth0 or enp*
			output, err := runSSHCommand(ctx, cfg.SSHKeyPath, "ubuntu", vm.ip, "ip addr")
			require.NoError(t, err, "failed to get network interfaces on %s", vm.name)
			t.Logf("Network interfaces on %s:\n%s", vm.name, output)
			// Verify the VM has the expected IP on some interface
			require.Contains(t, output, vm.ip, "VM should have its assigned IP address")
		})
	}
}
