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
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/alexandremahdhaoui/udplb/internal/util"
	"github.com/google/uuid"
)

var _ types.RawCluster = &clusterMux{}

// IMPLEMENTED: Protocol is defined in protocol.go as an abstraction over
// Send/Recv. UDP transport is implemented via NewUDPProtocol().

/*******************************************************************************
 * Concrete implementation
 *
 ******************************************************************************/

type clusterMux struct {
	ctx         context.Context
	protocol    Protocol
	topology    Topology
	listener    Listener
	listenAddr  string
	nodeId      uuid.UUID
	nodes       map[uuid.UUID]Node
	conns       map[uuid.UUID]Conn
	sendSources []<-chan []byte
	recvMux     *util.WatcherMux[types.RawData]
	mu          *sync.RWMutex
	running     bool
	closed      bool
	doneCh      chan struct{}
	terminateCh chan struct{}
}

/*******************************************************************************
 * New
 *
 ******************************************************************************/

// Ring strategy:
// - Let A, B, C, D and E 5 healthy nodes of a clutser.
// - 1. A send proposal to B, B to C, C to D, and D to E.
// - 1. 4 messages have been sent.
// - 2. E "knows it all".
// - 2. E sends to either A, B, C, D?
//      OR: E sends to A, A to B, B to C, C to D.
// DONE: 8 messages sent. (same as Leader based).
//
// However, the Ring strategy could actually be used to already start
// accepting when E knows it all.
// That is:
// - 1. First circular round around the ring, up to E. (4 messages)
// - 2. E accepts + send to A, A knows it all, accept + send to B; etc. up to D
// - 3. D send accept messages to E.
// DONE: 9 messsages sent: Phase I & II of FastWAL are both done.
//
// The big downside is that network is super slow; and saving 2(n-1)-1 messages
// doesn't really outway how slow this strategy could be if n goes big.
//
// Watching the overall state of the cluster is drastically simplified, as all
// peers just need to watch the state of one other node.
//
// Could be interesting how it performs in practice.
//
// TODO: Draft algorithm that handles failures, i.e. a node becomes unavailable.
// - 1. A realise that B is not available. A.nextNode = C
// - 2. A send message to C to let it know that B is down.
// - 3. Message propagate and the cluster create a new "order" of nodes.
// What happens when multiple nodes become unavailable?
//
// TODO: QUESTION? Can this algorithm be used to hold many WAL negociation rounds
// at once?
// What about multiplexing? Looks like it's a bit leaky.

// NewMux creates a new clusterMux that implements types.RawCluster.
// The mux uses the given protocol for network transport and topology
// for determining send targets.
func NewMux(
	nodeId uuid.UUID,
	listenAddr string,
	protocol Protocol, // IMPLEMENTED: abstraction over Send/Recv (e.g. UDP or GRPC).
	topology Topology, // Leader based, Ring or Fully connected.
	recvMux *util.WatcherMux[types.RawData],
) types.RawCluster {
	return &clusterMux{
		protocol:    protocol,
		topology:    topology,
		listenAddr:  listenAddr,
		nodeId:      nodeId,
		nodes:       make(map[uuid.UUID]Node),
		conns:       make(map[uuid.UUID]Conn),
		sendSources: nil,
		recvMux:     recvMux,
		mu:          &sync.RWMutex{},
		running:     false,
		closed:      false,
		doneCh:      make(chan struct{}),
		terminateCh: make(chan struct{}),
	}
}

/*******************************************************************************
 * Run
 *
 ******************************************************************************/

// Run implements types.Cluster.
// It opens the listener, starts the recv and send loops, and returns.
func (cm *clusterMux) Run(ctx context.Context) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.running {
		return types.ErrAlreadyRunning
	} else if cm.closed {
		return types.ErrCannotRunClosedRunnable
	}

	listener, err := cm.protocol.Listen(cm.listenAddr)
	if err != nil {
		return err
	}
	cm.listener = listener
	cm.ctx = ctx
	cm.running = true

	var wg sync.WaitGroup
	wg.Add(2)

	go cm.recvLoop(&wg)
	go cm.sendLoop(&wg)

	// A separate goroutine waits for both loops to finish, then closes doneCh.
	go func() {
		wg.Wait()
		close(cm.doneCh)
	}()

	return nil
}

/*******************************************************************************
 * recvLoop
 *
 ******************************************************************************/

var (
	recvBufSize  = 65536
	readDeadline = 100 * time.Millisecond
)

func (cm *clusterMux) recvLoop(wg *sync.WaitGroup) {
	defer wg.Done()

	buf := make([]byte, recvBufSize)
	for {
		select {
		case <-cm.terminateCh:
			goto terminate
		default:
		}

		if err := cm.listener.SetReadDeadline(time.Now().Add(readDeadline)); err != nil {
			slog.ErrorContext(cm.ctx, "setting read deadline", "err", err.Error())
			continue
		}

		n, _, err := cm.listener.ReadFrom(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			// Check if we were asked to terminate (listener closed).
			select {
			case <-cm.terminateCh:
				goto terminate
			default:
			}
			slog.ErrorContext(cm.ctx, "reading from listener", "err", err.Error())
			continue
		}

		// Copy the buffer before dispatching. The buf slice is reused
		// each iteration; dispatching without copying would cause all
		// watchers to see corrupted data.
		dataCopy := make([]byte, n)
		copy(dataCopy, buf[:n])

		cm.recvMux.Dispatch(dataCopy)
	}

terminate:
	_ = cm.listener.Close()
	_ = cm.recvMux.Close()
}

