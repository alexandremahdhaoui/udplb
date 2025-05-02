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

	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/google/uuid"
)

var _ types.Cluster[any] = &cluster[any]{}

/*******************************************************************************
 * Concrete implementation
 *
 ******************************************************************************/

type cluster[T any] struct {
	// mgmt
	terminateCh chan struct{}
	doneCh      chan struct{}
}

/*******************************************************************************
 * New
 *
 ******************************************************************************/

func New[T any]() types.Cluster[T] {
	return &cluster[T]{}
}

/*******************************************************************************
 * Join
 *
 ******************************************************************************/

// Join implements types.Cluster.
func (c *cluster[T]) Join() error {
	panic("unimplemented")
}

/*******************************************************************************
 * Leave
 *
 ******************************************************************************/

// Leave implements types.Cluster.
func (c *cluster[T]) Leave() error {
	panic("unimplemented")
}

/*******************************************************************************
 * ListNodes
 *
 ******************************************************************************/

// ListNodes implements types.Cluster.
func (c *cluster[T]) ListNodes() []uuid.UUID {
	panic("unimplemented")
}

/*******************************************************************************
 * Recv
 *
 ******************************************************************************/

// Recv implements types.Cluster.
func (c *cluster[T]) Recv() (<-chan T, error) {
	// needs a ClusterMultiplexer?
	panic("unimplemented")
}

/*******************************************************************************
 * Send
 *
 ******************************************************************************/

// Send implements types.Cluster.
func (c *cluster[T]) Send() (chan<- T, error) {
	// send to c.currentLeader().
	// or: send to all available member.

	panic("unimplemented")
}

/*******************************************************************************
 * Runnable
 *
 ******************************************************************************/

// Run implements types.Cluster.
func (c *cluster[T]) Run(ctx context.Context) error {
	// 1. open socket
	// 2. advertise sockaddr.
	panic("unimplemented")
}

/*******************************************************************************
 * DoneCloser
 *
 ******************************************************************************/

// Close implements types.Cluster.
func (c *cluster[T]) Close() error {
	close(c.terminateCh)
	panic("unimplemented")
}

// Done implements types.Cluster.
func (c *cluster[T]) Done() <-chan struct{} {
	return c.doneCh
}
