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
	"errors"

	"github.com/alexandremahdhaoui/udplb/internal/types"

	"github.com/google/uuid"
)

var (
	_ types.StateMachine[types.Assignment, map[uuid.UUID]uuid.UUID] = &genericMap[types.Assignment, uuid.UUID, uuid.UUID]{}
	_ stateSetter[map[uuid.UUID]struct{}]                           = &genericMap[types.Assignment, uuid.UUID, struct{}]{}
)

type TransformFunc[E any, K comparable, V any] func(obj E) (K, V, error)

var ErrTransformFuncMustNotBeNil = errors.New("TransformFunc[E, K, V] must not be nil")

// NewGenericMap can create a new state machine responsible for maintaining a map.
//
// Example: Create an Assignment map:
//
//	```go
//		_ types.StateMachine[types.Assignment, map[uuid.UUID]uuid.UUID] = util.IgnoreErr(NewGenericMap(
//			func(obj types.Assignment) (uuid.UUID, uuid.UUID, error) {
//				return obj.SessionId, obj.BackendId, nil
//			}))
//	```
func NewGenericMap[E any, K comparable, V any](
	// transformFunc must not lock.
	transformFunc TransformFunc[E, K, V],
	opts ...option[E, map[K]V],
) (types.StateMachine[E, map[K]V], error) {
	if transformFunc == nil {
		return nil, ErrTransformFuncMustNotBeNil
	}
	out := &genericMap[E, K, V]{
		state:         make(map[K]V),
		transformFunc: transformFunc,
	}
	return execOptions(out, opts)
}

// TODO: Add support for capacity (as in "max capacity")
// TODO: Add support for concurrency
type genericMap[E any, K comparable, V any] struct {
	state         map[K]V
	transformFunc TransformFunc[E, K, V]
}

// Decode implements types.StateMachine.
func (stm *genericMap[E, K, V]) Decode(buf []byte) error {
	return decodeBinary(buf, stm.state)
}

// DeepCopy implements types.StateMachine.
func (stm *genericMap[E, K, V]) DeepCopy() types.StateMachine[E, map[K]V] {
	return &genericMap[E, K, V]{
		state:         stm.State(),
		transformFunc: stm.transformFunc,
	}
}

// Encode implements types.StateMachine.
func (stm *genericMap[E, K, V]) Encode() ([]byte, error) {
	return encodeBinary(stm.state)
}

// Execute implements types.StateMachine.
func (stm *genericMap[E, K, V]) Execute(verb types.StateMachineCommand, obj E) error {
	k, v, err := stm.transformFunc(obj)
	if err != nil {
		return err
	}

	switch verb {
	default:
		return types.ErrUnsupportedStateMachineCommand
	case types.PutCommand:
		stm.state[k] = v
		return nil
	case types.DeleteCommand:
		delete(stm.state, k)
		return nil
	}
}

// State implements types.StateMachine.
func (stm *genericMap[E, K, V]) State() map[K]V {
	return copyMap(stm.state)
}

// setState implements stateSetter.
func (stm *genericMap[E, K, V]) setState(state map[K]V) {
	stm.state = state
}
