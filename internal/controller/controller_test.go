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
package controller

import (
	"fmt"
	"net"
	"testing"

	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------
// -- buildBackends
// -------------------------------------------------------------------

func TestBuildBackends(t *testing.T) {
	t.Run("converts enabled backends to Backend pointers", func(t *testing.T) {
		config := types.Config{
			Backends: []types.BackendConfig{
				{Enabled: true, IP: "10.0.0.1", MAC: "aa:bb:cc:dd:ee:ff", Port: 8080},
				{Enabled: true, IP: "10.0.0.2", MAC: "11:22:33:44:55:66", Port: 9090},
			},
		}
		backends, err := buildBackends(config)
		require.NoError(t, err)
		require.Len(t, backends, 2)

		assert.Equal(t, net.ParseIP("10.0.0.1"), backends[0].Spec.IP)
		assert.Equal(t, 8080, backends[0].Spec.Port)
		assert.Equal(t, net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}, backends[0].Spec.MacAddr)
		assert.Equal(t, types.StateAvailable, backends[0].Spec.State)
		assert.Equal(t, types.StateAvailable, backends[0].Status.State)

		assert.Equal(t, net.ParseIP("10.0.0.2"), backends[1].Spec.IP)
		assert.Equal(t, 9090, backends[1].Spec.Port)
	})

	t.Run("skips disabled backends", func(t *testing.T) {
		config := types.Config{
			Backends: []types.BackendConfig{
				{Enabled: false, IP: "10.0.0.1", MAC: "aa:bb:cc:dd:ee:ff", Port: 8080},
				{Enabled: true, IP: "10.0.0.2", MAC: "11:22:33:44:55:66", Port: 9090},
				{Enabled: false, IP: "10.0.0.3", MAC: "de:ad:be:ef:00:01", Port: 7070},
			},
		}
		backends, err := buildBackends(config)
		require.NoError(t, err)
		require.Len(t, backends, 1)
		assert.Equal(t, net.ParseIP("10.0.0.2"), backends[0].Spec.IP)
	})

	t.Run("returns empty slice for no enabled backends", func(t *testing.T) {
		config := types.Config{
			Backends: []types.BackendConfig{
				{Enabled: false, IP: "10.0.0.1", MAC: "aa:bb:cc:dd:ee:ff", Port: 8080},
			},
		}
		backends, err := buildBackends(config)
		require.NoError(t, err)
		assert.Empty(t, backends)
	})

	t.Run("returns empty slice for no backends in config", func(t *testing.T) {
		config := types.Config{}
		backends, err := buildBackends(config)
		require.NoError(t, err)
		assert.Empty(t, backends)
	})

	t.Run("generates deterministic UUIDs", func(t *testing.T) {
		config := types.Config{
			Backends: []types.BackendConfig{
				{Enabled: true, IP: "10.0.0.1", MAC: "aa:bb:cc:dd:ee:ff", Port: 8080},
			},
		}
		backends1, err := buildBackends(config)
		require.NoError(t, err)
		backends2, err := buildBackends(config)
		require.NoError(t, err)

		assert.Equal(t, backends1[0].Id, backends2[0].Id)

		expectedID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("%s:%d", "10.0.0.1", 8080)))
		assert.Equal(t, expectedID, backends1[0].Id)
	})

	t.Run("returns error for invalid IP", func(t *testing.T) {
		config := types.Config{
			Backends: []types.BackendConfig{
				{Enabled: true, IP: "not-an-ip", MAC: "aa:bb:cc:dd:ee:ff", Port: 8080},
			},
		}
		_, err := buildBackends(config)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidConfig)
	})

	t.Run("returns error for invalid MAC", func(t *testing.T) {
		config := types.Config{
			Backends: []types.BackendConfig{
				{Enabled: true, IP: "10.0.0.1", MAC: "not-a-mac", Port: 8080},
			},
		}
		_, err := buildBackends(config)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidConfig)
	})
}

// -------------------------------------------------------------------
// -- computeLookupTableSize
// -------------------------------------------------------------------

func TestComputeLookupTableSize(t *testing.T) {
	t.Run("returns smallest prime >= 2*n", func(t *testing.T) {
		// n=1, target=2, smallest prime >= 2 is 7
		assert.Equal(t, uint32(7), computeLookupTableSize(1))
		// n=3, target=6, smallest prime >= 6 is 7
		assert.Equal(t, uint32(7), computeLookupTableSize(3))
		// n=4, target=8, smallest prime >= 8 is 13
		assert.Equal(t, uint32(13), computeLookupTableSize(4))
		// n=10, target=20, smallest prime >= 20 is 23
		assert.Equal(t, uint32(23), computeLookupTableSize(10))
		// n=12, target=24, smallest prime >= 24 is 47
		assert.Equal(t, uint32(47), computeLookupTableSize(12))
		// n=50, target=100, smallest prime >= 100 is 197
		assert.Equal(t, uint32(197), computeLookupTableSize(50))
	})

	t.Run("returns 7 for zero backends", func(t *testing.T) {
		// n=0, target=0, smallest prime >= 0 is 7
		assert.Equal(t, uint32(7), computeLookupTableSize(0))
	})

	t.Run("returns largest prime for very large n", func(t *testing.T) {
		// n=1000, target=2000, no prime >= 2000, fallback to 797
		assert.Equal(t, uint32(797), computeLookupTableSize(1000))
	})
}

// -------------------------------------------------------------------
// -- filterAvailable
// -------------------------------------------------------------------

func TestFilterAvailable(t *testing.T) {
	t.Run("returns only backends with StateAvailable", func(t *testing.T) {
		backends := []*types.Backend{
			types.NewBackend(uuid.New(), types.BackendSpec{State: types.StateAvailable}, types.BackendStatus{State: types.StateAvailable}),
			types.NewBackend(uuid.New(), types.BackendSpec{State: types.StateUnavailable}, types.BackendStatus{State: types.StateUnavailable}),
			types.NewBackend(uuid.New(), types.BackendSpec{State: types.StateAvailable}, types.BackendStatus{State: types.StateAvailable}),
			types.NewBackend(uuid.New(), types.BackendSpec{State: types.StateUnschedulable}, types.BackendStatus{State: types.StateUnschedulable}),
		}

		result := filterAvailable(backends)
		require.Len(t, result, 2)
		assert.Equal(t, backends[0], result[0])
		assert.Equal(t, backends[2], result[1])
	})

	t.Run("filters backend with available spec but unavailable status", func(t *testing.T) {
		backends := []*types.Backend{
			types.NewBackend(uuid.New(), types.BackendSpec{State: types.StateAvailable}, types.BackendStatus{State: types.StateUnavailable}),
			types.NewBackend(uuid.New(), types.BackendSpec{State: types.StateAvailable}, types.BackendStatus{State: types.StateAvailable}),
		}
		result := filterAvailable(backends)
		require.Len(t, result, 1)
		assert.Equal(t, backends[1], result[0])
	})

	t.Run("returns empty for no available backends", func(t *testing.T) {
		backends := []*types.Backend{
			types.NewBackend(uuid.New(), types.BackendSpec{State: types.StateUnavailable}, types.BackendStatus{}),
		}
		result := filterAvailable(backends)
		assert.Empty(t, result)
	})

	t.Run("returns empty for nil input", func(t *testing.T) {
		result := filterAvailable(nil)
		assert.Empty(t, result)
	})
}
