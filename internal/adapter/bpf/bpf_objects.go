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

// -------------------------------------------------------------------
// -- OBJECTS
// -------------------------------------------------------------------

// Wraps the following ebpf objects with a convenient interface for testing.
type Objects struct {
	Backends    BPFMap[uint32, *udplbBackendSpec]
	LookupTable BPFArray[uint32]
}

func NewObjects(prog UDPLB) (Objects, error) {
	concrete, ok := prog.(*udplb)
	if !ok {
		return Objects{}, ErrCannotCreateObjectsFromUnknownUDPLBImplementation
	}
	objs := concrete.objs

	backends, err := NewBPFMap[uint32, *udplbBackendSpec](
		objs.BackendsA, objs.BackendsB,
		objs.BackendsA_len, objs.BackendsB_len,
		objs.ActivePointer,
	)
	if err != nil {
		return Objects{}, err
	}

	lup, err := NewBPFArray[uint32](
		objs.LookupTableA, objs.LookupTableB,
		objs.LookupTableA_len, objs.LookupTableB_len,
		objs.ActivePointer,
	)
	if err != nil {
		return Objects{}, err
	}

	return Objects{
		Backends:    backends,
		LookupTable: lup,
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
	}
}

// -------------------------------------------------------------------
// -- FAKE BPF ARRAY
// -------------------------------------------------------------------

type 

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
