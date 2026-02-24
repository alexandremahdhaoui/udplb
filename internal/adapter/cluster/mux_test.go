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
package clusteradpater

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/alexandremahdhaoui/udplb/internal/util"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*******************************************************************************
 * Mock Protocol / Listener / Conn for error-path tests
 *
 ******************************************************************************/

type mockListener struct {
	readFromFunc        func([]byte) (int, net.Addr, error)
	closeFunc           func() error
	localAddrFunc       func() net.Addr
	setReadDeadlineFunc func(time.Time) error
}

func (m *mockListener) ReadFrom(buf []byte) (int, net.Addr, error) { return m.readFromFunc(buf) }
func (m *mockListener) Close() error                               { return m.closeFunc() }
func (m *mockListener) LocalAddr() net.Addr                        { return m.localAddrFunc() }
func (m *mockListener) SetReadDeadline(t time.Time) error          { return m.setReadDeadlineFunc(t) }

type mockConn struct {
	writeFunc      func([]byte) (int, error)
	closeFunc      func() error
	remoteAddrFunc func() net.Addr
}

func (m *mockConn) Write(data []byte) (int, error) { return m.writeFunc(data) }
func (m *mockConn) Close() error                   { return m.closeFunc() }
func (m *mockConn) RemoteAddr() net.Addr           { return m.remoteAddrFunc() }

type mockProtocol struct {
	listenFunc func(string) (Listener, error)
	dialFunc   func(string) (Conn, error)
}

func (m *mockProtocol) Listen(addr string) (Listener, error) { return m.listenFunc(addr) }
func (m *mockProtocol) Dial(addr string) (Conn, error)       { return m.dialFunc(addr) }

type mockTopology struct {
	nextPeersFunc func(uuid.UUID, map[uuid.UUID]Node) ([]Node, error)
}

func (m *mockTopology) NextPeers(self uuid.UUID, nodes map[uuid.UUID]Node) ([]Node, error) {
	return m.nextPeersFunc(self, nodes)
}

// newTestMux creates a clusterMux with real UDP protocol on localhost:0 for testing.
func newTestMux(t *testing.T) *clusterMux {
	t.Helper()
	recvMux := util.NewWatcherMux[types.RawData](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[types.RawData],
	)
	mux := NewMux(
		uuid.New(),
		"127.0.0.1:0",
		NewUDPProtocol(),
		NewFullyConnectedTopology(),
		recvMux,
	)
	return mux.(*clusterMux)
}

func TestClusterMux_RunClose_Lifecycle(t *testing.T) {
	cm := newTestMux(t)

	// Run starts the mux.
	err := cm.Run(context.Background())
	require.NoError(t, err)

	// Verify running state.
	cm.mu.RLock()
	assert.True(t, cm.running)
	assert.False(t, cm.closed)
	cm.mu.RUnlock()

	// Close stops the mux.
	err = cm.Close()
	require.NoError(t, err)

	// Verify doneCh is closed.
	select {
	case <-cm.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("doneCh was not closed after Close()")
	}

	// Verify final state.
	cm.mu.RLock()
	assert.False(t, cm.running)
	assert.True(t, cm.closed)
	cm.mu.RUnlock()
}

func TestClusterMux_DoubleRun(t *testing.T) {
	cm := newTestMux(t)

	err := cm.Run(context.Background())
	require.NoError(t, err)
	defer cm.Close()

	err = cm.Run(context.Background())
	assert.ErrorIs(t, err, types.ErrAlreadyRunning)
}

func TestClusterMux_CloseWithoutRun(t *testing.T) {
	cm := newTestMux(t)

	err := cm.Close()
	assert.ErrorIs(t, err, types.ErrRunnableMustBeRunningToBeClosed)
}

func TestClusterMux_DoubleClose(t *testing.T) {
	cm := newTestMux(t)

	err := cm.Run(context.Background())
	require.NoError(t, err)

	err = cm.Close()
	require.NoError(t, err)

	err = cm.Close()
	assert.ErrorIs(t, err, types.ErrAlreadyClosed)
}

