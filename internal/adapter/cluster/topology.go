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

// TODO:
// - Should Node holds attributes related to the Topology?
// (e.g. leader/follower etc...)
type Node struct {
	id     any
	Spec   any
	Status any
}

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
type Topology interface {
	// NextPeers returns a list of nodes the cluster needs to send
	// messages to.
	// E.g.:
	// - Fully connected: all other nodes.
	// - Leader: the leader node.
	// - Ring: next node in the ring.
	//
	// Finguring out the next nodes requires communicating with them.
	NextPeers() ([]Node, error)

	send() // ?
	recv() // ?
}

func NewFullyConnectedTopology() Topology
func NewLeaderTopology() Topology
func NewRingTopology() Topology
