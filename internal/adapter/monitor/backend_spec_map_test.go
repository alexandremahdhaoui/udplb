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
package monitoradapter_test

import (
	"fmt"
	"testing"
	"time"

	monitoradapter "github.com/alexandremahdhaoui/udplb/internal/adapter/monitor"
	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/alexandremahdhaoui/udplb/internal/util"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackendSpecList_ParsesConfig(t *testing.T) {
	config := types.Config{
		Backends: []types.BackendConfig{
			{Enabled: true, IP: "10.0.0.1", MAC: "aa:bb:cc:dd:ee:01", Port: 8080},
			{Enabled: true, IP: "10.0.0.2", MAC: "aa:bb:cc:dd:ee:02", Port: 9090},
			{Enabled: false, IP: "10.0.0.3", MAC: "aa:bb:cc:dd:ee:03", Port: 7070},
		},
	}

	watcherMux := util.NewWatcherMux[monitoradapter.BackendSpecMap](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[monitoradapter.BackendSpecMap],
	)

	bsl := monitoradapter.NewBackendSpecList(config, watcherMux)

	ch, cancel := bsl.Watch()
	defer cancel()

	select {
	case specMap := <-ch:
		// 2 enabled backends, 1 disabled
		require.Len(t, specMap, 2)

		id1 := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(fmt.Sprintf("%s:%d", "10.0.0.1", 8080)))
		id2 := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(fmt.Sprintf("%s:%d", "10.0.0.2", 9090)))

		spec1, ok := specMap[id1]
		require.True(t, ok, "expected backend 10.0.0.1:8080 to be present")
		assert.Equal(t, 8080, spec1.Port)
		assert.Equal(t, types.StateAvailable, spec1.State)

		spec2, ok := specMap[id2]
		require.True(t, ok, "expected backend 10.0.0.2:9090 to be present")
		assert.Equal(t, 9090, spec2.Port)
		assert.Equal(t, types.StateAvailable, spec2.State)

		// Disabled backend must not be present
		id3 := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(fmt.Sprintf("%s:%d", "10.0.0.3", 7070)))
		_, ok = specMap[id3]
		assert.False(t, ok, "disabled backend must not be present")

	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for BackendSpecMap dispatch")
	}
}

func TestBackendSpecList_DeterministicUUID(t *testing.T) {
	// Same IP+Port always produces the same UUID.
	id1 := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(fmt.Sprintf("%s:%d", "10.0.0.1", 8080)))
	id2 := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(fmt.Sprintf("%s:%d", "10.0.0.1", 8080)))
	assert.Equal(t, id1, id2)

	// Different IP+Port produces different UUID.
	id3 := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(fmt.Sprintf("%s:%d", "10.0.0.2", 8080)))
	assert.NotEqual(t, id1, id3)
}

func TestBackendSpecList_InvalidIPSkipped(t *testing.T) {
	config := types.Config{
		Backends: []types.BackendConfig{
			{Enabled: true, IP: "not-an-ip", MAC: "aa:bb:cc:dd:ee:01", Port: 8080},
			{Enabled: true, IP: "10.0.0.1", MAC: "aa:bb:cc:dd:ee:02", Port: 9090},
		},
	}

	watcherMux := util.NewWatcherMux[monitoradapter.BackendSpecMap](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[monitoradapter.BackendSpecMap],
	)

	bsl := monitoradapter.NewBackendSpecList(config, watcherMux)

	ch, cancel := bsl.Watch()
	defer cancel()

	select {
	case specMap := <-ch:
		require.Len(t, specMap, 1, "invalid IP backend must be skipped")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for BackendSpecMap dispatch")
	}
}

func TestBackendSpecList_InvalidMACSkipped(t *testing.T) {
	config := types.Config{
		Backends: []types.BackendConfig{
			{Enabled: true, IP: "10.0.0.1", MAC: "invalid-mac", Port: 8080},
			{Enabled: true, IP: "10.0.0.2", MAC: "aa:bb:cc:dd:ee:02", Port: 9090},
		},
	}

	watcherMux := util.NewWatcherMux[monitoradapter.BackendSpecMap](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[monitoradapter.BackendSpecMap],
	)

	bsl := monitoradapter.NewBackendSpecList(config, watcherMux)

	ch, cancel := bsl.Watch()
	defer cancel()

	select {
	case specMap := <-ch:
		require.Len(t, specMap, 1, "invalid MAC backend must be skipped")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for BackendSpecMap dispatch")
	}
}

func TestBackendSpecList_CloseDone(t *testing.T) {
	config := types.Config{}

	watcherMux := util.NewWatcherMux[monitoradapter.BackendSpecMap](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[monitoradapter.BackendSpecMap],
	)

	bsl := monitoradapter.NewBackendSpecList(config, watcherMux)

	err := bsl.Close()
	require.NoError(t, err)

	select {
	case <-bsl.Done():
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Done channel to close")
	}
}
