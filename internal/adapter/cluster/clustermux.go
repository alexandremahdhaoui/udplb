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

var _ types.RawCluster = &clusterMux{}

/*******************************************************************************
 * Concrete implementation
 *
 ******************************************************************************/

type clusterMux struct{}

/*******************************************************************************
 * New
 *
 ******************************************************************************/

func NewMux() types.RawCluster {
	return &clusterMux{}
}

// Close implements types.Cluster.
func (cm *clusterMux) Close() error {
	panic("unimplemented")
}

// Done implements types.Cluster.
func (cm *clusterMux) Done() <-chan struct{} {
	panic("unimplemented")
}

// Join implements types.Cluster.
func (cm *clusterMux) Join() error {
	panic("unimplemented")
}

// Leave implements types.Cluster.
func (cm *clusterMux) Leave() error {
	panic("unimplemented")
}

// ListNodes implements types.Cluster.
func (cm *clusterMux) ListNodes() []uuid.UUID {
	panic("unimplemented")
}

// Recv implements types.Cluster.
func (cm *clusterMux) Recv() (<-chan []byte, func()) {
	panic("unimplemented")
}

// Run implements types.Cluster.
func (cm *clusterMux) Run(ctx context.Context) error {
	panic("unimplemented")
}

// Send implements types.Cluster.
func (cm *clusterMux) Send() (chan<- []byte, error) {
	panic("unimplemented")
}
