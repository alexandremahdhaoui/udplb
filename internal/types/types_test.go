//go:build unit

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
package types

import (
	"encoding/binary"
	"net"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

func TestNCoordinates(t *testing.T) {
	assert.Equal(t, 4, NCoordinates)
}

// ---------------------------------------------------------------------------
// State constants
// ---------------------------------------------------------------------------

func TestStateConstants(t *testing.T) {
	t.Run("StateUnknown is zero value", func(t *testing.T) {
		assert.Equal(t, State(0), StateUnknown)
	})

	t.Run("states are sequential iota values", func(t *testing.T) {
		assert.Equal(t, State(0), StateUnknown)
		assert.Equal(t, State(1), StateAvailable)
		assert.Equal(t, State(2), StateUnschedulable)
		assert.Equal(t, State(3), StateUnavailable)
	})

	t.Run("all states are distinct", func(t *testing.T) {
		states := []State{StateUnknown, StateAvailable, StateUnschedulable, StateUnavailable}
		seen := make(map[State]bool)
		for _, s := range states {
			assert.False(t, seen[s], "duplicate state value: %d", s)
			seen[s] = true
		}
	})
}

// ---------------------------------------------------------------------------
// StateMachineCommand constants
// ---------------------------------------------------------------------------

func TestStateMachineCommandConstants(t *testing.T) {
	t.Run("command string values", func(t *testing.T) {
		assert.Equal(t, StateMachineCommand("Add"), AddCommand)
		assert.Equal(t, StateMachineCommand("Append"), AppendCommand)
		assert.Equal(t, StateMachineCommand("Delete"), DeleteCommand)
		assert.Equal(t, StateMachineCommand("Put"), PutCommand)
		assert.Equal(t, StateMachineCommand("Subtract"), SubtractCommand)
	})

	t.Run("all commands are distinct", func(t *testing.T) {
		cmds := []StateMachineCommand{
			AddCommand, AppendCommand, DeleteCommand, PutCommand, SubtractCommand,
		}
		seen := make(map[StateMachineCommand]bool)
		for _, c := range cmds {
			assert.False(t, seen[c], "duplicate command: %s", c)
			seen[c] = true
		}
	})
}

// ---------------------------------------------------------------------------
// NewBackend / Backend.Coordinates
// ---------------------------------------------------------------------------

func TestNewBackend(t *testing.T) {
	t.Run("creates backend with correct fields", func(t *testing.T) {
		id := uuid.New()
		spec := BackendSpec{
			IP:      net.ParseIP("10.0.0.1"),
			Port:    8080,
			MacAddr: net.HardwareAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF},
			State:   StateAvailable,
		}
		status := BackendStatus{State: StateAvailable}

		b := NewBackend(id, spec, status)
		require.NotNil(t, b)
		assert.Equal(t, id, b.Id)
		assert.Equal(t, spec, b.Spec)
		assert.Equal(t, status, b.Status)
	})

	t.Run("computes coordinates from UUID bytes", func(t *testing.T) {
		// Use a well-known UUID so we can predict coordinates.
		id := uuid.MustParse("01020304-0506-0708-090a-0b0c0d0e0f10")
		b := NewBackend(id, BackendSpec{}, BackendStatus{})

		// The UUID bytes are [1,2,3,4, 5,6,7,8, 9,10,11,12, 13,14,15,16].
		// Coordinates are 4 uint32 values read via NativeEndian from 4-byte chunks.
		uuidBytes := id[:]
		expected := [NCoordinates]uint32{}
		for i := range NCoordinates {
			expected[i] = binary.NativeEndian.Uint32(uuidBytes[4*i : 4*(i+1)])
		}

		assert.Equal(t, expected, b.Coordinates())
	})

	t.Run("different UUIDs produce different coordinates", func(t *testing.T) {
		b1 := NewBackend(uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"), BackendSpec{}, BackendStatus{})
		b2 := NewBackend(uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"), BackendSpec{}, BackendStatus{})
		assert.NotEqual(t, b1.Coordinates(), b2.Coordinates())
	})

	t.Run("same UUID produces same coordinates", func(t *testing.T) {
		id := uuid.New()
		b1 := NewBackend(id, BackendSpec{}, BackendStatus{})
		b2 := NewBackend(id, BackendSpec{}, BackendStatus{})
		assert.Equal(t, b1.Coordinates(), b2.Coordinates())
	})
}

func TestBackendCoordinates(t *testing.T) {
	t.Run("returns cached coordinates", func(t *testing.T) {
		id := uuid.New()
		b := NewBackend(id, BackendSpec{}, BackendStatus{})
		// Call twice to confirm it returns the same cached value.
		c1 := b.Coordinates()
		c2 := b.Coordinates()
		assert.Equal(t, c1, c2)
	})

	t.Run("coordinate array length equals NCoordinates", func(t *testing.T) {
		b := NewBackend(uuid.New(), BackendSpec{}, BackendStatus{})
		coords := b.Coordinates()
		assert.Len(t, coords, NCoordinates)
	})
}

// ---------------------------------------------------------------------------
// GetConfig
// ---------------------------------------------------------------------------

func TestGetConfig(t *testing.T) {
	// GetConfig reads os.Args[1] when filepath != "-", so we must set os.Args
	// accordingly for file-based tests.

	t.Run("reads valid YAML config from file", func(t *testing.T) {
		yamlContent := `ifname: eth0
ip: 10.0.0.1
port: 5000
backends:
  - enabled: true
    ip: 10.0.0.2
    mac: "aa:bb:cc:dd:ee:ff"
    port: 8080
  - enabled: false
    ip: 10.0.0.3
    mac: "11:22:33:44:55:66"
    port: 9090
`
		tmpFile, err := os.CreateTemp("", "config-*.yaml")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())

		_, err = tmpFile.WriteString(yamlContent)
		require.NoError(t, err)
		require.NoError(t, tmpFile.Close())

		// GetConfig reads os.Args[1] for the file path (not the filepath param).
		origArgs := os.Args
		defer func() { os.Args = origArgs }()
		os.Args = []string{"test-binary", tmpFile.Name()}

		cfg, err := GetConfig(tmpFile.Name())
		require.NoError(t, err)

		assert.Equal(t, "eth0", cfg.Ifname)
		assert.Equal(t, "10.0.0.1", cfg.IP)
		assert.Equal(t, uint16(5000), cfg.Port)
		require.Len(t, cfg.Backends, 2)

		assert.True(t, cfg.Backends[0].Enabled)
		assert.Equal(t, "10.0.0.2", cfg.Backends[0].IP)
		assert.Equal(t, "aa:bb:cc:dd:ee:ff", cfg.Backends[0].MAC)
		assert.Equal(t, 8080, cfg.Backends[0].Port)

		assert.False(t, cfg.Backends[1].Enabled)
		assert.Equal(t, "10.0.0.3", cfg.Backends[1].IP)
		assert.Equal(t, "11:22:33:44:55:66", cfg.Backends[1].MAC)
		assert.Equal(t, 9090, cfg.Backends[1].Port)
	})

	t.Run("returns error for nonexistent file", func(t *testing.T) {
		origArgs := os.Args
		defer func() { os.Args = origArgs }()
		os.Args = []string{"test-binary", "/nonexistent/path/config.yaml"}

		_, err := GetConfig("/nonexistent/path/config.yaml")
		assert.Error(t, err)
	})

	t.Run("returns error for invalid YAML", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "config-invalid-*.yaml")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())

		_, err = tmpFile.WriteString("{{{{not valid yaml")
		require.NoError(t, err)
		require.NoError(t, tmpFile.Close())

		origArgs := os.Args
		defer func() { os.Args = origArgs }()
		os.Args = []string{"test-binary", tmpFile.Name()}

		_, err = GetConfig(tmpFile.Name())
		assert.Error(t, err)
	})

	t.Run("returns empty config for empty file", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "config-empty-*.yaml")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())
		require.NoError(t, tmpFile.Close())

		origArgs := os.Args
		defer func() { os.Args = origArgs }()
		os.Args = []string{"test-binary", tmpFile.Name()}

		cfg, err := GetConfig(tmpFile.Name())
		require.NoError(t, err)
		assert.Equal(t, Config{}, cfg)
	})

	t.Run("reads valid YAML config from stdin", func(t *testing.T) {
		yamlContent := `ifname: lo
ip: 127.0.0.1
port: 4000
backends:
  - enabled: true
    ip: 10.0.0.10
    mac: "de:ad:be:ef:00:01"
    port: 7070
`
		// Create a pipe and replace os.Stdin to simulate stdin input.
		r, w, err := os.Pipe()
		require.NoError(t, err)

		origStdin := os.Stdin
		defer func() { os.Stdin = origStdin }()
		os.Stdin = r

		// Write YAML content and close the writer so ReadAll returns.
		_, err = w.WriteString(yamlContent)
		require.NoError(t, err)
		require.NoError(t, w.Close())

		cfg, err := GetConfig("-")
		require.NoError(t, err)

		assert.Equal(t, "lo", cfg.Ifname)
		assert.Equal(t, "127.0.0.1", cfg.IP)
		assert.Equal(t, uint16(4000), cfg.Port)
		require.Len(t, cfg.Backends, 1)
		assert.True(t, cfg.Backends[0].Enabled)
		assert.Equal(t, "10.0.0.10", cfg.Backends[0].IP)
		assert.Equal(t, "de:ad:be:ef:00:01", cfg.Backends[0].MAC)
		assert.Equal(t, 7070, cfg.Backends[0].Port)
	})

	t.Run("returns error when stdin read fails", func(t *testing.T) {
		// Create a pipe, close the read end to cause an error, then
		// replace os.Stdin with the closed reader.
		r, w, err := os.Pipe()
		require.NoError(t, err)
		require.NoError(t, w.Close())
		require.NoError(t, r.Close())

		origStdin := os.Stdin
		defer func() { os.Stdin = origStdin }()
		os.Stdin = r

		_, err = GetConfig("-")
		assert.Error(t, err)
	})

	t.Run("config with only required fields", func(t *testing.T) {
		yamlContent := `ifname: lo
ip: 127.0.0.1
port: 3000
`
		tmpFile, err := os.CreateTemp("", "config-minimal-*.yaml")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())

		_, err = tmpFile.WriteString(yamlContent)
		require.NoError(t, err)
		require.NoError(t, tmpFile.Close())

		origArgs := os.Args
		defer func() { os.Args = origArgs }()
		os.Args = []string{"test-binary", tmpFile.Name()}

		cfg, err := GetConfig(tmpFile.Name())
		require.NoError(t, err)
		assert.Equal(t, "lo", cfg.Ifname)
		assert.Equal(t, "127.0.0.1", cfg.IP)
		assert.Equal(t, uint16(3000), cfg.Port)
		assert.Empty(t, cfg.Backends)
	})
}

