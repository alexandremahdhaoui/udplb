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
package types

import (
	"crypto/sha256"
	"encoding/binary"
	"net"

	"github.com/google/uuid"
)

// Must be constructed from diverse source of kubernetes resources.
func NewBackend(
	id uuid.UUID,
	spec BackendSpec,
	status BackendStatus,
) *Backend {
	h := sha256.Sum256([]byte(id[:]))

	// -- cache coordinates
	coordinates := [8]uint32{}
	for i := range 8 {
		coordinates[i] = binary.NativeEndian.Uint32(h[4*i : 4*(i+1)])
	}

	return &Backend{
		Id:          id,
		Spec:        spec,
		Status:      status,
		coordinates: coordinates,
	}
}

type (
	BackendSpec struct {
		IP      net.IP
		Port    int
		MacAddr net.HardwareAddr

		// The desired state of the backend.
		State State
	}

	BackendStatus struct {
		// The actual/current state of the backend.
		State State
	}

	Backend struct {
		Id     uuid.UUID
		Spec   BackendSpec
		Status BackendStatus

		coordinates [8]uint32
	}
)

func (b Backend) Coordinates() [8]uint32 {
	return b.coordinates
}
