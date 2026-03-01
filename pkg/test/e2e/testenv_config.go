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

package e2e

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ErrEnvVarNotSet indicates a required testenv environment variable is not set.
var ErrEnvVarNotSet = errors.New("testenv environment variable not set")

// ErrStateFileNotFound indicates the testenv-vm state file was not found.
var ErrStateFileNotFound = errors.New("testenv-vm state file not found")

// Default static IPs assigned via cloud-init networkConfig in forge.yaml.
// 8-VM topology: router + client + 3 LBs + 3 backends on flat L2 (10.100.0.0/24).
const (
	defaultRouterIP = "10.100.0.2"
	defaultClientIP = "10.100.0.100"
	defaultLB0IP    = "10.100.0.10"
	defaultLB1IP    = "10.100.0.11"
	defaultLB2IP    = "10.100.0.12"
	defaultBE0IP    = "10.100.0.20"
	defaultBE1IP    = "10.100.0.21"
	defaultBE2IP    = "10.100.0.22"
	defaultBridgeIP = "10.100.0.1"

	// defaultVIPSuffix is the last octet of the VIP address within the subnet.
	defaultVIPSuffix = "200"
	// VIPPort is the UDP port for the VIP service.
	VIPPort = 12345
	// BackendPort is the UDP port for echo backends.
	BackendPort = 8080
)

// TestenvConfig holds configuration from testenv-vm environment variables.
// Maps to the 8-VM topology defined in forge.yaml engines section.
type TestenvConfig struct {
	// RouterIP is the IP of the FRR BGP router. Env: TESTENV_VM_ROUTER_IP, fallback "10.100.0.2".
	RouterIP string
	// ClientIP is the IP of the traffic generator client. Env: TESTENV_VM_CLIENT_IP, fallback "10.100.0.100".
	ClientIP string
	// LB0IP is the IP of lb-0. Env: TESTENV_VM_LB_0_IP, fallback "10.100.0.10".
	LB0IP string
	// LB1IP is the IP of lb-1. Env: TESTENV_VM_LB_1_IP, fallback "10.100.0.11".
	LB1IP string
	// LB2IP is the IP of lb-2. Env: TESTENV_VM_LB_2_IP, fallback "10.100.0.12".
	LB2IP string
	// BE0IP is the IP of be-0. Env: TESTENV_VM_BE_0_IP, fallback "10.100.0.20".
	BE0IP string
	// BE1IP is the IP of be-1. Env: TESTENV_VM_BE_1_IP, fallback "10.100.0.21".
	BE1IP string
	// BE2IP is the IP of be-2. Env: TESTENV_VM_BE_2_IP, fallback "10.100.0.22".
	BE2IP string
	// SSHKeyPath is the path to the SSH private key. Env: TESTENV_KEY_VMSSH_PRIVATE_PATH.
	SSHKeyPath string
	// BridgeIP is the IP of the bridge network. Env: TESTENV_NETWORK_UDPLBNET_IP, fallback "10.100.0.1".
	BridgeIP string
	// VIP is the virtual IP address announced by LBs via BGP, derived from BridgeIP subnet.
	VIP string
}

// computeVIP derives the VIP address from the bridge IP by replacing the last octet with defaultVIPSuffix.
func computeVIP(bridgeIP string) string {
	parts := strings.Split(bridgeIP, ".")
	if len(parts) == 4 {
		return strings.Join(parts[:3], ".") + "." + defaultVIPSuffix
	}
	return "10.100.0." + defaultVIPSuffix
}

// testenvVMState represents the structure of the testenv-vm state file.
type testenvVMState struct {
	ID        string `json:"id"`
	Stage     string `json:"stage"`
	Status    string `json:"status"`
	Resources struct {
		Keys map[string]struct {
			State struct {
				PrivateKeyPath string `json:"privateKeyPath"`
			} `json:"state"`
		} `json:"keys"`
		Networks map[string]struct {
			State struct {
				IP            string `json:"ip"`
				InterfaceName string `json:"interfaceName"`
			} `json:"state"`
		} `json:"networks"`
		VMs map[string]struct {
			State struct {
				Name string `json:"name"`
				MAC  string `json:"mac"`
				UUID string `json:"uuid"`
			} `json:"state"`
		} `json:"vms"`
	} `json:"resources"`
}

// LoadTestenvConfig loads configuration from testenv-vm environment variables.
// If environment variables are not set, it falls back to reading the testenv-vm state file.
// Returns error if configuration cannot be loaded from either source.
func LoadTestenvConfig() (*TestenvConfig, error) {
	cfg, envErr := loadFromEnv()
	if envErr == nil {
		return cfg, nil
	}

	cfg, stateErr := loadFromStateFile()
	if stateErr == nil {
		return cfg, nil
	}

	return nil, errors.Join(envErr, stateErr)
}

