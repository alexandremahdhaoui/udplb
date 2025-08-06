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

const UnsupportedCommand types.StateMachineCommand = "UnsupportedCommand"

// go test -tags=unit ./internal/adapter/statemachine/array_test.go  | sed 's/interface\ {}/any/g' | less
func TestArray(t *testing.T) {
	// All state machines implement the same interfaces.
	// Use different test cases and test them all at once.

	// - What are the expected behavior for each functions?
	// E.g.: expected behaviors.
	// - We can test unexpected behavior as well e.g. for .Execute()
	//   unsupported commands.

	type Entry struct {
		Key   uint32
		Value uint32
	}

	filterFunc := func(obj, entry Entry) bool {
		return obj.Key == entry.Key
	}

	/*******************************************************************************
	 * vars
	 *
	 ******************************************************************************/

	var (
		err error
		stm types.StateMachine[Entry, []Entry]
	)

	/*******************************************************************************
	 * setup
	 *
	 ******************************************************************************/
	setup := func(t *testing.T) {
		t.Helper()
		stm, err = statemachineadapter.NewArray(filterFunc)
		require.NoError(t, err)
	}

	/*******************************************************************************
	 * Codec
	 *
	 ******************************************************************************/
	t.Run("Codec", func(t *testing.T) {
		setup(t)

		// Round trip
		b0, err := stm.Encode()
		assert.NoError(t, err)
		err = stm.Decode(b0)
		assert.NoError(t, err)
		b1, err := stm.Encode()
		assert.NoError(t, err)
		// must be equal
		assert.Equal(t, b0, b1)
	})

	/*******************************************************************************
	 * Execute
	 *
	 ******************************************************************************/

	t.Run("Execute", func(t *testing.T) {
		t.Run("supported", func(t *testing.T) {
			// set it up only once
			setup(t)

			for _, cmd := range []struct {
				Name            types.StateMachineCommand
				Input           Entry
				CheckSideEffect func(t *testing.T, input Entry)
			}{
				// -- APPEND 0,0 (at position 0)
				{
					Name:  types.AppendCommand,
					Input: Entry{0, 0},
					CheckSideEffect: func(t *testing.T, input Entry) {
						state := stm.State()
						assert.Len(t, state, 1)
						assert.Equal(t, input, state[0])
					},
				},

				// -- APPEND 1,1 (at position 1)
				{
					Name:  types.AppendCommand,
					Input: Entry{1, 1},
					CheckSideEffect: func(t *testing.T, input Entry) {
						state := stm.State()
						assert.Len(t, state, 2)
						assert.Equal(t, input, state[1])
					},
				},

				// -- PUT 0,0x1337 in place of item with key == 0
				{
					Name:  types.PutCommand,
					Input: Entry{0, 0x1337},
					CheckSideEffect: func(t *testing.T, input Entry) {
						state := stm.State()
						assert.Len(t, state, 2)
						assert.Equal(t, input, state[0])
					},
				},

				// -- DELETE item with key == 0
				{
					Name:  types.DeleteCommand,
					Input: Entry{0, 0},
					CheckSideEffect: func(t *testing.T, input Entry) {
						state := stm.State()
						assert.Len(t, state, 1)
						assert.Equal(t, Entry{1, 1}, state[0])
					},
				},
			} {
				t.Run(string(cmd.Name), func(t *testing.T) {
					t.Log("before: ", stm.State())
					err := stm.Execute(cmd.Name, cmd.Input)
					assert.NoError(t, err)
					t.Log("after:  ", stm.State())
					cmd.CheckSideEffect(t, cmd.Input)
				})
			}
		})

		t.Run("unsupported", func(t *testing.T) {
			setup(t)
			err := stm.Execute(UnsupportedCommand, Entry{})
			assert.ErrorIs(t, err, types.ErrUnsupportedStateMachineCommand)
		})
	})

	/*******************************************************************************
	 * State
	 *
	 ******************************************************************************/
	t.Run("State", func(t *testing.T) {
		input := []uint32{0, 1, 2}

		opt := statemachineadapter.WithInitialState[uint32](input)
		stm, err := statemachineadapter.NewArray(nil, opt)
		assert.NoError(t, err)

		// -- State are equal
		actual := stm.State()
		assert.Equal(t, input, actual)

		// -- States are mutually immutable
		actual[0] = 0x1337
		assert.NotEqual(t, input[0], actual[0])
	})

	/*******************************************************************************
	 * DeepCopy
	 *
	 ******************************************************************************/
	t.Run("DeepCopy", func(t *testing.T) {
		opt := statemachineadapter.WithInitialState[uint32]([]uint32{0, 1, 2})
		inputStm, err := statemachineadapter.NewArray(nil, opt)
		assert.NoError(t, err)

		// -- State are equal
		actual0 := inputStm.DeepCopy()
		actual1 := inputStm.DeepCopy()
		assert.Equal(t, inputStm, actual0)
		assert.Equal(t, inputStm, actual1)

		// -- States are mutually immutable
		err = actual0.Execute(types.AppendCommand, 3)
		assert.NoError(t, err)
		err = actual1.Execute(types.AppendCommand, 0x1337)
		assert.NoError(t, err)

		assert.NotEqual(t, actual0.State(), actual1.State())
		assert.NotEqual(t, inputStm.State(), actual0.State())
		assert.NotEqual(t, inputStm.State(), actual1.State())
	})
}