func TestClusterMux_RunClosedMux(t *testing.T) {
	cm := newTestMux(t)

	err := cm.Run(context.Background())
	require.NoError(t, err)

	err = cm.Close()
	require.NoError(t, err)

	err = cm.Run(context.Background())
	assert.ErrorIs(t, err, types.ErrCannotRunClosedRunnable)
}

func TestClusterMux_SendBeforeRun(t *testing.T) {
	cm := newTestMux(t)

	ch := make(chan []byte)
	err := cm.Send(ch)
	require.NoError(t, err)
	assert.Len(t, cm.sendSources, 1)
}

func TestClusterMux_SendAfterRun(t *testing.T) {
	cm := newTestMux(t)

	err := cm.Run(context.Background())
	require.NoError(t, err)
	defer cm.Close()

	ch := make(chan []byte)
	err = cm.Send(ch)
	assert.ErrorIs(t, err, types.ErrAlreadyRunning)
}

func TestClusterMux_ListNodes(t *testing.T) {
	cm := newTestMux(t)

	// Initially empty.
	assert.Empty(t, cm.ListNodes())

	// Add nodes manually for testing.
	id1 := uuid.New()
	id2 := uuid.New()
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:5000")
	cm.nodes[id1] = Node{ID: id1, Addr: addr}
	cm.nodes[id2] = Node{ID: id2, Addr: addr}

	ids := cm.ListNodes()
	assert.Len(t, ids, 2)
	assert.ElementsMatch(t, []uuid.UUID{id1, id2}, ids)
}

func TestClusterMux_JoinLeave(t *testing.T) {
	cm := newTestMux(t)

	assert.NoError(t, cm.Join())
	assert.NoError(t, cm.Leave())
}

func TestClusterMux_RecvDispatch(t *testing.T) {
	// Test that Recv returns a channel and dispatching data via recvMux
	// reaches the watcher.
	cm := newTestMux(t)

	ch, cancel := cm.Recv()
	defer cancel()

	payload := []byte("test data")
	cm.recvMux.Dispatch(payload)

	select {
	case data := <-ch:
		assert.Equal(t, payload, data)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for dispatched data")
	}
}

func TestClusterMux_SendRecv_TwoNodes(t *testing.T) {
	// Create two mux instances on localhost. Configure them as peers.
	// Send data from mux1, receive on mux2.
	proto := NewUDPProtocol()
	topo := NewFullyConnectedTopology()

	node1Id := uuid.New()
	node2Id := uuid.New()

	recvMux1 := util.NewWatcherMux[types.RawData](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[types.RawData],
	)
	recvMux2 := util.NewWatcherMux[types.RawData](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[types.RawData],
	)

	mux1 := NewMux(node1Id, "127.0.0.1:0", proto, topo, recvMux1).(*clusterMux)
	mux2 := NewMux(node2Id, "127.0.0.1:0", proto, topo, recvMux2).(*clusterMux)

	// Set up send channel on mux1.
	sendCh := make(chan []byte, 1)
	err := mux1.Send(sendCh)
	require.NoError(t, err)

	// Start mux2 first so we can get its listen address.
	err = mux2.Run(context.Background())
	require.NoError(t, err)
	defer mux2.Close()

	// Get mux2's listen address and register it as a peer of mux1.
	mux2Addr := mux2.listener.LocalAddr()
	mux1.nodes[node2Id] = Node{ID: node2Id, Addr: mux2Addr}

	// Start mux1.
	err = mux1.Run(context.Background())
	require.NoError(t, err)
	defer mux1.Close()

	// Subscribe to mux2's recv.
	recvCh, recvCancel := mux2.Recv()
	defer recvCancel()

	// Send data from mux1.
	payload := []byte("hello from node1")
	sendCh <- payload

	// Receive on mux2.
	select {
	case data := <-recvCh:
		assert.Equal(t, payload, data)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for data on mux2")
	}
}

func TestClusterMux_Done(t *testing.T) {
	cm := newTestMux(t)

	doneCh := cm.Done()
	require.NotNil(t, doneCh)

	// doneCh should not be closed before Run.
	select {
	case <-doneCh:
		t.Fatal("doneCh should not be closed before Run")
	default:
	}

	err := cm.Run(context.Background())
	require.NoError(t, err)

	// doneCh should not be closed while running.
	select {
	case <-doneCh:
		t.Fatal("doneCh should not be closed while running")
	default:
	}

	err = cm.Close()
	require.NoError(t, err)

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("doneCh was not closed after Close()")
	}
}

