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
	_ types.StateMachine[int, int] = &counterStateMachine{}
	_ stateSetter[int]             = &counterStateMachine{}
)

func NewGenericCounter(
	opts ...option[int, int],
) (types.StateMachine[int, int], error) {
	out := &counterStateMachine{}
	return execOptions(out, opts)
}

// TODO: Add support for capacity (as in "max capacity")
// TODO: Add support for concurrency
type counterStateMachine struct {
	state int
}

// Decode implements types.StateMachine.
func (stm *counterStateMachine) Decode(buf []byte) error {
	return decodeBinary(buf, stm.state)
}

// DeepCopy implements types.StateMachine.
func (stm *counterStateMachine) DeepCopy() types.StateMachine[int, int] {
	return &counterStateMachine{
		state: stm.State(),
	}
}

// Encode implements types.StateMachine.
func (stm *counterStateMachine) Encode() ([]byte, error) {
	return encodeBinary(stm.state)
}

// Execute implements types.StateMachine.
func (stm *counterStateMachine) Execute(verb types.StateMachineCommand, obj int) error {
	switch verb {
	default:
		return types.ErrUnsupportedStateMachineCommand
	case types.AddCommand:
		stm.state += obj
		return nil
	}
}

// State implements types.StateMachine.
func (stm *counterStateMachine) State() int {
	return stm.state
}

// setState implements stateSetter.
func (stm *counterStateMachine) setState(state int) {
	stm.state = state
}
