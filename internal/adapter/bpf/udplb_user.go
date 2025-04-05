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
	"log/slog"
	"net"
	"os"

	"github.com/alexandremahdhaoui/tooling/pkg/flaterrors"
	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/alexandremahdhaoui/udplb/internal/util"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"github.com/google/uuid"
)

// -------------------------------------------------------------------
// -- UDPLB (Interface)
// -------------------------------------------------------------------

type UDPLB interface {
	types.DoneCloser

	// Run udplb bpf program.
	Run(ctx context.Context) error

	// Logs bpf traces to stdout.
	TraceBPF() error
}

// -------------------------------------------------------------------
// -- udplb (concrete implementation)
// -------------------------------------------------------------------

type udplb struct {
	// -- loadbalancer spec

	// Name of the loadbalancer.
	name string
	// Id of this loadbalancer's instance.
	id uuid.UUID
	// Network interface the XDP program will attach to.
	iface *net.Interface

	// -- loadbalancer configuration.
	udplbConfigT udplbConfigT

	// -- bpf

	objs *udplbObjects

	// -- mgmt

	running     bool
	terminateCh chan struct{}
	doneCh      chan struct{}
}

// -------------------------------------------------------------------
// -- New
// -------------------------------------------------------------------

var ErrCannotCreateNewUDPLBAdapter = errors.New("cannot create new udplb adapter")

func New(
	name string,
	id uuid.UUID,
	iface *net.Interface,
	ip net.IP,
	port uint16,
	lookupTableLength uint32,
) (UDPLB, error) {
	return &udplb{
		name: name,
		udplbConfigT: udplbConfigT{
			Ip:              util.NetIPv4ToUint32(ip),
			Port:            port,
			LookupTableSize: lookupTableLength,
		},
		iface:       iface,
		objs:        new(udplbObjects),
		running:     false,
		terminateCh: make(chan struct{}),
		doneCh:      make(chan struct{}),
	}, nil
}

// Close implements UDPLB.
func (lb *udplb) Close() error {
	if !lb.running {
		return flaterrors.Join(ErrCannotTerminateDSManagerIfNotStarted, ErrClosingDSManager)
	}

	// Triggers termination of the event loop
	close(lb.terminateCh)
	// Await graceful termination
	<-lb.doneCh

	return nil
}

// Done implements UDPLB.
func (lb *udplb) Done() <-chan struct{} {
	return lb.doneCh
}

// -------------------------------------------------------------------
// -- RUN
// -------------------------------------------------------------------

var (
	ErrRemovingMemlock             = errors.New("removing memlock")
	ErrRunningUdplbUserlandProgram = errors.New("running udplb userland program")
)

func (lb *udplb) Run(ctx context.Context) error {
	// Remove resource limits for kernels <5.11.
	if err := rlimit.RemoveMemlock(); err != nil {
		return flaterrors.Join(err, ErrRemovingMemlock, ErrRunningUdplbUserlandProgram)
	}

	// -- Init constants

	spec, err := loadUdplb()
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	if err := spec.Variables["config"].Set(lb.udplbConfigT); err != nil {
		return flaterrors.Join(err, ErrRunningUdplbUserlandProgram)
	}

	// -- Load ebpf elf into kernel

	if err = spec.LoadAndAssign(lb.objs, nil); err != nil {
		return flaterrors.Join(err, ErrRunningUdplbUserlandProgram)
	}

	defer lb.objs.Close()

	// -- Attach udplb to iface

	link, err := link.AttachXDP(link.XDPOptions{
		Program:   lb.objs.Udplb,
		Interface: lb.iface.Index,
	})
	if err != nil {
		return flaterrors.Join(err, ErrRunningUdplbUserlandProgram)
	}

	lb.running = true
	slog.InfoContext(ctx, "XDP program loaded successfully", "ifname", lb.iface.Name)

	// TODO: receive updates of the new session assignment in order to propagate them
	// to other loadbalancers.

	<-lb.terminateCh
	_ = lb.objs.Close()
	_ = link.Close()
	close(lb.doneCh)

	return nil
}

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
