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
	"encoding/json"
	"errors"
	"maps"

	"github.com/alexandremahdhaoui/udplb/internal/types"
)

type (
	// stateSetter is a helper interface to support setting the internal state
	// of an abstract state machine.
	stateSetter[T, U any] interface{ setState(U) }
	Option[T, U any]      func(stateSetter[T, U]) error
)

func WithInitialState[T, U any](state U) Option[T, U] {
	return func(m stateSetter[T, U]) error {
		m.setState(state)
		return nil
	}
}

var ErrCannotExecuteOptionOntoUnexpectedStateMachine = errors.New(
	"cannot execute option onto unexpected state machine",
)

// execOptions check if any options are specified, then
// type assert the input stm for internal interfaces, and
// executes each options onto the input stm.
// It finally returns the stm pointer for convenience.
func execOptions[T, U any](
	stm types.StateMachine[T, U],
	options []Option[T, U],
) (types.StateMachine[T, U], error) {
	if len(options) < 1 {
		return stm, nil
	}
	sts, ok := stm.(stateSetter[T, U])
	if !ok {
		return nil, ErrCannotExecuteOptionOntoUnexpectedStateMachine
	}
	for _, o := range options {
		if err := o(sts); err != nil {
			return nil, err
		}
	}
	return stm, nil
}

func encodeDefault[T any](v T) ([]byte, error) {
	return json.Marshal(v)
}

func decodeDefault[T any](data []byte, v T) error {
	return json.Unmarshal(data, v)
}

func copyMap[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	maps.Copy(out, m)
	return out
}
