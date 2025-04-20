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
	"github.com/alexandremahdhaoui/ebpfstruct"
	"github.com/google/uuid"
)

// -------------------------------------------------------------------
// -- OBJECTS
// -------------------------------------------------------------------

type (
	BackendSpec = udplbBackendSpecT
	Assignment  = udplbAssignmentT
)

// Wraps the following ebpf Objects with a convenient interface for testing.
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
	BackendList ebpfstruct.Array[*BackendSpec]

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
	LookupTable ebpfstruct.Array[uint32]

	// AssignmentFIFO is a bpf.FIFO of udplbAssignmentT.
	// It is used to notify userland about newly discovered session-backend assignment.
	// The userland program MUST notify other loadbalancer instances about this newly
	// discovered assignment.
	// The userland program MUST update its internal session map in order to avoid
	// overwritting these newly discovered assignment when setting and switching over
	// the SessionMap bpf data structure.
	AssignmentFIFO ebpfstruct.FIFO[Assignment]

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
	SessionMap ebpfstruct.Map[uuid.UUID, uint32]
}

// The doneCh must signals as soon as possible that the ebpf data structures are not
// available anymore, either because they are closed or the bpf program
// is closed.
func newObjects(objs *udplbObjects, doneCh <-chan struct{}) (Objects, error) {
	backendList, err := ebpfstruct.NewArray[*udplbBackendSpecT](
		objs.BackendListA,
		objs.BackendListB,
		objs.BackendListA_len,
		objs.BackendListB_len,
		objs.ActivePointer,
		doneCh,
	)
	if err != nil {
		return Objects{}, err
	}

	lookupTable, err := ebpfstruct.NewArray[uint32](
		objs.LookupTableA,
		objs.LookupTableB,
		objs.LookupTableA_len,
		objs.LookupTableB_len,
		objs.ActivePointer,
		doneCh,
	)
	if err != nil {
		return Objects{}, err
	}

	sessionMap, err := ebpfstruct.NewMap[uuid.UUID, uint32](
		objs.SessionMapA,
		objs.SessionMapB,
		objs.SessionMapA_len,
		objs.SessionMapB_len,
		objs.ActivePointer,
		doneCh,
	)
	if err != nil {
		return Objects{}, err
	}

	assignmentFIFO, err := ebpfstruct.NewFIFO[udplbAssignmentT](
		objs.AssignmentRingbuf,
		doneCh,
	)
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
