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
package util

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// recvWithTimeout reads one value from ch or fails after timeout.
func recvWithTimeout[T any](t *testing.T, ch <-chan T, timeout time.Duration) T {
	t.Helper()
	select {
	case v, ok := <-ch:
		require.True(t, ok, "channel closed unexpectedly")
		return v
	case <-time.After(timeout):
		t.Fatal("timed out waiting to receive from channel")
		var zero T
		return zero
	}
}

// assertNoRecv asserts that no value arrives on ch within the given duration.
func assertNoRecv[T any](t *testing.T, ch <-chan T, wait time.Duration) {
	t.Helper()
	select {
	case v, ok := <-ch:
		if ok {
			t.Fatalf("unexpected value received: %v", v)
		}
	case <-time.After(wait):
		// expected: no value received
	}
}

// ---------------------------------------------------------------------------
// NewWatcherMux
// ---------------------------------------------------------------------------

func TestNewWatcherMux(t *testing.T) {
	wm := NewWatcherMux[int](5, NonBlockingDispatchFunc[int])
	require.NotNil(t, wm)
	assert.NotNil(t, wm.mu)
	assert.NotNil(t, wm.doneCh)
	assert.Equal(t, 5, wm.watcherChCapacity)
	assert.Len(t, wm.watchers, 0)
}

// ---------------------------------------------------------------------------
// Watch / cancel
// ---------------------------------------------------------------------------

func TestWatcherMux_WatchReturnsChannelAndCancel(t *testing.T) {
	wm := NewWatcherMux[int](5, NonBlockingDispatchFunc[int])

	ch, cancel := wm.Watch(NoFilter)
	require.NotNil(t, ch)
	require.NotNil(t, cancel)

	// There should be one watcher registered.
	assert.Len(t, wm.watchers, 1)

	// Cancel should remove the watcher.
	cancel()
	assert.Len(t, wm.watchers, 0)
}

func TestWatcherMux_CancelClosesChannel(t *testing.T) {
	wm := NewWatcherMux[int](5, NonBlockingDispatchFunc[int])

	ch, cancel := wm.Watch(NoFilter)
	cancel()

	// Channel should be closed.
	_, ok := <-ch
	assert.False(t, ok, "channel should be closed after cancel")
}

func TestWatcherMux_MultipleWatchers(t *testing.T) {
	wm := NewWatcherMux[int](5, NonBlockingDispatchFunc[int])

	ch1, cancel1 := wm.Watch(NoFilter)
	ch2, cancel2 := wm.Watch(NoFilter)
	ch3, cancel3 := wm.Watch(NoFilter)

	assert.Len(t, wm.watchers, 3)

	// Cancel middle watcher.
	cancel2()
	assert.Len(t, wm.watchers, 2)

	// ch2 should be closed.
	_, ok := <-ch2
	assert.False(t, ok)

	// ch1 and ch3 should still be open.
	cancel1()
	cancel3()
	_, ok = <-ch1
	assert.False(t, ok)
	_, ok = <-ch3
	assert.False(t, ok)
}

// ---------------------------------------------------------------------------
// Dispatch
// ---------------------------------------------------------------------------

func TestWatcherMux_DispatchSendsToAllWatchers(t *testing.T) {
	wm := NewWatcherMux[int](10, NonBlockingDispatchFunc[int])

	ch1, cancel1 := wm.Watch(NoFilter)
	defer cancel1()
	ch2, cancel2 := wm.Watch(NoFilter)
	defer cancel2()

	wm.Dispatch(42)

	v1 := recvWithTimeout(t, ch1, time.Second)
	v2 := recvWithTimeout(t, ch2, time.Second)
	assert.Equal(t, 42, v1)
	assert.Equal(t, 42, v2)
}

func TestWatcherMux_DispatchMultipleValues(t *testing.T) {
	wm := NewWatcherMux[string](10, NonBlockingDispatchFunc[string])

	ch, cancel := wm.Watch(NoFilter)
	defer cancel()

	wm.Dispatch("hello")
	wm.Dispatch("world")

	assert.Equal(t, "hello", recvWithTimeout(t, ch, time.Second))
	assert.Equal(t, "world", recvWithTimeout(t, ch, time.Second))
}

// ---------------------------------------------------------------------------
// Filter
// ---------------------------------------------------------------------------

