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
package types

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var (
	ErrAlreadyClosed  = errors.New("trying to close an already closed interface")
	ErrAlreadyRunning = errors.New("trying to run an already running interface")

	ErrCannotRunClosedRunnable         = errors.New("cannot run a closed runnable")
	ErrRunnableMustBeRunningToBeClosed = errors.New("runnable must be running to be closed")
)

// Simple interface that implements Encode and Decode method.
type Codec interface {
	Encode() ([]byte, error)
	// Decode will decode the buffer into itself.
	Decode(buf []byte) error
}

// DoneCloser wraps the Close and Done methods. This methods respectively
// signal the interface to terminate its execution gracefully, and returns
// a channel that's closed when work done on behalf of this interface has
// been gracefully terminated
//
// Closing an already closed interface MUST return types.ErrAlreadyClosed.
type DoneCloser interface {
	// Close signal the interface to terminate its execution gracefully.
	// Closing an already closed interface MUST return types.ErrAlreadyClosed.
	Close() error

	// Done returns a channel that's closed when work done on behalf of this
	// interface has been gracefully terminated.
	Done() <-chan struct{}
}

// Runnable can be run and gracefully shut down.
// - Please use `<-Done()` to await until the Runnable is done.
// - Please use `Close()` to terminate the Runnable execution.
type Runnable interface {
	DoneCloser

	// Run must return when it has successfully started.
	Run(ctx context.Context) error
}

type Watcher[T any] interface {
	DoneCloser

	// Watch returns a receiver channel and a cancel func to stop
	// watching.
	Watch() (<-chan T, func())
}

// A StateMachine that can be encoded and decoded.
type StateMachine[T any, U any] interface {
	Codec

	// Execute the StateMachine with a `verb` and `data` as input.
	Execute(verb string, data T)

	// State returns the current underlying state of the machine. E.g.:
	// - For a generic set StateMachine, U is:     map[T]struct{}
	// - For a generic array StateMachine, U is:   []T
	// - For a generic counter StateMachine, U is: map[T]int
	// - For the AssignmentStateMachine, U is:     map[uuid.UUID]uuid.UUID
	State() U

	// DeepCopy is usually used in order to fork the underlying state.
	DeepCopy() StateMachine[T, U]
}

// A Write-Ahead Log interface will provide joiner-nodes with the latest state of the
// underlying datastructure managed by a state machine and a consitent log.
// Nodes that misses messages or which state diverges can request a re-sync?
type WAL[T any] interface {
	Runnable
	Watcher[[]T]

	// Propose a new entry.
	Propose(proposal WALEntry[T]) error
}

type RawCluster = Cluster[RawData]

// Cluster does not know anything about consensus or write-ahead logs, it only cares
// about sending and receiving bytes to other nodes over the network.
type Cluster[T any] interface {
	Runnable

	Send(ch <-chan T) error
	// needs to multiplex the chan so multiple subsystem can use receive the same stream
	// of messages from the cluster.
	// The returned cancel function can be used to stop receiving.
	Recv() (<-chan T, func())

	// Join a cluster. Advertise itself and discover other nodes.
	Join() error
	// Signal other nodes we're leaving the cluster, e.g. for graceful shutdown.
	// Should be called before Close().
	// TODO: or should Close always leave the cluster?
	Leave() error

	// We need to be aware of the nodes in the cluster when trying to make a consensus.
	ListNodes() []uuid.UUID
}

// The WAL multiplexer is responsible for passing the right WAL entries to the
// right WAL interface.
//
// WALMux keeps a table that associate a WALId to a WAL[T].
// It uses that table to find the right WAL interface and decode the RawData into
// the Data field of a new WALEntry[T].
type WALMux interface {
	Register(walId uuid.UUID, v any) (Watcher[any], error)
}
