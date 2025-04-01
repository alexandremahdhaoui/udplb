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
	"errors"
	"slices"

	"github.com/alexandremahdhaoui/tooling/pkg/flaterrors"
	"github.com/alexandremahdhaoui/udplb/internal/types"

	"github.com/google/uuid"
)

// NB: The name of this file (i.e. ds_backends.go) stands for `"backends" data structure`.

// TODO: REFACTOR "Backends" INTO "DataStructures". IT SHOULD TAKE THE
// RESPONSIBILITY OF ALL BPF DATA STRUCTURES.
// -- OR:
// THIS BACKENDS INTERFACE MUST HOLD A REFERENCE TO AN "eventCh chan<- event" FROM THE
// "bpf.DataStructures" INTERFACE. IT WOULD THEN PUSH EVENTS TO THAT CHANNEL.
type Backends interface {
	types.DoneCloser

	// Deletes one backend from the map.
	// It locks the data structure on the bpf side and update it.
	Delete(ctx context.Context, id uuid.UUID) error

	// Put one backend into the backends map.
	// It creates or update an existing backend.
	// It locks the data structure on the bpf side and update it.
	Put(ctx context.Context, item types.Backend) error

	// Overwrites the backends map.
	// Calling SetBackends with list equal to nil is not supported and
	// will throw an error.
	// It locks the data structure on the bpf side and update it.
	Reset(ctx context.Context, list []types.Backend) error
}

func NewBackends(mgr DataStructureManager) Backends {
	return &backends{
		backends: make(map[uuid.UUID]*types.Backend),
		eventCh:  mgr.GetEventChannel(),
		doneCh:   make(chan struct{}),
	}
}

// -------------------------------------------------------------------
// -- backends (concrete implementation)
// -------------------------------------------------------------------

// It implements bpf.Backends.
//
// It needs to hold references to the following BPF data structures:
//   - backend count: is it still needed?
//   - backend lookup table map.
//   - session lookup table map: to delete entries of backends that are
//     in state Down.
//
// BUT: we may want to perform this changes outside the adapter.
// QUESTIONS:
//   - Do we want the adapter to coordinate backends and sessions?
//   - Or do we want the adapter to only manipulate the data structures?
//     E.g. Leaving the responsibility to delete a session mapping of a
//     specific backend when a backend becomes unavailable to the goroutine
//     calling the bpf adapter.
//
// It runs a control loop responsible for updating the bpf datastructure.
type backends struct {
	backends map[uuid.UUID]*types.Backend

	eventCh chan event

	// doneCh is a channel that must be closed once work done on behalf of this
	// data structure has been gracefully shut down.
	doneCh chan struct{}

	// how should we perform the sync?
	// - do not sync if the data structure did not actually changed.
	// - do not sync if no semantic change happened even though the data structure was changed.
	//   i.e. the state of a backend goes from down to
}

// Done implements Backends.
func (b *backends) Done() <-chan struct{} {
	panic("unimplemented")
}

// Delete implements Backends.
func (b *backends) Delete(ctx context.Context, id uuid.UUID) error {
	if _, ok := b.backends[id]; !ok { // skip if item does not exist.
		return nil
	}

	ctx.Done()

	return nil
}

// Put implements Backends.
func (b *backends) Put(ctx context.Context, item types.Backend) error {
	panic("unimplemented")
}

// Reset implements Backends.
func (b *backends) Reset(ctx context.Context, list []types.Backend) error {
	panic("unimplemented")
}

// --
// TODO: refactor below

// -------------------------------------------------------------------
// -- Helpers
// -------------------------------------------------------------------

var ErrSettingLookupTable = errors.New("setting lookup table")

func (lb *udplb) availableBackends() []types.Backend {
	availableBackends := make([]types.Backend, 0, len(lb.backends))
	for _, backend := range lb.backends {
		if backend.Status.State != types.StateAvailable ||
			backend.Spec.State != types.StateAvailable {
			// skip this backend if it's either reported or requested with a non-Available state.
			continue
		}

		availableBackends = append(availableBackends, backend)
	}

	// sort availableBackends alphabetically by HashedId. (it makes algorithm deterministic).
	slices.SortFunc(availableBackends, func(a, b types.Backend) int {
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

func (lb *udplb) setLookupTable(keys []int, values []types.Backend) error {
	// TODO: check what the int return by BatchUpdate refers to.
	if _, err := lb.objs.Backends.BatchUpdate(keys, values, nil); err != nil {
		return flaterrors.Join(err, ErrSettingLookupTable)
	}

	return nil
}