func TestClusterMux_RecvLoop_CopiesBuffer(t *testing.T) {
	// Verify that the recvLoop copies the buffer before dispatching.
	// Send two messages rapidly, verify both arrive with correct content.
	proto := NewUDPProtocol()
	topo := NewFullyConnectedTopology()

	nodeId := uuid.New()
	recvMux := util.NewWatcherMux[types.RawData](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[types.RawData],
	)

	mux := NewMux(nodeId, "127.0.0.1:0", proto, topo, recvMux).(*clusterMux)

	err := mux.Run(context.Background())
	require.NoError(t, err)
	defer mux.Close()

	recvCh, recvCancel := mux.Recv()
	defer recvCancel()

	// Send two different messages to the listener.
	addr := mux.listener.LocalAddr()
	conn, err := proto.Dial(addr.String())
	require.NoError(t, err)
	defer conn.Close()

	msg1 := []byte("message-one")
	msg2 := []byte("message-two")

	_, err = conn.Write(msg1)
	require.NoError(t, err)
	_, err = conn.Write(msg2)
	require.NoError(t, err)

	received := make([]string, 0, 2)
	timeout := time.After(3 * time.Second)
	for i := 0; i < 2; i++ {
		select {
		case data := <-recvCh:
			received = append(received, string(data))
		case <-timeout:
			t.Fatalf("timeout waiting for message %d, got %v", i+1, received)
		}
	}

	assert.ElementsMatch(t, []string{"message-one", "message-two"}, received)
}

/*******************************************************************************
 * Tests: mux.Run when protocol.Listen fails
 *
 ******************************************************************************/

func TestClusterMux_Run_ListenError(t *testing.T) {
	listenErr := errors.New("listen failed")
	proto := &mockProtocol{
		listenFunc: func(string) (Listener, error) {
			return nil, listenErr
		},
		dialFunc: func(string) (Conn, error) { return nil, nil },
	}
	recvMux := util.NewWatcherMux[types.RawData](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[types.RawData],
	)
	mux := NewMux(uuid.New(), "127.0.0.1:0", proto, NewFullyConnectedTopology(), recvMux)

	err := mux.Run(context.Background())
	assert.ErrorIs(t, err, listenErr)
}

/*******************************************************************************
 * Tests: sendLoop error paths (NextPeers error, Dial error, Write error)
 *
 ******************************************************************************/

func TestClusterMux_SendLoop_NextPeersError(t *testing.T) {
	// Use a mock topology that returns an error from NextPeers.
	topo := &mockTopology{
		nextPeersFunc: func(uuid.UUID, map[uuid.UUID]Node) ([]Node, error) {
			return nil, errors.New("topology error")
		},
	}

	recvMux := util.NewWatcherMux[types.RawData](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[types.RawData],
	)
	nodeId := uuid.New()
	mux := NewMux(nodeId, "127.0.0.1:0", NewUDPProtocol(), topo, recvMux).(*clusterMux)

	// Add a node so sendLoop actually tries to send.
	peerId := uuid.New()
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:9999")
	mux.nodes[peerId] = Node{ID: peerId, Addr: addr}

	sendCh := make(chan []byte, 1)
	err := mux.Send(sendCh)
	require.NoError(t, err)

	err = mux.Run(context.Background())
	require.NoError(t, err)
	defer mux.Close()

	// Send data; the sendLoop will call NextPeers which errors.
	// This should not crash, just log an error.
	sendCh <- []byte("test")

	// Give the goroutine time to process.
	time.Sleep(100 * time.Millisecond)
}

