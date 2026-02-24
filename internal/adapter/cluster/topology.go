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
	"net"

	"github.com/google/uuid"
)

/*******************************************************************************
 * Node
 *
 ******************************************************************************/

// TODO:
// - Should Node holds attributes related to the Topology?
// (e.g. leader/follower etc...)

// Node represents a cluster member with a unique ID and network address.
type Node struct {
	ID   uuid.UUID
	Addr net.Addr
}

/*******************************************************************************
 * Topology
 *
 ******************************************************************************/

// 1. Cluster advertise itself.
// 2. Wait for reply.
// 3. The reply contains cluster node list.
// 4. The Topology algorithm decides which nodes are the next peers.
//
// TODO: Answer Questions:
//   - How to receive information about particular strategy?
//     E.g. Who's the leader? What is my next peer in a ring.
//   - How to react to minor topology changes such as a node becoming
//     unavailable?
//   - How to handle multiple different Topology strategy in the same
//     cluster?
//     E.g.: When rolling out a different strategy.
//   - What is the responsibility of the Topology?
//     Should Topology be responsible for maintaining the list of
//     nodes participating in the cluster?

// Topology determines which peers a node should send messages to.
// The topology is stateless: it receives the current node set as a parameter
// rather than maintaining its own mutable node list.
type Topology interface {
	// NextPeers returns the nodes this node should send messages to.
	// The self parameter identifies the calling node; it is excluded
	// from the result. The nodes map contains all known cluster members.
	//
	// IMPLEMENTED: FullyConnected returns all other nodes.
	// TODO: Leader topology: return the leader node only.
	// TODO: Ring topology: return next node in the ring.
	//
	// ANSWERED: Figuring out the next nodes requires communicating with
	// them. This is handled by the clusterMux which maintains the nodes
	// map and passes it to NextPeers as a parameter.
	NextPeers(self uuid.UUID, nodes map[uuid.UUID]Node) ([]Node, error)

	// ANSWERED: send()/recv() were removed from Topology. The topology
	// is stateless and only decides *which* peers to target. Actual
	// send/recv is handled by clusterMux via Protocol.
}

/*******************************************************************************
 * FullyConnected
 *
 ******************************************************************************/

type fullyConnectedTopology struct{}

// NextPeers returns all nodes except self. Iteration order is
// non-deterministic (Go map), which is acceptable for a fully connected
// topology since all peers are targeted.
func (t *fullyConnectedTopology) NextPeers(self uuid.UUID, nodes map[uuid.UUID]Node) ([]Node, error) {
	peers := make([]Node, 0, len(nodes))
	for id, node := range nodes {
		if id == self {
			continue
		}
		peers = append(peers, node)
	}
	return peers, nil
}

// NewFullyConnectedTopology returns a Topology that targets all cluster
// members except the calling node.
func NewFullyConnectedTopology() Topology {
	return &fullyConnectedTopology{}
}

/*******************************************************************************
 * Leader (stub)
 *
 ******************************************************************************/

func NewLeaderTopology() Topology {
	// TODO: implement
	panic("unimplemented")
}

/*******************************************************************************
 * Ring (stub)
 *
 ******************************************************************************/

func NewRingTopology() Topology {
	// TODO: implement
	panic("unimplemented")
}