// loadFromEnv loads configuration from environment variables.
func loadFromEnv() (*TestenvConfig, error) {
	cfg := &TestenvConfig{}
	var missing []string

	// SSH key is required.
	cfg.SSHKeyPath = os.Getenv("TESTENV_KEY_VMSSH_PRIVATE_PATH")
	if cfg.SSHKeyPath == "" {
		missing = append(missing, "TESTENV_KEY_VMSSH_PRIVATE_PATH")
	}

	if len(missing) > 0 {
		return nil, errors.Join(ErrEnvVarNotSet,
			errors.New("missing: "+strings.Join(missing, ", ")))
	}

	// VM IPs: use env var if set, otherwise fall back to static cloud-init IPs.
	cfg.RouterIP = os.Getenv("TESTENV_VM_ROUTER_IP")
	if cfg.RouterIP == "" {
		cfg.RouterIP = defaultRouterIP
	}

	cfg.ClientIP = os.Getenv("TESTENV_VM_CLIENT_IP")
	if cfg.ClientIP == "" {
		cfg.ClientIP = defaultClientIP
	}

	cfg.LB0IP = os.Getenv("TESTENV_VM_LB_0_IP")
	if cfg.LB0IP == "" {
		cfg.LB0IP = defaultLB0IP
	}

	cfg.LB1IP = os.Getenv("TESTENV_VM_LB_1_IP")
	if cfg.LB1IP == "" {
		cfg.LB1IP = defaultLB1IP
	}

	cfg.LB2IP = os.Getenv("TESTENV_VM_LB_2_IP")
	if cfg.LB2IP == "" {
		cfg.LB2IP = defaultLB2IP
	}

	cfg.BE0IP = os.Getenv("TESTENV_VM_BE_0_IP")
	if cfg.BE0IP == "" {
		cfg.BE0IP = defaultBE0IP
	}

	cfg.BE1IP = os.Getenv("TESTENV_VM_BE_1_IP")
	if cfg.BE1IP == "" {
		cfg.BE1IP = defaultBE1IP
	}

	cfg.BE2IP = os.Getenv("TESTENV_VM_BE_2_IP")
	if cfg.BE2IP == "" {
		cfg.BE2IP = defaultBE2IP
	}

	// Bridge IP: use env var if set, otherwise fall back to default.
	cfg.BridgeIP = os.Getenv("TESTENV_NETWORK_UDPLBNET_IP")
	if cfg.BridgeIP == "" {
		cfg.BridgeIP = defaultBridgeIP
	}

	// Derive VIP from bridge IP subnet.
	cfg.VIP = computeVIP(cfg.BridgeIP)

	return cfg, nil
}

// loadFromStateFile loads configuration from the testenv-vm state file.
func loadFromStateFile() (*TestenvConfig, error) {
	stateDir := os.Getenv("TESTENV_VM_STATE_DIR")
	if stateDir == "" {
		stateDir = "/tmp/udplb-testenv-vm"
	}

	stateFilesDir := filepath.Join(stateDir, "state")
	entries, err := os.ReadDir(stateFilesDir)
	if err != nil {
		return nil, errors.Join(ErrStateFileNotFound, err)
	}

	// Find a state file for e2e stage with status "ready".
	var stateFile string
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "testenv-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		candidatePath := filepath.Join(stateFilesDir, entry.Name())
		data, err := os.ReadFile(candidatePath)
		if err != nil {
			continue
		}

		var state testenvVMState
		if err := json.Unmarshal(data, &state); err != nil {
			continue
		}

		if state.Stage == "e2e" && state.Status == "ready" {
			stateFile = candidatePath
			break
		}
	}

	if stateFile == "" {
		return nil, errors.Join(ErrStateFileNotFound,
			errors.New("no e2e state file found in "+stateFilesDir))
	}

	data, err := os.ReadFile(stateFile)
	if err != nil {
		return nil, errors.Join(ErrStateFileNotFound, err)
	}

	var state testenvVMState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, errors.Join(ErrStateFileNotFound, err)
	}

	cfg := &TestenvConfig{
		// Use static cloud-init IPs as defaults.
		RouterIP: defaultRouterIP,
		ClientIP: defaultClientIP,
		LB0IP:    defaultLB0IP,
		LB1IP:    defaultLB1IP,
		LB2IP:    defaultLB2IP,
		BE0IP:    defaultBE0IP,
		BE1IP:    defaultBE1IP,
		BE2IP:    defaultBE2IP,
		BridgeIP: defaultBridgeIP,
	}

	// Extract SSH key path from state.
	if keyState, ok := state.Resources.Keys["VmSsh"]; ok {
		cfg.SSHKeyPath = keyState.State.PrivateKeyPath
	}

	// Extract network IP from state.
	if netState, ok := state.Resources.Networks["UdplbNet"]; ok {
		cfg.BridgeIP = netState.State.IP
	}

	// Validate that SSH key path was found.
	if cfg.SSHKeyPath == "" {
		return nil, errors.Join(ErrStateFileNotFound,
			errors.New("SSH key path not found in state file"))
	}

	// Derive VIP from bridge IP subnet.
	cfg.VIP = computeVIP(cfg.BridgeIP)

	return cfg, nil
}

// MustLoadTestenvConfig loads configuration or panics.
// Use in tests where missing config should fail the test.
func MustLoadTestenvConfig() *TestenvConfig {
	cfg, err := LoadTestenvConfig()
	if err != nil {
		panic("failed to load testenv config: " + err.Error())
	}
	return cfg
}
