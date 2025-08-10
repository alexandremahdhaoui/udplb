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
package statemachineadapter

import (
	"errors"

	"github.com/alexandremahdhaoui/tooling/pkg/flaterrors"
	"github.com/alexandremahdhaoui/udplb/internal/types"
)

var (
	_ types.StateMachine[int64, int64] = &counter{}
	_ stateSetter[int64, int64]        = &counter{}
)

var ErrStateMachineMustBeCounter = errors.New("state machine must be a counter")

// Allows overflowing the int64 counter.
func CounterWithAllowOverflow() Option[int64, int64] {
	return func(m stateSetter[int64, int64]) error {
		c, ok := any(m).(*counter)
		if !ok {
			return flaterrors.Join(
				ErrStateMachineMustBeCounter,
				errors.New("executing CounterWithAllowOverflow option"),
			)
		}
		c.allowOverflow = true
		return nil
	}
}

// When setting a max, if the state is supposed to be bigger to the
func CounterWithMaximumValue(maxVal int64) Option[int64, int64] {
	return func(m stateSetter[int64, int64]) error {
		c, ok := any(m).(*counter)
		if !ok {
			return flaterrors.Join(
				ErrStateMachineMustBeCounter,
				errors.New("executing CounterWithMaximumValue option"),
			)
		}
		c.maxVal = &maxVal
		return nil
	}
}

// When setting minVal
func CounterWithMinimumValue(minVal int64) Option[int64, int64] {
	return func(m stateSetter[int64, int64]) error {
		c, ok := any(m).(*counter)
		if !ok {
			return flaterrors.Join(
				ErrStateMachineMustBeCounter,
				errors.New("executing CounterWithMinimumValue option"),
			)
		}
		c.minVal = &minVal
		return nil
	}
}

// Counter options:
//   - CounterWithAllowOverflow: if you want the counter to {under,over}flow
//     at the int64 min/max value without throwing an error.
//   - CounterWith{Minimum,Maximum}Value If you want the counter to block
//     at a specific value without throwing an error or without overflowing
func NewCounter(
	opts ...Option[int64, int64],
) (types.StateMachine[int64, int64], error) {
	out := &counter{
		allowOverflow: false,
		maxVal:        nil,
		minVal:        nil,
		state:         0,
	}
	return execOptions(out, opts)
}

// TODO: Add support for capacity (as in "max capacity")
// TODO: Add support for concurrency
type counter struct {
	allowOverflow  bool
	maxVal, minVal *int64
	state          int64
}

// Encode implements types.StateMachine.
func (stm *counter) Encode() ([]byte, error) {
	return encodeDefault(stm.state)
}

// Decode implements types.StateMachine.
func (stm *counter) Decode(buf []byte) error {
	return decodeDefault(buf, &stm.state)
}

// DeepCopy implements types.StateMachine.
func (stm *counter) DeepCopy() types.StateMachine[int64, int64] {
	out := &counter{
		allowOverflow: false,
		maxVal:        nil,
		minVal:        nil,
		state:         stm.State(),
	}

	if stm.maxVal != nil {
		v := *stm.maxVal
		out.maxVal = &v
	}

	if stm.minVal != nil {
		v := *stm.minVal
		out.minVal = &v
	}

	return out
}

var (
	ErrNoOverflow                  = errors.New("state is not allowed to overflow")
	ErrNoUnderflow                 = errors.New("state is not allowed to underflow")
	ErrInputMustBeAPositiveInteger = errors.New("input must be a positive integer")
)

// Execute implements types.StateMachine.
func (stm *counter) Execute(verb types.StateMachineCommand, obj int64) error {
	switch verb {
	default:
		return types.ErrUnsupportedStateMachineCommand
	case types.AddCommand:
		return stm.add(obj)
	case types.SubtractCommand:
		return stm.subtract(obj)
	}
}

func (stm *counter) add(i int64) error {
	if i < 0 {
		return ErrInputMustBeAPositiveInteger
	} else if i == 0 { // noop
		return nil
	}

	newState := stm.state + i
	hasOverflown := newState < stm.state
	if stm.maxVal != nil && (hasOverflown || stm.state > *stm.maxVal) {
		newState = *stm.maxVal
	} else if hasOverflown && !stm.allowOverflow {
		return ErrNoOverflow
	}
	stm.state = newState
	return nil
}

func (stm *counter) subtract(i int64) error {
	if i < 0 {
		return ErrInputMustBeAPositiveInteger
	} else if i == 0 { // noop
		return nil
	}

	newState := stm.state - i
	hasUnderFlown := newState > stm.state
	if stm.minVal != nil && (hasUnderFlown || stm.state < *stm.minVal) {
		newState = *stm.minVal
	} else if hasUnderFlown && !stm.allowOverflow {
		return ErrNoUnderflow
	}
	stm.state = newState
	return nil
}

// State implements types.StateMachine.
func (stm *counter) State() int64 {
	return stm.state
}

// setState implements stateSetter.
func (stm *counter) setState(state int64) {
	stm.state = state
}
