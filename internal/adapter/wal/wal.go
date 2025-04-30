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
package waladapter

import (
	"context"

	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/alexandremahdhaoui/udplb/internal/util"
	"github.com/google/uuid"
)

/*******************************************************************************
 * Concrete implementation
 *
 ******************************************************************************/

type wal[T any] struct {
	id      uuid.UUID
	cluster types.Cluster[T]

	// -- WAL internals

	// linkedList is the internal linked list of entries.
	linkedList *util.LinkedList[types.WALEntry[T]]
	// proposalBuffer is a set of wal entries the wal user currently wishes to propose
	// to the cluster. As this is a ringBuffer, older entries gets discarded.
	proposalBuffer *util.RingBuffer[types.WALEntry[T]]

	// -- mgmt
	doneCh      chan struct{}
	terminateCh chan struct{}

	// -- WatcherMux
	watcherMux *util.WatcherMux[[]T]
}

/*******************************************************************************
 * New
 *
 ******************************************************************************/

const proposingRingBufferSize = 32

func New[T any](
	cluster types.Cluster[T],
	capacity uint,
	watcherMux *util.WatcherMux[[]T],
) types.WAL[T] {
	return &wal[T]{
		id:             uuid.New(),
		cluster:        cluster,
		linkedList:     util.NewLinkedList[types.WALEntry[T]](capacity),
		proposalBuffer: util.NewRingBuffer[types.WALEntry[T]](proposingRingBufferSize),
		doneCh:         make(chan struct{}),
		terminateCh:    make(chan struct{}),
		watcherMux:     watcherMux,
	}
}

/*******************************************************************************
 * Propose
 *
 ******************************************************************************/

// Propose implements types.WAL.
func (w *wal[T]) Propose(proposal types.WALEntry[T]) error {
	// TODO: add the WALId to the proposal.
	proposal.WALId = w.id
	w.proposalBuffer.Write(proposal)
	return nil
}

/*******************************************************************************
 * types.Watcher
 *
 ******************************************************************************/

// Watch will return a channel sending representation of the underlying state
// as soon it changes.
func (w *wal[T]) Watch() (<-chan []T, error) {
	return w.watcherMux.Watch(), nil
}

/*******************************************************************************
 * types.Runnable
 *
 ******************************************************************************/

// Run implements types.WAL.
func (w *wal[T]) Run(ctx context.Context) error {
	panic("unimplemented")
}

/*******************************************************************************
 * types.DoneCloser
 *
 ******************************************************************************/

// Close implements types.WAL.
func (w *wal[T]) Close() error {
	// -- trigger termination.
	close(w.terminateCh)
	// -- await termination
	<-w.doneCh

	// w.cluster. Unregister ???
	panic("unimplemented")
}

// Done implements types.WAL.
func (w *wal[T]) Done() <-chan struct{} {
	return w.doneCh
}