// ---------------------------------------------------------------------------
// Struct types: basic field verification
// ---------------------------------------------------------------------------

func TestAssignment(t *testing.T) {
	backendId := uuid.New()
	sessionId := uuid.New()
	a := Assignment{BackendId: backendId, SessionId: sessionId}
	assert.Equal(t, backendId, a.BackendId)
	assert.Equal(t, sessionId, a.SessionId)
}

func TestBackendSpec(t *testing.T) {
	spec := BackendSpec{
		IP:      net.ParseIP("192.168.1.1"),
		Port:    443,
		MacAddr: net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		State:   StateUnschedulable,
	}
	assert.Equal(t, net.ParseIP("192.168.1.1"), spec.IP)
	assert.Equal(t, 443, spec.Port)
	assert.Equal(t, StateUnschedulable, spec.State)
}

func TestBackendStatus(t *testing.T) {
	status := BackendStatus{State: StateUnavailable}
	assert.Equal(t, StateUnavailable, status.State)
}

func TestBackendStatusEntry(t *testing.T) {
	id := uuid.New()
	entry := BackendStatusEntry{BackendId: id, State: StateAvailable}
	assert.Equal(t, id, entry.BackendId)
	assert.Equal(t, StateAvailable, entry.State)
}

func TestBackendConfig(t *testing.T) {
	bc := BackendConfig{
		Enabled: true,
		IP:      "10.0.0.5",
		MAC:     "aa:bb:cc:dd:ee:ff",
		Port:    9999,
	}
	assert.True(t, bc.Enabled)
	assert.Equal(t, "10.0.0.5", bc.IP)
	assert.Equal(t, "aa:bb:cc:dd:ee:ff", bc.MAC)
	assert.Equal(t, 9999, bc.Port)
}

func TestConfig(t *testing.T) {
	cfg := Config{
		Ifname: "eth0",
		IP:     "10.0.0.1",
		Port:   5000,
		Backends: []BackendConfig{
			{Enabled: true, IP: "10.0.0.2", MAC: "aa:bb:cc:dd:ee:ff", Port: 8080},
		},
	}
	assert.Equal(t, "eth0", cfg.Ifname)
	assert.Equal(t, "10.0.0.1", cfg.IP)
	assert.Equal(t, uint16(5000), cfg.Port)
	require.Len(t, cfg.Backends, 1)
	assert.Equal(t, "10.0.0.2", cfg.Backends[0].IP)
}
