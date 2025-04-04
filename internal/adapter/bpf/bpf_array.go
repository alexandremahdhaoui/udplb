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

const (
	deferableSwitchoverMaxTries = 3
	deferableSwitchoverTimeout  = 5 * time.Millisecond
)

// -------------------------------------------------------------------
// -- BPF ARRAY
// -------------------------------------------------------------------

// Wraps bpf objects with a convenient interface for testing.
type BPFArray[T any] interface {
	// -- Set all values of the BPF map to the one of the input map.
	//	  - DELETE all entries in the *ebpf.Map that have index > newL_en.
	//	  - UPDATE_BATCH all entries with index in the interval [0, oldLen].
	//	  - PUT all entries with index in the interval [oldLen, newLen].
	// -- Set new length if changed:
	//	  - SET a.length.
	//	  - a.oldLen = newLen.
	Set(values []T) error

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
	SetAndDeferSwitchover(values []T) (func(), error)
}

func NewBPFArray[T any](
	a, b *ebpf.Map,
	aLen, bLen *ebpf.Variable,
	activePointer *ebpf.Variable,
) (BPFArray[T], error) {
	if util.AnyPtrIsNil(a, b, aLen, bLen, activePointer) {
		return nil, ErrEBPFObjectsMustNotBeNil
	}

	return &bpfArray[T]{
		a:                  a,
		b:                  b,
		aLenCache:          0,
		bLenCache:          0,
		aLen:               aLen,
		bLen:               bLen,
		activePointer:      activePointer,
		activePointerCache: 0,
	}, nil
}

type bpfArray[T any] struct {
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

	// the bpf variable storing the length of the respective a or b map.
	aLen, bLen *ebpf.Variable
	// We save a few syscalls by caching `{a,b}len` instead of reading the bpf
	// variable.
	aLenCache, bLenCache uint32

	// activePointer must be defined in the bpf program as __u8.
	// - When set to 0, the "active map" is `a` & the "active length" is `aLen`.
	// - When set to 1, the "active map" is `b` & the "active length" is `bLen`.
	activePointer *ebpf.Variable
	// We save a few syscalls by caching `activePointer` value instead of reading
	// the bpf variable.
	activePointerCache uint8
}

// Set implements BPFMap.
func (arr *bpfArray[T]) Set(values []T) error {
	if err := arr.set(values); err != nil {
		return err
	}

	if err := arr.switchover(); err != nil {
		return err
	}

	return nil
}

func (arr *bpfArray[T]) SetAndDeferSwitchover(values []T) (func(), error) {
	if err := arr.set(values); err != nil {
		return nil, err
	}

	return sync.OnceFunc(func() {
		var err error
		for i := range deferableSwitchoverMaxTries {
			if err = arr.switchover(); err != nil {
				time.Sleep(time.Duration(i) * deferableSwitchoverTimeout)
				continue
			}
			return
		}
		slog.Error("cannot perform differable switchover", "err", err.Error())
		panic("CRITICAL ERROR")
	}), nil
}

func (arr *bpfArray[T]) set(values []T) error {
	keys := make([]uint32, len(values))
	for i := range len(values) {
		keys[i] = uint32(i)
	}

	passiveMap := arr.getPassiveMap()

	// - UPDATE_BATCH all entries with index in the interval [0, old_length].
	if _, err := passiveMap.BatchUpdate(keys, values, nil); err != nil {
		return err
	}

	oldLen := arr.getActiveLenFromCache()
	newLen := uint32(len(values))

	switch {
	default:
		return nil // return early if length did not change.
	case oldLen > newLen:
		// - DELETE all entries in the *ebpf.Map that have index > new_length.
		keys := make([]uint32, 0, oldLen-newLen) // TODO
		for i := newLen; i < oldLen; i++ {
			keys = append(keys, i)
		}

		if _, err := passiveMap.BatchDelete(keys, nil); err != nil {
			return err
		}
	case oldLen < newLen:
		// - PUT all entries with index in the interval [old_length, new_length].
		for i := oldLen; i < newLen; i++ {
			if err := passiveMap.Put(i, values[i]); err != nil {
				return err
			}
		}
	}

	// Update length
	if err := arr.setPassiveLen(newLen); err != nil {
		return err
	}

	return nil
}

func (arr *bpfArray[T]) switchover() error {
	newActive := 1 - arr.activePointerCache
	if err := arr.activePointer.Set(newActive); err != nil {
		return err
	}
	arr.activePointerCache = newActive
	return nil
}

func (arr *bpfArray[T]) getPassiveMap() *ebpf.Map {
	if arr.activePointerCache == 0 {
		return arr.b
	}
	return arr.a
}

func (arr *bpfArray[T]) getActiveLenFromCache() uint32 {
	if arr.activePointerCache == 0 {
		return arr.aLenCache
	}
	return arr.bLenCache
}

func (arr *bpfArray[T]) setPassiveLen(newLen uint32) error {
	if arr.activePointerCache == 0 {
		if err := arr.bLen.Set(newLen); err != nil {
			return err
		}
		arr.bLenCache = newLen
		return nil
	}
	if err := arr.aLen.Set(newLen); err != nil {
		return err
	}
	arr.aLenCache = newLen
	return nil
}
