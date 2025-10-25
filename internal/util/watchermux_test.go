package util_test

import (
	"sync"
	"testing"
	"time"

	"github.com/alexandremahdhaoui/udplb/internal/util"
	"github.com/stretchr/testify/assert"
)

func TestWatcherMux(t *testing.T) {
	t.Run("should create a new watcher mux", func(t *testing.T) {
		wm := util.NewWatcherMux[int](util.WatcherMuxRecommendedBufferSize, util.NonBlockingDispatchFunc[int])
		assert.NotNil(t, wm)
	})

	t.Run("should dispatch values to a single watcher", func(t *testing.T) {
		wm := util.NewWatcherMux[int](util.WatcherMuxRecommendedBufferSize, util.NonBlockingDispatchFunc[int])
		ch, _ := wm.Watch(util.NoFilter)

		go func() {
			wm.Dispatch(1)
		}()

		select {
		case val := <-ch:
			assert.Equal(t, 1, val)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timed out waiting for value")
		}
	})

	t.Run("should dispatch values to multiple watchers", func(t *testing.T) {
		wm := util.NewWatcherMux[int](util.WatcherMuxRecommendedBufferSize, util.NonBlockingDispatchFunc[int])
		ch1, _ := wm.Watch(util.NoFilter)
		ch2, _ := wm.Watch(util.NoFilter)

		go func() {
			wm.Dispatch(1)
		}()

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			select {
			case val := <-ch1:
				assert.Equal(t, 1, val)
			case <-time.After(100 * time.Millisecond):
				t.Fatal("timed out waiting for value on ch1")
			}
		}()

		go func() {
			defer wg.Done()
			select {
			case val := <-ch2:
				assert.Equal(t, 1, val)
			case <-time.After(100 * time.Millisecond):
				t.Fatal("timed out waiting for value on ch2")
			}
		}()

		wg.Wait()
	})

	t.Run("should close watcher channels", func(t *testing.T) {
		wm := util.NewWatcherMux[int](util.WatcherMuxRecommendedBufferSize, util.NonBlockingDispatchFunc[int])
		ch, cancel := wm.Watch(util.NoFilter)

		cancel()

		select {
		case _, ok := <-ch:
			assert.False(t, ok)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("channel not closed")
		}
	})

	t.Run("should close all watcher channels when closed", func(t *testing.T) {
		wm := util.NewWatcherMux[int](util.WatcherMuxRecommendedBufferSize, util.NonBlockingDispatchFunc[int])
		ch1, _ := wm.Watch(util.NoFilter)
		ch2, _ := wm.Watch(util.NoFilter)

		wm.Close()

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			select {
			case _, ok := <-ch1:
				assert.False(t, ok)
			case <-time.After(100 * time.Millisecond):
				t.Fatal("ch1 not closed")
			}
		}()

		go func() {
			defer wg.Done()
			select {
			case _, ok := <-ch2:
				assert.False(t, ok)
			case <-time.After(100 * time.Millisecond):
				t.Fatal("ch2 not closed")
			}
		}()

		wg.Wait()
	})

	t.Run("should stop dispatching after close", func(t *testing.T) {
		wm := util.NewWatcherMux[int](util.WatcherMuxRecommendedBufferSize, util.NonBlockingDispatchFunc[int])
		ch, _ := wm.Watch(util.NoFilter)

		wm.Close()
		wm.Dispatch(1) // This should not panic.

		select {
		case _, ok := <-ch:
			assert.False(t, ok, "channel should be closed")
		case <-time.After(100 * time.Millisecond):
			t.Fatal("channel not closed")
		}
	})

	t.Run("should signal done when closed", func(t *testing.T) {
		wm := util.NewWatcherMux[int](util.WatcherMuxRecommendedBufferSize, util.NonBlockingDispatchFunc[int])
		doneCh := wm.Done()

		go func() {
			time.Sleep(50 * time.Millisecond)
			wm.Close()
		}()

		select {
		case <-doneCh:
			// Test passed
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Done channel was not closed on Close()")
		}
	})
}
