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
	"encoding/binary"
	"net"

	"github.com/google/uuid"
)

/*******************************************************************************
 * BackendSpec
 *
 ******************************************************************************/

// BackendSpec is data kind of type `persisted`.
type BackendSpec struct {
	IP      net.IP
	Port    int
	MacAddr net.HardwareAddr

	// The desired state of the backend.
	State State
}

/*******************************************************************************
 * BackendStatus
 *
 ******************************************************************************/

// BackendStatus is a data kind of type `volatile`.
type BackendStatus struct {
	// The actual/current state of the backend.
	State State
}

/*******************************************************************************
 * Backend
 *
 ******************************************************************************/

const NCoordinates = 4

type Backend struct {
	Id     uuid.UUID
	Spec   BackendSpec
	Status BackendStatus

	coordinates [NCoordinates]uint32
}

func (b Backend) Coordinates() [NCoordinates]uint32 {
	return b.coordinates
}

// May be constructed from varied source, such as kubernetes resources.
func NewBackend(
	id uuid.UUID,
	spec BackendSpec,
	status BackendStatus,
) *Backend {
	h := id // sha256.Sum256([]byte(id[:]))

	// -- cache coordinates
	coordinates := [NCoordinates]uint32{}
	for i := range NCoordinates {
		coordinates[i] = binary.NativeEndian.Uint32(h[4*i : 4*(i+1)])
	}

	return &Backend{
		Id:          id,
		Spec:        spec,
		Status:      status,
		coordinates: coordinates,
	}
}
