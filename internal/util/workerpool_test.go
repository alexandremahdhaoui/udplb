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
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// NewFunctionalWorkerPool
// ---------------------------------------------------------------------------

func TestNewFunctionalWorkerPool_ExecutesFunctions(t *testing.T) {
	q := make(chan func(), 10)
	terminateCh := make(chan struct{})

	doneCh := NewFunctionalWorkerPool(2, q, terminateCh)

	var counter atomic.Int64
	const jobs = 20

	for range jobs {
		q <- func() {
			counter.Add(1)
		}
	}

	// Close the queue to signal workers to finish after draining.
	close(q)

	select {
	case <-doneCh:
		assert.Equal(t, int64(jobs), counter.Load())
	case <-time.After(5 * time.Second):
		t.Fatal("workers did not finish in time")
	}
}

func TestNewFunctionalWorkerPool_TerminateChStopsWorkers(t *testing.T) {
	q := make(chan func(), 100)
	terminateCh := make(chan struct{})

	var counter atomic.Int64

	// Enqueue some jobs first.
	const jobs = 5
	for range jobs {
		q <- func() {
			counter.Add(1)
		}
	}

	doneCh := NewFunctionalWorkerPool(2, q, terminateCh)

	// Give workers time to process queued jobs.
	time.Sleep(50 * time.Millisecond)

	// Signal termination.
	close(terminateCh)

	select {
	case <-doneCh:
		// Workers terminated. They should have processed the queued jobs.
		assert.GreaterOrEqual(t, counter.Load(), int64(jobs))
	case <-time.After(5 * time.Second):
		t.Fatal("workers did not terminate in time")
	}
}

func TestNewFunctionalWorkerPool_TerminateDrainsRemaining(t *testing.T) {
	q := make(chan func(), 100)
	terminateCh := make(chan struct{})

	var counter atomic.Int64

	// Enqueue jobs.
	const jobs = 10
	for range jobs {
		q <- func() {
			counter.Add(1)
		}
	}

	doneCh := NewFunctionalWorkerPool(1, q, terminateCh)

	// Immediately signal termination -- the worker should drain the queue.
	close(terminateCh)

	select {
	case <-doneCh:
		assert.Equal(t, int64(jobs), counter.Load())
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not drain remaining jobs")
	}
}

func TestNewFunctionalWorkerPool_MultipleWorkersConcurrent(t *testing.T) {
	q := make(chan func(), 100)
	terminateCh := make(chan struct{})

	var counter atomic.Int64
	const jobs = 100

	doneCh := NewFunctionalWorkerPool(4, q, terminateCh)

	for range jobs {
		q <- func() {
			counter.Add(1)
		}
	}

	close(q)

	select {
	case <-doneCh:
		assert.Equal(t, int64(jobs), counter.Load())
	case <-time.After(5 * time.Second):
		t.Fatal("workers did not finish in time")
	}
}

// ---------------------------------------------------------------------------
// WorkerPool (struct wrapper)
// ---------------------------------------------------------------------------

func TestNewWorkerPool_Lifecycle(t *testing.T) {
	q := make(chan func(), 10)
	wp := NewWorkerPool(2, q)
	require.NotNil(t, wp)

	var counter atomic.Int64
	const jobs = 10

	var wg sync.WaitGroup
	wg.Add(jobs)
	for range jobs {
		q <- func() {
			defer wg.Done()
			counter.Add(1)
		}
	}

	// Wait for all jobs to complete.
	wg.Wait()

	// Close the worker pool.
	err := wp.Close()
	assert.NoError(t, err)

	select {
	case <-wp.Done():
		assert.Equal(t, int64(jobs), counter.Load())
	case <-time.After(5 * time.Second):
		t.Fatal("WorkerPool.Done() did not close in time")
	}
}

func TestNewWorkerPool_CloseAndDone(t *testing.T) {
	q := make(chan func(), 10)
	wp := NewWorkerPool(1, q)

	// Immediately close.
	err := wp.Close()
	assert.NoError(t, err)

	select {
	case <-wp.Done():
		// Done channel closed as expected.
	case <-time.After(5 * time.Second):
		t.Fatal("WorkerPool.Done() did not close after Close()")
	}
}
