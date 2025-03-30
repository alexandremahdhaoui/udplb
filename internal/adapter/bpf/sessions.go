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

	"github.com/google/uuid"
)

type Sessions interface {
	// Put one session mapping into the "MappedSessions" lookup table.
	// It looks the data structure and update it.
	Put(ctx context.Context, item SessionBackendMapping) error

	// Delete on session mapping.
	// It looks the data structure and update it.
	Delete(ctx context.Context, id uuid.UUID) error

	// Overwrites the mapped sessions map.
	// Calling SetMappedSessions with m equal to nil will empty the
	// map.
	// It looks the data structure and update it.
	Reset(ctx context.Context, m MappedSessions) error
}

// -------------------------------------------------------------------
// -- sessionMap (concrete implementation)
// -------------------------------------------------------------------

// --
// TODO: refactor below

type (
	MappedSessions = map[uuid.UUID]uuid.UUID

	SessionBackendMapping struct {
		SessionId uuid.UUID
		BackendId uuid.UUID

		MarkedForDeletion bool
	}
)

// SetMappedSessions implements UDPLB.
func (lb *udplb) SetMappedSessions(ctx context.Context, m MappedSessions) error {
	panic("unimplemented")
}

// SetSessionBackendMapping implements UDPLB.
func (lb *udplb) SetSessionBackendMapping(ctx context.Context, item SessionBackendMapping) error {
	panic("unimplemented")
}
