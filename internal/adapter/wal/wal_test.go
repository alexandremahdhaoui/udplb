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
package waladapter_test

import (
	"context"
	"testing"
	"time"

	waladapter "github.com/alexandremahdhaoui/udplb/internal/adapter/wal"
	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/alexandremahdhaoui/udplb/internal/types/mocks"
	"github.com/alexandremahdhaoui/udplb/internal/util"

	testifymock "github.com/stretchr/testify/mock"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*******************************************************************************
 * newTestWAL builds a WAL using a generated testify mock cluster.
 *
 ******************************************************************************/

func newTestWAL(t *testing.T) types.WAL[string] {
	cluster := mocks.NewMockCluster[string](t)

	// The WAL's single-node MVP event loop does not call any cluster methods.
	// Set up expectations with Maybe() so testify does not fail if they are
	// never called, but the mock is ready if the implementation changes.
	cluster.EXPECT().Run(testifymock.Anything).Return(nil).Maybe()
	cluster.EXPECT().Close().Return(nil).Maybe()
	cluster.EXPECT().Done().Return((<-chan struct{})(make(chan struct{}))).Maybe()
	cluster.EXPECT().Send(testifymock.Anything).Return(nil).Maybe()
	cluster.EXPECT().Recv().Return((<-chan string)(make(chan string)), func() {}).Maybe()
	cluster.EXPECT().Join().Return(nil).Maybe()
	cluster.EXPECT().Leave().Return(nil).Maybe()
	cluster.EXPECT().ListNodes().Return(nil).Maybe()

	watcherMux := util.NewWatcherMux[[]string](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[[]string],
	)
	return waladapter.New[string]("test-wal", cluster, 100, watcherMux)
}

/*******************************************************************************
 * Tests
 *
 ******************************************************************************/

func TestRunCloseLifecycle(t *testing.T) {
	w := newTestWAL(t)

	ctx := context.Background()

	// Run should succeed.
	err := w.Run(ctx)
	require.NoError(t, err)

	// Close should succeed.
	err = w.Close()
	require.NoError(t, err)

	// doneCh should be closed after Close returns.
	select {
	case <-w.Done():
	case <-time.After(time.Second):
		t.Fatal("doneCh was not closed after Close")
	}
}

func TestProposeAndWatchRoundTrip(t *testing.T) {
	w := newTestWAL(t)

	ctx := context.Background()
	err := w.Run(ctx)
	require.NoError(t, err)

	// Subscribe before proposing.
	ch, cancel := w.Watch()
	defer cancel()

	// Propose an entry.
	proposal := types.WALEntry[string]{
		Key:       "key-1",
		Data:      "hello",
		Verb:      types.PutCommand,
		Timestamp: time.Now(),
	}
	err = w.Propose(proposal)
	require.NoError(t, err)

	// Read from watch channel.
	select {
	case data := <-ch:
		require.Len(t, data, 1)
		assert.Equal(t, "hello", data[0])
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for data on watch channel")
	}

	err = w.Close()
	require.NoError(t, err)
}

func TestMultipleProposalsDispatchedInOrder(t *testing.T) {
	w := newTestWAL(t)

	ctx := context.Background()
	err := w.Run(ctx)
	require.NoError(t, err)

	ch, cancel := w.Watch()
	defer cancel()

	// Propose 3 entries.
	values := []string{"first", "second", "third"}
	for _, v := range values {
		err = w.Propose(types.WALEntry[string]{
			Key:       "key",
			Data:      v,
			Verb:      types.PutCommand,
			Timestamp: time.Now(),
		})
		require.NoError(t, err)
	}

	// Read all 3 dispatches in order.
	received := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		select {
		case data := <-ch:
			require.Len(t, data, 1)
			received = append(received, data[0])
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for dispatch %d", i)
		}
	}

	assert.Equal(t, values, received)

	err = w.Close()
	require.NoError(t, err)
}

func TestCloseWithEmptyProposalBuffer(t *testing.T) {
	w := newTestWAL(t)

	ctx := context.Background()
	err := w.Run(ctx)
	require.NoError(t, err)

	// Close immediately without proposing anything.
	// The event loop should exit cleanly via terminateCh.
	err = w.Close()
	require.NoError(t, err)

	select {
	case <-w.Done():
	case <-time.After(time.Second):
		t.Fatal("doneCh was not closed after Close")
	}
}

func TestErrorGuards(t *testing.T) {
	t.Run("double run returns ErrAlreadyRunning", func(t *testing.T) {
		w := newTestWAL(t)

		ctx := context.Background()
		err := w.Run(ctx)
		require.NoError(t, err)

		// Second Run should fail.
		err = w.Run(ctx)
		assert.ErrorIs(t, err, types.ErrAlreadyRunning)

		err = w.Close()
		require.NoError(t, err)
	})

	t.Run("close without run returns ErrRunnableMustBeRunningToBeClosed", func(t *testing.T) {
		w := newTestWAL(t)

		err := w.Close()
		assert.ErrorIs(t, err, types.ErrRunnableMustBeRunningToBeClosed)
	})

	t.Run("run after close returns ErrCannotRunClosedRunnable", func(t *testing.T) {
		w := newTestWAL(t)

		ctx := context.Background()
		err := w.Run(ctx)
		require.NoError(t, err)

		err = w.Close()
		require.NoError(t, err)

		// Run after close should fail.
		err = w.Run(ctx)
		assert.ErrorIs(t, err, types.ErrCannotRunClosedRunnable)
	})
}