func TestClusterMux_SendLoop_DialError(t *testing.T) {
	dialErr := errors.New("dial failed")
	proto := &mockProtocol{
		listenFunc: func(addr string) (Listener, error) {
			return net.ListenPacket("udp", addr)
		},
		dialFunc: func(string) (Conn, error) {
			return nil, dialErr
		},
	}

	recvMux := util.NewWatcherMux[types.RawData](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[types.RawData],
	)
	nodeId := uuid.New()
	mux := NewMux(nodeId, "127.0.0.1:0", proto, NewFullyConnectedTopology(), recvMux).(*clusterMux)

	// Add a peer so NextPeers returns it.
	peerId := uuid.New()
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:9999")
	mux.nodes[peerId] = Node{ID: peerId, Addr: addr}

	sendCh := make(chan []byte, 1)
	err := mux.Send(sendCh)
	require.NoError(t, err)

	err = mux.Run(context.Background())
	require.NoError(t, err)
	defer mux.Close()

	// Send data; the sendLoop will call Dial which errors.
	sendCh <- []byte("test")

	// Give the goroutine time to process.
	time.Sleep(100 * time.Millisecond)
}

func TestClusterMux_SendLoop_WriteError(t *testing.T) {
	writeErr := errors.New("write failed")

	proto := &mockProtocol{
		listenFunc: func(addr string) (Listener, error) {
			return net.ListenPacket("udp", addr)
		},
		dialFunc: func(string) (Conn, error) {
			return &mockConn{
				writeFunc:      func([]byte) (int, error) { return 0, writeErr },
				closeFunc:      func() error { return nil },
				remoteAddrFunc: func() net.Addr { a, _ := net.ResolveUDPAddr("udp", "127.0.0.1:9999"); return a },
			}, nil
		},
	}

	recvMux := util.NewWatcherMux[types.RawData](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[types.RawData],
	)
	nodeId := uuid.New()
	mux := NewMux(nodeId, "127.0.0.1:0", proto, NewFullyConnectedTopology(), recvMux).(*clusterMux)

	// Add a peer so NextPeers returns it.
	peerId := uuid.New()
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:9999")
	mux.nodes[peerId] = Node{ID: peerId, Addr: addr}

	sendCh := make(chan []byte, 1)
	err := mux.Send(sendCh)
	require.NoError(t, err)

	err = mux.Run(context.Background())
	require.NoError(t, err)
	defer mux.Close()

	// Send data; the sendLoop will dial successfully but Write will error.
	sendCh <- []byte("test")

	// Give the goroutine time to process.
	time.Sleep(100 * time.Millisecond)
}

/*******************************************************************************
 * Tests: getOrDialConn returns existing connection
 *
 ******************************************************************************/

func TestClusterMux_GetOrDialConn_ExistingConn(t *testing.T) {
	recvMux := util.NewWatcherMux[types.RawData](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[types.RawData],
	)
	mux := NewMux(uuid.New(), "127.0.0.1:0", NewUDPProtocol(), NewFullyConnectedTopology(), recvMux).(*clusterMux)

	// Pre-populate a connection in the conns map.
	peerId := uuid.New()
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:9999")
	existingConn := &mockConn{
		writeFunc:      func([]byte) (int, error) { return 0, nil },
		closeFunc:      func() error { return nil },
		remoteAddrFunc: func() net.Addr { return addr },
	}
	mux.conns[peerId] = existingConn

	// getOrDialConn should return the existing connection without dialing.
	conn, err := mux.getOrDialConn(Node{ID: peerId, Addr: addr})
	require.NoError(t, err)
	assert.Equal(t, existingConn, conn)
}

/*******************************************************************************
 * Tests: getOrDialConn dial error
 *
 ******************************************************************************/

func TestClusterMux_GetOrDialConn_DialError(t *testing.T) {
	dialErr := errors.New("dial error")
	proto := &mockProtocol{
		listenFunc: func(string) (Listener, error) { return nil, nil },
		dialFunc:   func(string) (Conn, error) { return nil, dialErr },
	}

	recvMux := util.NewWatcherMux[types.RawData](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[types.RawData],
	)
	mux := NewMux(uuid.New(), "127.0.0.1:0", proto, NewFullyConnectedTopology(), recvMux).(*clusterMux)

	peerId := uuid.New()
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:9999")

	conn, err := mux.getOrDialConn(Node{ID: peerId, Addr: addr})
	assert.Nil(t, conn)
	assert.ErrorIs(t, err, dialErr)
}

