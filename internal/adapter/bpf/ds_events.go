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
	"github.com/google/uuid"
)

// -------------------------------------------------------------------
// -- EVENT
// -------------------------------------------------------------------

type EventObjectKind string

const (
	EventObjectKindBackends    EventObjectKind = "Backends"
	EventObjectKindLookupTable EventObjectKind = "LookupTable"
	EventObjectKindSessions    EventObjectKind = "Sessions"
)

// All objects must be set.
type EventObjects struct {
	Backends    []*udplbBackendSpec
	LookupTable []uint32
	Sessions    map[uuid.UUID]uint32
}

type EventKind string

const (
	EventKindUnknown    EventKind = "UNKNOWN"
	EventKindSetObjects EventKind = "SetObjects"
)

// Available operations:
// - Set all object with `EventKindSetObjects`.
type Event struct {
	Kind       EventKind
	SetObjects EventObjects
}