/*******************************************************************************
 * sendLoop
 *
 ******************************************************************************/

func (cm *clusterMux) sendLoop(wg *sync.WaitGroup) {
	defer wg.Done()

	// Merge all sendSources into a single channel using one goroutine per source.
	merged := make(chan []byte)
	var mergeWg sync.WaitGroup

	cm.mu.RLock()
	sources := cm.sendSources
	cm.mu.RUnlock()

	for _, src := range sources {
		mergeWg.Add(1)
		go func(ch <-chan []byte) {
			defer mergeWg.Done()
			for {
				select {
				case <-cm.terminateCh:
					return
				case data, ok := <-ch:
					if !ok {
						return
					}
					select {
					case merged <- data:
					case <-cm.terminateCh:
						return
					}
				}
			}
		}(src)
	}

	// Close merged channel when all source goroutines exit.
	go func() {
		mergeWg.Wait()
		close(merged)
	}()

	for {
		select {
		case <-cm.terminateCh:
			goto terminate
		case data, ok := <-merged:
			if !ok {
				goto terminate
			}
			cm.mu.RLock()
			peers, err := cm.topology.NextPeers(cm.nodeId, cm.nodes)
			cm.mu.RUnlock()
			if err != nil {
				slog.ErrorContext(cm.ctx, "getting next peers", "err", err.Error())
				continue
			}

			for _, peer := range peers {
				conn, err := cm.getOrDialConn(peer)
				if err != nil {
					slog.ErrorContext(cm.ctx, "dialing peer",
						"peerId", peer.ID.String(),
						"err", err.Error())
					continue
				}
				if _, err := conn.Write(data); err != nil {
					slog.ErrorContext(cm.ctx, "writing to peer",
						"peerId", peer.ID.String(),
						"err", err.Error())
				}
			}
		}
	}

terminate:
	cm.mu.Lock()
	for id, conn := range cm.conns {
		_ = conn.Close()
		delete(cm.conns, id)
	}
	cm.mu.Unlock()
}

// getOrDialConn returns an existing connection to the peer, or dials a new one.
func (cm *clusterMux) getOrDialConn(peer Node) (Conn, error) {
	cm.mu.RLock()
	conn, ok := cm.conns[peer.ID]
	cm.mu.RUnlock()
	if ok {
		return conn, nil
	}

	conn, err := cm.protocol.Dial(peer.Addr.String())
	if err != nil {
		return nil, err
	}

	cm.mu.Lock()
	cm.conns[peer.ID] = conn
	cm.mu.Unlock()

	return conn, nil
}

/*******************************************************************************
 * Send
 *
 ******************************************************************************/

// Send implements types.Cluster.
// Append ch to sendSources. Must be called before Run().
//
// IMPLEMENTED: Fan-in/merge of many channels is solved using one goroutine per
// source feeding into a single merged channel (see sendLoop). This avoids
// reflect.Select and the associated complexity.
func (cm *clusterMux) Send(ch <-chan []byte) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.running {
		return types.ErrAlreadyRunning
	}

	cm.sendSources = append(cm.sendSources, ch)
	return nil
}

/*******************************************************************************
 * Recv
 *
 ******************************************************************************/

// Recv implements types.Cluster.
// ANSWERED: Recv delegates to recvMux.Watch(). Data flows from
// recvLoop -> WatcherMux -> watchers. Neither currentLeader() nor
// sending to all members is needed here; the recvLoop handles
// incoming data from all peers and recvMux fans out to watchers.
func (cm *clusterMux) Recv() (<-chan []byte, func()) {
	return cm.recvMux.Watch(util.NoFilter)
}

/*******************************************************************************
 * Join
 *
 ******************************************************************************/

// Join implements types.Cluster.
// For MVP: no-op that returns nil.
func (cm *clusterMux) Join() error {
	return nil
}

/*******************************************************************************
 * Leave
 *
 ******************************************************************************/

// Leave implements types.Cluster.
// For MVP: no-op that returns nil.
func (cm *clusterMux) Leave() error {
	return nil
}

/*******************************************************************************
 * ListNodes
 *
 ******************************************************************************/

// ListNodes implements types.Cluster.
func (cm *clusterMux) ListNodes() []uuid.UUID {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	ids := make([]uuid.UUID, 0, len(cm.nodes))
	for id := range cm.nodes {
		ids = append(ids, id)
	}
	return ids
}

/*******************************************************************************
 * Close
 *
 ******************************************************************************/

var closeTimeoutDuration = 5 * time.Second

// Close implements types.Cluster.
// Follows the bpf/manager.go Close pattern.
func (cm *clusterMux) Close() error {
	cm.mu.Lock()

	if cm.closed {
		cm.mu.Unlock()
		return types.ErrAlreadyClosed
	} else if !cm.running {
		cm.mu.Unlock()
		return types.ErrRunnableMustBeRunningToBeClosed
	}

	close(cm.terminateCh)
	cm.mu.Unlock()

	// Wait for both loops to finish without holding the lock.
	// The sendLoop acquires mu.Lock in its terminate block to close conns.
	timeoutCh := time.After(closeTimeoutDuration)
	select {
	case <-cm.doneCh:
	case <-timeoutCh:
	}

	cm.mu.Lock()
	cm.running = false
	cm.closed = true
	cm.mu.Unlock()

	return nil
}

/*******************************************************************************
 * Done
 *
 ******************************************************************************/

// Done implements types.Cluster.
func (cm *clusterMux) Done() <-chan struct{} {
	return cm.doneCh
}
