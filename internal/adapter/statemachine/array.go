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

	"github.com/alexandremahdhaoui/udplb/internal/types"
)

var (
	_ types.StateMachine[any, []any] = &array[any]{}
	_ stateSetter[any, []any]        = &array[any]{}
)

// TODO: How to manage a full write-ahead log?
// TODO: Add support for capacity (as in "max capacity")
// TODO: Add support for concurrency
func NewArray[T any](
	filter types.FilterFunc[T],
	opts ...Option[T, []T],
) (types.StateMachine[T, []T], error) {
	out := &array[T]{
		state:  make([]T, 0),
		filter: filter,
	}
	return execOptions(out, opts)
}

type array[T any] struct {
	state []T
	// Filter allows updating or deleting an entry by key.
	// Put and Delete operations are therefore O(n).
	filter types.FilterFunc[T]
}

/*******************************************************************************
 * Decode
 *
 ******************************************************************************/

// Decode implements types.StateMachine.
func (stm *array[T]) Decode(buf []byte) error {
	return decodeBinary(buf, stm.state)
}

/*******************************************************************************
 * DeepCopy
 *
 ******************************************************************************/

// DeepCopy implements types.StateMachine.
func (stm *array[T]) DeepCopy() types.StateMachine[T, []T] {
	return &array[T]{
		state:  stm.State(),
		filter: stm.filter,
	}
}

/*******************************************************************************
 * Encode
 *
 ******************************************************************************/

// Encode implements types.StateMachine.
func (stm *array[T]) Encode() ([]byte, error) {
	return encodeBinary(stm.state)
}

/*******************************************************************************
 * Execute
 *
 ******************************************************************************/

// Execute implements types.StateMachine.
// The subject of the verb is always the underlying state of the types.StateMachine.
func (stm *array[T]) Execute(
	verb types.StateMachineCommand,
	obj T,
) error {
	switch verb {
	default:
		return types.ErrUnsupportedStateMachineCommand
	case types.AppendCommand:
		stm.executeAppend(obj)
		return nil
	case types.PutCommand:
		return stm.executePut(obj)
	case types.DeleteCommand:
		return stm.executeDelete(obj)
	}
}

func (stm *array[T]) executeAppend(obj T) {
	stm.state = append(stm.state, obj)
}

var ErrOperationRequiresAFilterFunc = errors.New("operation requires a filter func")

func (stm *array[T]) executePut(obj T) error {
	if stm.filter == nil {
		return ErrOperationRequiresAFilterFunc
	}
	for i, o := range stm.state {
		if stm.filter(obj, o) {
			stm.state[i] = obj
		}
	}
	return nil
}

func (stm *array[T]) executeDelete(obj T) error {
	if stm.filter == nil {
		return ErrOperationRequiresAFilterFunc
	}
	for i, o := range stm.state {
		if !stm.filter(obj, o) {
			continue
		}
		out := make([]T, 0)
		if i > 0 {
			out = stm.state[0:i]
		}
		if i < len(stm.state)-1 {
			out = append(out, stm.state[i+1:]...)
		}
		stm.state = out
		return nil
	}
	return types.ErrNotFound
}

/*******************************************************************************
 * State
 *
 ******************************************************************************/

// State implements types.StateMachine.
func (stm *array[T]) State() []T {
	out := make([]T, len(stm.state))
	copy(out, stm.state)
	return out
}

// setState implements stateSetter.
func (stm *array[T]) setState(state []T) {
	stm.state = state
}
