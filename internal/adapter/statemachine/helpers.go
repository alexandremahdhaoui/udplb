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
)

type (
	// stateSetter is a helper interface to support setting the internal state
	// of an abstract state machine.
	stateSetter[U any] interface{ setState(U) }
	option[T, U any]   func(stateSetter[U])
)

func WithInitialState[T, U any](state U) option[T, U] {
	return func(m stateSetter[U]) {
		m.setState(state)
	}
}

// execOptions check if any options are specified, then
// type assert the input stm for internal interfaces, and
// executes each options onto the input stm.
// It finally returns the stm pointer for convenience.
func execOptions[T, U any](
	stm types.StateMachine[T, U],
	options []option[T, U],
) types.StateMachine[T, U] {
	if len(options) < 1 {
		return stm
	}
	sts, ok := stm.(stateSetter[U])
	if !ok {
		panic("cannot execute option onto unexpected state machine")
	}
	for _, o := range options {
		o(sts)
	}
	return stm
}

// DeepCopy returns a deep copy of the state machine.
// Useful to fork the state of a data structure.
func DeepCopy[T, U any](old types.StateMachine[T, U]) types.StateMachine[T, U] {
	panic("unimplemented")
}