func TestWatcherMux_FilterRejectsValues(t *testing.T) {
	wm := NewWatcherMux[int](10, NonBlockingDispatchFunc[int])

	// Only accept even numbers.
	evenFilter := FilterFunc(func(v any) bool {
		n, ok := v.(int)
		return ok && n%2 == 0
	})

	chFiltered, cancelFiltered := wm.Watch(evenFilter)
	defer cancelFiltered()

	chAll, cancelAll := wm.Watch(NoFilter)
	defer cancelAll()

	wm.Dispatch(1)
	wm.Dispatch(2)
	wm.Dispatch(3)
	wm.Dispatch(4)

	// Unfiltered watcher gets all values.
	assert.Equal(t, 1, recvWithTimeout(t, chAll, time.Second))
	assert.Equal(t, 2, recvWithTimeout(t, chAll, time.Second))
	assert.Equal(t, 3, recvWithTimeout(t, chAll, time.Second))
	assert.Equal(t, 4, recvWithTimeout(t, chAll, time.Second))

	// Filtered watcher only gets even numbers.
	assert.Equal(t, 2, recvWithTimeout(t, chFiltered, time.Second))
	assert.Equal(t, 4, recvWithTimeout(t, chFiltered, time.Second))

	// No more values for the filtered watcher.
	assertNoRecv(t, chFiltered, 50*time.Millisecond)
}

// ---------------------------------------------------------------------------
// NonBlockingDispatchFunc
// ---------------------------------------------------------------------------

func TestNonBlockingDispatchFunc_DoesNotBlock(t *testing.T) {
	// Buffer size 1 so the channel fills up immediately.
	wm := NewWatcherMux[int](1, NonBlockingDispatchFunc[int])

	ch, cancel := wm.Watch(NoFilter)
	defer cancel()

	// Fill the channel buffer.
	wm.Dispatch(1)

	// This dispatch should not block even though the channel is full.
	done := make(chan struct{})
	go func() {
		wm.Dispatch(2)
		close(done)
	}()

	select {
	case <-done:
		// NonBlockingDispatchFunc did not block.
	case <-time.After(time.Second):
		t.Fatal("NonBlockingDispatchFunc blocked on full channel")
	}

	// We should get the first value; the second was dropped.
	assert.Equal(t, 1, recvWithTimeout(t, ch, time.Second))
}

func TestNonBlockingDispatchFunc_SkipsClosedWatcher(t *testing.T) {
	wm := NewWatcherMux[int](10, NonBlockingDispatchFunc[int])

	_, cancel := wm.Watch(NoFilter)
	ch2, cancel2 := wm.Watch(NoFilter)
	defer cancel2()

	// Cancel first watcher before dispatching.
	cancel()

	// Dispatch should succeed without panic.
	require.NotPanics(t, func() {
		wm.Dispatch(99)
	})

	assert.Equal(t, 99, recvWithTimeout(t, ch2, time.Second))
}

// ---------------------------------------------------------------------------
// NewDispatchFuncWithTimeout
// ---------------------------------------------------------------------------

func TestNewDispatchFuncWithTimeout_Delivers(t *testing.T) {
	dispatchFn := NewDispatchFuncWithTimeout[int](time.Second)
	wm := NewWatcherMux[int](10, dispatchFn)

	ch, cancel := wm.Watch(NoFilter)
	defer cancel()

	wm.Dispatch(7)
	assert.Equal(t, 7, recvWithTimeout(t, ch, time.Second))
}

func TestNewDispatchFuncWithTimeout_TimesOutOnFullChannel(t *testing.T) {
	dispatchFn := NewDispatchFuncWithTimeout[int](50 * time.Millisecond)
	wm := NewWatcherMux[int](1, dispatchFn)

	ch, cancel := wm.Watch(NoFilter)
	defer cancel()

	// Fill channel.
	wm.Dispatch(1)

	// This should time out (not block indefinitely).
	done := make(chan struct{})
	go func() {
		wm.Dispatch(2)
		close(done)
	}()

	select {
	case <-done:
		// Timed out as expected.
	case <-time.After(5 * time.Second):
		t.Fatal("dispatch with timeout blocked indefinitely")
	}

	assert.Equal(t, 1, recvWithTimeout(t, ch, time.Second))
}