/*******************************************************************************
 * Tests: recvLoop error paths (SetReadDeadline error, non-timeout read error)
 *
 ******************************************************************************/

func TestClusterMux_RecvLoop_SetReadDeadlineError(t *testing.T) {
	// Use a mock listener that fails SetReadDeadline, then succeeds,
	// then returns data.
	var mu sync.Mutex
	callCount := 0
	terminateCh := make(chan struct{})

	listener := &mockListener{
		setReadDeadlineFunc: func(time.Time) error {
			mu.Lock()
			callCount++
			c := callCount
			mu.Unlock()
			if c == 1 {
				return errors.New("deadline error")
			}
			return nil
		},
		readFromFunc: func(buf []byte) (int, net.Addr, error) {
			// After the deadline error, block until terminate.
			<-terminateCh
			return 0, nil, errors.New("closed")
		},
		closeFunc:     func() error { return nil },
		localAddrFunc: func() net.Addr { a, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0"); return a },
	}

	proto := &mockProtocol{
		listenFunc: func(string) (Listener, error) { return listener, nil },
		dialFunc:   func(string) (Conn, error) { return nil, nil },
	}

	recvMux := util.NewWatcherMux[types.RawData](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[types.RawData],
	)
	mux := NewMux(uuid.New(), "127.0.0.1:0", proto, NewFullyConnectedTopology(), recvMux).(*clusterMux)

	err := mux.Run(context.Background())
	require.NoError(t, err)

	// Give the recvLoop time to hit the SetReadDeadline error.
	time.Sleep(100 * time.Millisecond)

	// Verify it continued running (didn't crash).
	close(terminateCh)
	err = mux.Close()
	require.NoError(t, err)
}

func TestClusterMux_RecvLoop_NonTimeoutReadError(t *testing.T) {
	// Use a mock listener that returns a non-timeout error on ReadFrom.
	callCount := 0
	var mu sync.Mutex
	terminateCh := make(chan struct{})

	listener := &mockListener{
		setReadDeadlineFunc: func(time.Time) error { return nil },
		readFromFunc: func(buf []byte) (int, net.Addr, error) {
			mu.Lock()
			callCount++
			c := callCount
			mu.Unlock()
			if c == 1 {
				// Return a non-timeout error.
				return 0, nil, errors.New("non-timeout read error")
			}
			// Block until terminate.
			<-terminateCh
			return 0, nil, errors.New("closed")
		},
		closeFunc:     func() error { return nil },
		localAddrFunc: func() net.Addr { a, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0"); return a },
	}

	proto := &mockProtocol{
		listenFunc: func(string) (Listener, error) { return listener, nil },
		dialFunc:   func(string) (Conn, error) { return nil, nil },
	}

	recvMux := util.NewWatcherMux[types.RawData](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[types.RawData],
	)
	mux := NewMux(uuid.New(), "127.0.0.1:0", proto, NewFullyConnectedTopology(), recvMux).(*clusterMux)

	err := mux.Run(context.Background())
	require.NoError(t, err)

	// Give the recvLoop time to hit the non-timeout read error.
	time.Sleep(100 * time.Millisecond)

	close(terminateCh)
	err = mux.Close()
	require.NoError(t, err)
}

/*******************************************************************************
 * Tests: sendLoop exits when source channel is closed
 *
 ******************************************************************************/

func TestClusterMux_SendLoop_SourceChannelClosed(t *testing.T) {
	recvMux := util.NewWatcherMux[types.RawData](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[types.RawData],
	)
	mux := NewMux(uuid.New(), "127.0.0.1:0", NewUDPProtocol(), NewFullyConnectedTopology(), recvMux).(*clusterMux)

	sendCh := make(chan []byte, 1)
	err := mux.Send(sendCh)
	require.NoError(t, err)

	err = mux.Run(context.Background())
	require.NoError(t, err)
	defer mux.Close()

	// Close the source channel. The merge goroutine should exit gracefully.
	close(sendCh)

	// Give the goroutine time to process.
	time.Sleep(100 * time.Millisecond)
}
