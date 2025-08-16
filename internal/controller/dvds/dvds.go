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
 * New
 *
 ******************************************************************************/

// - T is the type of the data field of a write-ahead log entry.
// - U is the representation of the internal state of the state machine executing the
// Write-Ahead Log.
func New[T any, U any](
	stateMachine types.StateMachine[T, U],
	wal types.WAL[T],
	watcherMux *util.WatcherMux[U],
) types.DVDS[T, U] {
	return &dvds[T, U]{
		stateMachine: stateMachine,
		lastCommit:   types.Hash{},
		wal:          wal,
		watcherMux:   watcherMux,
		doneCh:       make(chan struct{}),
		terminateCh:  make(chan struct{}),
	}
}

/*******************************************************************************
 * dvds
 *
 ******************************************************************************/

type dvds[T any, U any] struct {
	// -- State
	stateMachine types.StateMachine[T, U]
	lastCommit   types.Hash

	// -- WAL
	wal types.WAL[T]

	// -- WatcherMux
	watcherMux *util.WatcherMux[U]

	// -- mgmt
	doneCh      chan struct{}
	terminateCh chan struct{}
}

/*******************************************************************************
 * Propose
 *
 ******************************************************************************/

// Propose implements types.WAL.
func (ds *dvds[T, U]) Propose(key string, command types.StateMachineCommand, obj T) error {
	// TODO: Implement types.NewWALEntry() that computes the Hash
	entry := types.WALEntry[T]{
		Hash:         types.Hash{}, // TODO:
		PreviousHash: ds.lastCommit,
		NextHash:     types.Hash{},
		Timestamp:    time.Now(),
		WALId:        "", // TODO:
		Key:          key,
		Command:      command,
		Object:       obj,
	}

	// Initially the goal was to execute the proposal and commit it to the dvds,
	// but we won't be doing that (WAL.Propose() is non-blocking).
	// Once a proposed value is accepted, the WAL sends an udpate and the value
	// will be eventually committed.
	return ds.wal.Propose(entry)
}

/*******************************************************************************
 * types.Runnable
 *
 ******************************************************************************/

// Run implements types.WAL.
func (ds *dvds[T, U]) Run(ctx context.Context) error {
	if err := ds.wal.Run(ctx); err != nil { // TODO: wrap ctx
		return err
	}

	ch, cancel := ds.wal.Watch()
	for {
		select {
		case <-ds.terminateCh:
			cancel()
			close(ds.doneCh)
		case wal := <-ch:
			switch {
			default:
				if err := ds.replayWAL(wal); err != nil {
					panic(err) // TODO: Handle error
				}
			case ds.lastCommit == wal.TailHash():
				continue // Nothing to do
			case !wal.Contains(ds.lastCommit):
				// Case where the dvds last commit is below the WAL's low watermark.
				// In other words, this case happen if:
				// - This node was not able to commit for a while and diverged.
				// - This node is joining the cluster.
				if err := ds.init(); err != nil {
					panic(err) // TODO: Handle error
				}
			}

		}
	}
}

func (ds *dvds[T, U]) init() error {
	// TODO: implement me
	panic("unimplemented")
}

func (ds *dvds[T, U]) replayWAL(ll types.WalLinkedList[T]) error {
	prev, err := ll.Get(ds.lastCommit)
	if err != nil {
		return err
	}

	// play the wal until we reach the tail
	for prev.Hash != ll.TailHash() {
		curr, err := ll.GetNext(prev.Hash)
		if err != nil {
			return err
		}

		if err := ds.stateMachine.Execute(curr.Command, curr.Object); err != nil {
			return err
		}

		ds.lastCommit = curr.Hash
		prev = curr
	}

	return nil
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

/*******************************************************************************
 * types.Watcher
 *
 ******************************************************************************/

// Watch will return a representation of the underlying state as soon as the
// state machine is mutated.
func (ds *dvds[T, U]) Watch() (<-chan U, func()) {
	return ds.watcherMux.Watch(util.NoFilter)
}
