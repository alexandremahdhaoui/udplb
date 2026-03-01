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
package controller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	bpfadapter "github.com/alexandremahdhaoui/udplb/internal/adapter/bpf"
	monitoradapter "github.com/alexandremahdhaoui/udplb/internal/adapter/monitor"
	rltadapter "github.com/alexandremahdhaoui/udplb/internal/adapter/rlt"
	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/alexandremahdhaoui/udplb/internal/util"
	"github.com/google/uuid"
)

// -------------------------------------------------------------------
// -- Errors
// -------------------------------------------------------------------

var (
	ErrCreatingController = errors.New("creating controller")
	ErrInvalidConfig      = errors.New("invalid config")
)

const (
	defaultProbeInterval = 5 * time.Second
	defaultProbeTimeout  = 2 * time.Second
)

// -------------------------------------------------------------------
// -- Controller
// -------------------------------------------------------------------

// Controller wires the BPF adapter with the RLT computation and
// provides Run/Close lifecycle methods.
type Controller struct {
	config   types.Config
	backends []*types.Backend

	lookupTableSize uint32

	bpfProg types.Runnable
	bpfMgr  bpfadapter.DataStructureManager

	// Health monitoring
	mu            sync.Mutex
	sessionMap    map[uuid.UUID]uint32
	healthMux     *util.WatcherMux[types.BackendStatusEntry]
	healthMonitor types.Runnable
	healthCh      <-chan types.BackendStatusEntry
	healthCancel  func()
}

// New builds and wires the controller from a parsed config.
func New(config types.Config) (*Controller, error) {
	// -- Parse network interface.
	iface, err := net.InterfaceByName(config.Ifname)
	if err != nil {
		return nil, fmt.Errorf("%w: interface %q: %w", ErrCreatingController, config.Ifname, err)
	}

	// -- Parse IP.
	ip := net.ParseIP(config.IP)
	if ip == nil {
		return nil, fmt.Errorf("%w: %w: cannot parse IP %q", ErrCreatingController, ErrInvalidConfig, config.IP)
	}

	// -- Build backends from config.
	backends, err := buildBackends(config)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreatingController, err)
	}

	lookupTableSize := computeLookupTableSize(len(backends))

	// -- Build backend spec map for health monitor.
	backendSpecs := make(map[uuid.UUID]types.BackendSpec)
	for _, b := range backends {
		backendSpecs[b.Id] = b.Spec
	}

	// -- Create health monitor.
	healthMux := util.NewWatcherMux[types.BackendStatusEntry](
		100,
		util.NewDispatchFuncWithTimeout[types.BackendStatusEntry](time.Second),
	)
	healthMonitor := monitoradapter.NewBackendState(
		backendSpecs,
		defaultProbeInterval,
		defaultProbeTimeout,
		healthMux,
	)
	healthCh, healthCancel := healthMonitor.Watch()

	// -- Create assignment watcher mux.
	assignmentMux := util.NewWatcherMux[types.Assignment](
		100,
		util.NewDispatchFuncWithTimeout[types.Assignment](time.Second),
	)

	// -- Create BPF adapter.
	bpfProg, bpfMgr, err := bpfadapter.New(
		uuid.New(),
		iface,
		ip,
		config.Port,
		lookupTableSize,
		assignmentMux,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreatingController, err)
	}

	return &Controller{
		config:          config,
		backends:        backends,
		lookupTableSize: lookupTableSize,
		bpfProg:         bpfProg,
		bpfMgr:          bpfMgr,
		sessionMap:      make(map[uuid.UUID]uint32),
		healthMux:       healthMux,
		healthMonitor:   healthMonitor,
		healthCh:        healthCh,
		healthCancel:    healthCancel,
	}, nil
}

// -------------------------------------------------------------------
// -- Lifecycle
// -------------------------------------------------------------------

// Run starts the BPF program and the data structure manager, then
// computes and sets the initial lookup table.
func (c *Controller) Run(ctx context.Context) error {
	if err := c.bpfProg.Run(ctx); err != nil {
		return err
	}
	if err := c.bpfMgr.Run(ctx); err != nil {
		return err
	}

	go c.assignmentLoop(ctx)

	if err := c.healthMonitor.Run(ctx); err != nil {
		return err
	}
	go c.healthLoop(ctx)

	// -- Compute initial RLT and set BPF objects.
	// SetObjects must be called after bpfMgr.Run() because the
	// manager's event loop must be running to process the event.
	c.mu.Lock()
	if err := c.recomputeAndApply(); err != nil {
		c.mu.Unlock()
		return err
	}
	c.mu.Unlock()

	return nil
}

// Close shuts down the data structure manager then the BPF program
// (reverse order of startup).
func (c *Controller) Close() error {
	var errs error
	c.healthCancel()
	if err := c.healthMonitor.Close(); err != nil {
		errs = errors.Join(errs, err)
	}
	if err := c.bpfMgr.Close(); err != nil {
		errs = errors.Join(errs, err)
	}
	if err := c.bpfProg.Close(); err != nil {
		errs = errors.Join(errs, err)
	}
	return errs
}

