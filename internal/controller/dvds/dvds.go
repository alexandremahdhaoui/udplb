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
package dvds

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/alexandremahdhaoui/udplb/internal/util"
)

/*******************************************************************************
 * Interface
 *
 ******************************************************************************/

// DVDS or Distributed Volatile Data Structure is a controller that consent with a
// cluster of nodes about a consistent state of a given data structure U.
//
// NB: this is maybe a bad implementation and the WAL should be streamed to the state
// machine. But this implementation ensures the state is fetched from other nodes on
// restart and that the WAL is properly purged etc...
type DVDS[T any, U any] interface {
	types.Runnable
	types.Watcher[U]

	// Propose a new data entry.
	Propose(proposal T) error
}

/*******************************************************************************
 * Concrete implementation
 *
 ******************************************************************************/

type dvds[T any, U any] struct {
	stateMachine types.StateMachine[T, U]

	// -- WAL
	cmdWAL types.WAL[T]
	// Raw snapshot that can be used to initialize a new state machine.
	snapshotWAL types.WAL[U]

	// -- WatcherMux
	watcherMux *util.WatcherMux[U]

	// -- mgmt
	ctx         context.Context
	running     bool
	closed      bool
	mu          *sync.Mutex
	doneCh      chan struct{}
	terminateCh chan struct{}
}

/*******************************************************************************
 * New
 *
 ******************************************************************************/

// - T is the type of the data field of a write-ahead log entry.
// - U is the representation of the internal state of the state machine executing the
// Write-Ahead Log.
func New[T any, U any](
	stateMachine types.StateMachine[T, U],
	cmdWAL types.WAL[T],
	snapshotWAL types.WAL[U],
	watcherMux *util.WatcherMux[U],
) DVDS[T, U] {
	return &dvds[T, U]{
		stateMachine: stateMachine,
		cmdWAL:       cmdWAL,
		snapshotWAL:  snapshotWAL,
		watcherMux:   watcherMux,
		mu:           &sync.Mutex{},
		doneCh:       make(chan struct{}),
		terminateCh:  make(chan struct{}),
	}
}

/*******************************************************************************
 * Propose
 *
 ******************************************************************************/

// Propose creates a WALEntry with PutCommand verb and submits it to the command WAL.
//
// IMPLEMENTED: The WALEntry fields are populated as follows:
// - Data:         proposal value
// - Verb:         PutCommand (verb-based routing)
// - Timestamp:    time.Now()
// - WALName:      set by wal.Propose() from w.name
// - Key, ProposalHash, PreviousHash, Hash: set during consensus (future work)
func (ds *dvds[T, U]) Propose(proposal T) error {
	entry := types.WALEntry[T]{
		Data:      proposal,
		Verb:      types.PutCommand,
		Timestamp: time.Now(),
	}
	return ds.cmdWAL.Propose(entry)
}

/*******************************************************************************
 * types.Watcher
 *
 ******************************************************************************/

// Watch will return a representation of the underlying state as soon as the
// state machine is mutated.
func (ds *dvds[T, U]) Watch() (<-chan U, func()) {
	return ds.watcherMux.Watch(util.NoFilter)
}

/*******************************************************************************
 * types.Runnable
 *
 ******************************************************************************/

var closeTimeoutDuration = 5 * time.Second

// Run subscribes to the command and snapshot WALs, then starts the event loop.
// The DVDS does NOT own WAL lifecycle: it only subscribes to Watch channels.
// WALs must be started externally by the bootstrap code.
func (ds *dvds[T, U]) Run(ctx context.Context) error {
	ds.mu.Lock()
	if ds.running {
		ds.mu.Unlock()
		return types.ErrAlreadyRunning
	} else if ds.closed {
		ds.mu.Unlock()
		return types.ErrCannotRunClosedRunnable
	}

	cmdCh, cmdCancel := ds.cmdWAL.Watch()
	snapCh, snapCancel := ds.snapshotWAL.Watch()

	ds.ctx = ctx
	ds.running = true
	go ds.eventLoop(cmdCh, cmdCancel, snapCh, snapCancel)
	ds.mu.Unlock()

	return nil
}

// eventLoop processes WAL entries by applying them to the state machine and
// dispatching the resulting state to watchers.
func (ds *dvds[T, U]) eventLoop(
	cmdCh <-chan []T,
	cmdCancel func(),
	snapCh <-chan []U,
	snapCancel func(),
) {
	for {
		select {
		case <-ds.terminateCh:
			goto terminate
		case entries, ok := <-cmdCh:
			if !ok {
				goto terminate
			}
			for _, entry := range entries {
				// For MVP: use PutCommand for all entries.
				// The WAL dispatches []T (not []WALEntry[T]), so the Verb
				// field is not available here. Future work: change WAL Watch
				// to dispatch []WALEntry[T] to enable verb routing.
				_ = ds.stateMachine.Execute(types.PutCommand, entry)
			}
			ds.watcherMux.Dispatch(ds.stateMachine.State())
		case snapshots, ok := <-snapCh:
			if !ok {
				goto terminate
			}
			// Apply the last snapshot (most recent state).
			if len(snapshots) > 0 {
				lastSnap := snapshots[len(snapshots)-1]
				buf, err := json.Marshal(lastSnap)
				if err != nil {
					continue
				}
				_ = ds.stateMachine.Decode(buf)
				ds.watcherMux.Dispatch(ds.stateMachine.State())
			}
		}
	}

terminate:
	cmdCancel()
	snapCancel()
	_ = ds.watcherMux.Close()
	close(ds.doneCh)
}

/*******************************************************************************
 * types.DoneCloser
 *
 ******************************************************************************/

// Close gracefully stops the event loop following the terminateCh/doneCh pattern.
// ANSWERED: w.cluster.Unregister is not needed. DVDS does not own WAL or cluster
// lifecycle. It only cancels its Watch subscriptions (cmdCancel, snapCancel) in
// the terminate block. WAL and cluster teardown is managed by the bootstrap code.
func (ds *dvds[T, U]) Close() error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if !ds.running {
		return types.ErrRunnableMustBeRunningToBeClosed
	} else if ds.closed {
		return types.ErrAlreadyClosed
	}

	// Trigger termination of the event loop.
	close(ds.terminateCh)

	// Await graceful termination within timeout.
	timeoutCh := time.After(closeTimeoutDuration)
	select {
	case <-ds.doneCh:
	case <-timeoutCh:
	}

	ds.running = false
	ds.closed = true

	return nil
}

// Done returns a channel that is closed when the event loop has exited.
func (ds *dvds[T, U]) Done() <-chan struct{} {
	return ds.doneCh
}
