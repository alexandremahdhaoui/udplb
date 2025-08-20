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
	"sync"

	"github.com/alexandremahdhaoui/tooling/pkg/flaterrors"
	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/alexandremahdhaoui/udplb/internal/util"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"github.com/google/uuid"
)

// -------------------------------------------------------------------
// -- runnable (concrete implementation)
// -------------------------------------------------------------------

// runnable is thread-safe and can be gracefully shut down.
//
// On Close() it will:
// - Propagate Close to its dependencies: [bpfadapter.DataStructureManager].
// - Close BPF objects
type runnable struct {
	// -- loadbalancer spec

	// Id of this loadbalancer's instance.
	id uuid.UUID
	// Network interface the XDP program will attach to.
	iface *net.Interface

	// -- bpf

	objs *udplbObjects

	// -- mgmt

	running              bool
	doneCh               chan struct{}
	closePropagationFunc func() error
	mu                   *sync.Mutex
}

// -------------------------------------------------------------------
// -- New
// -------------------------------------------------------------------

var ErrCannotCreateNewUDPLBAdapter = errors.New("cannot create new udplb adapter")

// New loads the bpf program into the kernel and returns 2 distinct
// data structures to achieve separation of concern between:
// - Running the bpf program itself.
// - Interacting with the bpf kernel data
// structures.
func New(
	id uuid.UUID,
	iface *net.Interface,
	ip net.IP,
	port uint16,
	lookupTableSize uint32,
	watcherMux *util.WatcherMux[types.Assignment],
) (types.Runnable, DataStructureManager, error) {
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, nil, flaterrors.Join(err, ErrRemovingMemlock, ErrCreatingNewBPFProgram)
	}

	cfg := udplbConfigT{
		Ip:              util.NetIPv4ToUint32(ip),
		Port:            port,
		LookupTableSize: lookupTableSize,
	}

	// -- load bpf program
	spec, err := loadUdplb()
	if err != nil {
		return nil, nil, flaterrors.Join(err, ErrCreatingNewBPFProgram)
	}

	// -- set config
	if err = spec.Variables["config"].Set(cfg); err != nil {
		return nil, nil, flaterrors.Join(err, ErrCreatingNewBPFProgram)
	}

	// -- Load ebpf elf into kernel
	bpfObjects := new(udplbObjects)
	if err = spec.LoadAndAssign(bpfObjects, nil); err != nil {
		return nil, nil, flaterrors.Join(err, ErrCreatingNewBPFProgram)
	}

	// -- init done channel
	doneCh := make(chan struct{})
	// -- init objects
	objs, err := newObjects(bpfObjects, doneCh)
	if err != nil {
		return nil, nil, flaterrors.Join(err, ErrCreatingNewBPFProgram)
	}

	// -- init & run manager
	manager := NewDataStructureManager(objs, watcherMux)
	closePropagationFunc := func() error {
		return manager.Close()
	}

	return &runnable{
		id:                   id,
		iface:                iface,
		objs:                 bpfObjects,
		running:              false,
		doneCh:               doneCh,
		closePropagationFunc: closePropagationFunc,
		mu:                   &sync.Mutex{},
	}, manager, nil
}

// -------------------------------------------------------------------
// -- RUN
// -------------------------------------------------------------------

var (
	ErrRemovingMemlock             = errors.New("removing memlock")
	ErrCreatingNewBPFProgram       = errors.New("creating new bpf program")
	ErrRunningUdplbUserlandProgram = errors.New("running udplb userland program")
)

// Run implements types.Runnable.
func (r *runnable) Run(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return nil // return nil to make Run idempotent.
	}

	// -- Attach udplb to iface

	link, err := link.AttachXDP(link.XDPOptions{
		Program:   r.objs.Udplb,
		Interface: r.iface.Index,
	})
	if err != nil {
		return flaterrors.Join(err, ErrRunningUdplbUserlandProgram)
	}
	defer func() {
		if err := link.Close(); err != nil {
			slog.ErrorContext(ctx, "Closing XDP link", "err", err)
		}
	}()

	r.running = true
	slog.InfoContext(ctx, "XDP program loaded successfully", "ifname", r.iface.Name)

	return nil
}

// -------------------------------------------------------------------
// -- TraceBPF
// -------------------------------------------------------------------

const tracepipeFilepath = "/sys/kernel/debug/tracing/trace_pipe"

func (r *runnable) TraceBPF() error {
	// -- print bpf trace logs
	fd, err := os.OpenFile(tracepipeFilepath, os.O_RDONLY, os.ModeAppend)
	if err != nil {
		return err
	}

	defer func() {
		if err := fd.Close(); err != nil {
			slog.Error(fmt.Sprintf("Closing %q file descriptor", tracepipeFilepath), "err", err)
		}
	}()
	go func() {
		scanner := bufio.NewScanner(fd)
		scanner.Split(bufio.ScanLines)
		for scanner.Scan() {
			_, _ = fmt.Fprintf(os.Stdout, "%s\n", scanner.Text())
		}
	}()

	return nil
}

// -------------------------------------------------------------------
// -- DoneCloser
// -------------------------------------------------------------------

// Close implements types.Runnable.
func (r *runnable) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running {
		return nil
	}

	var errs error
	for _, f := range []func() error{
		r.closePropagationFunc,
		r.objs.Close,
	} {
		if err := f(); err != nil {
			errs = flaterrors.Join(errs, err)
		}
	}
	close(r.doneCh)

	return errs
}

// Done implements types.Runnable.
func (r *runnable) Done() <-chan struct{} {
	return r.doneCh
}
