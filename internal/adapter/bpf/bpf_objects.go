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
	"errors"

	"github.com/alexandremahdhaoui/tooling/pkg/flaterrors"
	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/cilium/ebpf"
)

var (
	ErrNewBPFVariable          = errors.New("cannot create new BPFVariable")
	ErrEBPFObjectsMustNotBeNil = errors.New("ebpf objects must not be nil")

	ErrCannotCreateObjectsFromUnknownUDPLBImplementation = errors.New(
		"cannot create bpf.Objects struct from unknown bpf.UDPLB implementation",
	)
)

// -------------------------------------------------------------------
// -- OBJECTS
// -------------------------------------------------------------------

// Wraps the following ebpf objects with a convenient interface for testing.
// Backends   *ebpf.Map      // through udplbMaps
// UDPLB_IP   *ebpf.Variable // through udplbVariables
// UDPLB_PORT *ebpf.Variable // through udplbVariables
// N_backends *ebpf.Variable // through udplbVariables
type Objects struct {
	Backends   BPFMap[uint32, *types.Backend]
	Config     BPFVariable[string] // TODO
	UDPLB_IP   BPFVariable[uint32]
	UDPLB_PORT BPFVariable[uint32]
	N_backends BPFVariable[uint32]
}

func NewObjects(prog UDPLB) (Objects, error) {
	concrete, ok := prog.(udplb)
	if !ok {
		return Objects{}, ErrCannotCreateObjectsFromUnknownUDPLBImplementation
	}

	return Objects{
		Backends: NewBPFMap(concrete.objs.udplbMaps.Backends),
		// Config:     NewBPFVariable(concrete.objs.udplbVariables.Config),
		UDPLB_IP:   NewBPFVariable[uint32](concrete.objs.udplbVariables.UDPLB_IP),
		UDPLB_PORT: NewBPFVariable[uint32](concrete.objs.udplbVariables.UDPLB_PORT),
		N_backends: NewBPFVariable[uint32](concrete.objs.udplbVariables.N_backends),
	}, nil
}

// -------------------------------------------------------------------
// -- BPF ARRAY
// -------------------------------------------------------------------

// TODO: use 2 maps to avoid locking.
//
// The idea is to internally use 2 BPF maps for each data structure and
// when updating 1 BPFArray or 1 BPFMap, we update the internal BPF map
// that is not currently being used by the BPF program.
//
// The BPF program will check a variable that will let it know which map
// to read from.
//
// This is a sort of a blue/green deployment of the new data structure.

// Wraps bpf objects with a convenient interface for testing.
type BPFArray[T any] interface {
	// -- Set all values of the BPF map to the one of the input map.
	//	  - DELETE all entries in the *ebpf.Map that have index > newL_en.
	//	  - UPDATE_BATCH all entries with index in the interval [0, oldLen].
	//	  - PUT all entries with index in the interval [oldLen, newLen].
	// -- Set new length if changed:
	//	  - SET a.length.
	//	  - a.oldLen = newLen.
	Reset(values []T) error
}

type bpfArray[T any] struct {
	obj *ebpf.Map

	// We save a few syscalls by storing it instead of reading the bpf variable.
	oldLen uint32
	lenObj *ebpf.Variable

	// This is not really a spinlock, it's a variable that can be set to 1 if locked.
	spinlock *ebpf.Variable
}

// Reset implements BPFMap.
func (a *bpfArray[T]) Reset(values []T) error {
	keys := make([]uint32, len(values))
	for i := range len(values) {
		keys[i] = uint32(i)
	}

	a.lock()
	defer a.unlock()

	// - UPDATE_BATCH all entries with index in the interval [0, old_length].
	if _, err := a.obj.BatchUpdate(keys, values, nil); err != nil {
		return err
	}

	// - DELETE all entries in the *ebpf.Map that have index > new_length.
	// - PUT all entries with index in the interval [old_length, new_length].
	newLen := uint32(len(values))
	switch {
	default:
		return nil // return early if length did not change.
	case a.oldLen > newLen:
		keys := make([]uint32, 0, a.oldLen-newLen) // TODO
		for i := newLen; i < a.oldLen; i++ {
			keys = append(keys, i)
		}

		if _, err := a.obj.BatchDelete(keys, nil); err != nil {
			return err
		}
	case a.oldLen < newLen:
		for i := a.oldLen; i < newLen; i++ {
			if err := a.obj.Put(i, values[i]); err != nil {
				return err
			}
		}
	}

	// Update length
	a.oldLen = newLen
	if err := a.lenObj.Set(newLen); err != nil {
		return err
	}

	return nil
}

const lockBackoff = 10

const (
	unlocked uint8 = 0
	locked   uint8 = 1
)

