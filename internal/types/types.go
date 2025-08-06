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
	"io"
	"net"
	"os"

	"github.com/google/uuid"
	yaml "sigs.k8s.io/yaml/goyaml.v2"
)

/*******************************************************************************
 * Assignment
 *
 ******************************************************************************/

type Assignment struct {
	BackendId uuid.UUID
	SessionId uuid.UUID
}

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

/*******************************************************************************
 * Config
 *
 ******************************************************************************/

type BackendConfig struct {
	Enabled bool   `json:"enabled"`
	IP      string `json:"ip"`
	MAC     string `json:"mac"`
	Port    int    `json:"port"`
}

type Config struct {
	Ifname string `json:"ifname"`
	IP     string `json:"ip"`
	Port   uint16 `json:"port"`

	Backends []BackendConfig `json:"backends"`
}

func GetConfig(filepath string) (Config, error) {
	var (
		b   []byte
		err error
	)

	if filepath == "-" { // read from stdin
		// TODO: unblock when reading from stdin
		b, err = io.ReadAll(os.Stdin)
	} else {
		b, err = os.ReadFile(os.Args[1])
	}

	if err != nil {
		return Config{}, err
	}

	out := Config{}
	if err := yaml.Unmarshal(b, &out); err != nil {
		return Config{}, err
	}

	return out, nil
}

/*******************************************************************************
 * FilterFunc
 *
 ******************************************************************************/

type FilterFunc[T any] func(obj T, entry T) bool

/*******************************************************************************
 * State
 *
 ******************************************************************************/

type (
	State int
)

const (
	// A subsystem is in Unknown state and must be considered Unavailable
	// until it state becomes known again.
	StateUnknown State = iota

	// A subsystem is healthy available.
	// E.g.:
	//		A backend is healthy and available. It can accept packets destinated
	//		to new sessions.
	StateAvailable

	// A subsystem is healthy however scheduling new workload on this subsystem
	// is not permitted.
	// E.g.:
	//		A backend is healthy can receive packets for mapped sessions but
	//		cannot accept packets destinated to new sessions.
	StateUnschedulable

	//
	// E.g.:
	//		A backend is down and cannot accept any packet. We can evict a backend
	//      by marking it as unavailable.
	StateUnavailable
)
