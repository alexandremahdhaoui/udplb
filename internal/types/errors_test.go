//go:build unit

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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrors(t *testing.T) {
	t.Run("ErrNotFound is non-nil", func(t *testing.T) {
		assert.NotNil(t, ErrNotFound)
		assert.Equal(t, "not found", ErrNotFound.Error())
	})

	t.Run("ErrAlreadyClosed is non-nil", func(t *testing.T) {
		assert.NotNil(t, ErrAlreadyClosed)
		assert.Equal(t, "trying to close an already closed interface", ErrAlreadyClosed.Error())
	})

	t.Run("ErrAlreadyRunning is non-nil", func(t *testing.T) {
		assert.NotNil(t, ErrAlreadyRunning)
		assert.Equal(t, "trying to run an already running interface", ErrAlreadyRunning.Error())
	})

	t.Run("ErrCannotRunClosedRunnable is non-nil", func(t *testing.T) {
		assert.NotNil(t, ErrCannotRunClosedRunnable)
		assert.Equal(t, "cannot run a closed runnable", ErrCannotRunClosedRunnable.Error())
	})

	t.Run("ErrRunnableMustBeRunningToBeClosed is non-nil", func(t *testing.T) {
		assert.NotNil(t, ErrRunnableMustBeRunningToBeClosed)
		assert.Equal(t, "runnable must be running to be closed", ErrRunnableMustBeRunningToBeClosed.Error())
	})

	t.Run("ErrUnsupportedStateMachineCommand is non-nil", func(t *testing.T) {
		assert.NotNil(t, ErrUnsupportedStateMachineCommand)
		assert.Equal(t, "unsupported state machine command", ErrUnsupportedStateMachineCommand.Error())
	})

	t.Run("all error variables are distinct", func(t *testing.T) {
		errs := []error{
			ErrNotFound,
			ErrAlreadyClosed,
			ErrAlreadyRunning,
			ErrCannotRunClosedRunnable,
			ErrRunnableMustBeRunningToBeClosed,
			ErrUnsupportedStateMachineCommand,
		}
		for i := 0; i < len(errs); i++ {
			for j := i + 1; j < len(errs); j++ {
				assert.NotEqual(t, errs[i], errs[j],
					"error at index %d should differ from error at index %d", i, j)
			}
		}
	})
}
