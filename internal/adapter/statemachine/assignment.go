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
	"maps"

	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/google/uuid"
)

// TO THINK ABOUT
// - Change: types.StateMachine[types.Assignment, map[uuid.UUID]uuid.UUID]
// - To something like: types.StateMachine[AssignmentWALEntry, map[uuid.UUID]uuid.UUID]
// Why?
// The idea is to have AssignmentWALEntry as a wrapper around types.Assignment and map[UUID]UUID
// in order to allow resetting the whole data structure when a WAL is reset and a snapshot is made.

// OR: do not complexify the thing.
// -> Each entry in the WAL has a hash and the snapshot can be made by any node independently.
// However? When a new node joins, how do we send it the snapshot? which node send the snapshot?
// Elect a leader?

var (
	_ types.StateMachine[types.Assignment, map[uuid.UUID]uuid.UUID] = &assignmentStateMachine{}
	_ stateSetter[map[uuid.UUID]uuid.UUID]                          = &assignmentStateMachine{}
)

func NewAssignment(
	options ...option[types.Assignment, map[uuid.UUID]uuid.UUID],
) types.StateMachine[types.Assignment, map[uuid.UUID]uuid.UUID] {
	out := &assignmentStateMachine{
		state: make(map[uuid.UUID]uuid.UUID),
	}
	return execOptions(out, options)
}

type assignmentStateMachine struct {
	state map[uuid.UUID]uuid.UUID
}

// Decode implements types.StateMachine.
func (stm *assignmentStateMachine) Decode(buf []byte) error {
	_, err := binary.Decode(buf, binary.LittleEndian, stm.state)
	return err
}

// DeepCopy implements types.StateMachine.
func (stn *assignmentStateMachine) DeepCopy() types.StateMachine[types.Assignment, map[uuid.UUID]uuid.UUID] {
	return &assignmentStateMachine{
		state: stn.State(),
	}
}

// Encode implements types.StateMachine.
func (stm *assignmentStateMachine) Encode() ([]byte, error) {
	out := make([]byte, 0)
	_, err := binary.Encode(out, binary.LittleEndian, stm.state)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Execute implements types.StateMachine.
// The subject of the verb is always the underlying state of the types.StateMachine.
func (stm *assignmentStateMachine) Execute(
	verb types.StateMachineCommand,
	obj types.Assignment,
) error {
	switch verb {
	default:
		return types.ErrUnsupportedStateMachineCommand
	case types.PutCommand:
		stm.executePut(obj)
	case types.DeleteCommand:
		stm.executeDelete(obj)
	}
	return nil
}

func (stm *assignmentStateMachine) executePut(obj types.Assignment) {
	stm.state[obj.SessionId] = obj.BackendId
}

func (stm *assignmentStateMachine) executeDelete(obj types.Assignment) {
	delete(stm.state, obj.SessionId)
}

// State returns a copy of the underlying state of the machine. E.g.:
// - For a generic set StateMachine, U is:     map[T]struct{}
// - For a generic array StateMachine, U is:   []T
// - For a generic counter StateMachine, U is: map[T]int
// - For the AssignmentStateMachine, U is:     map[uuid.UUID]uuid.UUID
func (stm *assignmentStateMachine) State() map[uuid.UUID]uuid.UUID {
	out := make(map[uuid.UUID]uuid.UUID, len(stm.state))
	maps.Copy(out, stm.state)
	return out
}

// setState implements stateSetter.
func (stm *assignmentStateMachine) setState(state map[uuid.UUID]uuid.UUID) {
	stm.state = state
}
