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
	"sync"
	"time"

	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/alexandremahdhaoui/udplb/internal/util"
)

// Fast multi master WAL algorithm:
// - A generation is made of 3 phases: proposal, acceptance and deliberation.
// - Any nodes CAN make a proposal during the proposal phase.
// - All nodes MUST accept EXACTLY one proposal during the acceptance phase.
//
// START NEW GENERATION: (g=0)
// 1. Node(x): Propose(y). Identified by a hash
// 2. Node(others): Accept(y). Identified by Hash+PreviousHash.
// 3. IF a majority of "Accept(y)" messages have equal Hash+PreviousHash,
//    THEN this entry is added to the WAL.
// DONE
//
// 3. ELSE no majority, e.g. different proposal came at the same time, and
//    nodes sent different Accept messages.
//    THEN terminate and start a new generation (g=1).
// DONE
//
// START NEW GENERATION: (g=1)
// 0. Use deterministic algorithm based on previous results to ensure
//    convergence.
// 1. Some nodes start to propose again.
// 2. Nodes based on the deterministic algorithm in 0.: accept a proposal.
//    Please note that IF none of the proposal in g=1 is equal to a proposal
//    made in g=0; THEN the algorithm is not guarranted to converge.
// 3. CONVERGENCE or terminate and start new generation.
// DONE
//
// Complexity calculation in a fully connected cluster topology:
// - Let `n` the number of nodes in the cluster.
// - Let `c(n)` a function that associate the number of nodes
//   to the number of messages sent during phase 1 and 2.
// - The time complexity O(n) is of order `c(n)`.
// - During phase 1: up to `n` nodes CAN make a proposal to `n-1` nodes:
//   phase1(n): n*(n-1) messages.
// - During phase 2: `n` nodes MUST accept a proposal and communicate that
//   information to `n-1` nodes: n*(n-1).
// - THUS c(n): 2n*(n-1).
// The time complexity O(n) of the fully connected cluster topology is
// quadratic: O(n): n^2.
//
// Complexity calculation in a leader based cluster topology:
// - In a leader based approach, there are 4 phases: 1.a. 1.b. 2.a. 2.b.
// - All phases creates `(n-1)` messages.
// - The time complexity is O(n).
// => Worth from 5 nodes upwards. For 3 nodes, it's almost similar.
//
// Protocol of leader based approach:
// 1.a.: every node can make a proposal and send it to the leader. (n-1)
// 1.b.: leader redistributes proposal messages to all followers. (n-1)
// 2.a.: every node accept a proposal and send it to the leader. (n-1)
// 2.b.: leader redistributes acceptance messages to all followers (n-1)
// DONE
//
// NB: the leader-based approach requires electing a leader. This can be done on
// the data exchange layer (i.e. the types.Cluster[T]).
// Also, instead of electing a leader one could be deterministically choosen
// using a hash table of available leaders and a modulo of the generation number.

/*******************************************************************************
 * Concrete implementation
 *
 ******************************************************************************/

type wal[T any] struct {
	name    string
	cluster types.Cluster[T]

	// -- WAL internals

	// linkedList is the internal linked list of entries.
	linkedList *util.LinkedList[types.WALEntry[T]]
	// proposalBuffer is a set of wal entries the wal user currently wishes to propose
	// to the cluster. As this is a ringBuffer, older entries gets discarded.
	proposalBuffer *util.RingBuffer[types.WALEntry[T]]

	// -- mgmt
	ctx         context.Context
	running     bool
	closed      bool
	mu          *sync.Mutex
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
	name string,
	cluster types.Cluster[T],
	capacity uint,
	watcherMux *util.WatcherMux[[]T],
) types.WAL[T] {
	return &wal[T]{
		name:           name,
		cluster:        cluster,
		linkedList:     util.NewLinkedList[types.WALEntry[T]](capacity),
		proposalBuffer: util.NewRingBuffer[types.WALEntry[T]](proposingRingBufferSize),
		mu:             &sync.Mutex{},
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
//
// IMPLEMENTED: WALName is set from w.name. Fields are populated as follows:
// - Data, Verb, Timestamp: set by the caller (e.g. DVDS.Propose)
// - WALName: set here from w.name
// - Key, ProposalHash, PreviousHash, Hash: set during consensus (future work)
func (w *wal[T]) Propose(proposal types.WALEntry[T]) error {
	proposal.WALName = w.name
	w.proposalBuffer.Write(proposal)
	return nil
}

/*******************************************************************************
 * types.Watcher
 *
 ******************************************************************************/

// Watch will return a channel sending representation of the underlying state
// as soon it changes.
func (w *wal[T]) Watch() (<-chan []T, func()) {
	return w.watcherMux.Watch(util.NoFilter)
}

/*******************************************************************************
 * types.Runnable
 *
 ******************************************************************************/

var closeTimeoutDuration = 5 * time.Second

// Run implements types.WAL.
func (w *wal[T]) Run(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return types.ErrAlreadyRunning
	} else if w.closed {
		w.mu.Unlock()
		return types.ErrCannotRunClosedRunnable
	}

	w.ctx = ctx
	w.running = true
	go w.eventLoop()
	w.mu.Unlock()

	return nil
}

// eventLoop reads proposals from the proposalBuffer and dispatches them to
// watchers. In single-node MVP mode, proposals are immediately accepted
// without cluster consensus.
func (w *wal[T]) eventLoop() {
	// The proposalBuffer.Next() call blocks until data is available.
	// A reader goroutine is needed so the event loop can check terminateCh
	// while blocked on Next().
	proposalCh := make(chan types.WALEntry[T])
	go func() {
		for {
			entry := w.proposalBuffer.Next()
			select {
			case proposalCh <- entry:
			case <-w.terminateCh:
				return
			}
		}
	}()

	for {
		select {
		case <-w.terminateCh:
			goto terminate
		case proposal := <-proposalCh:
			w.linkedList.Append(proposal)
			w.watcherMux.Dispatch([]T{proposal.Data})
		}
	}

terminate:
	_ = w.watcherMux.Close()
	close(w.doneCh)
}

/*******************************************************************************
 * types.DoneCloser
 *
 ******************************************************************************/

// Close implements types.WAL.
// ANSWERED: w.cluster.Unregister is not needed. The WAL subscribes to the
// cluster via Watch() which returns a cancel function. The event loop calls
// that cancel on termination. The cluster connection is managed externally.
func (w *wal[T]) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running {
		return types.ErrRunnableMustBeRunningToBeClosed
	} else if w.closed {
		return types.ErrAlreadyClosed
	}

	// Trigger termination of the event loop.
	close(w.terminateCh)

	// Await graceful termination within timeout.
	timeoutCh := time.After(closeTimeoutDuration)
	select {
	case <-w.doneCh:
	case <-timeoutCh:
	}

	w.running = false
	w.closed = true

	return nil
}

// Done implements types.WAL.
func (w *wal[T]) Done() <-chan struct{} {
	return w.doneCh
}
