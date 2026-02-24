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
	"encoding/json"
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

func TestRemoteAssignment_RunClose(t *testing.T) {
	watcherMux := util.NewWatcherMux[types.Assignment](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[types.Assignment],
	)

	ra := monitoradapter.NewRemoteAssignment("127.0.0.1:0", watcherMux)

	err := ra.Run(context.Background())
	require.NoError(t, err)

	err = ra.Close()
	require.NoError(t, err)

	select {
	case <-ra.Done():
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Done channel to close")
	}
}

func TestRemoteAssignment_ErrorGuards(t *testing.T) {
	watcherMux := util.NewWatcherMux[types.Assignment](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[types.Assignment],
	)

	ra := monitoradapter.NewRemoteAssignment("127.0.0.1:0", watcherMux)

	// Close before Run must fail.
	err := ra.Close()
	assert.ErrorIs(t, err, types.ErrRunnableMustBeRunningToBeClosed)

	// Run succeeds.
	err = ra.Run(context.Background())
	require.NoError(t, err)

	// Double Run must fail.
	err = ra.Run(context.Background())
	assert.ErrorIs(t, err, types.ErrAlreadyRunning)

	// Close succeeds.
	err = ra.Close()
	require.NoError(t, err)

	// Double Close must fail.
	// After Close: running=false, closed=true. The guard checks !running
	// first, so double-close returns ErrRunnableMustBeRunningToBeClosed.
	err = ra.Close()
	assert.ErrorIs(t, err, types.ErrRunnableMustBeRunningToBeClosed)

	// Run after Close must fail.
	err = ra.Run(context.Background())
	assert.ErrorIs(t, err, types.ErrCannotRunClosedRunnable)
}

func TestRemoteAssignment_ReceiveAssignment(t *testing.T) {
	// Find an available port first so we know the address to send to.
	tmpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	listenAddr := tmpConn.LocalAddr().String()
	tmpConn.Close()

	watcherMux := util.NewWatcherMux[types.Assignment](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[types.Assignment],
	)

	ra := monitoradapter.NewRemoteAssignment(listenAddr, watcherMux)

	ch, cancel := ra.Watch()
	defer cancel()

	err = ra.Run(context.Background())
	require.NoError(t, err)
	defer func() { _ = ra.Close() }()

	// Give the listener a moment to start.
	time.Sleep(50 * time.Millisecond)

	// Send a JSON-encoded Assignment via UDP.
	expectedAssignment := types.Assignment{
		BackendId: uuid.New(),
		SessionId: uuid.New(),
	}

	data, err := json.Marshal(expectedAssignment)
	require.NoError(t, err)

	conn, err := net.Dial("udp", listenAddr)
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.Write(data)
	require.NoError(t, err)

	// Wait for the assignment to arrive on the watch channel.
	select {
	case received := <-ch:
		assert.Equal(t, expectedAssignment.BackendId, received.BackendId)
		assert.Equal(t, expectedAssignment.SessionId, received.SessionId)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for assignment on watch channel")
	}
}

func TestRemoteAssignment_MalformedJSONDropped(t *testing.T) {
	tmpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	listenAddr := tmpConn.LocalAddr().String()
	tmpConn.Close()

	watcherMux := util.NewWatcherMux[types.Assignment](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[types.Assignment],
	)

	ra := monitoradapter.NewRemoteAssignment(listenAddr, watcherMux)

	ch, cancel := ra.Watch()
	defer cancel()

	err = ra.Run(context.Background())
	require.NoError(t, err)
	defer func() { _ = ra.Close() }()

	time.Sleep(50 * time.Millisecond)

	// Send malformed JSON.
	conn, err := net.Dial("udp", listenAddr)
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.Write([]byte("this is not json"))
	require.NoError(t, err)

	// Send a valid assignment after the malformed one.
	validAssignment := types.Assignment{
		BackendId: uuid.New(),
		SessionId: uuid.New(),
	}
	validData, err := json.Marshal(validAssignment)
	require.NoError(t, err)

	_, err = conn.Write(validData)
	require.NoError(t, err)

	// The malformed packet must be dropped, and the valid one received.
	select {
	case received := <-ch:
		assert.Equal(t, validAssignment.BackendId, received.BackendId)
		assert.Equal(t, validAssignment.SessionId, received.SessionId)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for valid assignment after malformed packet")
	}
}

func TestRemoteAssignment_EmptyPacketDropped(t *testing.T) {
	tmpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	listenAddr := tmpConn.LocalAddr().String()
	tmpConn.Close()

	watcherMux := util.NewWatcherMux[types.Assignment](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[types.Assignment],
	)

	ra := monitoradapter.NewRemoteAssignment(listenAddr, watcherMux)

	ch, cancel := ra.Watch()
	defer cancel()

	err = ra.Run(context.Background())
	require.NoError(t, err)
	defer func() { _ = ra.Close() }()

	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("udp", listenAddr)
	require.NoError(t, err)
	defer conn.Close()

	// Send empty packet.
	_, err = conn.Write([]byte{})
	require.NoError(t, err)

	// Send a valid assignment after the empty one.
	validAssignment := types.Assignment{
		BackendId: uuid.New(),
		SessionId: uuid.New(),
	}
	validData, err := json.Marshal(validAssignment)
	require.NoError(t, err)

	_, err = conn.Write(validData)
	require.NoError(t, err)

	// The empty packet must be dropped, and the valid one received.
	select {
	case received := <-ch:
		assert.Equal(t, validAssignment.BackendId, received.BackendId)
		assert.Equal(t, validAssignment.SessionId, received.SessionId)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for valid assignment after empty packet")
	}
}
