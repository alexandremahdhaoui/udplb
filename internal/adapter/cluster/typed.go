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
package clusteradpater

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/alexandremahdhaoui/udplb/internal/util"
	"github.com/google/uuid"
)

var _ types.Cluster[any] = &typedCluster[any]{}

/*******************************************************************************
 * Concrete implementation
 *
 ******************************************************************************/

type typedCluster[T any] struct {
	raw       types.RawCluster
	rawSendCh chan types.RawData
	recvMux   *util.WatcherMux[T]

	ctx           context.Context
	running       bool
	closed        bool
	mu            *sync.Mutex
	doneCh        chan struct{}
	terminateCh   chan struct{}
	rawRecvCancel func()
}

/*******************************************************************************
 * New
 *
 ******************************************************************************/

func New[T any](raw types.RawCluster, recvMux *util.WatcherMux[T]) types.Cluster[T] {
	return &typedCluster[T]{
		raw:         raw,
		rawSendCh:   make(chan types.RawData),
		recvMux:     recvMux,
		mu:          &sync.Mutex{},
		doneCh:      make(chan struct{}),
		terminateCh: make(chan struct{}),
	}
}

/*******************************************************************************
 * Run
 *
 ******************************************************************************/

// Run implements types.Cluster.
// ANSWERED: The original stub noted "1. open socket, 2. advertise sockaddr."
// Run delegates socket management to the underlying raw cluster (clusterMux).
// typedCluster only registers its rawSendCh and subscribes to raw.Recv().
func (c *typedCluster[T]) Run(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return types.ErrAlreadyRunning
	} else if c.closed {
		return types.ErrCannotRunClosedRunnable
	}

	// Register rawSendCh with the underlying raw cluster for sending.
	if err := c.raw.Send(c.rawSendCh); err != nil {
		return err
	}

	// Subscribe to raw cluster for receiving.
	rawRecvCh, rawRecvCancel := c.raw.Recv()
	c.rawRecvCancel = rawRecvCancel

	c.ctx = ctx
	c.running = true

	// Start decode goroutine: reads raw bytes, JSON decodes to T, dispatches.
	go c.decodeLoop(rawRecvCh)

	return nil
}

/*******************************************************************************
 * decodeLoop
 *
 ******************************************************************************/

func (c *typedCluster[T]) decodeLoop(rawRecvCh <-chan types.RawData) {
	defer func() {
		_ = c.recvMux.Close()
		close(c.doneCh)
	}()

	for {
		select {
		case <-c.terminateCh:
			return
		case raw, ok := <-rawRecvCh:
			if !ok {
				return
			}
			var decoded T
			if err := json.Unmarshal(raw, &decoded); err != nil {
				slog.ErrorContext(c.ctx, "typedCluster: decoding received data", "err", err.Error())
				continue
			}
			c.recvMux.Dispatch(decoded)
		}
	}
}

/*******************************************************************************
 * Send
 *
 ******************************************************************************/

// Send implements types.Cluster.
// Starts a goroutine that reads from ch, JSON encodes each T to []byte,
// and writes to rawSendCh. The goroutine exits when ch is closed or
// terminateCh is closed.
func (c *typedCluster[T]) Send(ch <-chan T) error {
	go func() {
		for {
			select {
			case <-c.terminateCh:
				return
			case val, ok := <-ch:
				if !ok {
					return
				}
				buf, err := json.Marshal(val)
				if err != nil {
					slog.ErrorContext(c.ctx, "typedCluster: encoding data for send", "err", err.Error())
					continue
				}
				select {
				case c.rawSendCh <- buf:
				case <-c.terminateCh:
					return
				}
			}
		}
	}()
	return nil
}

/*******************************************************************************
 * Recv
 *
 ******************************************************************************/

// Recv implements types.Cluster.
// ANSWERED: Yes, this needs a ClusterMultiplexer. The recvMux (WatcherMux[T])
// serves that role: raw bytes are decoded in decodeLoop and fanned out to
// all watchers via recvMux.
func (c *typedCluster[T]) Recv() (<-chan T, func()) {
	return c.recvMux.Watch(util.NoFilter)
}

/*******************************************************************************
 * Join
 *
 ******************************************************************************/

// Join implements types.Cluster.
// Delegates to the underlying raw cluster.
func (c *typedCluster[T]) Join() error {
	return c.raw.Join()
}

/*******************************************************************************
 * Leave
 *
 ******************************************************************************/

// Leave implements types.Cluster.
// Delegates to the underlying raw cluster.
func (c *typedCluster[T]) Leave() error {
	return c.raw.Leave()
}

/*******************************************************************************
 * ListNodes
 *
 ******************************************************************************/

// ListNodes implements types.Cluster.
// Delegates to the underlying raw cluster.
func (c *typedCluster[T]) ListNodes() []uuid.UUID {
	return c.raw.ListNodes()
}

/*******************************************************************************
 * Close
 *
 ******************************************************************************/

// Close implements types.Cluster.
// Follows the standard terminateCh/doneCh pattern.
func (c *typedCluster[T]) Close() error {
	c.mu.Lock()

	if c.closed {
		c.mu.Unlock()
		return types.ErrAlreadyClosed
	} else if !c.running {
		c.mu.Unlock()
		return types.ErrRunnableMustBeRunningToBeClosed
	}

	close(c.terminateCh)
	c.mu.Unlock()

	if c.rawRecvCancel != nil {
		c.rawRecvCancel()
	}

	// Wait for decodeLoop to finish without holding the lock.
	timeoutCh := time.After(closeTimeoutDuration)
	select {
	case <-c.doneCh:
	case <-timeoutCh:
	}

	c.mu.Lock()
	c.running = false
	c.closed = true
	c.mu.Unlock()

	return nil
}

/*******************************************************************************
 * Done
 *
 ******************************************************************************/

// Done implements types.Cluster.
func (c *typedCluster[T]) Done() <-chan struct{} {
	return c.doneCh
}
