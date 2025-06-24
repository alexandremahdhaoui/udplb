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
	_ types.StateMachine[int, int] = &counter{}
	_ stateSetter[int]             = &counter{}
)

func NewCounter(
	opts ...option[int, int],
) (types.StateMachine[int, int], error) {
	out := &counter{}
	return execOptions(out, opts)
}

// TODO: Add support for capacity (as in "max capacity")
// TODO: Add support for concurrency
type counter struct {
	state int
}

// Decode implements types.StateMachine.
func (stm *counter) Decode(buf []byte) error {
	return decodeBinary(buf, stm.state)
}

// DeepCopy implements types.StateMachine.
func (stm *counter) DeepCopy() types.StateMachine[int, int] {
	return &counter{
		state: stm.State(),
	}
}

// Encode implements types.StateMachine.
func (stm *counter) Encode() ([]byte, error) {
	return encodeBinary(stm.state)
}

// Execute implements types.StateMachine.
func (stm *counter) Execute(verb types.StateMachineCommand, obj int) error {
	switch verb {
	default:
		return types.ErrUnsupportedStateMachineCommand
	case types.AddCommand:
		stm.state += obj
		return nil
	}
}

// State implements types.StateMachine.
func (stm *counter) State() int {
	return stm.state
}

// setState implements stateSetter.
func (stm *counter) setState(state int) {
	stm.state = state
}
