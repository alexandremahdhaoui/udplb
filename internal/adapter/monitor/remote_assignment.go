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

// INFO: "LocalAssignment" does not exist and would be bpf.DatastructureManager.WatchAssignment()

// TODO: Define & Implement RemoteAssignment
// - open a UDP socket and listen for any assignment event.
// - Assignment events can be updates or deletions.

type RemoteAssignment interface {
	types.DoneCloser

	Watch() <-chan types.Assignment // TODO:
}
