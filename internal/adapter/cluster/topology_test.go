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
package clusteradpater_test

import (
	"net"
	"testing"

	clusteradpater "github.com/alexandremahdhaoui/udplb/internal/adapter/cluster"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFullyConnectedTopology(t *testing.T) {
	topo := clusteradpater.NewFullyConnectedTopology()

	// Helper to create a Node with a resolved UDP address.
	makeNode := func(t *testing.T, id uuid.UUID, addr string) clusteradpater.Node {
		t.Helper()
		udpAddr, err := net.ResolveUDPAddr("udp", addr)
		require.NoError(t, err)
		return clusteradpater.Node{ID: id, Addr: udpAddr}
	}

	t.Run("EmptyNodeMap", func(t *testing.T) {
		self := uuid.New()
		nodes := map[uuid.UUID]clusteradpater.Node{}

		peers, err := topo.NextPeers(self, nodes)
		require.NoError(t, err)
		assert.Empty(t, peers)
	})

	t.Run("SelfOnly", func(t *testing.T) {
		self := uuid.New()
		nodes := map[uuid.UUID]clusteradpater.Node{
			self: makeNode(t, self, "127.0.0.1:5000"),
		}

		peers, err := topo.NextPeers(self, nodes)
		require.NoError(t, err)
		assert.Empty(t, peers)
	})

	t.Run("ThreeNodes", func(t *testing.T) {
		self := uuid.New()
		id1 := uuid.New()
		id2 := uuid.New()

		nodes := map[uuid.UUID]clusteradpater.Node{
			self: makeNode(t, self, "127.0.0.1:5000"),
			id1:  makeNode(t, id1, "127.0.0.1:5001"),
			id2:  makeNode(t, id2, "127.0.0.1:5002"),
		}

		peers, err := topo.NextPeers(self, nodes)
		require.NoError(t, err)
		require.Len(t, peers, 2)

		// Verify self is excluded and all other nodes are returned.
		peerIDs := make([]uuid.UUID, len(peers))
		for i, p := range peers {
			peerIDs[i] = p.ID
		}
		assert.ElementsMatch(t, []uuid.UUID{id1, id2}, peerIDs)
	})

	t.Run("FiveNodes", func(t *testing.T) {
		self := uuid.New()
		id1 := uuid.New()
		id2 := uuid.New()
		id3 := uuid.New()
		id4 := uuid.New()

		nodes := map[uuid.UUID]clusteradpater.Node{
			self: makeNode(t, self, "127.0.0.1:5000"),
			id1:  makeNode(t, id1, "127.0.0.1:5001"),
			id2:  makeNode(t, id2, "127.0.0.1:5002"),
			id3:  makeNode(t, id3, "127.0.0.1:5003"),
			id4:  makeNode(t, id4, "127.0.0.1:5004"),
		}

		peers, err := topo.NextPeers(self, nodes)
		require.NoError(t, err)
		require.Len(t, peers, 4)

		peerIDs := make([]uuid.UUID, len(peers))
		for i, p := range peers {
			peerIDs[i] = p.ID
		}
		assert.ElementsMatch(t, []uuid.UUID{id1, id2, id3, id4}, peerIDs)
	})

	t.Run("SelfNotInMap", func(t *testing.T) {
		self := uuid.New()
		id1 := uuid.New()
		id2 := uuid.New()

		nodes := map[uuid.UUID]clusteradpater.Node{
			id1: makeNode(t, id1, "127.0.0.1:5001"),
			id2: makeNode(t, id2, "127.0.0.1:5002"),
		}

		peers, err := topo.NextPeers(self, nodes)
		require.NoError(t, err)
		require.Len(t, peers, 2)

		peerIDs := make([]uuid.UUID, len(peers))
		for i, p := range peers {
			peerIDs[i] = p.ID
		}
		assert.ElementsMatch(t, []uuid.UUID{id1, id2}, peerIDs)
	})
}
