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
	"errors"
	"maps"

	"github.com/alexandremahdhaoui/udplb/internal/types"
)

type (
	// stateSetter is a helper interface to support setting the internal state
	// of an abstract state machine.
	stateSetter[U any] interface{ setState(U) }
	option[T, U any]   func(stateSetter[U]) error
)

func WithInitialState[T, U any](state U) option[T, U] {
	return func(m stateSetter[U]) error {
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
	options []option[T, U],
) (types.StateMachine[T, U], error) {
	if len(options) < 1 {
		return stm, nil
	}
	sts, ok := stm.(stateSetter[U])
	if !ok {
		return nil, ErrCannotExecuteOptionOntoUnexpectedStateMachine
	}
	for _, o := range options {
		o(sts)
	}
	return stm, nil
}

func encodeBinary[T any](data T) ([]byte, error) {
	out := make([]byte, 0)
	_, err := binary.Encode(out, binary.LittleEndian, data)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func decodeBinary[T any](buf []byte, data T) error {
	_, err := binary.Decode(buf, binary.LittleEndian, data)
	return err
}

func copyMap[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	maps.Copy(out, m)
	return out
}
