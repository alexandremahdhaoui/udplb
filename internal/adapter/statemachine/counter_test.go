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
	"math"
	"testing"

	statemachineadapter "github.com/alexandremahdhaoui/udplb/internal/adapter/statemachine"
	"github.com/alexandremahdhaoui/udplb/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// go test -tags=unit ./internal/adapter/statemachine/array_test.go  | sed 's/interface\ {}/any/g' | less
func TestCounter(t *testing.T) {
	type opt = statemachineadapter.Option[int64, int64]

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
		err  error
		stm  types.StateMachine[int64, int64]
		opts []opt
	)

	/*******************************************************************************
	 * setup
	 *
	 ******************************************************************************/
	setup := func(t *testing.T) {
		t.Helper()
		stm, err = statemachineadapter.NewCounter(opts...)
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
		// TODO:
		// Test cases:
		// 1. no options
		// 2. allow overflow
		// 3. set min and max value
		cases := []struct {
			Name    string
			Options []opt
		}{
			{
				Name:    "NoOptions",
				Options: nil,
			},

			{
				Name:    "AllowOverflow",
				Options: []opt{statemachineadapter.CounterWithAllowOverflow()},
			},

			{
				Name: "WithMinAndMaxVal",
				Options: []opt{
					statemachineadapter.CounterWithMaximumValue(5),
					statemachineadapter.CounterWithMinimumValue(-5),
				},
			},
		}

		t.Run("BasicOperations", func(t *testing.T) {
			// TODO:
			// test struct:
			// - forceOverflowVal
			// - forceOverflowErr
			// - forceUnderflowVal
			// - forceUnderflowErr
			commandSequence := []struct {
				Command types.StateMachineCommand
				Input   int64
			}{
				// -- Add 0: noop
				{
					Command: types.AddCommand,
					Input:   0,
				},

				// -- Add 1:       state=1
				{
					Command: types.AddCommand,
					Input:   1,
				},

				// -- Subtract 3:  state=-2
				{
					Command: types.SubtractCommand,
					Input:   3,
				},

				// TODO: Run this in a different test block to reset state
				// -- force overflow
				// -- add(3),add(math.MaxInt64)
				{
					Command: types.AddCommand,
					Input:   3,
				},
				{
					Command: types.AddCommand,
					Input:   math.MaxInt64,
				},

				// TODO: Run this in a different test block to reset state
				// -- force underflow
				// -- sub(2),sub(math.MaxInt64)
				{
					Command: types.SubtractCommand,
					Input:   2,
				},
				{
					Command: types.SubtractCommand,
					Input:   math.MaxInt64,
				},
			}

			// set it up only once
			setup(t)

			for _, cmd := range []struct {
				Name            types.StateMachineCommand
				Input           int64
				CheckSideEffect func(t *testing.T, input int64)
			}{
				{
					Name:  types.AddCommand,
					Input: 0,
					CheckSideEffect: func(t *testing.T, input int64) {
						state := stm.State()
						assert.Equal(t, 0, state)
					},
				},
			} {
				t.Run(string(cmd.Name), func(t *testing.T) {
					err := stm.Execute(cmd.Name, cmd.Input)

					// or
					assert.ErrorIs(t, nil, err)
					assert.NoError(t, err)

					cmd.CheckSideEffect(t, cmd.Input)
				})
			}
		})

		t.Run("overflow", func(t *testing.T) {})
		t.Run("underflow", func(t *testing.T) {})

		t.Run("unsupported commands", func(t *testing.T) {
			setup(t)
			err := stm.Execute(UnsupportedCommand, 0)
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
