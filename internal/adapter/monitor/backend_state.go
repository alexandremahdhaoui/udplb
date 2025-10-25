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

import "github.com/alexandremahdhaoui/udplb/internal/types"

var _ types.Watcher[types.BackendStatus] = &BackendState{}

// TODO: Define & Implement the Backend monitor.

// BackendState is data kind of type `volatile`.
//
// Please see `internal/controller/README.md` to learn more about data
// kinds, types, signal transduction and pathways.
type BackendState struct{}

func (b *BackendState) Watch() (<-chan types.BackendStatus, func()) {
	panic("unimplemented")
}

func (b *BackendState) Close() error {
	panic("unimplemented")
}

func (b *BackendState) Done() <-chan struct{} {
	panic("unimplemented")
}
