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
package types

type (
	State int
)

const (
	// A subsystem is in Unknown state and must be considered Unavailable
	// until it state becomes known again.
	StateUnknown State = iota

	// A subsystem is healthy available.
	// E.g.:
	//		A backend is healthy and available. It can accept packets destinated
	//		to new sessions.
	StateAvailable

	// A subsystem is healthy however scheduling new workload on this subsystem
	// is not permitted.
	// E.g.:
	//		A backend is healthy can receive packets for mapped sessions but
	//		cannot accept packets destinated to new sessions.
	StateUnschedulable

	//
	// E.g.:
	//		A backend is down and cannot accept any packet. We can evict a backend
	//      by marking it as unavailable.
	StateUnavailable
)