// -------------------------------------------------------------------
// -- Loops
// -------------------------------------------------------------------

// assignmentLoop consumes assignment events from the BPF ring buffer
// and maintains the local sessionMap. Each assignment maps a session
// UUID to the index of its backend in c.backends.
func (c *Controller) assignmentLoop(ctx context.Context) {
	ch, cancel := c.bpfMgr.WatchAssignment()
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return
		case assignment, ok := <-ch:
			if !ok {
				return
			}
			c.mu.Lock()
			for i, b := range c.backends {
				if b.Id == assignment.BackendId {
					c.sessionMap[assignment.SessionId] = uint32(i)
					break
				}
			}
			c.mu.Unlock()
		}
	}
}

// healthLoop consumes health status events and triggers BPF map
// updates when a backend's state changes.
func (c *Controller) healthLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case entry, ok := <-c.healthCh:
			if !ok {
				return
			}
			c.mu.Lock()
			var changed bool
			for _, b := range c.backends {
				if b.Id == entry.BackendId {
					if b.Status.State != entry.State {
						b.Status.State = entry.State
						changed = true
					}
					break
				}
			}
			if changed {
				if err := c.recomputeAndApply(); err != nil {
					slog.ErrorContext(ctx, "failed to recompute and apply after health change",
						"err", err.Error())
				}
			}
			c.mu.Unlock()
		}
	}
}

// recomputeAndApply filters available backends, recomputes the RLT,
// remaps the session map, and calls SetObjects. Must be called with
// c.mu held.
func (c *Controller) recomputeAndApply() error {
	available := filterAvailable(c.backends)
	if len(available) == 0 {
		slog.Warn("no available backends -- skipping SetObjects")
		return nil
	}

	lookupTable := rltadapter.ReverseCoordinatesLookupTable(available, c.lookupTableSize)

	newIndexByID := make(map[uuid.UUID]uint32, len(available))
	for i, b := range available {
		newIndexByID[b.Id] = uint32(i)
	}

	remapped := make(map[uuid.UUID]uint32, len(c.sessionMap))
	for sessionID, oldIdx := range c.sessionMap {
		if oldIdx >= uint32(len(c.backends)) {
			continue
		}
		oldBackend := c.backends[oldIdx]
		if newIdx, ok := newIndexByID[oldBackend.Id]; ok {
			remapped[sessionID] = newIdx
		}
	}
	c.sessionMap = remapped

	backendValues := make([]types.Backend, len(available))
	for i, b := range available {
		backendValues[i] = *b
	}

	return c.bpfMgr.SetObjects(backendValues, lookupTable, c.sessionMap)
}

// -------------------------------------------------------------------
// -- Helpers
// -------------------------------------------------------------------

// buildBackends converts BackendConfig entries into Backend pointers.
// Only enabled backends are included.
func buildBackends(config types.Config) ([]*types.Backend, error) {
	var out []*types.Backend
	for i, bc := range config.Backends {
		if !bc.Enabled {
			continue
		}

		ip := net.ParseIP(bc.IP)
		if ip == nil {
			return nil, fmt.Errorf("%w: backend[%d]: cannot parse IP %q", ErrInvalidConfig, i, bc.IP)
		}

		mac, err := net.ParseMAC(bc.MAC)
		if err != nil {
			return nil, fmt.Errorf("%w: backend[%d]: %w", ErrInvalidConfig, i, err)
		}

		// Deterministic UUID from namespace + backend IP:port string.
		id := uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("%s:%d", bc.IP, bc.Port)))

		b := types.NewBackend(id, types.BackendSpec{
			IP:      ip,
			Port:    bc.Port,
			MacAddr: mac,
			State:   types.StateAvailable,
		}, types.BackendStatus{
			State: types.StateAvailable,
		})

		out = append(out, b)
	}
	return out, nil
}

// lookupTablePrimes is a list of primes used for lookup table sizing.
var lookupTablePrimes = []uint32{7, 13, 23, 47, 97, 197, 397, 797}

// computeLookupTableSize picks the smallest prime from lookupTablePrimes
// that is >= 2*n. Falls back to the largest prime if none qualifies.
func computeLookupTableSize(n int) uint32 {
	target := uint32(2 * n)
	for _, p := range lookupTablePrimes {
		if p >= target {
			return p
		}
	}
	return lookupTablePrimes[len(lookupTablePrimes)-1]
}

// filterAvailable returns backends whose desired state (Spec.State)
// and actual state (Status.State) are both StateAvailable.
func filterAvailable(backends []*types.Backend) []*types.Backend {
	var out []*types.Backend
	for _, b := range backends {
		if b.Spec.State == types.StateAvailable && b.Status.State == types.StateAvailable {
			out = append(out, b)
		}
	}
	return out
}
