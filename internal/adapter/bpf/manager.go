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
	"log/slog"
	"sync"
	"time"

	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/alexandremahdhaoui/udplb/internal/util"

	"github.com/alexandremahdhaoui/tooling/pkg/flaterrors"
	"github.com/google/uuid"
)

var (
	ErrClosingDSManager = errors.New("closing bpf.DataStructureManager")

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

// DataStructureManager is thread-safe and can be gracefully shut down.
//
// Please note that closing the DataStructureManager does not close the
// underlying bpf data structures.
type DataStructureManager interface {
	types.Runnable

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

	// It returns a chan notifying new session assignments. It can be called
	// many times.
	//
	// However the user of WatchAssignment must update their
	// internal representation of the assignment/session map.
	//
	// Please note, there is no point in calling SessionBatchUpdate with
	// assignments obtained with this method, because the BPF program already
	// set them to the active map.
	WatchAssignment() <-chan types.Assignment

	// -- Session

	// Quickly update keys from the active session map.
	// NB: To mutate passive maps and trigger a switchover please use SetObjects.
	BatchUpdateSession(map[uuid.UUID]uint32) error
	// Quickly delete keys from the active session map.
	// NB: To mutate passive maps and trigger a switchover please use SetObjects.
	BatchDeleteSession(...uuid.UUID) error
}

// -------------------------------------------------------------------
// -- CONCRETE IMPLEMENTATION
// -------------------------------------------------------------------

type dsManager struct {
	ctx     context.Context
	objs    Objects
	eventCh chan *event

	// -- multiplexers

	assignmentWatcherMux *util.WatcherMux[types.Assignment]

	// -- mgmt
	running     bool
	closed      bool
	doneCh      chan struct{}
	terminateCh chan struct{}
	mu          *sync.Mutex
}

func NewDataStructureManager(
	objs Objects,
	assignmentWatcherMux *util.WatcherMux[types.Assignment],
) DataStructureManager {
	return &dsManager{
		ctx:                  nil, // must be set when Run is called.
		objs:                 objs,
		eventCh:              make(chan *event),
		running:              false,
		closed:               false,
		doneCh:               make(chan struct{}),
		terminateCh:          make(chan struct{}),
		mu:                   &sync.Mutex{},
		assignmentWatcherMux: assignmentWatcherMux,
	}
}

// -------------------------------------------------------------------
// -- Run
// -------------------------------------------------------------------

func (mgr *dsManager) Run(ctx context.Context) error {
	mgr.mu.Lock()
	if mgr.running {
		return types.ErrAlreadyRunning
	} else if mgr.closed {
		return types.ErrCannotRunClosedRunnable
	}
	mgr.mu.Unlock()

	// start subscribing to assignment.
	assignmentCh, err := mgr.objs.AssignmentFIFO.Subscribe()
	if err != nil {
		return err
	}

	// start loops
	mgr.mu.Lock()
	mgr.ctx = ctx
	mgr.running = true
	go mgr.eventLoop(assignmentCh)
	mgr.mu.Unlock()

	slog.InfoContext(ctx, "bpf data structure manager started successfully")
	return nil
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

// TODO: refactor event using a closure rather than pseudo typed events.
// - remove kind.
// - remove set, sessionBatch{Update,Delete}.
// - add closure f func() error
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

var (
	qSize               = 10
	watchWorkerPoolSize = 3 // events are executed sequentially
)

// This loop ensures that only one goroutine is updating the internal bpf data
// structures at a time. This synchronization pattern avoids using mutexes. Hence,
// we do not lock these datastructures, and changes are propagated as quickly as
// possible.
//
// Please note this function must be executed only once. If the struct was gracefully
// shut down and you want to start it again, then you must initialize another dsManager
// struct.
func (mgr *dsManager) eventLoop(assignmentCh <-chan Assignment) {
	// -- Define 2 worker pools to avoid competition b/w write events and watch.

	eventQ := make(chan func(), qSize)
	defer close(eventQ)
	eventDoneCh := util.NewWorkerPool(1, eventQ, mgr.terminateCh)

	watchQ := make(chan func(), qSize)
	defer close(watchQ)
	watchDoneCh := util.NewWorkerPool(watchWorkerPoolSize, watchQ, mgr.terminateCh)

	for {
		// -- mgmt
		select {
		default:
		case <-mgr.terminateCh:
			// return from the event loop if graceful shutdown signal is received.
			goto terminate
		}

		// -- events
		select {
		// receive a write event.
		case e, ok := <-mgr.eventCh:
			if !ok {
				go mgr.triggerCloseInternally("internal event channel closed unexpectedly")
				goto terminate
			}

			eventQ <- func() {
				mgr.handleEvent(e)
			}

		// watch assignments
		case in, ok := <-assignmentCh:
			if !ok {
				go mgr.triggerCloseInternally("FIFO channel closed while watching assignment")
				goto terminate
			}

			watchQ <- func() {
				mgr.assignmentWatcherMux.Dispatch(types.Assignment{
					BackendId: in.BackendId,
					SessionId: in.SessionId,
				})
			}
		// don't forget to await a termination signal here as well,
		// otherwise the event loop may hang for ever.
		case <-mgr.terminateCh:
			goto terminate
		}
	}

terminate:
	// -- await until worker pools are all done.
	<-eventDoneCh
	<-watchDoneCh

	// -- close mux channels after watch-workers are done,
	//    otherwise workers will send on closed channel.
	if err := mgr.assignmentWatcherMux.Close(); err != nil {
		slog.ErrorContext(mgr.ctx, "an error occured while closing DatastructureManager",
			"err", err.Error())
	}

	// -- signal this subsystem is done.
	close(mgr.doneCh)
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
// -- WATCH ASSIGNMENT
// -------------------------------------------------------------------

func (mgr *dsManager) WatchAssignment() <-chan types.Assignment {
	return mgr.assignmentWatcherMux.Watch()
}

// -------------------------------------------------------------------
// -- SESSION BATCH DELETE/UPDATE
// -------------------------------------------------------------------

func (mgr *dsManager) BatchDeleteSession(batch ...uuid.UUID) error {
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

func (mgr *dsManager) BatchUpdateSession(batch map[uuid.UUID]uint32) error {
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

func (mgr *dsManager) triggerCloseInternally(reason string) {
	slog.InfoContext(mgr.ctx, "shutting down DataStructureManager",
		"reason", reason)
	if err := mgr.Close(); err != nil {
		slog.ErrorContext(mgr.ctx, "an error occured while shutting down DataStructureManager",
			"err", err.Error())
	}
}

var closeTimeoutDuration = 5 * time.Second

// Close is idempotent.
// Close always returns nil.
// TODO: force closing by using timeout?
func (mgr *dsManager) Close() error {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	// -- handle errors
	if !mgr.running {
		return types.ErrRunnableMustBeRunningToBeClosed
	} else if mgr.closed {
		return types.ErrAlreadyClosed
	}

	// -- Triggers termination of the event loop
	close(mgr.terminateCh)

	// -- Await graceful termination in timely manner.
	toCh := time.After(closeTimeoutDuration)
	select {
	case <-mgr.doneCh:
	case <-toCh:
	}

	// -- Safely close the event channel.
	close(mgr.eventCh)

	// -- Persist information that the manager is not running anymore.
	mgr.running = false
	mgr.closed = true

	slog.InfoContext(mgr.ctx, "successfully shut down DataStructureManager")
	return nil
}

// Done implements DataStructureManager.
func (mgr *dsManager) Done() <-chan struct{} {
	return mgr.doneCh
}
