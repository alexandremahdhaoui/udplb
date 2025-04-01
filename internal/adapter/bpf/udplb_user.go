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
	"bufio"
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"os"

	"github.com/alexandremahdhaoui/tooling/pkg/flaterrors"
	"github.com/google/uuid"
)

// -------------------------------------------------------------------
// -- UDPLB (Interface)
// -------------------------------------------------------------------

type UDPLB interface {
	// Run udplb bpf program. Please note SetBackends or SetBackend must
	// be called at least once.
	Run(ctx context.Context) error

	// Logs bpf traces to stdout.
	TraceBPF() error

	// TODO: move that to its own adapter that can be initialized using
	// the chan obtained from UDPLB.GetEventChannel.
	// Return an interface to manage bpf backend configuration.
	Backends() Backends

	// TODO: move that to its own adapter that can be initialized using
	// the chan obtained from UDPLB.GetEventChannel.
	// Return an interface to manage bpf session configuration.
	Sessions() Sessions
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

// -------------------------------------------------------------------
// -- TraceBPF
// -------------------------------------------------------------------

func (lb *udplb) Run(ctx context.Context) error { panic("unimplemented") }

// func (lb *udplb) Run(ctx context.Context) error {
// 	// Remove resource limits for kernels <5.11.
// 	if err := rlimit.RemoveMemlock(); err != nil {
// 		log.Fatal("Removing memlock:", err)
// 	}
//
// 	// -- Init constants
//
// 	spec, err := loadUdplb()
// 	if err != nil {
// 		slog.Error(err.Error())
// 		os.Exit(1)
// 	}
//
// 	if err = InitConstants(spec, cfg); err != nil {
// 		slog.Error(err.Error())
// 		os.Exit(1)
// 	}
//
// 	// -- Load ebpf elf into kernel
// 	var objs udplbObjects
// 	if err = spec.LoadAndAssign(&objs, nil); err != nil {
// 		slog.Error(err.Error())
// 		os.Exit(1)
// 	}
// 	defer objs.Close()
//
// 	if err = InitObjects(&objs, cfg); err != nil {
// 		slog.Error(err.Error())
// 		os.Exit(1)
// 	}
//
// 	// -- Get iface
// 	iface, err := net.InterfaceByName(cfg.Ifname)
// 	if err != nil {
// 		slog.Error(err.Error())
// 		os.Exit(1)
// 	}
//
// 	// -- Attach udplb to iface
// 	link, err := link.AttachXDP(link.XDPOptions{
// 		Program:   objs.Udplb,
// 		Interface: iface.Index,
// 	})
// 	if err != nil {
// 		slog.Error(err.Error())
// 		os.Exit(1)
// 	}
// 	defer link.Close()
//
// 	slog.Info("XDP program loaded successfully", "ifname", cfg.Ifname)
//
// 	// -- 4. Fetch packet counter every second & print content.
// 	stop := make(chan os.Signal, 1)
// 	signal.Notify(stop, os.Interrupt)
//
// 	<-stop
// 	log.Printf("Stopping...")
//
// 	return nil
// }
//
// // -------------------------------------------------------------------
// // -- BPF INITIALIZATION
// // -------------------------------------------------------------------
//
// func InitConstants(spec *ebpf.CollectionSpec, cfg InitialConfig) error {
// 	ip, err := util.ParseIPToUint32(cfg.spec.IP)
// 	if err != nil {
// 		return err
// 	}
//
// 	if err := spec.Variables["UDPLB_IP"].Set(ip); err != nil {
// 		return err
// 	}
//
// 	return spec.Variables["UDPLB_PORT"].Set(cfg.Port)
// }
//
// func InitObjects(objs *udplbObjects, cfg types.Config) error {
// 	var nBackends uint32
//
// 	for i, t := range cfg.Backends {
// 		nBackends += 1
//
// 		ip, err := util.ParseIPToUint32(t.IP)
// 		if err != nil {
// 			return err
// 		}
//
// 		mac, err := util.ParseIEEE802MAC(t.MAC)
// 		if err != nil {
// 			return err
// 		}
//
// 		// TODO: parse mac addr
// 		backend := udplbBackendSpec{
// 			Mac:     mac,
// 			Ip:      ip,
// 			Port:    uint16(t.Port),
// 			Enabled: t.Enabled,
// 		}
//
// 		// -- Set backend in maps
// 		if err := objs.Backends.Put(uint32(i), &backend); err != nil {
// 			return err
// 		}
// 	}
//
// 	// Set n_backends
// 	if err := objs.N_backends.Set(nBackends); err != nil {
// 		return err
// 	}
//
// 	return nil
// }

// -------------------------------------------------------------------
// -- TraceBPF
// -------------------------------------------------------------------

// TraceBPF implements UDPLB.
func (lb *udplb) TraceBPF() error {
	// -- print bpf trace logs
	fd, err := os.OpenFile("/sys/kernel/debug/tracing/trace_pipe", os.O_RDONLY, os.ModeAppend)
	if err != nil {
		return err
	}

	defer fd.Close()
	go func() {
		scanner := bufio.NewScanner(fd)
		scanner.Split(bufio.ScanLines)
		for scanner.Scan() {
			fmt.Fprintf(os.Stdout, "%s\n", scanner.Text())
		}
	}()

	return nil
}
