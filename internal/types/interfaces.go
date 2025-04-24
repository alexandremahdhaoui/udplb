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
)

var (
	ErrAlreadyClosed  = errors.New("trying to close an already closed interface")
	ErrAlreadyRunning = errors.New("trying to run an already running interface")

	ErrCannotRunClosedRunnable         = errors.New("cannot run a closed runnable")
	ErrRunnableMustBeRunningToBeClosed = errors.New("runnable must be running to be closed")
)

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
