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
package monitoradapter

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"time"

	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/alexandremahdhaoui/udplb/internal/util"
)

// INFO: "LocalAssignment" does not exist and would be bpf.DatastructureManager.WatchAssignment()

// IMPLEMENTED: RemoteAssignment opens a UDP socket and listens for assignment
// events. Assignment events are JSON-decoded and dispatched to watchers.
// Both updates and deletions are received as types.Assignment structs.

var (
	_ types.Watcher[types.Assignment] = &RemoteAssignment{}
	_ types.Runnable                  = &RemoteAssignment{}
)

// RemoteAssignment listens on a UDP socket for assignment events from
// other UDPLB nodes (paracrine signaling).
type RemoteAssignment struct {
	listenAddr  string
	conn        net.PacketConn
	watcherMux  *util.WatcherMux[types.Assignment]
	ctx         context.Context
	running     bool
	closed      bool
	mu          *sync.Mutex
	doneCh      chan struct{}
	terminateCh chan struct{}
}

// NewRemoteAssignment creates a new RemoteAssignment watcher.
func NewRemoteAssignment(
	listenAddr string,
	watcherMux *util.WatcherMux[types.Assignment],
) *RemoteAssignment {
	return &RemoteAssignment{
		listenAddr:  listenAddr,
		watcherMux:  watcherMux,
		mu:          &sync.Mutex{},
		doneCh:      make(chan struct{}),
		terminateCh: make(chan struct{}),
	}
}

func (r *RemoteAssignment) Run(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return types.ErrAlreadyRunning
	}
	if r.closed {
		return types.ErrCannotRunClosedRunnable
	}

	conn, err := net.ListenPacket("udp", r.listenAddr)
	if err != nil {
		return err
	}

	r.ctx = ctx
	r.conn = conn
	r.running = true
	go r.recvLoop()

	return nil
}

func (r *RemoteAssignment) recvLoop() {
	buf := make([]byte, 65536)

	for {
		select {
		case <-r.terminateCh:
			goto terminate
		default:
			_ = r.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, _, err := r.conn.ReadFrom(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue // timeout, loop back to check terminateCh
				}
				// Non-timeout error (e.g. conn.Close() called).
				// Check terminateCh to see if we should exit.
				select {
				case <-r.terminateCh:
					goto terminate
				default:
					continue
				}
			}
			if n == 0 {
				continue
			}

			var assignment types.Assignment
			if err := json.Unmarshal(buf[:n], &assignment); err != nil {
				continue // malformed data, skip
			}
			r.watcherMux.Dispatch(assignment)
		}
	}

terminate:
	// conn is closed by Close() which also unblocks ReadFrom.
	// Closing it here again is harmless but unnecessary.
	_ = r.watcherMux.Close()
	close(r.doneCh)
}

func (r *RemoteAssignment) Watch() (<-chan types.Assignment, func()) {
	return r.watcherMux.Watch(util.NoFilter)
}

func (r *RemoteAssignment) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return types.ErrRunnableMustBeRunningToBeClosed
	}
	if r.closed {
		return types.ErrAlreadyClosed
	}

	close(r.terminateCh)
	// Close the conn to unblock a ReadFrom that may be in progress.
	_ = r.conn.Close()

	timeoutCh := time.After(5 * time.Second)
	select {
	case <-r.doneCh:
	case <-timeoutCh:
	}

	r.running = false
	r.closed = true
	return nil
}

func (r *RemoteAssignment) Done() <-chan struct{} {
	return r.doneCh
}
