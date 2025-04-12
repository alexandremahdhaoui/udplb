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

// DoneCloser wraps the Close and Done methods. This methods respectively
// signal the interface to terminate its execution gracefully, and returns
// a channel that's closed when work done on behalf of this interface has
// been gracefully terminated
//
// The behavior of Close after the first call MUST be ineffective.
type DoneCloser interface {
	// Close signal the interface to terminate its execution gracefully.
	//
	// The behavior of Close after the first call MUST be ineffective.
	Close() error

	// Done returns a channel that's closed when work done on behalf of this
	// interface has been gracefully terminated
	Done() <-chan struct{}
}
