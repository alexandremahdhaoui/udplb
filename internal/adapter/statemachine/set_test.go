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
package statemachineadapter_test

import (
	"testing"

	statemachineadapter "github.com/alexandremahdhaoui/udplb/internal/adapter/statemachine"
	"github.com/alexandremahdhaoui/udplb/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// go test -tags=unit ./internal/adapter/statemachine/map_test.go  | sed 's/interface\ {}/any/g' | less
func TestSet(t *testing.T) {
	// All state machines implement the same interfaces.
	// Use different test cases and test them all at once.

	// - What are the expected behavior for each functions?
	// E.g.: expected behaviors.
	// - We can test unexpected behavior as well e.g. for .Execute()
	//   unsupported commands.

	/*******************************************************************************
	 * vars
	 *
	 ******************************************************************************/

	var (
		err error
		stm types.StateMachine[string, map[string]struct{}]
	)

	/*******************************************************************************
	 * setup
	 *
	 ******************************************************************************/
	setup := func(t *testing.T) {
		t.Helper()
		stm, err = statemachineadapter.NewSet[string]()
		require.NoError(t, err)
		err = nil
	}

	/*******************************************************************************
	 * Codec
	 *
	 ******************************************************************************/
	t.Run("Codec", func(t *testing.T) {
		setup(t)

		var b0, b1 []byte
		t.Run("Encode", func(t *testing.T) {
			b0, err = stm.Encode()
			assert.NoError(t, err)
		})

		t.Run("Decode", func(t *testing.T) {
			err = stm.Decode(b0)
			assert.NoError(t, err)
		})

		t.Run("EncodeAgain", func(t *testing.T) {
			b1, err = stm.Encode()
			assert.NoError(t, err)
		})

		t.Run("CheckResult", func(t *testing.T) {
			// must be equal
			assert.Equal(t, b0, b1)
		})
	})

	/*******************************************************************************
	 * Execute
	 *
	 ******************************************************************************/

	t.Run("Execute", func(t *testing.T) {
		t.Run("SupportedCommands", func(t *testing.T) {
			setup(t)
			input := "input"

			t.Run("Put", func(t *testing.T) {
				err := stm.Execute(types.PutCommand, input)
				assert.NoError(t, err)
				assert.Equal(t, map[string]struct{}{input: {}}, stm.State())
			})

			t.Run("Delete", func(t *testing.T) {
				err := stm.Execute(types.DeleteCommand, input)
				assert.NoError(t, err)
				assert.Equal(t, map[string]struct{}{}, stm.State())
			})
		})

		t.Run("unsupported", func(t *testing.T) {
			setup(t)
			err := stm.Execute("UnsupportedCommand", "")
			assert.ErrorIs(t, err, types.ErrUnsupportedStateMachineCommand)
		})
	})

	/*******************************************************************************
	 * State
	 *
	 ******************************************************************************/
	t.Run("State", func(t *testing.T) {
		input := map[string]struct{}{"key": {}}
		opt := statemachineadapter.WithInitialState[string](input)

		stm, err := statemachineadapter.NewSet(opt)
		assert.NoError(t, err)

		// -- State are equal
		actual := stm.State()
		assert.Equal(t, input, actual)

		// -- States are mutually immutable
		actual["test"] = struct{}{}
		assert.NotEqual(t, input, actual)
	})

	/*******************************************************************************
	 * DeepCopy
	 *
	 ******************************************************************************/
	t.Run("DeepCopy", func(t *testing.T) {
		input := map[string]struct{}{"key": {}}
		opt := statemachineadapter.WithInitialState[string](input)

		inputStm, err := statemachineadapter.NewSet(opt)
		assert.NoError(t, err)

		// -- States are equal
		actual0 := inputStm.DeepCopy()
		actual1 := inputStm.DeepCopy()
		assert.Equal(t, inputStm.State(), actual0.State())
		assert.Equal(t, inputStm.State(), actual1.State())

		// -- States are mutually immutable
		err = actual0.Execute(types.PutCommand, "test")
		assert.NoError(t, err)
		err = actual1.Execute(types.DeleteCommand, "key")
		assert.NoError(t, err)

		assert.NotEqual(t, actual0.State(), actual1.State())
		assert.NotEqual(t, inputStm.State(), actual0.State())
		assert.NotEqual(t, inputStm.State(), actual1.State())
	})
}
