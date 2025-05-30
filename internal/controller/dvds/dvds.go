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
	watcherMux *util.WatcherMux[U],
) DVDS[T, U] {
	return &dvds[T, U]{
		stateMachine: stateMachine,
		cmdWAL:       nil,
		snapshotWAL:  nil,
		watcherMux:   watcherMux,
		doneCh:       make(chan struct{}),
		terminateCh:  make(chan struct{}),
	}
}

/*******************************************************************************
 * Propose
 *
 ******************************************************************************/

// Propose implements types.WAL.
func (ds *dvds[T, U]) Propose(proposal T) error {
	// TODO: how?
	entry := types.WALEntry[T]{
		Key:          "",
		Data:         proposal,
		Timestamp:    time.Time{},
		WALName:      "",
		ProposalHash: [32]byte{},
		PreviousHash: [32]byte{},
		Hash:         [32]byte{},
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

// Run implements types.WAL.
func (ds *dvds[T, U]) Run(ctx context.Context) error {
	panic("unimplemented")
}

/*******************************************************************************
 * types.DoneCloser
 *
 ******************************************************************************/

// Close implements types.WAL.
func (ds *dvds[T, U]) Close() error {
	// -- trigger termination.
	close(ds.terminateCh)
	// -- await termination
	<-ds.doneCh

	// w.cluster. Unregister ???
	panic("unimplemented")
}

// Done implements types.WAL.
func (ds *dvds[T, U]) Done() <-chan struct{} {
	return ds.doneCh
}