func (a *bpfArray[T]) lock() {
	var err error
	for range lockBackoff {
		err = a.spinlock.Set(locked)
		if err == nil {
			return
		}
	}

	panic("CRITICAL ERROR")
}

func (a *bpfArray[T]) unlock() {
	var err error
	for range lockBackoff {
		err = a.spinlock.Set(unlocked)
		if err == nil {
			return
		}
	}

	panic("CRITICAL ERROR")
}

func NewBPFArray[T any](obj *ebpf.Map, lenObj, spinlock *ebpf.Variable) (BPFArray[T], error) {
	if obj == nil || lenObj == nil || spinlock == nil {
		return nil, ErrEBPFObjectsMustNotBeNil
	}

	return &bpfArray[T]{
		obj:      obj,
		oldLen:   0,
		lenObj:   lenObj,
		spinlock: spinlock,
	}, nil
}

// -------------------------------------------------------------------
// -- BPF MAP
// -------------------------------------------------------------------

// Wraps bpf objects with a convenient interface for testing.
type BPFMap[K comparable, V any] interface {
	// Set all values of the BPF map to the one of the input map.
	Reset(kv map[K]V) error
}

type bpfMap[K comparable, V any] struct {
	obj *ebpf.Map
	// We save a few syscalls by storing it instead of reading the bpf map.
	oldKeys map[K]struct{}

	lenObj *ebpf.Variable

	// This is not really a spinlock, it's a variable that can be set to 1 if locked.
	spinlock *ebpf.Variable
}

// Reset implements BPFMap.
func (m *bpfMap[K, V]) Reset(newMap map[K]V) error {
	newKeys := make(map[K]struct{}, len(newMap))

	deleteKeys := make([]K, 0)    // - -> n calls
	putBucket := make(map[K]V, 0) // + -> n calls
	updateKeys := make([]K, 0)    // = -> 1 call
	updateValues := make([]V, 0)

	// for each key-value pairs in the new map:
	// - we set the encountered key in the set of new keys.
	// - if the key is not in the old map, we add the pair in the "putBucket".
	// - else we:
	//		- set the key in the "updateKeys" slice.
	//		- set the value in the "updateValues" slice.
	//      - delete the key from the set of old keys. (it will be used to delete old keys)
	for k, v := range newMap {
		newKeys[k] = struct{}{}
		if _, ok := m.oldKeys[k]; !ok {
			putBucket[k] = v // add keys that did not previously exist in the map.
			continue
		}

		updateKeys = append(updateKeys, k)
		updateValues = append(updateValues, v)
		delete(m.oldKeys, k)
	}

	// -- iterate over old keys that does not exist in the new map.
	for k := range m.oldKeys {
		deleteKeys = append(deleteKeys, k)
	}

	// -- DELETE
	if len(deleteKeys) > 0 {
		if _, err := m.obj.BatchDelete(deleteKeys, nil); err != nil {
			return err
		}
	}

	// -- UPDATE
	if len(updateKeys) > 0 {
		if _, err := m.obj.BatchUpdate(updateKeys, updateValues, nil); err != nil {
			return err
		}
	}

	// -- PUT
	for k, v := range putBucket {
		if err := m.obj.Put(k, v); err != nil {
			return err
		}
	}

	// -- UPDATE map length
	newLen := uint32(len(newMap))
	oldLen := uint32(len(m.oldKeys))
	if newLen != oldLen {
		if err := m.lenObj.Set(newLen); err != nil {
			return err
		}
	}

	// -- persist keys
	m.oldKeys = newKeys

	// TODO: perform the blue-green "swap".

	return nil
}

func NewBPFMap[K comparable, V any](
	obj *ebpf.Map,
	lenObj, spinlock *ebpf.Variable,
) (BPFMap[K, V], error) {
	if obj == nil || lenObj == nil || spinlock == nil {
		return nil, ErrEBPFObjectsMustNotBeNil
	}

	return &bpfMap[K, V]{
		obj:      obj,
		oldKeys:  make(map[K]struct{}),
		lenObj:   lenObj,
		spinlock: spinlock,
	}, nil
}

// -------------------------------------------------------------------
// -- BPF VARIABLE
// -------------------------------------------------------------------

type BPFVariable[T any] interface {
	// Set the variable.
	Set(v T)
}

type bpfVariable[T any] struct {
	obj *ebpf.Variable
}

// Set implements BPFVariable.
func (b *bpfVariable[T]) Set(v T) {
	panic("unimplemented")
}

func NewBPFVariable[T any](obj *ebpf.Variable) (BPFVariable[T], error) {
	if obj == nil {
		return nil, flaterrors.Join(ErrEBPFObjectsMustNotBeNil, ErrNewBPFVariable)
	}

	return &bpfVariable[T]{obj: obj}, nil
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
