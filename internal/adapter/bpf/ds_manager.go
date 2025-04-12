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

type Assignment = udplbAssignmentT

// -------------------------------------------------------------------
// -- DATA STRUCTURE MANAGER
// -------------------------------------------------------------------

// All method of DataStructureManager must be thread-safe
type DataStructureManager interface {
	types.DoneCloser

	// Overwrite existing objects with new objects.
	SetObjects(
		backendList []types.Backend,
		// The items refers to the index of a backend in the backendList.
		lookupTable []uint32,
		// The keys refers to a session id.
		// The values refers to the index of a backend in the backendList.
		sessionMap map[uuid.UUID]uint32,
	) error

	// -- Assignment

	// It returns a chan notifying new session assignments.
	//
	// There is no point in calling SessionBatchUpdate with
	// assignment obtains by this method, because the BPF program
	// already set them to the active map.
	//
	// However the user of AssignmentSubscribe must update their
	// internal representation of the assignment/session map.
	AssignmentSubscribe() (<-chan Assignment, error)

	// -- Session

	// Quickly update keys from the active session map.
	// NB: To mutate passive maps and trigger a switchover please use SetObjects.
	SessionBatchUpdate(map[uuid.UUID]uint32) error
	// Quickly delete keys from the active session map.
	// NB: To mutate passive maps and trigger a switchover please use SetObjects.
	SessionBatchDelete(...uuid.UUID) error
}

// -------------------------------------------------------------------
// -- CONCRETE IMPLEMENTATION
// -------------------------------------------------------------------

type dsManager struct {
	name string
	objs Objects

	eventCh chan *event

	running     bool
	doneCh      chan struct{}
	terminateCh chan struct{}
}

func NewDataStructureManager(name string, objs Objects) (DataStructureManager, error) {
	mgr := &dsManager{
		name: name,
		objs: objs,

		eventCh: make(chan *event),

		running:     false,
		doneCh:      make(chan struct{}),
		terminateCh: make(chan struct{}),
	}

	go mgr.eventLoop()
	mgr.running = true

	return mgr, nil
}

// -------------------------------------------------------------------
// -- eventLoop
// -------------------------------------------------------------------

// -- internal events

type eventKind string

const (
	eventKindUnknown            eventKind = "UNKNOWN"
	eventKindSet                eventKind = "Set"
	eventKindSessionBatchUpdate eventKind = "SessionBatchUpdate"
	eventKindSessionBatchDelete eventKind = "SessionBatchDelete"
)

// All objects must be set.
type eventObjects struct {
	BackendList []*udplbBackendSpecT
	LookupTable []uint32
	SessionMap  map[uuid.UUID]uint32
}

type event struct {
	kind  eventKind
	errCh chan error

	set                eventObjects
	sessionBatchUpdate map[uuid.UUID]uint32
	sessionBatchDelete []uuid.UUID // a list of session id to delete
}

func (e *event) Close() error {
	close(e.errCh)
	return nil
}

var (
	ErrInvalidObjectType     = errors.New("invalid object type")
	ErrCreatingInternalEvent = errors.New("creating internal event")
)

func newEvent(kind eventKind, v any) (*event, error) {
	var (
		e   event
		ok  bool
		err error
	)

	switch kind {
	default:
		err = ErrUnknownEventKind
	case eventKindSet:
		e.set, ok = v.(eventObjects)
	case eventKindSessionBatchUpdate:
		e.sessionBatchUpdate, ok = v.(map[uuid.UUID]uint32)
	case eventKindSessionBatchDelete:
		e.sessionBatchDelete, ok = v.([]uuid.UUID)
	}

	if err != nil {
		return nil, flaterrors.Join(err, ErrCreatingInternalEvent)
	}

	if !ok {
		return nil, flaterrors.Join(ErrInvalidObjectType, ErrCreatingInternalEvent)
	}

	return &event{
		kind:               kind,
		errCh:              make(chan error),
		set:                e.set,
		sessionBatchUpdate: e.sessionBatchUpdate,
		sessionBatchDelete: e.sessionBatchDelete,
	}, nil
}

// -- eventloop

// This loop ensures that only one goroutine is updating the internal bpf data
// structures at a time. This synchronization pattern avoids using mutexes. Hence,
// we do not lock these datastructures, and changes are propagated as quickly as
// possible.
//
// Please note this function must be executed only once. If the struct was gracefully
// shut down and you want to start it again, then you must initialize another dsManager
// struct.
func (mgr *dsManager) eventLoop() {
	for {
		// TODO: terminate eventLoop if "objs struct" is closed. (require objs.DoneCloser implemented)
		// case <- mgr.objs.Done():
		//     mgr.Close()
		//     return
		select {
		// receive an event.
		case e := <-mgr.eventCh:
			mgr.handleEvent(e)
		// break the event loop if graceful shutdown signal is received.
		case <-mgr.terminateCh:
			close(mgr.doneCh)
			return
		// also break the loop if the manager is set as "done" for any reason.
		case <-mgr.doneCh:
			return
		}
	}
}

