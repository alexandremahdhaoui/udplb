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

	e2eutil "github.com/alexandremahdhaoui/udplb/pkg/test/e2e"
)

// runSSHCommand executes a command on a remote VM via SSH.
func runSSHCommand(ctx context.Context, sshKeyPath, user, host, command string) (string, error) {
	args := []string{
		"-i", sshKeyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
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
		"-o", "LogLevel=ERROR",
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

// findBinary finds a binary in the build directory by name.
func findBinary(name string) (string, error) {
	locations := []string{
		"./build/bin/" + name,
		"./" + name,
		"../../build/bin/" + name,
		"../../" + name,
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

	return "", errors.New(name + " binary not found - run 'forge build " + name + "' first")
}

// TestUDPLBE2E_VMsReachable tests that all 8 VMs are reachable via SSH.
func TestUDPLBE2E_VMsReachable(t *testing.T) {
	cfg, err := e2eutil.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available - run with 'forge test run e2e'")

	t.Logf("Using testenv configuration:")
	t.Logf("  Router IP: %s", cfg.RouterIP)
	t.Logf("  Client IP: %s", cfg.ClientIP)
	t.Logf("  LB-0 IP:   %s", cfg.LB0IP)
	t.Logf("  LB-1 IP:   %s", cfg.LB1IP)
	t.Logf("  LB-2 IP:   %s", cfg.LB2IP)
	t.Logf("  BE-0 IP:   %s", cfg.BE0IP)
	t.Logf("  BE-1 IP:   %s", cfg.BE1IP)
	t.Logf("  BE-2 IP:   %s", cfg.BE2IP)
	t.Logf("  SSH Key:   %s", cfg.SSHKeyPath)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	vms := []struct {
		name string
		ip   string
	}{
		{"router", cfg.RouterIP},
		{"client", cfg.ClientIP},
		{"lb-0", cfg.LB0IP},
		{"lb-1", cfg.LB1IP},
		{"lb-2", cfg.LB2IP},
		{"be-0", cfg.BE0IP},
		{"be-1", cfg.BE1IP},
		{"be-2", cfg.BE2IP},
	}

	for _, vm := range vms {
		t.Run(vm.name, func(t *testing.T) {
			output, err := runSSHCommand(ctx, cfg.SSHKeyPath, "ubuntu", vm.ip, "hostname")
			require.NoError(t, err, "failed to connect to %s", vm.name)
			t.Logf("Connected to %s, hostname: %s", vm.name, strings.TrimSpace(output))
		})
	}
}

// TestUDPLBE2E_CopyBinary tests copying the udplb and support binaries to VMs.
func TestUDPLBE2E_CopyBinary(t *testing.T) {
	cfg, err := e2eutil.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available - run with 'forge test run e2e'")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Copy udplb binary to all LB VMs.
	udplbPath, err := findBinary("udplb")
	require.NoError(t, err, "udplb binary must exist")
	t.Logf("Found udplb binary at: %s", udplbPath)

	lbVMs := []struct {
		name string
		ip   string
	}{
		{"lb-0", cfg.LB0IP},
		{"lb-1", cfg.LB1IP},
		{"lb-2", cfg.LB2IP},
	}

	for _, vm := range lbVMs {
		t.Run("udplb/"+vm.name, func(t *testing.T) {
			err := copyFileToVM(ctx, cfg.SSHKeyPath, "ubuntu", vm.ip, udplbPath, "/tmp/udplb")
			require.NoError(t, err, "failed to copy udplb binary to %s", vm.name)

			_, err = runSSHCommand(ctx, cfg.SSHKeyPath, "ubuntu", vm.ip, "chmod +x /tmp/udplb")
			require.NoError(t, err, "failed to make udplb executable on %s", vm.name)

			output, err := runSSHCommand(ctx, cfg.SSHKeyPath, "ubuntu", vm.ip, "ls -la /tmp/udplb")
			require.NoError(t, err, "failed to verify udplb on %s", vm.name)
			t.Logf("udplb binary on %s: %s", vm.name, strings.TrimSpace(output))
		})
	}

	// Copy echo-backend binary to all backend VMs.
	echoPath, err := findBinary("udplb-echo-backend")
	require.NoError(t, err, "udplb-echo-backend binary must exist")
	t.Logf("Found udplb-echo-backend binary at: %s", echoPath)

	beVMs := []struct {
		name string
		ip   string
	}{
		{"be-0", cfg.BE0IP},
		{"be-1", cfg.BE1IP},
		{"be-2", cfg.BE2IP},
	}

	for _, vm := range beVMs {
		t.Run("echo-backend/"+vm.name, func(t *testing.T) {
			err := copyFileToVM(ctx, cfg.SSHKeyPath, "ubuntu", vm.ip, echoPath, "/tmp/udplb-echo-backend")
			require.NoError(t, err, "failed to copy udplb-echo-backend binary to %s", vm.name)

			_, err = runSSHCommand(ctx, cfg.SSHKeyPath, "ubuntu", vm.ip, "chmod +x /tmp/udplb-echo-backend")
			require.NoError(t, err, "failed to make udplb-echo-backend executable on %s", vm.name)

			output, err := runSSHCommand(ctx, cfg.SSHKeyPath, "ubuntu", vm.ip, "ls -la /tmp/udplb-echo-backend")
			require.NoError(t, err, "failed to verify udplb-echo-backend on %s", vm.name)
			t.Logf("udplb-echo-backend binary on %s: %s", vm.name, strings.TrimSpace(output))
		})
	}
}

// TestUDPLBE2E_UDPConnectivity tests basic UDP connectivity between VMs.
func TestUDPLBE2E_UDPConnectivity(t *testing.T) {
	cfg, err := e2eutil.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available - run with 'forge test run e2e'")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Cleanup any leftover socat processes from previous test runs on be-0.
	_, _ = runSSHCommand(ctx, cfg.SSHKeyPath, "ubuntu", cfg.BE0IP, "pkill socat; rm -f /tmp/udp_received.txt")
	time.Sleep(1 * time.Second)

	// Start a UDP listener on be-0 using socat.
	t.Log("Starting UDP listener on be-0...")
	listenerCmd := fmt.Sprintf("nohup socat -u UDP4-LISTEN:%d,reuseaddr,fork OPEN:/tmp/udp_received.txt,creat,append > /dev/null 2>&1 &", e2eutil.UDPTestPort)
	_, err = runSSHCommand(ctx, cfg.SSHKeyPath, "ubuntu", cfg.BE0IP, listenerCmd)
	require.NoError(t, err, "failed to start UDP listener on be-0")

	// Give the listener time to start.
	time.Sleep(2 * time.Second)

	// Send a UDP packet from lb-0 to be-0.
	t.Log("Sending UDP packet from lb-0 to be-0...")
	sendCmd := fmt.Sprintf("echo 'hello from lb-0' | socat -u - UDP4-SENDTO:%s:%d", cfg.BE0IP, e2eutil.UDPTestPort)
	_, err = runSSHCommand(ctx, cfg.SSHKeyPath, "ubuntu", cfg.LB0IP, sendCmd)
	require.NoError(t, err, "failed to send UDP packet from lb-0")

	// Give time for packet to arrive.
	time.Sleep(2 * time.Second)

	// Check if packet was received.
	t.Log("Checking if packet was received on be-0...")
	output, err := runSSHCommand(ctx, cfg.SSHKeyPath, "ubuntu", cfg.BE0IP, "cat /tmp/udp_received.txt")
	require.NoError(t, err, "failed to check received packets on be-0")
	require.Contains(t, output, "hello from lb-0", "UDP packet should have been received")

	t.Log("UDP connectivity verified between VMs")

	// Cleanup.
	_, _ = runSSHCommand(ctx, cfg.SSHKeyPath, "ubuntu", cfg.BE0IP, "pkill socat; rm -f /tmp/udp_received.txt")
}

// TestUDPLBE2E_NetworkInterfaces verifies network interfaces on all 8 VMs.
func TestUDPLBE2E_NetworkInterfaces(t *testing.T) {
	cfg, err := e2eutil.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available - run with 'forge test run e2e'")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	vms := []struct {
		name string
		ip   string
	}{
		{"router", cfg.RouterIP},
		{"client", cfg.ClientIP},
		{"lb-0", cfg.LB0IP},
		{"lb-1", cfg.LB1IP},
		{"lb-2", cfg.LB2IP},
		{"be-0", cfg.BE0IP},
		{"be-1", cfg.BE1IP},
		{"be-2", cfg.BE2IP},
	}

	for _, vm := range vms {
		t.Run(vm.name, func(t *testing.T) {
			output, err := runSSHCommand(ctx, cfg.SSHKeyPath, "ubuntu", vm.ip, "ip addr")
			require.NoError(t, err, "failed to get network interfaces on %s", vm.name)
			t.Logf("Network interfaces on %s:\n%s", vm.name, output)
			// Verify the VM has the expected IP on some interface.
			require.Contains(t, output, vm.ip, "VM should have its assigned IP address")
		})
	}
}
