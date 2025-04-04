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
	"log/slog"
	"sync"

	"github.com/alexandremahdhaoui/tooling/pkg/flaterrors"
	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/alexandremahdhaoui/udplb/internal/util"
)

// -------------------------------------------------------------------
// -- DATA STRUCTURE MANAGER
// -------------------------------------------------------------------

type DataStructureManager interface {
	types.DoneCloser

	// Initialize and starts the DataStructureManager.
	Start() error

	// GetEventChannel returns a sender channel to manage bpf data structures.
	GetEventChannel() chan<- Event
}

func NewDataStructureManager(name string, objs Objects) DataStructureManager {
	mgr := &dsManager{
		name:              name,
		objs:              objs,
		started:           false,
		doneCh:            make(chan struct{}),
		eventCh:           make(chan Event),
		terminateCh:       make(chan struct{}),
		eventLoopOnceFunc: nil,
	}

	mgr.eventLoopOnceFunc = sync.OnceFunc(mgr.eventLoop)

	return mgr
}

// -------------------------------------------------------------------
// -- CONCRETE IMPLEMENTATION
// -------------------------------------------------------------------

type dsManager struct {
	name string
	objs Objects

	started     bool
	doneCh      chan struct{}
	eventCh     chan Event
	terminateCh chan struct{}

	eventLoopOnceFunc func()
}

var (
	ErrCannotTerminateDSManagerIfNotStarted = errors.New(
		"bpf.DataStructureManager must be started once before termination",
	)

	ErrClosingDSManager = errors.New("closing bpf.DataStructureManager")
)

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

// GetEventChannel implements DataStructureManager.
func (mgr *dsManager) GetEventChannel() chan<- Event {
	return mgr.eventCh
}

// Start implements DataStructureManager.
func (mgr *dsManager) Start() error {
	mgr.eventLoopOnceFunc()
	return nil
}

// -------------------------------------------------------------------
// -- eventLoop
// -------------------------------------------------------------------

var ErrUnknownEventKind = errors.New("unknown event kind")

// This loop ensures that only one goroutine is updating the internal bpf data
// structures at a time. This synchronization pattern avoids using mutexes. Hence,
// we do not lock these datastructures, and changes are propagated as quickly as
// possible.
//
// Please note this function must be executed only once. If the struct was gracefully
// shut down and you want to start it again, then you must initialize another struct.
func (mgr *dsManager) eventLoop() {
	for {
		var e Event

		select {
		// receive an event.
		case e = <-mgr.eventCh:
		// break the event loop if graceful shutdown signal is received.
		case _ = <-mgr.terminateCh:
			close(mgr.doneCh)
			break
		// also break the loop if the manager is set as "done" for any reason.
		case _ = <-mgr.doneCh:
			break
		}

		// perform event
		var err error
		switch e.Kind {
		default:
			err = ErrUnknownEventKind
			e.Kind = EventKindUnknown
		case EventKindSetObjects:
			if err = validateObjects(e.SetObjects); err != nil {
				break
			}
			err = retry(func() error { return mgr.setObjects(e.SetObjects) })
		}

		if err != nil {
			slog.Error(
				"an error occured while processing an event",
				"eventManagerKind", "bpf.DataStructureManager",
				"eventManagerName", mgr.name,
				"eventKind", e.Kind,
				"err", err.Error(),
			)
		}
	}
}

// -------------------------------------------------------------------
// -- setObjects
// -------------------------------------------------------------------

var (
	ErrEventObjectMustNotBeNil      = errors.New("event object must not be nil")
	ErrInvalidEventObjects          = errors.New("invalid struct EventObjects")
	ErrInvalidEventOfTypeSetObjects = errors.New("invalid event of type SetObjects")
)

func validateObjects(objs EventObjects) error {
	if util.AnyPtrIsNil(objs.Backends, objs.LookupTable, objs.Sessions) {
		return flaterrors.Join(
			ErrEventObjectMustNotBeNil,
			ErrInvalidEventObjects,
			ErrInvalidEventOfTypeSetObjects,
		)
	}

	return nil
}

func (mgr *dsManager) setObjects(objs EventObjects) error {
	backendsSwitchover, err := mgr.objs.Backends.SetAndDeferSwitchover(objs.Backends)
	if err != nil {
		return err
	}

	lupSwitchover, err := mgr.objs.LookupTable.SetAndDeferSwitchover(objs.LookupTable)
	if err != nil {
		return err
	}

	sessionsSwitchover, err := mgr.objs.Sessions.SetAndDeferSwitchover(objs.Sessions)
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
