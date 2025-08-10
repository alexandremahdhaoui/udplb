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
	_ types.StateMachine[uuid.UUID, map[uuid.UUID]struct{}] = &set[uuid.UUID]{}
	_ stateSetter[
		uuid.UUID,
		map[struct {
			TestValue  int
			OnePointer *string
		}]struct{},
	] = &set[struct {
		TestValue  int
		OnePointer *string
	}]{}
)

func NewSet[T comparable](
	// transformFunc must not lock.
	opts ...Option[T, map[T]struct{}],
) (types.StateMachine[T, map[T]struct{}], error) {
	out := &set[T]{
		state: make(map[T]struct{}),
	}
	return execOptions(out, opts)
}

// TODO: Add support for capacity (as in "max capacity")
// TODO: Add support for concurrency
type set[T comparable] struct {
	state map[T]struct{}
}

// Decode implements types.StateMachine.
func (stm *set[T]) Decode(buf []byte) error {
	return decodeDefault(buf, &stm.state)
}

// DeepCopy implements types.StateMachine.
func (stm *set[T]) DeepCopy() types.StateMachine[T, map[T]struct{}] {
	return &set[T]{
		state: stm.State(),
	}
}

// Encode implements types.StateMachine.
func (stm *set[T]) Encode() ([]byte, error) {
	return encodeDefault(stm.state)
}

// Execute implements types.StateMachine.
func (stm *set[T]) Execute(verb types.StateMachineCommand, obj T) error {
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
func (stm *set[T]) State() map[T]struct{} {
	return copyMap(stm.state)
}

// setState implements stateSetter.
func (stm *set[T]) setState(state map[T]struct{}) {
	stm.state = state
}
