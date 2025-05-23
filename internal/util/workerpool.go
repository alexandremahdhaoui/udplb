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

	"github.com/alexandremahdhaoui/udplb/internal/types"
)

// NewFunctionalWorkerPool returns a channel that's closed when work done on behalf
// of all workers is done.
func NewFunctionalWorkerPool(
	n int,
	q <-chan func(),
	terminateCh <-chan struct{}, // worker will be gracefully shutdown if closed
) <-chan struct{} {
	doneCh := make(chan struct{})

	wg := &sync.WaitGroup{}
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			for {
				// if the q is busy, this select ensures the
				// worker is closed as soon as possible.
				select {
				case <-terminateCh:
					return
				default:
				}
				select {
				case f, ok := <-q:
					if !ok { // stop worker if q is closed
						return
					}
					f()
				case <-terminateCh:
					return
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(doneCh)
	}()

	return doneCh
}

var _ types.DoneCloser = &WorkerPool{}

type WorkerPool struct {
	doneCh      <-chan struct{}
	terminateCh chan struct{}
}

// The argument `q` must be specified by the user, this allow the user the choice to
// buffer or unbuffer the `q` channel.
func NewWorkerPool(
	n int,
	q <-chan func(),
) *WorkerPool {
	terminateCh := make(chan struct{})
	doneCh := NewFunctionalWorkerPool(n, q, terminateCh)

	return &WorkerPool{
		doneCh:      doneCh,
		terminateCh: terminateCh,
	}
}

// Close does not block. Please use Done().
func (w *WorkerPool) Close() error {
	close(w.terminateCh)
	return nil
}

// Done implements types.DoneCloser.
func (w *WorkerPool) Done() <-chan struct{} {
	return w.doneCh
}
