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
	"encoding/binary"
	"errors"

	"github.com/alexandremahdhaoui/udplb/internal/types"
)

var (
	_ types.StateMachine[any, []any] = &arrayStateMachine[any, []any]{}
	_ stateSetter[[]any]             = &arrayStateMachine[any, []any]{}
)

type FilterFunc[T any] func(obj T, entry T) bool

// TODO: Add support for capacity and max length.
// TODO: How to manage a full write-ahead log?
func NewArray[T any, U []T](
	filter FilterFunc[T],
) types.StateMachine[T, U] {
	return &arrayStateMachine[T, U]{
		state:  make(U, 0),
		filter: filter,
	}
}

type arrayStateMachine[T any, U []T] struct {
	state []T
	// Filter allows updating or deleting an entry by key.
	// Put and Delete operations are therefore O(n).
	filter FilterFunc[T]
}

// Decode implements types.StateMachine.
func (stm *arrayStateMachine[T, U]) Decode(buf []byte) error {
	_, err := binary.Decode(buf, binary.LittleEndian, stm.state)
	return err
}

// DeepCopy implements types.StateMachine.
func (stm *arrayStateMachine[T, U]) DeepCopy() types.StateMachine[T, U] {
	return &arrayStateMachine[T, U]{
		state:  stm.State(),
		filter: stm.filter,
	}
}

// Encode implements types.StateMachine.
func (stm *arrayStateMachine[T, U]) Encode() ([]byte, error) {
	out := make([]byte, 0)
	_, err := binary.Encode(out, binary.LittleEndian, stm.state)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Execute implements types.StateMachine.
// The subject of the verb is always the underlying state of the types.StateMachine.
func (stm *arrayStateMachine[T, U]) Execute(
	verb types.StateMachineCommand,
	obj T,
) error {
	switch verb {
	default:
		return types.ErrUnsupportedStateMachineCommand
	case types.AppendCommand:
		stm.executeAppend(obj)
	case types.PutCommand:
		return stm.executePut(obj)
	case types.DeleteCommand:
		return stm.executeDelete(obj)
	}
	return nil
}

func (stm *arrayStateMachine[T, U]) executeAppend(obj T) {
	stm.state = append(stm.state, obj)
}

var ErrOperationRequiresAFilterFunc = errors.New("operation requires a filter func")

func (stm *arrayStateMachine[T, U]) executePut(obj T) error {
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

func (stm *arrayStateMachine[T, U]) executeDelete(obj T) error {
	if stm.filter == nil {
		return ErrOperationRequiresAFilterFunc
	}
	for i, o := range stm.state {
		if stm.filter(obj, o) {
			out := make(U, 0)
			if i != 0 {
				out = stm.state[0:i]
			}
			if i != len(stm.state)-1 {
				out = append(out, stm.state[i:len(stm.state)-1]...)
			}
			stm.state = out
		}
	}
	return nil
}

// State implements types.StateMachine.
func (stm *arrayStateMachine[T, U]) State() U {
	out := make(U, len(stm.state))
	copy(out, stm.state)
	return out
}

// setState implements stateSetter.
func (stm *arrayStateMachine[T, U]) setState(state U) {
	stm.state = state
}
