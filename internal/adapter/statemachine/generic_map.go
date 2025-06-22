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
	_ types.StateMachine[types.Assignment, map[uuid.UUID]any] = &genericMap[types.Assignment, uuid.UUID, any]{}
	_ stateSetter[map[uuid.UUID]any]                          = &genericMap[types.Assignment, uuid.UUID, any]{}
)

type TransformFunc[E any, K comparable, V any] func(obj any) (K, V, error)

type genericMap[E any, K comparable, V any] struct {
	state         map[K]V
	transformFunc TransformFunc[E, K, V]
}

// Decode implements types.StateMachine.
func (stm *genericMap[E, K, V]) Decode(buf []byte) error {
	panic("unimplemented")
}

// DeepCopy implements types.StateMachine.
func (stm *genericMap[E, K, V]) DeepCopy() types.StateMachine[E, map[uuid.UUID]any] {
	panic("unimplemented")
}

// Encode implements types.StateMachine.
func (stm *genericMap[E, K, V]) Encode() ([]byte, error) {
	panic("unimplemented")
}

// Execute implements types.StateMachine.
func (stm *genericMap[E, K, V]) Execute(verb types.StateMachineCommand, obj E) error {
	k, v, err := stm.transformFunc(obj)
	if err != nil {
		return err
	}

	// TODO: define operations onto k, v & state
	panic("unimplemented")
}

// State implements types.StateMachine.
func (stm *genericMap[E, K, V]) State() map[K]V {
	panic("unimplemented")
}

// setState implements stateSetter.
func (stm *genericMap[E, K, V]) setState(state map[K]V) {
	stm.state = state
}
