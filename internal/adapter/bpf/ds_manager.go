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
	"errors"
	"sync"

	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/alexandremahdhaoui/udplb/internal/util"

	"github.com/alexandremahdhaoui/tooling/pkg/flaterrors"
	"github.com/google/uuid"
)

var (
	ErrClosingDSManager = errors.New("closing bpf.DataStructureManager")

	ErrCannotTerminateDSManagerIfNotStarted = errors.New(
		"bpf.DataStructureManager must be started once before termination",
	)

	ErrObjectsMustNotBeNil     = errors.New("object must not be nil")
	ErrInvalidArguments        = errors.New("invalid arguments")
	ErrSettingObjectsDSManager = errors.New("setting objects with dsManager")

	ErrUnknownEventKind    = errors.New("unknown event kind")
	ErrInvalidEventObjects = errors.New("invalid struct EventObjects")

	ErrOperationAborted        = errors.New("operation aborted")
	ErrDSManagerIsShuttingDown = errors.New("dsManager is shutting down")
)

// -------------------------------------------------------------------
// -- DATA STRUCTURE MANAGER
// -------------------------------------------------------------------

type DataStructureManager interface {
	types.DoneCloser

	// Set is thread-safe.
	Set(
		backends []types.Backend,
		lookupTable []uint32,
		sessions map[uuid.UUID]uint32,
	) error
}

func NewDataStructureManager(name string, objs Objects) (DataStructureManager, error) {
	mgr := &dsManager{
		name:              name,
		objs:              objs,
		started:           false,
		doneCh:            make(chan struct{}),
		eventCh:           make(chan event),
		terminateCh:       make(chan struct{}),
		eventLoopOnceFunc: nil,
	}

	mgr.eventLoopOnceFunc = sync.OnceFunc(mgr.eventLoop)

	if err := mgr.Start(); err != nil {
		return nil, err
	}

	return mgr, nil
}

// -------------------------------------------------------------------
// -- CONCRETE IMPLEMENTATION
// -------------------------------------------------------------------

type dsManager struct {
	name string
	objs Objects

	started     bool
	doneCh      chan struct{}
	eventCh     chan event
	terminateCh chan struct{}

	eventLoopOnceFunc func()
}

// -------------------------------------------------------------------
// -- DoneCloser
// -------------------------------------------------------------------

// Close implements DataStructureManager.
func (mgr *dsManager) Close() error {
	if !mgr.started {
		return flaterrors.Join(ErrCannotTerminateDSManagerIfNotStarted, ErrClosingDSManager)
	}

	// Triggers termination of the event loop
	close(mgr.terminateCh)
	// Await graceful termination
	<-mgr.doneCh
	// Safely close the event channel.
	close(mgr.eventCh)

	return nil
}

// Done implements DataStructureManager.
func (mgr *dsManager) Done() <-chan struct{} {
	return mgr.doneCh
}

// -------------------------------------------------------------------
// -- Set
// -------------------------------------------------------------------

func (mgr *dsManager) Set(
	backends []types.Backend,
	lookupTable []uint32,
	sessions map[uuid.UUID]uint32,
) error {
	if util.AnyPtrIsNil(backends, lookupTable, sessions) {
		return flaterrors.Join(
			ErrObjectsMustNotBeNil,
			ErrInvalidArguments,
			ErrSettingObjectsDSManager,
		)
	}

	e := newEvent(eventKindSet, eventObjects{
		Backends:    TransformBackends(backends),
		LookupTable: lookupTable,
		Sessions:    sessions,
	})

	// send event.
	mgr.eventCh <- e

	// await until the operation is done or the manager is terminated.
	select {
	case err := <-e.errCh:
		if err != nil {
			return flaterrors.Join(err, ErrSettingObjectsDSManager)
		}
	case <-mgr.doneCh:
		return flaterrors.Join(
			ErrOperationAborted,
			ErrDSManagerIsShuttingDown,
			ErrSettingObjectsDSManager,
		)
	}

	return nil
}

func TransformBackends(in []types.Backend) (out []*udplbBackendSpecT) {
	out = make([]*udplbBackendSpecT, len(in))

	for i, b := range in {
		out[i] = &udplbBackendSpecT{
			Id:   b.Id,
			Ip:   util.NetIPv4ToUint32(b.Spec.IP),
			Port: uint16(b.Spec.Port),
			Mac:  [6]uint8(b.Spec.MacAddr),
		}
	}

	return
}

// -------------------------------------------------------------------
// -- Start
// -------------------------------------------------------------------

// Start implements DataStructureManager.
func (mgr *dsManager) Start() error {
	mgr.eventLoopOnceFunc()
	return nil
}

// -------------------------------------------------------------------
// -- eventLoop
// -------------------------------------------------------------------

// This loop ensures that only one goroutine is updating the internal bpf data
// structures at a time. This synchronization pattern avoids using mutexes. Hence,
// we do not lock these datastructures, and changes are propagated as quickly as
// possible.
//
// Please note this function must be executed only once. If the struct was gracefully
// shut down and you want to start it again, then you must initialize another struct.
func (mgr *dsManager) eventLoop() {
	for {
		var e event

		select {
		// receive an event.
		case e = <-mgr.eventCh:
		// break the event loop if graceful shutdown signal is received.
		case <-mgr.terminateCh:
			close(mgr.doneCh)
			break
		// also break the loop if the manager is set as "done" for any reason.
		case <-mgr.doneCh:
			break
		}

		// perform event
		var err error
		switch e.kind {
		default:
			err = ErrUnknownEventKind
			e.kind = eventKindUnknown
		case eventKindSet:
			err = mgr.setObjects(e.set)
		}

		if err != nil {
			e.errCh <- err
		}

		close(e.errCh)
	}
}

// -------------------------------------------------------------------
// -- setObjects
// -------------------------------------------------------------------

func (mgr *dsManager) setObjects(objs eventObjects) error {
	backendsSwitchover, err := mgr.objs.BackendList.SetAndDeferSwitchover(objs.Backends)
	if err != nil {
		return err
	}

	lupSwitchover, err := mgr.objs.LookupTable.SetAndDeferSwitchover(objs.LookupTable)
	if err != nil {
		return err
	}

	sessionsSwitchover, err := mgr.objs.SessionMap.SetAndDeferSwitchover(objs.Sessions)
	if err != nil {
		return err
	}

	backendsSwitchover()
	lupSwitchover()
	sessionsSwitchover()

	return nil
}

// -------------------------------------------------------------------
// -- HELPERS
// -------------------------------------------------------------------

const maxRetries = 3

var ErrExceededMaxRetries = errors.New("exceeded maximum retries")

func retry(f func() error) error {
	var errs error
	for range maxRetries {
		if err := f(); err != nil {
			errs = flaterrors.Join(err, errs)
		}
	}
	return flaterrors.Join(ErrExceededMaxRetries, errs)
}

// -------------------------------------------------------------------
// -- EVENT
// -------------------------------------------------------------------

func newEvent(kind eventKind, obj eventObjects) event {
	e := event{
		kind:  kind,
		errCh: make(chan error),
	}

	switch kind {
	default:
		panic("unknown event")
	case eventKindSet:
		e.set = obj
	}

	return e
}

type event struct {
	kind  eventKind
	set   eventObjects
	errCh chan error
}

type eventKind string

const (
	eventKindUnknown eventKind = "UNKNOWN"
	eventKindSet     eventKind = "Set"
)

// All objects must be set.
type eventObjects struct {
	Backends    []*udplbBackendSpecT
	LookupTable []uint32
	Sessions    map[uuid.UUID]uint32
}
