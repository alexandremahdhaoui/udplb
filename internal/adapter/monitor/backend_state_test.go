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
	"context"
	"net"
	"testing"
	"time"

	monitoradapter "github.com/alexandremahdhaoui/udplb/internal/adapter/monitor"
	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/alexandremahdhaoui/udplb/internal/util"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackendState_RunClose(t *testing.T) {
	watcherMux := util.NewWatcherMux[types.BackendStatusEntry](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[types.BackendStatusEntry],
	)

	bs := monitoradapter.NewBackendState(
		map[uuid.UUID]types.BackendSpec{},
		50*time.Millisecond,
		100*time.Millisecond,
		watcherMux,
	)

	err := bs.Run(context.Background())
	require.NoError(t, err)

	err = bs.Close()
	require.NoError(t, err)

	select {
	case <-bs.Done():
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Done channel to close")
	}
}

func TestBackendState_ErrorGuards(t *testing.T) {
	watcherMux := util.NewWatcherMux[types.BackendStatusEntry](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[types.BackendStatusEntry],
	)

	bs := monitoradapter.NewBackendState(
		map[uuid.UUID]types.BackendSpec{},
		50*time.Millisecond,
		100*time.Millisecond,
		watcherMux,
	)

	// Close before Run must fail.
	err := bs.Close()
	assert.ErrorIs(t, err, types.ErrRunnableMustBeRunningToBeClosed)

	// Run succeeds.
	err = bs.Run(context.Background())
	require.NoError(t, err)

	// Double Run must fail.
	err = bs.Run(context.Background())
	assert.ErrorIs(t, err, types.ErrAlreadyRunning)

	// Close succeeds.
	err = bs.Close()
	require.NoError(t, err)

	// Double Close must fail.
	// After Close: running=false, closed=true. The guard checks !running
	// first, so double-close returns ErrRunnableMustBeRunningToBeClosed.
	err = bs.Close()
	assert.ErrorIs(t, err, types.ErrRunnableMustBeRunningToBeClosed)

	// Run after Close must fail.
	err = bs.Run(context.Background())
	assert.ErrorIs(t, err, types.ErrCannotRunClosedRunnable)
}

func TestBackendState_HealthCheckDispatch(t *testing.T) {
	// Start a real UDP listener on localhost.
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer pc.Close()

	// Echo goroutine: read and echo back all packets.
	go func() {
		buf := make([]byte, 64)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return // listener closed
			}
			_, _ = pc.WriteTo(buf[:n], addr)
		}
	}()

	addr := pc.LocalAddr().(*net.UDPAddr)

	backendID := uuid.New()
	backends := map[uuid.UUID]types.BackendSpec{
		backendID: {
			IP:   net.ParseIP("127.0.0.1"),
			Port: addr.Port,
		},
	}

	watcherMux := util.NewWatcherMux[types.BackendStatusEntry](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[types.BackendStatusEntry],
	)

	bs := monitoradapter.NewBackendState(
		backends,
		50*time.Millisecond,
		200*time.Millisecond,
		watcherMux,
	)

	ch, cancel := bs.Watch()
	defer cancel()

	err = bs.Run(context.Background())
	require.NoError(t, err)
	defer func() { _ = bs.Close() }()

	// Wait for at least one health check dispatch.
	select {
	case entry := <-ch:
		assert.Equal(t, backendID, entry.BackendId)
		// UDP echo probe reports StateAvailable when the backend echoes back.
		assert.Equal(t, types.StateAvailable, entry.State)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for health check dispatch")
	}
}

func TestBackendState_MultipleBackendsDispatched(t *testing.T) {
	// Start two real UDP listeners.
	pc1, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer pc1.Close()

	pc2, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer pc2.Close()

	// Echo goroutines: read and echo back all packets.
	go func() {
		buf := make([]byte, 64)
		for {
			n, addr, err := pc1.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = pc1.WriteTo(buf[:n], addr)
		}
	}()
	go func() {
		buf := make([]byte, 64)
		for {
			n, addr, err := pc2.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = pc2.WriteTo(buf[:n], addr)
		}
	}()

	addr1 := pc1.LocalAddr().(*net.UDPAddr)
	addr2 := pc2.LocalAddr().(*net.UDPAddr)

	id1 := uuid.New()
	id2 := uuid.New()
	backends := map[uuid.UUID]types.BackendSpec{
		id1: {IP: net.ParseIP("127.0.0.1"), Port: addr1.Port},
		id2: {IP: net.ParseIP("127.0.0.1"), Port: addr2.Port},
	}

	watcherMux := util.NewWatcherMux[types.BackendStatusEntry](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[types.BackendStatusEntry],
	)

	bs := monitoradapter.NewBackendState(
		backends,
		50*time.Millisecond,
		200*time.Millisecond,
		watcherMux,
	)

	ch, cancel := bs.Watch()
	defer cancel()

	err = bs.Run(context.Background())
	require.NoError(t, err)
	defer func() { _ = bs.Close() }()

	// Collect entries from one tick (2 backends = 2 entries).
	seen := make(map[uuid.UUID]bool)
	timeout := time.After(2 * time.Second)
	for len(seen) < 2 {
		select {
		case entry := <-ch:
			seen[entry.BackendId] = true
		case <-timeout:
			t.Fatalf("timed out: only saw %d of 2 backends", len(seen))
		}
	}

	assert.True(t, seen[id1], "expected backend 1 to be probed")
	assert.True(t, seen[id2], "expected backend 2 to be probed")
}

func TestBackendState_ProbeTimeout(t *testing.T) {
	// Use a port with no listener -- the probe should time out.
	backendID := uuid.New()
	backends := map[uuid.UUID]types.BackendSpec{
		backendID: {
			IP:   net.ParseIP("127.0.0.1"),
			Port: 1, // port 1 has no listener; probe will time out
		},
	}

	watcherMux := util.NewWatcherMux[types.BackendStatusEntry](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[types.BackendStatusEntry],
	)

	bs := monitoradapter.NewBackendState(
		backends,
		100*time.Millisecond, // interval: probe every 100ms
		200*time.Millisecond, // timeout: 200ms
		watcherMux,
	)

	ch, cancel := bs.Watch()
	defer cancel()

	err := bs.Run(context.Background())
	require.NoError(t, err)
	defer func() { _ = bs.Close() }()

	select {
	case entry := <-ch:
		assert.Equal(t, backendID, entry.BackendId)
		assert.Equal(t, types.StateUnavailable, entry.State,
			"backend with no listener should be StateUnavailable")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for health check dispatch")
	}
}
