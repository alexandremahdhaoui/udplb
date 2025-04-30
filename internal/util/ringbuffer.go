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

import "sync"

// TODO: write unit tests

// TODO: Fix bug where the ring buffer will overwrite a non-read value but
// it does not increment the value of readIdx.
//
// FYI: not incrementing readIdx on "overwrite" will imply that the reader
// will read a sequence of elements of the buffer that did not appear in
// the same order.
//
// E.g.: size=3
// 1. rb=[0, nil, nil]; readIdx=0; writeIdx=1; Write 0.
// 2. rb=[0, 1, 2]; readIdx=0; writeIdx=0; Write 1, Write 2.
// 3. rb=[nil, 1, 2]; readIdx=1; writeIdx=0; Read->0.
// 4. rb=[3, 4, 2]; readIdx=1; writeIdx=2; Write 3, Write 4.
// 5. rb=[3, nil, nil]; readIdx=0; writeIdx=2; Read->4, Write->2.
//
// Expected:
// 4. readIdx=2; writeIdx=2;
// 5. Read->2, Read->3

func NewRingBuffer[T any](size int) *RingBuffer[T] {
	return &RingBuffer[T]{
		buf:      make([]*T, size),
		size:     size,
		readIdx:  0,
		writeIdx: 0,
		mu:       &sync.Mutex{},
		safeNext: make(chan struct{}, size),
	}
}

type RingBuffer[T any] struct {
	buf      []*T
	size     int
	readIdx  int
	writeIdx int
	mu       *sync.Mutex

	// This channel ensures that on calling Next, there is a
	// value always set at readIdx.
	// safeNext must be initialized with a capacity equal to
	// the ring buffer's size.
	safeNext chan struct{}
}

func (rb *RingBuffer[T]) Write(v T) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.buf[rb.writeIdx] = &v

	// increment writeIdx.
	rb.writeIdx += 1
	if rb.writeIdx > rb.size-1 {
		rb.writeIdx = 0
	}

	// Will add one entry to safeNext until it's filled.
	// If it's filled we do not block.
	select {
	case rb.safeNext <- struct{}{}:
	default:
	}
}

func (rb *RingBuffer[T]) Next() T {
	// Ensures a value is set at readIdx.
	<-rb.safeNext

	// Only lock after checking safeNext to avoid deadlocks.
	rb.mu.Lock()
	defer rb.mu.Unlock()

	out := rb.buf[rb.readIdx]
	rb.buf[rb.readIdx] = nil

	// increment readIdx.
	rb.readIdx += 1
	if rb.readIdx > rb.size-1 {
		rb.readIdx = 0
	}

	// dereferencing out is always safe.
	return *out
}
