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
// Constructor
// ---------------------------------------------------------------------------

func TestNewRingBuffer(t *testing.T) {
	rb := NewRingBuffer[int](5)
	require.NotNil(t, rb)
	assert.Equal(t, 5, rb.size)
	assert.Equal(t, 0, rb.readIdx)
	assert.Equal(t, 0, rb.writeIdx)
	assert.Len(t, rb.buf, 5)
}

// ---------------------------------------------------------------------------
// Basic Write / Next round-trip
// ---------------------------------------------------------------------------

func TestRingBuffer_WriteAndNext(t *testing.T) {
	rb := NewRingBuffer[string](3)

	rb.Write("a")
	rb.Write("b")
	rb.Write("c")

	assert.Equal(t, "a", rb.Next())
	assert.Equal(t, "b", rb.Next())
	assert.Equal(t, "c", rb.Next())
}

func TestRingBuffer_SingleElement(t *testing.T) {
	rb := NewRingBuffer[int](1)
	rb.Write(42)
	assert.Equal(t, 42, rb.Next())
}

// ---------------------------------------------------------------------------
// Wrapping behavior -- write more than capacity
// ---------------------------------------------------------------------------

func TestRingBuffer_Wrapping(t *testing.T) {
	rb := NewRingBuffer[int](3)

	// Write 5 values into a buffer of size 3.
	// Values 0 and 1 will be overwritten by 3 and 4.
	rb.Write(0)
	rb.Write(1)
	rb.Write(2)
	rb.Write(3)
	rb.Write(4)

	// After overwriting, the readable values should be 2, 3, 4 in order.
	assert.Equal(t, 2, rb.Next())
	assert.Equal(t, 3, rb.Next())
	assert.Equal(t, 4, rb.Next())
}

func TestRingBuffer_WrappingSize1(t *testing.T) {
	rb := NewRingBuffer[int](1)

	rb.Write(10)
	rb.Write(20)
	rb.Write(30)

	// Only the last value should be readable.
	assert.Equal(t, 30, rb.Next())
}

// ---------------------------------------------------------------------------
// Next blocks when buffer is empty
// ---------------------------------------------------------------------------

func TestRingBuffer_NextBlocksWhenEmpty(t *testing.T) {
	rb := NewRingBuffer[int](3)

	received := make(chan int, 1)
	go func() {
		received <- rb.Next()
	}()

	// Ensure Next is blocked (nothing to read yet).
	select {
	case <-received:
		t.Fatal("Next should block when buffer is empty")
	case <-time.After(50 * time.Millisecond):
		// expected: still blocking
	}

	// Unblock by writing a value.
	rb.Write(99)

	select {
	case v := <-received:
		assert.Equal(t, 99, v)
	case <-time.After(time.Second):
		t.Fatal("Next did not unblock after Write")
	}
}

// ---------------------------------------------------------------------------
// Interleaved Write/Next
// ---------------------------------------------------------------------------

func TestRingBuffer_InterleavedWriteNext(t *testing.T) {
	rb := NewRingBuffer[int](3)

	rb.Write(1)
	assert.Equal(t, 1, rb.Next())

	rb.Write(2)
	rb.Write(3)
	assert.Equal(t, 2, rb.Next())
	assert.Equal(t, 3, rb.Next())

	rb.Write(4)
	assert.Equal(t, 4, rb.Next())
}

// ---------------------------------------------------------------------------
// Concurrent Write/Next
// ---------------------------------------------------------------------------

func TestRingBuffer_ConcurrentWriteNext(t *testing.T) {
	// Use a buffer large enough to hold all values so the safeNext semaphore
	// produces a token for every write and the reader never blocks forever.
	const count = 200
	rb := NewRingBuffer[int](count)

	var wg sync.WaitGroup
	wg.Add(2)

	// Writer goroutine.
	go func() {
		defer wg.Done()
		for i := 0; i < count; i++ {
			rb.Write(i)
		}
	}()

	// Reader goroutine collects values.
	results := make([]int, 0, count)
	go func() {
		defer wg.Done()
		for range count {
			results = append(results, rb.Next())
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		assert.Len(t, results, count)
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent Write/Next deadlocked")
	}
}
