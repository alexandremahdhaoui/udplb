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

import "github.com/alexandremahdhaoui/udplb/internal/types"

var (
	_ types.StateMachine[int, int] = &counterStateMachine[int, int]{}
	_ stateSetter[int]             = &counterStateMachine[int, int]{}
)

func NewGenericCounter[T, U int]() types.StateMachine[T, U] {
	return &counterStateMachine[T, U]{}
}

type counterStateMachine[T, U int] struct {
	state U
}

// Decode implements types.StateMachine.
func (stm *counterStateMachine[T, U]) Decode(buf []byte) error {
	panic("unimplemented")
}

// DeepCopy implements types.StateMachine.
func (stm *counterStateMachine[T, U]) DeepCopy() types.StateMachine[T, U] {
	panic("unimplemented")
}

// Encode implements types.StateMachine.
func (stm *counterStateMachine[T, U]) Encode() ([]byte, error) {
	panic("unimplemented")
}

// Execute implements types.StateMachine.
func (stm *counterStateMachine[T, U]) Execute(verb types.StateMachineCommand, obj T) error {
	panic("unimplemented")
}

// State implements types.StateMachine.
func (stm *counterStateMachine[T, U]) State() U {
	panic("unimplemented")
}

// setState implements stateSetter.
func (stm *counterStateMachine[T, U]) setState(state U) {
	stm.state = state
}
