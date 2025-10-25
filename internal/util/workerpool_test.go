package util_test

import (
	"sync"
	"testing"
	"time"

	"github.com/alexandremahdhaoui/udplb/internal/util"
	"github.com/stretchr/testify/assert"
)

func TestWorkerPool(t *testing.T) {
	t.Run("should create a new worker pool", func(t *testing.T) {
		q := make(chan func())
		wp := util.NewWorkerPool(10, q)
		assert.NotNil(t, wp)
	})

	t.Run("should execute a task", func(t *testing.T) {
		q := make(chan func())
		wp := util.NewWorkerPool(10, q)
		defer wp.Close()

		var wg sync.WaitGroup
		wg.Add(1)

		taskExecuted := false
		q <- func() {
			taskExecuted = true
			wg.Done()
		}

		wg.Wait()
		assert.True(t, taskExecuted)
	})

	t.Run("should execute multiple tasks", func(t *testing.T) {
		q := make(chan func(), 100)
		wp := util.NewWorkerPool(10, q)
		defer wp.Close()

		var wg sync.WaitGroup
		taskCount := 100
		wg.Add(taskCount)

		var executionCount int
		var mu sync.Mutex

		for i := 0; i < taskCount; i++ {
			q <- func() {
				mu.Lock()
				executionCount++
				mu.Unlock()
				wg.Done()
			}
		}

		wg.Wait()
		assert.Equal(t, taskCount, executionCount)
	})

	t.Run("should terminate workers when queue is closed", func(t *testing.T) {
		q := make(chan func())
		wp := util.NewWorkerPool(10, q)
		doneCh := wp.Done()

		close(q)

		select {
		case <-doneCh:
			// Test passed
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Done channel was not closed after closing the queue")
		}
	})

	t.Run("should signal done when closed", func(t *testing.T) {
		q := make(chan func())
		wp := util.NewWorkerPool(10, q)
		doneCh := wp.Done()

		go func() {
			time.Sleep(50 * time.Millisecond)
			wp.Close()
		}()

		select {
		case <-doneCh:
			// Test passed
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Done channel was not closed on Close()")
		}
	})
}
