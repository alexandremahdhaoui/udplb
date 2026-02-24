//go:build unit

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
package dvds_test

import (
	"context"
	"testing"
	"time"

	statemachineadapter "github.com/alexandremahdhaoui/udplb/internal/adapter/statemachine"
	"github.com/alexandremahdhaoui/udplb/internal/controller/dvds"
	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/alexandremahdhaoui/udplb/internal/types/mocks"
	"github.com/alexandremahdhaoui/udplb/internal/util"

	"github.com/stretchr/testify/assert"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

/*******************************************************************************
 * Helper: create a DVDS with a map state machine for testing.
 *
 * T = Entry (key-value pair)
 * U = map[string]uint32 (state machine state)
 ******************************************************************************/

type Entry struct {
	Key   string
	Value uint32
}

func newTestDVDS(t *testing.T) (
	dvds.DVDS[Entry, map[string]uint32],
	*mocks.MockWAL[Entry],
	*mocks.MockWAL[map[string]uint32],
	chan []map[string]uint32,
) {
	t.Helper()

	transformFunc := func(obj Entry) (string, uint32, error) {
		return obj.Key, obj.Value, nil
	}

	stm, err := statemachineadapter.NewMap(transformFunc)
	require.NoError(t, err)

	// Wire up the command WAL mock with a real channel.
	cmdCh := make(chan []Entry, 10)
	cmdWAL := mocks.NewMockWAL[Entry](t)
	cmdWAL.EXPECT().Watch().Return((<-chan []Entry)(cmdCh), func() {}).Maybe()
	cmdWAL.EXPECT().Propose(testifymock.Anything).RunAndReturn(func(entry types.WALEntry[Entry]) error {
		// Simulate what the real WAL does: dispatch the data on the watch channel.
		cmdCh <- []Entry{entry.Data}
		return nil
	}).Maybe()
	cmdWAL.EXPECT().Close().Return(nil).Maybe()
	cmdWAL.EXPECT().Done().Return((<-chan struct{})(make(chan struct{}))).Maybe()
	cmdWAL.EXPECT().Run(testifymock.Anything).Return(nil).Maybe()

	// Wire up the snapshot WAL mock with a real channel.
	snapCh := make(chan []map[string]uint32, 10)
	snapshotWAL := mocks.NewMockWAL[map[string]uint32](t)
	snapshotWAL.EXPECT().Watch().Return((<-chan []map[string]uint32)(snapCh), func() {}).Maybe()
	snapshotWAL.EXPECT().Propose(testifymock.Anything).Return(nil).Maybe()
	snapshotWAL.EXPECT().Close().Return(nil).Maybe()
	snapshotWAL.EXPECT().Done().Return((<-chan struct{})(make(chan struct{}))).Maybe()
	snapshotWAL.EXPECT().Run(testifymock.Anything).Return(nil).Maybe()

	watcherMux := util.NewWatcherMux[map[string]uint32](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[map[string]uint32],
	)

	d := dvds.New[Entry, map[string]uint32](stm, cmdWAL, snapshotWAL, watcherMux)
	return d, cmdWAL, snapshotWAL, snapCh
}

/*******************************************************************************
 * Tests
 *
 ******************************************************************************/

func TestRunCloseLifecycle(t *testing.T) {
	d, _, _, _ := newTestDVDS(t)

	ctx := context.Background()

	// Run should succeed.
	err := d.Run(ctx)
	require.NoError(t, err)

	// Close should succeed.
	err = d.Close()
	require.NoError(t, err)

	// doneCh should be closed after Close returns.
	select {
	case <-d.Done():
	case <-time.After(time.Second):
		t.Fatal("doneCh was not closed after Close")
	}
}

func TestProposeAndWatchRoundTrip(t *testing.T) {
	d, _, _, _ := newTestDVDS(t)

	ctx := context.Background()
	err := d.Run(ctx)
	require.NoError(t, err)

	// Subscribe before proposing.
	ch, cancel := d.Watch()
	defer cancel()

	// Propose an entry.
	err = d.Propose(Entry{Key: "foo", Value: 42})
	require.NoError(t, err)

	// Read from watch channel: should get the state machine state after
	// applying the entry.
	select {
	case state := <-ch:
		assert.Equal(t, map[string]uint32{"foo": 42}, state)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for state on watch channel")
	}

	err = d.Close()
	require.NoError(t, err)
}

func TestMultipleProposalsAccumulate(t *testing.T) {
	d, _, _, _ := newTestDVDS(t)

	ctx := context.Background()
	err := d.Run(ctx)
	require.NoError(t, err)

	ch, cancel := d.Watch()
	defer cancel()

	// Propose 3 entries.
	entries := []Entry{
		{Key: "a", Value: 1},
		{Key: "b", Value: 2},
		{Key: "c", Value: 3},
	}

	for _, e := range entries {
		err = d.Propose(e)
		require.NoError(t, err)
	}

	// Collect dispatched states. Each proposal triggers a dispatch, so we
	// expect 3 state updates. The last one should contain all 3 keys.
	var lastState map[string]uint32
	for i := 0; i < 3; i++ {
		select {
		case state := <-ch:
			lastState = state
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for dispatch %d", i)
		}
	}

	assert.Equal(t, map[string]uint32{"a": 1, "b": 2, "c": 3}, lastState)

	err = d.Close()
	require.NoError(t, err)
}

func TestSnapshotOverridesState(t *testing.T) {
	d, _, _, snapCh := newTestDVDS(t)

	ctx := context.Background()
	err := d.Run(ctx)
	require.NoError(t, err)

	ch, cancel := d.Watch()
	defer cancel()

	// First, propose an entry via the command WAL.
	err = d.Propose(Entry{Key: "initial", Value: 100})
	require.NoError(t, err)

	// Wait for the command to be applied.
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for initial command")
	}

	// Now dispatch a snapshot via the snapshot WAL channel.
	// The snapshot is decoded via json.Unmarshal into the state machine's map.
	// NOTE: json.Unmarshal merges keys into existing maps rather than replacing
	// them. The snapshot updates existing keys and adds new ones, but does not
	// remove keys not present in the snapshot. This is a known limitation of
	// the state machine's Decode method.
	snapshot := map[string]uint32{"initial": 0, "override": 999}

	// Inject snapshot directly into the snapshot WAL's watch channel.
	snapCh <- []map[string]uint32{snapshot}

	// Wait for the snapshot to be applied.
	select {
	case state := <-ch:
		assert.Equal(t, map[string]uint32{"initial": 0, "override": 999}, state)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for snapshot state")
	}

	err = d.Close()
	require.NoError(t, err)
}

