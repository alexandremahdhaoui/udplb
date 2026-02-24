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
package statemachineadapter_test

import (
	"math"
	"strconv"
	"testing"

	statemachineadapter "github.com/alexandremahdhaoui/udplb/internal/adapter/statemachine"
	"github.com/alexandremahdhaoui/udplb/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// go test -tags=unit ./internal/adapter/statemachine/counter_test.go  | sed 's/interface\ {}/any/g' | less
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
		err error
		stm types.StateMachine[int64, int64]
	)

	/*******************************************************************************
	 * setup
	 *
	 ******************************************************************************/
	setup := func(t *testing.T, opts []opt) {
		t.Helper()
		stm, err = statemachineadapter.NewCounter(opts...)
		require.NoError(t, err)
	}

	/*******************************************************************************
	 * Codec
	 *
	 ******************************************************************************/
	t.Run("Codec", func(t *testing.T) {
		setup(t, []opt{statemachineadapter.WithInitialState[int64, int64](0x1337)})

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
		add := types.AddCommand
		sub := types.SubtractCommand

		t.Run("BasicOperations", func(t *testing.T) {
			sequence := []struct {
				Name          string
				Options       []opt
				Commands      [7]types.StateMachineCommand
				Inputs        [7]int64
				ExpectedErrs  [7]error
				ExpectedState [7]int64
			}{
				{
					Name:    "NoOptions",
					Options: nil,
					Commands: [7]types.StateMachineCommand{
						add, add, sub, add, add, sub, sub,
					},
					Inputs: [7]int64{
						0, 1, 2, 6, math.MaxInt64, 10, math.MaxInt64,
					},
					ExpectedErrs: [7]error{
						nil, nil, nil, nil, statemachineadapter.ErrNoOverflow, nil, statemachineadapter.ErrNoUnderflow,
					},
					ExpectedState: [7]int64{
						0, 1, -1, 5, 5, -5, -5,
					},
				},

				{
					Name:    "AllowOverflow",
					Options: []opt{statemachineadapter.CounterWithAllowOverflow()},
					Commands: [7]types.StateMachineCommand{
						add, add, add, sub, sub, sub, add,
					},
					Inputs: [7]int64{
						5, math.MaxInt64, math.MaxInt64 - 8, math.MaxInt64, math.MaxInt64 - 3, 0, 0,
					},
					ExpectedErrs: [7]error{},
					ExpectedState: [7]int64{
						5, math.MinInt64 + 4, -5, math.MaxInt64 - 3, 0, 0,
					},
				},

				{
					Name: "WithMinAndMaxVal",
					Options: []opt{
						statemachineadapter.CounterWithMaximumValue(5),
						statemachineadapter.CounterWithMinimumValue(-5),
					},
					Commands: [7]types.StateMachineCommand{
						add, add, sub, add, add, sub, sub,
					},
					Inputs: [7]int64{
						0, 1, 2, 6, math.MaxInt64, 10, math.MaxInt64,
					},
					ExpectedErrs: [7]error{},
					ExpectedState: [7]int64{
						0, 1, -1, 5, 5, -5, -5,
					},
				},
			}

			for _, spec := range sequence {
				t.Run(string(spec.Name), func(t *testing.T) {
					// set it up only once
					setup(t, spec.Options)

					for i := range 7 {
						t.Run(strconv.Itoa(i), func(t *testing.T) {
							command := spec.Commands[i]
							input := spec.Inputs[i]
							expectedErr := spec.ExpectedErrs[i]
							expectedState := spec.ExpectedState[i]

							err := stm.Execute(command, input)
							if expectedErr != nil {
								assert.ErrorIs(t, err, expectedErr)
							} else {
								assert.NoError(t, err)
							}

							assert.Equal(t, expectedState, stm.State())
						})
					}
				})
			}
		})

		t.Run("UnsupportedCommand", func(t *testing.T) {
			setup(t, nil)
			err := stm.Execute("UnsupportedCommand", 0)
			assert.ErrorIs(t, err, types.ErrUnsupportedStateMachineCommand)
		})

		t.Run("ErrInputMustBeAPositiveInteger", func(t *testing.T) {
			setup(t, nil)
			err := stm.Execute(types.AddCommand, -1)
			assert.ErrorIs(t, err, statemachineadapter.ErrInputMustBeAPositiveInteger)
		})
	})

	/*******************************************************************************
	 * State
	 *
	 ******************************************************************************/
	t.Run("State", func(t *testing.T) {
		var initialVal int64 = 42
		opt := statemachineadapter.WithInitialState[int64, int64](initialVal)

		stm, err := statemachineadapter.NewCounter(opt)
		require.NoError(t, err)

		actual := stm.State()
		assert.Equal(t, initialVal, actual)
	})

	/*******************************************************************************
	 * DeepCopy
	 *
	 ******************************************************************************/
	t.Run("DeepCopy", func(t *testing.T) {
		t.Run("basic deep copy preserves state", func(t *testing.T) {
			var initialVal int64 = 100
			opt := statemachineadapter.WithInitialState[int64, int64](initialVal)
			input, err := statemachineadapter.NewCounter(opt)
			require.NoError(t, err)

			actual := input.DeepCopy()
			assert.Equal(t, input.State(), actual.State())
		})

		t.Run("deep copy with max and min val", func(t *testing.T) {
			input, err := statemachineadapter.NewCounter(
				statemachineadapter.WithInitialState[int64, int64](50),
				statemachineadapter.CounterWithMaximumValue(100),
				statemachineadapter.CounterWithMinimumValue(0),
			)
			require.NoError(t, err)

			copied := input.DeepCopy()
			assert.Equal(t, input.State(), copied.State())

			// The copy should be independent: mutating the copy
			// should not affect the original.
			err = copied.Execute(types.AddCommand, 10)
			require.NoError(t, err)
			assert.Equal(t, int64(60), copied.State())
			assert.Equal(t, int64(50), input.State())
		})

		t.Run("deep copy without max/min val", func(t *testing.T) {
			input, err := statemachineadapter.NewCounter(
				statemachineadapter.WithInitialState[int64, int64](10),
			)
			require.NoError(t, err)

			copied := input.DeepCopy()
			assert.Equal(t, int64(10), copied.State())
		})
	})

	/*******************************************************************************
	 * Subtract negative input
	 *
	 ******************************************************************************/
	t.Run("SubtractNegativeInput", func(t *testing.T) {
		setup(t, nil)
		err := stm.Execute(types.SubtractCommand, -1)
		assert.ErrorIs(t, err, statemachineadapter.ErrInputMustBeAPositiveInteger)
	})
}
