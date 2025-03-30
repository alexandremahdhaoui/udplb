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

func NewBackend(
	id uuid.UUID,
	spec BackendSpec,
	status BackendStatus,
) *Backend {
	h := sha256.Sum256([]byte(id[:]))
	i := binary.NativeEndian.Uint64(h[24:32])

	return &Backend{
		Id:       id,
		Spec:     spec,
		Status:   status,
		hashedId: i,
	}
}

type (
	BackendSpec struct {
		IP   net.IP
		Port int

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

		hashedId uint64
	}
)

func (b Backend) HashedId() uint64 {
	return b.hashedId
}
