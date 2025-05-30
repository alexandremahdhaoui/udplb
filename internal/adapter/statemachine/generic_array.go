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
	state  []T
	filter FilterFunc[T]
}

// Decode implements types.StateMachine.
func (m *arrayStateMachine[T, U]) Decode(buf []byte) error {
	_, err := binary.Decode(buf, binary.LittleEndian, m.state)
	return err
}

// DeepCopy implements types.StateMachine.
func (m *arrayStateMachine[T, U]) DeepCopy() types.StateMachine[T, U] {
	return &arrayStateMachine[T, U]{
		state:  m.State(),
		filter: m.filter,
	}
}

// Encode implements types.StateMachine.
func (m *arrayStateMachine[T, U]) Encode() ([]byte, error) {
	out := make([]byte, 0)
	_, err := binary.Encode(out, binary.LittleEndian, m.state)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Execute implements types.StateMachine.
// The subject of the verb is always the underlying state of the types.StateMachine.
func (m *arrayStateMachine[T, U]) Execute(
	verb types.StateMachineCommand,
	obj T,
) error {
	switch verb {
	default:
		return types.ErrUnsupportedStateMachineCommand
	case types.AppendCommand:
		m.executeAppend(obj)
	case types.PutCommand:
		return m.executePut(obj)
	case types.DeleteCommand:
		return m.executeDelete(obj)
	}
	return nil
}

func (m *arrayStateMachine[T, U]) executeAppend(obj T) {
	m.state = append(m.state, obj)
}

var ErrOperationIsNotSupported = errors.New("operation is not supported if FilterFunc is nil")

func (m *arrayStateMachine[T, U]) executePut(obj T) error {
	if m.filter == nil {
		return ErrOperationIsNotSupported
	}
	for i, o := range m.state {
		if m.filter(obj, o) {
			m.state[i] = obj
		}
	}
	return nil
}

func (m *arrayStateMachine[T, U]) executeDelete(obj T) error {
	if m.filter == nil {
		return ErrOperationIsNotSupported
	}
	for i, o := range m.state {
		if m.filter(obj, o) {
			out := make(U, 0)
			if i != 0 {
				out = m.state[0:i]
			}
			if i != len(m.state)-1 {
				out = append(out, m.state[i:len(m.state)-1]...)
			}
			m.state = out
		}
	}
	return nil
}

// State implements types.StateMachine.
func (m *arrayStateMachine[T, U]) State() U {
	out := make(U, len(m.state))
	copy(out, m.state)
	return out
}
