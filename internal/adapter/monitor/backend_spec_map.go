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
package monitoradapter

import (
	"github.com/alexandremahdhaoui/udplb/internal/types"

	"github.com/google/uuid"
)

// TODO: Define & Implement LiveConfig
// - List&Watch kubernetes resource definitions such as Service type loadbalancer.

// TODO: define what's in a types.LiveConfig or in types.LiveSpec.

// -- LBSpec
// The LB spec or config cannot be updated at runtime.

type (
	BackendSpecMap        = map[uuid.UUID]types.BackendSpec
	BackendSpecMapWatcher = types.Watcher[BackendSpecMap]
)

type BackendSpecList interface {
	types.DoneCloser

	Watch() (<-chan map[uuid.UUID]types.BackendSpec, error)
}
