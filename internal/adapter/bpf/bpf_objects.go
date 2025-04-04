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

import "github.com/google/uuid"

// -------------------------------------------------------------------
// -- OBJECTS
// -------------------------------------------------------------------

// Wraps the following ebpf objects with a convenient interface for testing.
type Objects struct {
	// Backends is a BPFArray of available udplbBackendSpec ordered by id of size n'.
	// NB:
	// - n is defined as the number of available backends.
	// - n' is defined as the number of backends, e.g. in StateAvailable, StateUnschedulable...
	Backends BPFArray[*udplbBackendSpec]

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
	LookupTable BPFArray[uint32]

	// Sessions maps a session id to a specific backend, if the backend transitions from
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
	Sessions BPFMap[uuid.UUID, uint32]
}

func NewObjects(prog UDPLB) (Objects, error) {
	concrete, ok := prog.(*udplb)
	if !ok {
		return Objects{}, ErrCannotCreateObjectsFromUnknownUDPLBImplementation
	}
	objs := concrete.objs

	backends, err := NewBPFArray[*udplbBackendSpec](
		objs.BackendsA,
		objs.BackendsB,
		objs.BackendsA_len,
		objs.BackendsB_len,
		objs.ActivePointer,
	)
	if err != nil {
		return Objects{}, err
	}

	lookupTable, err := NewBPFArray[uint32](
		objs.LookupTableA,
		objs.LookupTableB,
		objs.LookupTableA_len,
		objs.LookupTableB_len,
		objs.ActivePointer,
	)
	if err != nil {
		return Objects{}, err
	}

	sessions, err := NewBPFMap[uuid.UUID, uint32](
		objs.SessionsA,
		objs.SessionsB,
		objs.SessionsA_len,
		objs.SessionsB_len,
		objs.ActivePointer,
	)

	return Objects{
		Backends:    backends,
		LookupTable: lookupTable,
		Sessions:    sessions,
	}, nil
}

// -------------------------------------------------------------------
// -- FAKE OBJECTS
// -------------------------------------------------------------------

var (
	_ BPFArray[any]       = &FakeBPFArray[any]{}
	_ BPFMap[uint32, any] = &FakeBPFMap[uint32, any]{}
	_ BPFVariable[any]    = &FakeBPFVariable[any]{}
)

func NewFakeObjects() Objects {
	return Objects{
		Backends:    NewFakeBackends(),
		LookupTable: NewFakeLookupTable(),
		Sessions:    NewFakeSessions,
	}
}

// -------------------------------------------------------------------
// -- FAKE BPF ARRAY
// -------------------------------------------------------------------

// TODO: either create the fake bpf array or just use mocks.

// -------------------------------------------------------------------
// -- FAKE BPF MAP
// -------------------------------------------------------------------

type FakeBPFMap[K comparable, V any] struct {
	Map map[K]V
}

// Set implements BPFMap.
func (m *FakeBPFMap[K, V]) Set(kv map[K]V) error {
	m.Map = kv
	return nil
}

// SetAndDeferSwitchover implements BPFMap.
func (m *FakeBPFMap[K, V]) SetAndDeferSwitchover(kv map[K]V) (func(), error) {
	return func() {
		m.Map = kv
	}, nil
}

func NewFakeBackends() *FakeBPFMap[uint32, *udplbBackendSpec] {
	return &FakeBPFMap[uint32, *udplbBackendSpec]{
		Map: make(map[uint32]*udplbBackendSpec),
	}
}

// -------------------------------------------------------------------
// -- FAKE BPF VARIABLE
// -------------------------------------------------------------------

type FakeBPFVariable[T any] struct {
	v T
}

// Set implements BPFVariable.
func (v *FakeBPFVariable[T]) Set(newVar T) error {
	v.v = newVar
	return nil
}

func NewFakeVariable[T any](v T) *FakeBPFVariable[T] {
	return &FakeBPFVariable[T]{v}
}
