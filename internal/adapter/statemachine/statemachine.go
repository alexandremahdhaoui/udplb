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

func NewAssignment() types.StateMachine[types.Assignment, map[uuid.UUID]uuid.UUID] {
	return &AssignmentStateMachine{}
}

type AssignmentStateMachine struct{}

// Encode implements types.StateMachine.
func (a *AssignmentStateMachine) Encode() ([]byte, error) {
	panic("unimplemented")
}

// Decode implements types.StateMachine.
func (a *AssignmentStateMachine) Decode(buf []byte) error {
	panic("unimplemented")
}

// Execute implements types.StateMachine.
func (a *AssignmentStateMachine) Execute(verb string, data types.Assignment) {
	panic("unimplemented")
}

// State implements types.StateMachine.
func (a *AssignmentStateMachine) State() map[uuid.UUID]uuid.UUID {
	panic("unimplemented")
}

// DeepCopy implements types.StateMachine.
func (a *AssignmentStateMachine) DeepCopy() types.StateMachine[types.Assignment, map[uuid.UUID]uuid.UUID] {
	panic("unimplemented")
}
