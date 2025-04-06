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
	"log/slog"
	"sync"
	"time"

	"github.com/alexandremahdhaoui/udplb/internal/util"
	"github.com/cilium/ebpf"
)

// -------------------------------------------------------------------
// -- BPF MAP
// -------------------------------------------------------------------

// Wraps bpf objects with a convenient interface for testing.
type Map[K comparable, V any] interface {
	// Set all values of the BPF map to the one of the input map.
	Set(kv map[K]V) error

	// SetAndDeferSwitchover updates the passive internal map but does
	// not perform the switchover.
	//
	// It allows users to "pseudo-atomically" update multiple BPF data
	// structures from the bpf-program point of view, by sharing the same
	// "activePointer" bpf variable with multiple BPF data structures.
	//
	// SetAndDeferSwitchover returns a function that can be called once to
	// perform the switchover.
	// The returned function can only be called once.
	// Please note that the returned function will retry if errors are
	// encountered.
	//
	// The deferable switchover function must be called, even if the same
	// "activePointer" bpf variable is used for multiple data structures.
	// The deferable switchover function must be called because it updates
	// internal variables in userspace.
	SetAndDeferSwitchover(kv map[K]V) (func(), error)
}

type bpfMap[K comparable, V any] struct {
	// The idea is to internally use 2 BPF maps for each data structure and
	// when updating 1 BPFArray or 1 BPFMap, we update the internal BPF map
	// that is not currently being used by the BPF program.
	//
	// The BPF program will check a variable that will let it know which map
	// to read from.
	//
	// This is a sort of a blue/green deployment of the new data structure.
	// This solution simplifies error handling as it ensures the whole data
	// structure is atomically updated from the bpf program point of view.
	//
	// a & b are the internal maps, either one or the other is in the active
	// state while the other one is in passive state.
	// The userland program will update data structures in passive states and
	// perform a switchover to "notify" the BPF program which map is in active
	// state.
	a, b *ebpf.Map

	// holds keys of each maps.
	aKeysCache, bKeysCache map[K]struct{}
	// the bpf variable storing the length of the respective a or b map.
	// We don't need to cache {a,b}Len as they can safely be retrieved from
	// from {a,b}KeysCache.
	aLen, bLen *ebpf.Variable

	// activePointer must be defined in the bpf program as __u8.
	// - When set to 0, the "active map" is `a` & the "active length" is `aLen`.
	// - When set to 1, the "active map" is `b` & the "active length" is `bLen`.
	activePointer *ebpf.Variable
	// We save a few syscalls by caching `activePointer` value instead of reading
	// the bpf variable.
	activePointerCache uint8
}

func NewMap[K comparable, V any](
	a, b *ebpf.Map,
	aLen, bLen *ebpf.Variable,
	activePointer *ebpf.Variable,
) (Map[K, V], error) {
	if util.AnyPtrIsNil(a, b, aLen, bLen, activePointer) {
		return nil, ErrEBPFObjectsMustNotBeNil
	}

	return &bpfMap[K, V]{
		a:                  a,
		b:                  b,
		aKeysCache:         make(map[K]struct{}),
		bKeysCache:         make(map[K]struct{}),
		aLen:               aLen,
		bLen:               bLen,
		activePointer:      activePointer,
		activePointerCache: 0,
	}, nil
}

// Set implements BPFMap.
func (m *bpfMap[K, V]) Set(newMap map[K]V) error {
	if err := m.set(newMap); err != nil {
		return err
	}

	if err := m.switchover(); err != nil {
		return err
	}

	return nil
}

func (m *bpfMap[K, V]) SetAndDeferSwitchover(newMap map[K]V) (func(), error) {
	if err := m.set(newMap); err != nil {
		return nil, err
	}

	return sync.OnceFunc(func() {
		var err error
		for i := range deferableSwitchoverMaxTries {
			if err = m.switchover(); err != nil {
				time.Sleep(time.Duration(i) * deferableSwitchoverTimeout)
				continue
			}
			return
		}
		slog.Error("cannot perform differable switchover", "err", err.Error())
		panic("CRITICAL ERROR")
	}), nil
}

// set performs at most 2 syscalls.
func (m *bpfMap[K, V]) set(newMap map[K]V) error {
	passiveMap := m.getPassiveMap()
	activeKeys := m.getActiveKeysFromCache()

	newLen := uint32(len(newMap))
	oldLen := uint32(len(activeKeys))

	newKeys := make(map[K]struct{}, newLen)
	oldKeys := make(map[K]struct{}, oldLen)

	// Because we mutate the oldKeys map below, we must copy
	// oldKeys in order to avoid side-effects on retry after
	// an error occured.
	for k := range activeKeys {
		oldKeys[k] = struct{}{}
	}

	keys := make([]K, 0)
	values := make([]V, 0)
	toDelete := make([]K, 0)

	// for each key-value pairs in the new map:
	// - we set the encountered key in the set of new keys.
	// - if the key is not in the old map, we add the pair in the "putBucket".
	// - else we:
	//		- set the key in the "updateKeys" slice.
	//		- set the value in the "updateValues" slice.
	//      - delete the key from the set of old keys. (it will be used to delete old keys)
	for k, v := range newMap {
		newKeys[k] = struct{}{}

		keys = append(keys, k)
		values = append(values, v)
		delete(oldKeys, k)
	}

	// -- iterate over old keys that does not exist in the new map.
	for k := range oldKeys {
		toDelete = append(toDelete, k)
	}

	// -- DELETE_BATCH
	// Delete entries first to avoid exceeding max map size.
	if len(toDelete) > 0 {
		if _, err := passiveMap.BatchDelete(toDelete, nil); err != nil {
			return err
		}
	}

	// -- UPDATE_BATCH
	// Only BPF_ANY is supported, hence UPDATE_BATCH is in fact a PUT operation.
	// - https://github.com/torvalds/linux/blob/master/kernel/bpf/syscall.c#L1981
	// - https://github.com/torvalds/linux/blob/master/kernel/bpf/arraymap.c#L888
	if len(keys) > 0 {
		if _, err := passiveMap.BatchUpdate(keys, values, nil); err != nil {
			return err
		}
	}

	// -- set new map length
	if err := m.setPassiveLen(newLen); err != nil {
		return err
	}

	// -- cache new keys
	m.cachePassiveKeys(newKeys)

	return nil
}

func (m *bpfMap[K, V]) switchover() error {
	newActive := 1 - m.activePointerCache
	if err := m.activePointer.Set(newActive); err != nil {
		return err
	}
	m.activePointerCache = newActive
	return nil
}

func (m *bpfMap[K, V]) getPassiveMap() *ebpf.Map {
	if m.activePointerCache == 0 {
		return m.b
	}
	return m.a
}

func (m *bpfMap[K, V]) getActiveKeysFromCache() map[K]struct{} {
	if m.activePointerCache == 0 {
		return m.aKeysCache
	}
	return m.bKeysCache
}

func (m *bpfMap[K, V]) setPassiveLen(newLen uint32) error {
	if m.activePointerCache == 0 {
		if err := m.bLen.Set(newLen); err != nil {
			return err
		}
	}
	if err := m.aLen.Set(newLen); err != nil {
		return err
	}
	return nil
}

func (m *bpfMap[K, V]) cachePassiveKeys(newKeys map[K]struct{}) {
	if m.activePointerCache == 0 {
		m.bKeysCache = newKeys
		return
	}
	m.aKeysCache = newKeys
}
