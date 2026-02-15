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
const (
	defaultVM0IP    = "192.168.200.10"
	defaultVM1IP    = "192.168.200.11"
	defaultVM2IP    = "192.168.200.12"
	defaultBridgeIP = "192.168.200.1"
)

// TestenvConfig holds configuration from testenv-vm environment variables.
type TestenvConfig struct {
	// VM0IP is the IP of vm0-lb. Env: TESTENV_VM_VM0_LB_IP, fallback "192.168.200.10".
	VM0IP string
	// VM1IP is the IP of vm1-lb. Env: TESTENV_VM_VM1_LB_IP, fallback "192.168.200.11".
	VM1IP string
	// VM2IP is the IP of vm2-lb. Env: TESTENV_VM_VM2_LB_IP, fallback "192.168.200.12".
	VM2IP string
	// SSHKeyPath is the path to the SSH private key. Env: TESTENV_KEY_VMSSH_PRIVATE_PATH.
	SSHKeyPath string
	// BridgeIP is the IP of the bridge network. Env: TESTENV_NETWORK_UDPLBNET_IP, fallback "192.168.200.1".
	BridgeIP string
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
	cfg.VM0IP = os.Getenv("TESTENV_VM_VM0_LB_IP")
	if cfg.VM0IP == "" {
		cfg.VM0IP = defaultVM0IP
	}

	cfg.VM1IP = os.Getenv("TESTENV_VM_VM1_LB_IP")
	if cfg.VM1IP == "" {
		cfg.VM1IP = defaultVM1IP
	}

	cfg.VM2IP = os.Getenv("TESTENV_VM_VM2_LB_IP")
	if cfg.VM2IP == "" {
		cfg.VM2IP = defaultVM2IP
	}

	// Bridge IP: use env var if set, otherwise fall back to default.
	cfg.BridgeIP = os.Getenv("TESTENV_NETWORK_UDPLBNET_IP")
	if cfg.BridgeIP == "" {
		cfg.BridgeIP = defaultBridgeIP
	}

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
		VM0IP:    defaultVM0IP,
		VM1IP:    defaultVM1IP,
		VM2IP:    defaultVM2IP,
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
