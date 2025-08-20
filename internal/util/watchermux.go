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
	"time"
)

var WatcherMuxRecommendedBufferSize = 10

/*******************************************************************************
 * New
 *
 ******************************************************************************/

func NewWatcherMux[T any](
	// channelBufferSize ensures the message multiplexing is performed as fast
	// as possible. However, it does not reduce risk of deadlocks.
	channelBufferSize int,
	// Please choose a dispatchFunc that does not block indefinitely in order to
	// avoid deadlocks.
	dispatchFunc WatcherMuxDispatchFunc[T],
) *WatcherMux[T] {
	return &WatcherMux[T]{
		watcherChCapacity: channelBufferSize,
		mu:                &sync.Mutex{},
		dispatchFunc:      dispatchFunc,
		doneCh:            make(chan struct{}),
	}
}

/*******************************************************************************
 * WatcherMux[T any]
 *
 *
 ******************************************************************************/

type FilterFunc func(v any) bool

var NoFilter FilterFunc = nil

type watcher[T any] struct {
	ch     chan T
	doneCh chan struct{}
	filter FilterFunc
}

func (w *watcher[T]) shouldSkip(v T) bool {
	return w.filter != nil && !w.filter(v)
}

type WatcherMux[T any] struct {
	watchers          map[int]*watcher[T] // set of watchers
	watcherIdCount    int
	watcherChCapacity int

	dispatchFunc WatcherMuxDispatchFunc[T]

	mu     *sync.Mutex
	doneCh chan struct{}
}

func (wm *WatcherMux[T]) Watch(filter FilterFunc) (<-chan T, func()) {
	w := &watcher[T]{
		ch:     make(chan T, wm.watcherChCapacity),
		doneCh: make(chan struct{}),
		filter: filter,
	}

	wm.mu.Lock()
	defer wm.mu.Unlock()
	id := wm.watcherIdCount
	wm.watcherIdCount += 1
	wm.watchers[id] = w

	return w.ch, func() {
		wm.mu.Lock()
		defer wm.mu.Unlock()
		close(w.doneCh)
		close(w.ch)
		delete(wm.watchers, id)
	}
}

func (wm *WatcherMux[T]) Dispatch(v T) {
	wm.dispatchFunc(wm, v)
}

func (wm *WatcherMux[T]) getWatcherList() []*watcher[T] {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	out := make([]*watcher[T], len(wm.watchers))
	i := 0
	for w := range wm.watchers {
		out[i] = wm.watchers[w]
		i++
	}
	return out
}

func (wm *WatcherMux[T]) Done() <-chan struct{} {
	return wm.doneCh
}

func (wm *WatcherMux[T]) Close() error {
	for _, w := range wm.getWatcherList() {
		close(w.ch)
	}
	close(wm.doneCh)
	return nil
}

/*******************************************************************************
 * DispatchFunc[T any]
 *
 ******************************************************************************/

type (
	WatcherMuxDispatchFunc[T any] func(*WatcherMux[T], T)
)

// NonBlockingDispatchFunc may drop items if receiver does not read from the
// channel in a timely manner.
func NonBlockingDispatchFunc[T any](wm *WatcherMux[T], v T) {
	for _, w := range wm.getWatcherList() {
		if w.shouldSkip(v) {
			continue // skip
		}
		select {
		case <-w.doneCh:
		case w.ch <- v:
		default:
		}
	}
}

// The closure returned by NewDispatchFuncWithTimeout sends to the outgoing channels
// sequentially. If a channel is full it will block and try to send to it for
// `timeoutDuration` and move on to the next channel after that duration.
//
// This implementation may be very slow but avoids the deadlock scenarios implied
// by the BlockingDispatchFunc, and potentially reduces the amount of items lost
// compared to the NonBlockingDispatchFunc.
func NewDispatchFuncWithTimeout[T any](timeoutDuration time.Duration) WatcherMuxDispatchFunc[T] {
	return func(w *WatcherMux[T], v T) {
		for _, w := range w.getWatcherList() {
			dispatchOne(w, timeoutDuration, v)
		}
	}
}

// Please see internal/util/workerpool.go for more information about the
// workerPoolQueue parameter.
func NewDispatchFuncWithTimeoutAndWorkerPool[T any](
	workerPoolQueue chan<- func(),
	timeoutDuration time.Duration,
) WatcherMuxDispatchFunc[T] {
	return func(w *WatcherMux[T], v T) {
		for _, w := range w.getWatcherList() {
			workerPoolQueue <- func() {
				dispatchOne(w, timeoutDuration, v)
			}
		}
	}
}

func dispatchOne[T any](w *watcher[T], timeoutDuration time.Duration, v T) {
	if w.shouldSkip(v) {
		return
	}

	timeoutCh := time.After(timeoutDuration)
	select {
	case <-w.doneCh:
	case w.ch <- v:
	case <-timeoutCh:
	}
}
