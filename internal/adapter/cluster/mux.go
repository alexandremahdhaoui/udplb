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
	"github.com/alexandremahdhaoui/udplb/internal/util"
	"github.com/google/uuid"
)

var _ types.RawCluster = &clusterMux{}

/*******************************************************************************
 * Concrete implementation
 *
 ******************************************************************************/

type clusterMux struct {
	recvWorkerPool *util.WorkerPool
	sendWorkerPool *util.WorkerPool
}

/*******************************************************************************
 * New
 *
 ******************************************************************************/

// Ring strategy:
// - Let A, B, C, D and E 5 healthy nodes of a clutser.
// - 1. A send proposal to B, B to C, C to D, and D to E.
// - 1. 4 messages have been sent.
// - 2. E "knows it all".
// - 2. E sends to either A, B, C, D?
//      OR: E sends to A, A to B, B to C, C to D.
// DONE: 8 messages sent. (same as Leader based).
//
// However, the Ring strategy could actually be used to already start
// accepting when E knows it all.
// That is:
// - 1. First circular round around the ring, up to E. (4 messages)
// - 2. E accepts + send to A, A knows it all, accept + send to B; etc. up to D
// - 3. D send accept messages to E.
// DONE: 9 messsages sent: Phase I & II of FastWAL are both done.
//
// The big downside is that network is super slow; and saving 2(n-1)-1 messages
// doesn't really outway how slow this strategy could be if n goes big.
//
// Watching the overall state of the cluster is drastically simplified, as all
// peers just need to watch the state of one other node.
//
// Could be interesting how it performs in practice.
//
// TODO: Draft algorithm that handles failures, i.e. a node becomes unavailable.
// - 1. A realise that B is not available. A.nextNode = C
// - 2. A send message to C to let it know that B is down.
// - 3. Message propagate and the cluster create a new "order" of nodes.
// What happens when multiple nodes become unavailable?
//
// TODO: QUESTION? Can this algorithm be used to hold many WAL negociation rounds
// at once?
// What about multiplexing? Looks like it's a bit leaky.

func NewMux(
	// TODO: Protocol will be an abstraction over Send/Recv.
	// E.g. UDP or GRPC.
	protocol Protocol,
	topology Topology, // Leader based, Ring or Fully connected.
	recvWorkerPool *util.WorkerPool,
	sendWorkerPool *util.WorkerPool,
) types.RawCluster {
	return &clusterMux{
		recvWorkerPool: recvWorkerPool,
		sendWorkerPool: sendWorkerPool,
	}
}

/*******************************************************************************
 * Recv
 *
 ******************************************************************************/

// Recv implements types.Cluster.
func (cm *clusterMux) Recv() (<-chan []byte, func()) {
	// Q:
	// - send to c.currentLeader().
	// - or send to all available member.
	// or not implemented here but in run?
	panic("unimplemented")
}

/*******************************************************************************
 * Send
 *
 ******************************************************************************/

// Send implements types.Cluster.
func (cm *clusterMux) Send(ch <-chan []byte) error {
	// needs to fan-in/merge
	// TODO: figure out how to select from many channels programmatically.
	// e.g. using reflect.Select() || OR https://stackoverflow.com/a/32342741 ||
	// OR Check code about merging channels.
	panic("unimplemented")
}

/*******************************************************************************
 * Join
 *
 ******************************************************************************/

// Join implements types.Cluster.
func (cm *clusterMux) Join() error {
	panic("unimplemented")
}

/*******************************************************************************
 * Leave
 *
 ******************************************************************************/

// Leave implements types.Cluster.
func (cm *clusterMux) Leave() error {
	panic("unimplemented")
}

/*******************************************************************************
 * ListNodes
 *
 ******************************************************************************/

// ListNodes implements types.Cluster.
func (cm *clusterMux) ListNodes() []uuid.UUID {
	panic("unimplemented")
}

/*******************************************************************************
 * Runnable
 *
 ******************************************************************************/

// Run implements types.Cluster.
func (cm *clusterMux) Run(ctx context.Context) error {
	panic("unimplemented")
}

/*******************************************************************************
 * DoneCloser
 *
 ******************************************************************************/

// Close implements types.Cluster.
func (cm *clusterMux) Close() error {
	panic("unimplemented")
}

// Done implements types.Cluster.
func (cm *clusterMux) Done() <-chan struct{} {
	panic("unimplemented")
}
