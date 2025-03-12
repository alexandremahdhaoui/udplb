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
	"math"
	"net"

	"github.com/alexandremahdhaoui/tooling/pkg/flaterrors"
	"github.com/google/uuid"
)

// -------------------------------------------------------------------
// -- UDPLB (Interface)
// -------------------------------------------------------------------

type UDPLB interface {
	// ---------------------------------------------------------------
	// -- General operations.
	// ---------------------------------------------------------------

	// Run udplb bpf program. Please note SetBackends or SetBackend must
	// be called at least once.
	Run(ctx context.Context) error

	// Logs bpf traces to stdout.
	TraceBPF() error

	// ---------------------------------------------------------------
	// -- BPF data structures operations.
	// ---------------------------------------------------------------

	// Put one backend into the backends map.
	SetBackend(ctx context.Context, item Backend) error

	// Overwrites the backends map.
	// Calling SetBackends with list equal to nil is not supported and
	// will throw an error.
	SetBackends(ctx context.Context, list []Backend) error

	// Put one SessionBackendMapping into the MappedSessions map.
	SetSessionBackendMapping(ctx context.Context, item SessionBackendMapping) error

	// Overwrites the mapped sessions map.
	// Calling SetMappedSessions with m equal to nil will empty the
	// map.
	SetMappedSessions(ctx context.Context, m MappedSessions) error
}

// -------------------------------------------------------------------
// -- udplb (concrete implementation)
// -------------------------------------------------------------------

type udplb struct {
	// -- udplb spec

	// Id of the loadbalancer
	id uuid.UUID
	// IP of the loadbalancer.
	ip net.IP
	// Port of the loadbalancer.
	port uint16
	// Network interface the XDP program will attach to.
	iface *net.Interface

	// -- data structures

	// List of backends.
	backends []Backend
	// Known SessionBackendMapping.
	// This datastructure should only hold a max amount of entries.
	// Old entries must be garbage collected.
	mappedSessions MappedSessions

	// -- bpf

	objs udplbObjects
	// Length of the backend lookup table.
	lookupTableLength Prime
}

// -------------------------------------------------------------------
// -- New
// -------------------------------------------------------------------

var (
	ErrCannotCreateNewUDPLBAdapter = errors.New("cannot create new udplb adapter")
	ErrInvalidUDPLBPort            = errors.New("udplb port is invalid")
)

func New(id string, ip string, port int, ifname string) (UDPLB, error) {
	// parse & validate id
	parsedId, err := uuid.Parse(id)
	if err != nil {
		return nil, flaterrors.Join(err, ErrCannotCreateNewUDPLBAdapter)
	}

	// parse & validate ip
	parsedIP := net.ParseIP(ip)

	// parse & validate port
	if port < 1000 || port > math.MaxUint16 {
		return nil, flaterrors.Join(ErrInvalidUDPLBPort, ErrCannotCreateNewUDPLBAdapter)
	}

	parsedPort := uint16(port)

	// get iface
	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		flaterrors.Join(err, ErrCannotCreateNewUDPLBAdapter)
	}

	return &udplb{
		id:    parsedId,
		ip:    parsedIP,
		port:  parsedPort,
		iface: iface,
	}, nil
}
