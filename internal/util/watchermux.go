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

// The channelBufferSize ensures the message multiplexing is performed as fast
// as possible. However, it does not reduce risk of deadlocks.
func NewWatcherMux[T any](
	channelBufferSize int,
	dispatchFunc WatcherMuxDispatchFunc[T],
) *WatcherMux[T] {
	return &WatcherMux[T]{
		chList:       make([]chan<- T, 0),
		chSize:       channelBufferSize,
		mu:           &sync.Mutex{},
		dispatchFunc: dispatchFunc,
		doneCh:       make(chan struct{}),
	}
}

/*******************************************************************************
 * WatcherMux[T any]
 *
 *
 ******************************************************************************/

// TODO: Let watcher deregister itself from the watch list.
// - With a defer func returned to the user and an internal hashmap?

type WatcherMux[T any] struct {
	chList       []chan<- T // list of watcher channels
	chSize       int
	mu           *sync.Mutex
	dispatchFunc WatcherMuxDispatchFunc[T]

	doneCh chan struct{}
}

func (w *WatcherMux[T]) Watch() <-chan T {
	ch := make(chan T, w.chSize)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.chList = append(w.chList, ch)
	return ch
}

func (w *WatcherMux[T]) GetChanList() []chan<- T {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]chan<- T, len(w.chList))
	_ = copy(out, w.chList)
	return out
}

func (w *WatcherMux[T]) Dispatch(v T) {
	w.dispatchFunc(w, v)
}

func (w *WatcherMux[T]) Done() <-chan struct{} {
	return w.doneCh
}

func (w *WatcherMux[T]) Close() error {
	for _, ch := range w.GetChanList() {
		close(ch)
	}
	close(w.doneCh)
	return nil
}

/*******************************************************************************
 * DispatchFunc[T any]
 *
 ******************************************************************************/

type WatcherMuxDispatchFunc[T any] func(*WatcherMux[T], T)

// BlockingDispatchFunc may block indefinitely if receiver does not read from
// the channel in a timely manner.
func BlockingDispatchFunc[T any](w *WatcherMux[T], v T) {
	for _, ch := range w.GetChanList() {
		ch <- v
	}
}

// NonBlockingDispatchFunc may drop items if receiver does not read from the
// channel in a timely manner.
func NonBlockingDispatchFunc[T any](w *WatcherMux[T], v T) {
	for _, ch := range w.GetChanList() {
		select {
		case ch <- v:
		default:
		}
	}
}

// The closure returned by NewTimedOutDispatchFunc sends to the outgoing channels
// sequentially. If a channel is full it will block and try to send to it for
// `timeoutDuration` and move on to the next channel after that duration.
//
// This implementation may be very slow but avoids the deadlock scenarios implied
// by the BlockingDispatchFunc, and potentially reduces the amount of items of the
// NonBlockingDispatchFunc.
func NewTimedOutDispatchFunc[T any](timeoutDuration time.Duration) WatcherMuxDispatchFunc[T] {
	return func(w *WatcherMux[T], v T) {
		for _, ch := range w.GetChanList() {
			timeoutCh := time.After(timeoutDuration)
			select {
			case ch <- v:
			case <-timeoutCh:
			}
		}
	}
}

// Please see internal/util/workerpool.go for more information about the
// workerPoolQueue parameter.
func NewWorkerPoolTimedOutDispatchFunc[T any](
	workerPoolQueue chan<- func(),
	timeoutDuration time.Duration,
) WatcherMuxDispatchFunc[T] {
	return func(w *WatcherMux[T], v T) {
		for _, ch := range w.GetChanList() {
			workerPoolQueue <- func() {
				timeoutCh := time.After(timeoutDuration)
				select {
				case ch <- v:
				case <-timeoutCh:
				}
			}
		}
	}
}
