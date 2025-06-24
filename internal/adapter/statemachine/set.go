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
	"github.com/alexandremahdhaoui/udplb/internal/types"

	"github.com/google/uuid"
)

var (
	_ types.StateMachine[uuid.UUID, map[uuid.UUID]struct{}] = &genericSet[uuid.UUID]{}
	_ stateSetter[map[struct {
		TestValue  int
		OnePointer *string
	}]struct{}] = &genericSet[struct {
		TestValue  int
		OnePointer *string
	}]{}
)

func NewGenericSet[T comparable](
	// transformFunc must not lock.
	opts ...option[T, map[T]struct{}],
) (types.StateMachine[T, map[T]struct{}], error) {
	out := &genericSet[T]{
		state: make(map[T]struct{}),
	}
	return execOptions(out, opts)
}

// TODO: Add support for capacity (as in "max capacity")
// TODO: Add support for concurrency
type genericSet[T comparable] struct {
	state map[T]struct{}
}

// Decode implements types.StateMachine.
func (stm *genericSet[T]) Decode(buf []byte) error {
	return decodeBinary(buf, stm.state)
}

// DeepCopy implements types.StateMachine.
func (stm *genericSet[T]) DeepCopy() types.StateMachine[T, map[T]struct{}] {
	return &genericSet[T]{
		state: stm.State(),
	}
}

// Encode implements types.StateMachine.
func (stm *genericSet[T]) Encode() ([]byte, error) {
	return encodeBinary(stm.state)
}

// Execute implements types.StateMachine.
func (stm *genericSet[T]) Execute(verb types.StateMachineCommand, obj T) error {
	switch verb {
	default:
		return types.ErrUnsupportedStateMachineCommand
	case types.PutCommand:
		stm.state[obj] = struct{}{}
		return nil
	case types.DeleteCommand:
		delete(stm.state, obj)
		return nil
	}
}

// State implements types.StateMachine.
func (stm *genericSet[T]) State() map[T]struct{} {
	return copyMap(stm.state)
}

// setState implements stateSetter.
func (stm *genericSet[T]) setState(state map[T]struct{}) {
	stm.state = state
}
