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
)

// -------------------------------------------------------------------
// -- OBJECTS
// -------------------------------------------------------------------

// Wraps the following ebpf objects with a convenient interface for testing.
type Objects struct {
	Backends    BPFMap[uint32, udplbBackendSpec]
	Config      BPFVariable[udplbConfigT]
	LookupTable BPFArray[uint32]
}

func NewObjects(prog UDPLB) (Objects, error) {
	concrete, ok := prog.(*udplb)
	if !ok {
		return Objects{}, ErrCannotCreateObjectsFromUnknownUDPLBImplementation
	}

	// concrete.objs.udplbMaps.Backends
	backends, err := NewBPFMap[uint32, udplbBackendSpec](
		a, b,
		aLen, bLen,
		activePointer,
	)
	if err != nil {
		return Objects{}, err
	}

	cfg, err := NewBPFVariable[udplbConfigT](concrete.objs.udplbVariables.Config)
	if err != nil {
		return Objects{}, err
	}

	lup, err := NewBPFArray[uint32](
		a, b,
		aLen, bLen,
		activePointer,
	)

	return Objects{
		Backends:    backends,
		Config:      cfg,
		LookupTable: lup,
	}, nil
}

// -------------------------------------------------------------------
// -- FAKE OBJECTS
// -------------------------------------------------------------------

func NewFakeObjects() Objects {
	return Objects{
		Backends:    NewFakeBackends(),
		LookupTable: NewFakeLookupTable(),
		Config:      NewFakeVariable(T),
		N_backends:  NewFakeVariable(T),
	}
}

// -------------------------------------------------------------------
// -- FAKE BPF MAP
// -------------------------------------------------------------------

type FakeBackends struct {
	Map map[uint32]*types.Backend
}

// Reset implements BPFMap.
func (m FakeBackends) Reset(map[uint32]*types.Backend) {
	// - deletes all xor keys if they exist in the bpf map.
	// - updates in batch all existing elements.
	// - put each xor keys of the input map.
	panic("unimplemented")
}

func NewFakeBackends() BPFMap[uint32, *types.Backend] {
	return FakeBackends{Map: make(map[uint32]*types.Backend)}
}

// -------------------------------------------------------------------
// -- FAKE BPF VARIABLE
// -------------------------------------------------------------------

type FakeBPFVariable[T any] struct {
	v T
}

// Set implements BPFVariable.
func (f *FakeBPFVariable[T]) Set(v T) {
	panic("unimplemented")
}

func NewFakeVariable[T any](v T) BPFVariable[T] {
	return &FakeBPFVariable[T]{v}
}
