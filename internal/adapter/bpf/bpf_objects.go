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
package bpfadapter

import (
	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/google/uuid"
)

// -------------------------------------------------------------------
// -- OBJECTS
// -------------------------------------------------------------------

var _ types.DoneCloser = Objects{}

// Wraps the following ebpf objects with a convenient interface for testing.
//
// Objects implements DoneCloser.
//   - Close will propagate to all internal data structures. It will also close the
//     bpf data structure.
//   - The expression "<- Done()" returns when all internal data structures are done.
//     done.
type Objects struct {
	// BackendList is a BPFArray of available udplbBackendSpecT ordered by id of size n'.
	// NB:
	// - n is defined as the number of available backends.
	// - n' is defined as the number of backends, e.g. in StateAvailable, StateUnschedulable...
	BackendList Array[*udplbBackendSpecT]

	// LookupTable is a BPFArray of uint32 and of size m.
	// The integers stored in this list represents the index of an available backend in the
	// backends slice.
	//
	// The BPF program will:
	// - Check if session id is mapped (see "sessions"). If not, then continue.
	// - Compute a hash of a packet's session id modulo the lookup table's size.
	// - With that computed key get the associated value in the lookup table.
	// - The above operation returned the index of an available backend in the
	//   backends array.
	// - Use that index to get the spec of the associated backend in the backends array.
	LookupTable Array[uint32]

	// AssignmentFIFO is a bpf.FIFO of udplbAssignmentT.
	// It is used to notify userland about newly discovered session-backend assignment.
	// The userland program MUST notify other loadbalancer instances about this newly
	// discovered assignment.
	// The userland program MUST update its internal session map in order to avoid
	// overwritting these newly discovered assignment when setting and switching over
	// the SessionMap bpf data structure.
	AssignmentFIFO FIFO[udplbAssignmentT]

	// SessionMap maps a session id to a specific backend, if the backend transitions from
	// StateAvailable to StateUnschedulable, existing packet destinated to an existing session
	// will continue to hit this backend.
	//
	// We can use a BPF_MAP_TYPE_HASH to store the __u128 uuid.
	// Provisioning a large BPF_MAP_TYPE_LRU_HASH could be interesting in order to avoid
	// manually clean up old session ids from the map at a cost.
	//
	// The BPF program will:
	// - Check if session id is in the sessions map. If it does, then continue.
	// - The above operation returned the index of a backend in the backends array.
	// - Use that index to get the spec of the associated backend in the backends array.
	SessionMap Map[uuid.UUID, uint32]
}

func NewObjects(prog UDPLB) (Objects, error) {
	concrete, ok := prog.(*udplb)
	if !ok {
		return Objects{}, ErrCannotCreateObjectsFromUnknownUDPLBImplementation
	}
	objs := concrete.objs

	backendList, err := NewArray[*udplbBackendSpecT](
		objs.BackendListA,
		objs.BackendListB,
		objs.BackendListA_len,
		objs.BackendListB_len,
		objs.ActivePointer,
	)
	if err != nil {
		return Objects{}, err
	}

	lookupTable, err := NewArray[uint32](
		objs.LookupTableA,
		objs.LookupTableB,
		objs.LookupTableA_len,
		objs.LookupTableB_len,
		objs.ActivePointer,
	)
	if err != nil {
		return Objects{}, err
	}

	sessionMap, err := NewMap[uuid.UUID, uint32](
		objs.SessionMapA,
		objs.SessionMapB,
		objs.SessionMapA_len,
		objs.SessionMapB_len,
		objs.ActivePointer,
	)
	if err != nil {
		return Objects{}, err
	}

	assignmentFIFO, err := NewFIFO[udplbAssignmentT](objs.AssignmentRingbuf)
	if err != nil {
		return Objects{}, err
	}

	return Objects{
		BackendList:    backendList,
		LookupTable:    lookupTable,
		AssignmentFIFO: assignmentFIFO,
		SessionMap:     sessionMap,
	}, nil
}

// Close implements types.DoneCloser.
func (o Objects) Close() error {
	panic("unimplemented")
}

// Done implements types.DoneCloser.
func (o Objects) Done() <-chan struct{} {
	panic("unimplemented")
}

// -------------------------------------------------------------------
// -- FAKE OBJECTS
// -------------------------------------------------------------------

var (
	_ Array[any]       = &FakeArray[any]{}
	_ FIFO[any]        = &FakeFIFO[any]{}
	_ Map[uint32, any] = &FakeMap[uint32, any]{}
	// _ Variable[any]    = &FakeVariable[any]{}
)

// TODO: fix fake objects

func NewFakeObjects() Objects {
	return Objects{
		BackendList:    NewFakeArray[*udplbBackendSpecT](),
		LookupTable:    NewFakeArray[uint32](),
		AssignmentFIFO: NewFakeFIFO[udplbAssignmentT](),
		SessionMap:     NewFakeMap[uuid.UUID, uint32](),
	}
}

// -------------------------------------------------------------------
// -- FAKE BPF ARRAY
// -------------------------------------------------------------------

type FakeArray[T any] struct {
	Array []T
}

// Set implements Array.
func (f *FakeArray[T]) Set(values []T) error {
	panic("unimplemented")
}

// SetAndDeferSwitchover implements Array.
func (f *FakeArray[T]) SetAndDeferSwitchover(values []T) (func(), error) {
	panic("unimplemented")
}

func NewFakeArray[T any]() *FakeArray[T] {
	return &FakeArray[T]{
		Array: make([]T, 0),
	}
}

// -------------------------------------------------------------------
// -- FAKE MAP
// -------------------------------------------------------------------

type expector struct {
	expectationList []FakeExpectation
}

func (e *expector) AppendExpectation(expectation FakeExpectation) *expector {
	e.expectations = append(e.expectations, expectation)
	return e
}

func checkExpectationAndIncrement(expectation FakeExpectation) {
}

type FakeExpectation struct {
	Method string
	Err    error
}

type FakeMap[K comparable, V any] struct {
	Map          map[K]V
	Expectations []FakeExpectation
	expector
}

// BatchDelete implements Map.
func (f *FakeMap[K, V]) BatchDelete(keys []K) error {
	if len(f.Expectations) == 0 {
		panic("did not expect call to FakeMap.BatchDelete")
	}
	if f.Expectations[0].Method != "BatchDelete" {
		panic("did not expect call to FakeMap.BatchDelete")
	}
	if err := f.Expectations[0].Err; err != nil {
		return err
	}
	for _, k := range keys {
		delete(f.Map, k)
	}
	return nil
}

// BatchUpdate implements Map.
func (f *FakeMap[K, V]) BatchUpdate(kv map[K]V) error {
	panic("unimplemented")
}

// Set implements Map.
func (f *FakeMap[K, V]) Set(newMap map[K]V) error {
	panic("unimplemented")
}

// SetAndDeferSwitchover implements Map.
func (f *FakeMap[K, V]) SetAndDeferSwitchover(newMap map[K]V) (func(), error) {
	panic("unimplemented")
}

func NewFakeMap[K comparable, V any]() *FakeMap[K, V] {
	return &FakeMap[K, V]{
		Map: make(map[K]V),
	}
}

// -------------------------------------------------------------------
// -- FAKE FIFO
// -------------------------------------------------------------------

type FakeFIFO[T any] struct {
	Chan chan T
}

// Subscribe implements FIFO.
func (f *FakeFIFO[T]) Subscribe() (<-chan T, error) {
	panic("unimplemented")
}

func NewFakeFIFO[T any]() *FakeFIFO[T] {
	return &FakeFIFO[T]{
		Chan: make(chan T),
	}
}
