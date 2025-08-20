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

/******************************************************************************
 * Cluster
 *
 ******************************************************************************/

// Cluster does not know anything about consensus or write-ahead logs, it only cares
// about sending and receiving bytes to other nodes over the network.
type Cluster[T any] interface {
	Runnable

	Send(ch <-chan T) error
	// needs to multiplex the chan so multiple subsystem can receive the same stream
	// of messages from the cluster.
	// The returned cancel function can be used to stop receiving.
	Recv() (<-chan T, func()) // this is basically types.Watcher[T]

	// Join a cluster. Advertise itself and discover other nodes.
	Join() error
	// Signal other nodes we're leaving the cluster, e.g. for graceful shutdown.
	// Should be called before Close().
	// TODO: or should Close always leave the cluster?
	Leave() error

	// We need to be aware of the nodes in the cluster when trying to make a consensus.
	ListNodes() []uuid.UUID
}

type RawCluster = Cluster[RawData]

/******************************************************************************
 * Codec
 *
 ******************************************************************************/

// Simple interface that implements Encode and Decode method.
type Codec interface {
	Encode() ([]byte, error)
	// Decode will decode the buffer into itself.
	Decode(buf []byte) error
}

/******************************************************************************
 * DoneCloser
 *
 ******************************************************************************/

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

/******************************************************************************
 * DVDS
 *
 ******************************************************************************/

// DVDS or Distributed Volatile Data Structure is a controller that consent with a
// cluster of nodes about a consistent state of a given data structure U.
//
// NB: this is maybe a bad implementation and the WAL should be streamed to the state
// machine. But this implementation ensures the state is fetched from other nodes on
// restart and that the WAL is properly purged etc...
//
// On start: (omits all calls related to joining a cluster etc...)
// - getWAL() -> returns the latest state of the WAL
// - getStateAt(commit) -> returns the state of the data structure at commit
type DVDS[T any, U any] interface {
	Runnable
	Watcher[U]

	// Propose a new data entry.
	Propose(key string, command StateMachineCommand, obj T) error
}

/******************************************************************************
 * Runnable
 *
 ******************************************************************************/

// Runnable can be run and gracefully shut down.
// - Please use `<-Done()` to await until the Runnable is done.
// - Please use `Close()` to terminate the Runnable execution.
type Runnable interface {
	DoneCloser

	// Run must return when it has successfully started.
	Run(ctx context.Context) error
}

/******************************************************************************
 * StateMachine
 *
 ******************************************************************************/

type StateMachineCommand string

const (
	AddCommand      StateMachineCommand = "Add"
	AppendCommand   StateMachineCommand = "Append"
	DeleteCommand   StateMachineCommand = "Delete"
	PutCommand      StateMachineCommand = "Put"
	SubtractCommand StateMachineCommand = "Subtract"
)

var ErrUnsupportedStateMachineCommand = errors.New("unsupported state machine command")

// A StateMachine that can be encoded and decoded.
type StateMachine[T any, U any] interface {
	Codec

	// Execute the StateMachine with a `verb` and `data` as input.
	// NB: the subject of the command is always the underlying state
	// of type U of this types.StateMachine instance.
	Execute(verb StateMachineCommand, obj T) error

	// State returns the current underlying state of the machine. E.g.:
	// - For a generic set StateMachine, U is:     map[T]struct{}
	// - For a generic array StateMachine, U is:   []T
	// - For a generic counter StateMachine, U is: map[T]int
	// - For the AssignmentStateMachine, U is:     map[uuid.UUID]uuid.UUID
	State() U

	// DeepCopy is usually used in order to fork the underlying state.
	DeepCopy() StateMachine[T, U]
}

/******************************************************************************
 * WAL
 *
 ******************************************************************************/

// A Write-Ahead Log interface will provide joiner-nodes with the latest state of the
// underlying datastructure managed by a state machine and a consitent log.
// Nodes that misses messages or which state diverges can request a re-sync?
type WAL[T any] interface {
	Runnable
	Watcher[WalLinkedList[T]]

	// Propose a new entry. This is a non blocking call.
	Propose(entry WALEntry[T]) error
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

/******************************************************************************
 * Watcher
 *
 ******************************************************************************/

type Watcher[T any] interface {
	DoneCloser

	// Watch returns a receiver channel and a cancel func to stop
	// watching.
	Watch() (<-chan T, func())
}