func TestErrorGuards(t *testing.T) {
	t.Run("double run returns ErrAlreadyRunning", func(t *testing.T) {
		d, _, _, _ := newTestDVDS(t)

		ctx := context.Background()
		err := d.Run(ctx)
		require.NoError(t, err)

		// Second Run should fail.
		err = d.Run(ctx)
		assert.ErrorIs(t, err, types.ErrAlreadyRunning)

		err = d.Close()
		require.NoError(t, err)
	})

	t.Run("close without run returns ErrRunnableMustBeRunningToBeClosed", func(t *testing.T) {
		d, _, _, _ := newTestDVDS(t)

		err := d.Close()
		assert.ErrorIs(t, err, types.ErrRunnableMustBeRunningToBeClosed)
	})

	t.Run("run after close returns ErrCannotRunClosedRunnable", func(t *testing.T) {
		d, _, _, _ := newTestDVDS(t)

		ctx := context.Background()
		err := d.Run(ctx)
		require.NoError(t, err)

		err = d.Close()
		require.NoError(t, err)

		// Run after close should fail.
		err = d.Run(ctx)
		assert.ErrorIs(t, err, types.ErrCannotRunClosedRunnable)
	})
}

func TestEventLoopTerminatesOnCmdChannelClose(t *testing.T) {
	transformFunc := func(obj Entry) (string, uint32, error) {
		return obj.Key, obj.Value, nil
	}

	stm, err := statemachineadapter.NewMap(transformFunc)
	require.NoError(t, err)

	// Create a cmd channel that we control and can close.
	cmdCh := make(chan []Entry, 10)
	cmdWAL := mocks.NewMockWAL[Entry](t)
	cmdWAL.EXPECT().Watch().Return((<-chan []Entry)(cmdCh), func() {}).Once()

	// Snapshot WAL with a channel that stays open.
	snapCh := make(chan []map[string]uint32, 10)
	snapshotWAL := mocks.NewMockWAL[map[string]uint32](t)
	snapshotWAL.EXPECT().Watch().Return((<-chan []map[string]uint32)(snapCh), func() {}).Once()

	watcherMux := util.NewWatcherMux[map[string]uint32](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[map[string]uint32],
	)

	d := dvds.New[Entry, map[string]uint32](stm, cmdWAL, snapshotWAL, watcherMux)

	err = d.Run(context.Background())
	require.NoError(t, err)

	// Close the cmd channel to trigger the "!ok" branch in the event loop.
	close(cmdCh)

	// The event loop should terminate, closing doneCh.
	select {
	case <-d.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("doneCh was not closed after cmd channel closed")
	}
}

func TestEventLoopTerminatesOnSnapChannelClose(t *testing.T) {
	transformFunc := func(obj Entry) (string, uint32, error) {
		return obj.Key, obj.Value, nil
	}

	stm, err := statemachineadapter.NewMap(transformFunc)
	require.NoError(t, err)

	// Cmd channel stays open.
	cmdCh := make(chan []Entry, 10)
	cmdWAL := mocks.NewMockWAL[Entry](t)
	cmdWAL.EXPECT().Watch().Return((<-chan []Entry)(cmdCh), func() {}).Once()

	// Create a snapshot channel that we control and can close.
	snapCh := make(chan []map[string]uint32, 10)
	snapshotWAL := mocks.NewMockWAL[map[string]uint32](t)
	snapshotWAL.EXPECT().Watch().Return((<-chan []map[string]uint32)(snapCh), func() {}).Once()

	watcherMux := util.NewWatcherMux[map[string]uint32](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[map[string]uint32],
	)

	d := dvds.New[Entry, map[string]uint32](stm, cmdWAL, snapshotWAL, watcherMux)

	err = d.Run(context.Background())
	require.NoError(t, err)

	// Close the snapshot channel to trigger the "!ok" branch in the event loop.
	close(snapCh)

	// The event loop should terminate, closing doneCh.
	select {
	case <-d.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("doneCh was not closed after snapshot channel closed")
	}
}
