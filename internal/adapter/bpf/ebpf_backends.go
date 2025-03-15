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
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"net"
	"slices"

	"github.com/alexandremahdhaoui/tooling/pkg/flaterrors"
	"github.com/google/uuid"
)

const (
	// A backend in Unknown state must be considered Down until it state becomes known.
	Unknown BackendState = iota
	// A backend is healthy and available. It can accept packets destinated to new sessions.
	Available
	// A backend is healthy can receive packets for mapped sessions but cannot accept packets
	// destinated to new sessions.
	Unschedulable
	// A down backend cannot accept any packet. To evict a backend it can be "downed".
	Down
)

type (
	BackendState int

	BackendSpec struct {
		IP   net.IP
		Port int

		// The desired state of the backend.
		State BackendState
	}

	BackendStatus struct {
		// The actual/current state of the backend.
		State BackendState
	}

	Backend struct {
		Id uuid.UUID

		Spec   BackendSpec
		Status BackendStatus

		MarkedForDeletion bool
		hashedId          uint64
	}
)

func NewBackend(
	id uuid.UUID,
	spec BackendSpec,
	status BackendStatus,
	isMarkedForDeletion bool,
) *Backend {
	h := sha256.Sum256([]byte(id.String()))
	i := binary.NativeEndian.Uint64(h[24:32])

	return &Backend{
		Id:                id,
		Spec:              spec,
		Status:            status,
		MarkedForDeletion: isMarkedForDeletion,
		hashedId:          i,
	}
}

func (b Backend) HashedId() uint64 {
	return b.hashedId
}

// -------------------------------------------------------------------
// -- SetBackend
// -------------------------------------------------------------------

// SetBackend sets the desired item in the set of backend.
func (lb *udplb) SetBackend(ctx context.Context, item Backend) error {
	if item.MarkedForDeletion {
		// we must delete the item from the map.
		// lb.objs.Backends.Delete()
	}

	// lb.objs.Backends.Put(item.Id.String(), value interface{})
	panic("unimplemented")
}

// SetBackends implements UDPLB.
func (lb *udplb) SetBackends(ctx context.Context, list []Backend) error {
	panic("unimplemented")
}

// -------------------------------------------------------------------
// -- Helpers
// -------------------------------------------------------------------

var ErrSettingLookupTable = errors.New("setting lookup table")

func (lb *udplb) availableBackends() []Backend {
	availableBackends := make([]Backend, 0, len(lb.backends))
	for _, backend := range lb.backends {
		if backend.Status.State != Available || backend.Spec.State != Available {
			// skip this backend if it's either reported or requested with a non-Available state.
			continue
		}

		availableBackends = append(availableBackends, backend)
	}

	// sort availableBackends alphabetically by HashedId. (it makes algorithm deterministic).
	slices.SortFunc(availableBackends, func(a, b Backend) int {
		switch {
		default:
			return 0
		case a.HashedId() < b.HashedId():
			return -1
		case a.HashedId() > b.HashedId():
			return 1
		}
	})

	return availableBackends
}

func (lb *udplb) setLookupTable(keys []int, values []Backend) error {
	// TODO: check what the int return by BatchUpdate refers to.
	if _, err := lb.objs.Backends.BatchUpdate(keys, values, nil); err != nil {
		return flaterrors.Join(err, ErrSettingLookupTable)
	}

	return nil
}