func TestNewDispatchFuncWithTimeout_FilterApplied(t *testing.T) {
	dispatchFn := NewDispatchFuncWithTimeout[int](time.Second)
	wm := NewWatcherMux[int](10, dispatchFn)

	rejectAll := FilterFunc(func(v any) bool {
		return false
	})

	ch, cancel := wm.Watch(rejectAll)
	defer cancel()

	wm.Dispatch(42)

	// Filter rejects everything -- nothing should be received.
	assertNoRecv(t, ch, 100*time.Millisecond)
}

// ---------------------------------------------------------------------------
// NewDispatchFuncWithTimeoutAndWorkerPool
// ---------------------------------------------------------------------------

func TestNewDispatchFuncWithTimeoutAndWorkerPool(t *testing.T) {
	workerQ := make(chan func(), 100)
	terminateCh := make(chan struct{})
	workerDone := NewFunctionalWorkerPool(2, workerQ, terminateCh)

	dispatchFn := NewDispatchFuncWithTimeoutAndWorkerPool[int](workerQ, time.Second)
	wm := NewWatcherMux[int](10, dispatchFn)

	ch, cancel := wm.Watch(NoFilter)
	defer cancel()

	wm.Dispatch(55)

	assert.Equal(t, 55, recvWithTimeout(t, ch, time.Second))

	close(terminateCh)
	select {
	case <-workerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("worker pool did not shut down")
	}
}

// ---------------------------------------------------------------------------
// Close / Done
// ---------------------------------------------------------------------------

func TestWatcherMux_CloseClosesAllWatcherChannels(t *testing.T) {
	wm := NewWatcherMux[int](5, NonBlockingDispatchFunc[int])

	ch1, _ := wm.Watch(NoFilter)
	ch2, _ := wm.Watch(NoFilter)

	err := wm.Close()
	require.NoError(t, err)

	// Both watcher channels should be closed.
	_, ok1 := <-ch1
	_, ok2 := <-ch2
	assert.False(t, ok1, "ch1 should be closed after Close()")
	assert.False(t, ok2, "ch2 should be closed after Close()")
}

func TestWatcherMux_DoneClosesAfterClose(t *testing.T) {
	wm := NewWatcherMux[int](5, NonBlockingDispatchFunc[int])

	doneCh := wm.Done()

	// Done should not be closed yet.
	select {
	case <-doneCh:
		t.Fatal("Done channel should not be closed before Close()")
	default:
		// expected
	}

	err := wm.Close()
	require.NoError(t, err)

	select {
	case <-doneCh:
		// Done closed as expected.
	case <-time.After(time.Second):
		t.Fatal("Done channel not closed after Close()")
	}
}

// ---------------------------------------------------------------------------
// Concurrent watchers receiving the same dispatch
// ---------------------------------------------------------------------------

func TestWatcherMux_ConcurrentWatchersReceiveSameDispatch(t *testing.T) {
	wm := NewWatcherMux[int](10, NonBlockingDispatchFunc[int])

	const numWatchers = 10
	channels := make([]<-chan int, numWatchers)
	cancels := make([]func(), numWatchers)

	for i := range numWatchers {
		channels[i], cancels[i] = wm.Watch(NoFilter)
	}
	defer func() {
		for _, cancel := range cancels {
			cancel()
		}
	}()

	wm.Dispatch(100)

	var wg sync.WaitGroup
	wg.Add(numWatchers)
	results := make([]int, numWatchers)

	for i := range numWatchers {
		go func(idx int) {
			defer wg.Done()
			results[idx] = recvWithTimeout(t, channels[idx], time.Second)
		}(i)
	}

	wg.Wait()

	for i, v := range results {
		assert.Equal(t, 100, v, "watcher %d did not receive dispatched value", i)
	}
}

// ---------------------------------------------------------------------------
// getWatcherList
// ---------------------------------------------------------------------------

func TestWatcherMux_GetWatcherList(t *testing.T) {
	wm := NewWatcherMux[int](5, NonBlockingDispatchFunc[int])

	_, cancel1 := wm.Watch(NoFilter)
	defer cancel1()
	_, cancel2 := wm.Watch(NoFilter)
	defer cancel2()

	list := wm.getWatcherList()
	assert.Len(t, list, 2)
}
