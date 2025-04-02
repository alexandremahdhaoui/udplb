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
	"time"

	"github.com/alexandremahdhaoui/tooling/pkg/flaterrors"
	"github.com/alexandremahdhaoui/udplb/internal/types"
)

type Event struct {
	// TODO
}

type DataStructureManager interface {
	types.DoneCloser

	// Initialize and starts the DataStructureManager.
	Start() error

	// GetEventChannel returns a sender channel to manage bpf data structures.
	GetEventChannel() chan<- Event
}

// -------------------------------------------------------------------
// -- NEW
// -------------------------------------------------------------------

func NewDataStructureManager(objs Objects) DataStructureManager {
	mgr := &dsManager{
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
	// TODO
	objs Objects

	started     bool
	doneCh      chan struct{}
	eventCh     chan Event
	terminateCh chan struct{}

	eventLoopOnceFunc         func()
	eventLoopDebounceDuration time.Duration
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

// This loop ensures that only one goroutine is updating the internal and bpf data
// structures at a time. This synchronization pattern avoids using mutexes. Hence,
// we do not lock these datastructures, and changes are propagated as quickly as
// possible.
//
// Please note this function must be executed only once. If the struct was gracefully
// shut down and you want to start it again, then you must initialize another struct.
//
// TODO: THIS FUNCTION SHOULD CHECK SEMANTIC MUTATION OF ALL INTERNAL DATA STRUCTURES.
func (mgr *dsManager) eventLoop() {
	for {
		var e Event

		select {
		// receive an event.
		case e = <-mgr.eventCh:
		// break the event loop for graceful shutdown.
		case _ = <-mgr.terminateCh:
			close(mgr.doneCh)
			break
		case _ = <-mgr.doneCh:
			break
		}

		semanticMutation := false
		// perform event
		switch e.Type {
		case eventTypeDelete:
		case eventTypePut:
		case eventTypeReset:
		}

		// Skip sync if the above changes implies no semantic change such as deleting
		// a backend that was previously in state Unavailable.
		if !semanticMutation {
			continue
		}

		// sync datastructures
		// use flags to figure out which data structures needs to be synced?
		mgr.sync()
	}
}