func (mgr *dsManager) handleEvent(e *event) {
	defer e.Close()
	var err error

	switch e.kind {
	default:
		err = ErrUnknownEventKind
		e.kind = eventKindUnknown
	case eventKindSet:
		err = mgr.setObjects(e.set)
	case eventKindSessionBatchUpdate:
		err = mgr.assignmentBatchUpdate(e.sessionBatchUpdate)
	case eventKindSessionBatchDelete:
		err = mgr.assignmentBatchDelete(e.sessionBatchDelete)
	}

	if err != nil {
		e.errCh <- err
	}
}

// -- await event

// await until the operation is done or the manager is terminated.
func (mgr *dsManager) await(e *event) error {
	// await until the operation is done or the manager is terminated.
	select {
	case err := <-e.errCh:
		if err != nil {
			return err
		}
	case <-mgr.doneCh:
		return flaterrors.Join(
			ErrOperationAborted,
			ErrDSManagerIsShuttingDown,
		)
	}

	return nil
}

// -------------------------------------------------------------------
// -- SUBSCRIBE ASSIGNMENT
// -------------------------------------------------------------------

func (mgr *dsManager) AssignmentSubscribe() (<-chan Assignment, error) {
	ch, err := mgr.objs.AssignmentFIFO.Subscribe()
	if err != nil {
		return nil, err
	}
	return ch, nil
}

// -------------------------------------------------------------------
// -- SESSION BATCH DELETE/UPDATE
// -------------------------------------------------------------------

func (mgr *dsManager) SessionBatchDelete(batch ...uuid.UUID) error {
	e, err := newEvent(eventKindSessionBatchDelete, batch)
	if err != nil {
		return err
	}
	mgr.eventCh <- e
	if err := mgr.await(e); err != nil {
		return err
	}
	return nil
}

func (mgr *dsManager) assignmentBatchDelete(batch []uuid.UUID) error {
	if err := mgr.objs.SessionMap.BatchDelete(batch); err != nil {
		return err
	}
	return nil
}

func (mgr *dsManager) SessionBatchUpdate(batch map[uuid.UUID]uint32) error {
	e, err := newEvent(eventKindSessionBatchUpdate, batch)
	if err != nil {
		return err
	}
	mgr.eventCh <- e
	if err := mgr.await(e); err != nil {
		return err
	}
	return nil
}

func (mgr *dsManager) assignmentBatchUpdate(batch map[uuid.UUID]uint32) error {
	if err := mgr.objs.SessionMap.BatchUpdate(batch); err != nil {
		return err
	}
	return nil
}

// -------------------------------------------------------------------
// -- Set
// -------------------------------------------------------------------

func (mgr *dsManager) SetObjects(
	backendList []types.Backend,
	lookupTable []uint32,
	sessionMap map[uuid.UUID]uint32,
) error {
	if util.AnyPtrIsNil(backendList, lookupTable, sessionMap) {
		return flaterrors.Join(
			ErrObjectsMustNotBeNil,
			ErrInvalidArguments,
			ErrSettingObjectsDSManager,
		)
	}

	e, err := newEvent(eventKindSet, eventObjects{
		BackendList: transformBackendList(backendList),
		LookupTable: lookupTable,
		SessionMap:  sessionMap,
	})
	if err != nil {
		return flaterrors.Join(err, ErrSettingObjectsDSManager)
	}

	// send event.
	mgr.eventCh <- e
	if err := mgr.await(e); err != nil {
		return flaterrors.Join(err, ErrSettingObjectsDSManager)
	}

	return nil
}

func (mgr *dsManager) setObjects(objs eventObjects) error {
	bSwitchover, err := mgr.objs.BackendList.SetAndDeferSwitchover(objs.BackendList)
	if err != nil {
		return err
	}

	lupSwitchover, err := mgr.objs.LookupTable.SetAndDeferSwitchover(objs.LookupTable)
	if err != nil {
		return err
	}

	sSwitchover, err := mgr.objs.SessionMap.SetAndDeferSwitchover(objs.SessionMap)
	if err != nil {
		return err
	}

	bSwitchover()
	lupSwitchover()
	sSwitchover()

	return nil
}

func transformBackendList(in []types.Backend) (out []*udplbBackendSpecT) {
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
// -- DoneCloser
// -------------------------------------------------------------------

// Close is idempotent.
// Close always returns nil.
// TODO: force closing by using timeout?
func (mgr *dsManager) Close() error {
	if !mgr.running {
		return nil
	}

	// Triggers termination of the event loop
	close(mgr.terminateCh)
	// Await graceful termination
	<-mgr.doneCh
	// Safely close the event channel.
	close(mgr.eventCh)
	// Inform that the manager is not running anymore.
	mgr.running = false

	return nil
}

// Done implements DataStructureManager.
func (mgr *dsManager) Done() <-chan struct{} {
	return mgr.doneCh
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
