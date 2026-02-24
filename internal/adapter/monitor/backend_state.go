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
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/alexandremahdhaoui/udplb/internal/util"

	"github.com/google/uuid"
)

// IMPLEMENTED: Backend monitor probes backends via UDP dial at a configurable
// interval and dispatches BackendStatusEntry events to watchers.

// BackendState is data kind of type `volatile`.
//
// Please see `internal/controller/README.md` to learn more about data
// kinds, types, signal transduction and pathways.

var (
	_ types.Watcher[types.BackendStatusEntry] = &backendState{}
	_ types.Runnable                          = &backendState{}
)

// backendState periodically probes backends and dispatches
// BackendStatusEntry events to watchers.
type backendState struct {
	backends    map[uuid.UUID]types.BackendSpec
	interval    time.Duration
	timeout     time.Duration
	watcherMux  *util.WatcherMux[types.BackendStatusEntry]
	ctx         context.Context
	running     bool
	closed      bool
	mu          *sync.Mutex
	doneCh      chan struct{}
	terminateCh chan struct{}
}

// NewBackendState creates a new backendState health-check watcher.
func NewBackendState(
	backends map[uuid.UUID]types.BackendSpec,
	interval, timeout time.Duration,
	watcherMux *util.WatcherMux[types.BackendStatusEntry],
) *backendState {
	return &backendState{
		backends:    backends,
		interval:    interval,
		timeout:     timeout,
		watcherMux:  watcherMux,
		running:     false,
		closed:      false,
		mu:          &sync.Mutex{},
		doneCh:      make(chan struct{}),
		terminateCh: make(chan struct{}),
	}
}

func (b *backendState) Run(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.running {
		return types.ErrAlreadyRunning
	}
	if b.closed {
		return types.ErrCannotRunClosedRunnable
	}

	b.ctx = ctx
	b.running = true
	go b.healthCheckLoop()

	return nil
}

func (b *backendState) healthCheckLoop() {
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()

	for {
		select {
		case <-b.terminateCh:
			goto terminate
		case <-ticker.C:
			for id, spec := range b.backends {
				state := probe(spec, b.timeout)
				b.watcherMux.Dispatch(types.BackendStatusEntry{
					BackendId: id,
					State:     state,
				})
			}
		}
	}

terminate:
	_ = b.watcherMux.Close()
	close(b.doneCh)
}

// probe attempts a UDP dial to the backend address and returns the
// resulting state.
//
// LIMITATION: net.DialTimeout("udp", ...) resolves the address but does
// not verify reachability because UDP is connectionless. A real health
// check sends a probe packet and awaits a response. For MVP, the
// dial-only approach is a placeholder that always reports StateAvailable
// for any routable address.
func probe(spec types.BackendSpec, timeout time.Duration) types.State {
	addr := net.JoinHostPort(spec.IP.String(), fmt.Sprintf("%d", spec.Port))
	conn, err := net.DialTimeout("udp", addr, timeout)
	if err != nil {
		return types.StateUnavailable
	}
	_ = conn.Close()
	return types.StateAvailable
}

func (b *backendState) Watch() (<-chan types.BackendStatusEntry, func()) {
	return b.watcherMux.Watch(util.NoFilter)
}

func (b *backendState) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.running {
		return types.ErrRunnableMustBeRunningToBeClosed
	}
	if b.closed {
		return types.ErrAlreadyClosed
	}

	close(b.terminateCh)

	timeoutCh := time.After(5 * time.Second)
	select {
	case <-b.doneCh:
	case <-timeoutCh:
	}

	b.running = false
	b.closed = true
	return nil
}

func (b *backendState) Done() <-chan struct{} {
	return b.doneCh
}
