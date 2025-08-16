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

import "errors"

var ErrNotFound = errors.New("not found")

var ( // Runnable
	ErrAlreadyClosed                   = errors.New("trying to close an already closed interface")
	ErrAlreadyRunning                  = errors.New("trying to run an already running interface")
	ErrCannotRunClosedRunnable         = errors.New("cannot run a closed runnable")
	ErrRunnableMustBeRunningToBeClosed = errors.New("runnable must be running to be closed")
)
