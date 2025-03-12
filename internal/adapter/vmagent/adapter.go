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

package vmagentadapter

import "github.com/google/uuid"

/******************************************************************************
 * VMAgent
 *
 *
 ******************************************************************************/

// TODO: define the Recorder state machine.
// TODO: Define it with protobuf and communicate via grpc.
type VMAgent interface {
	// SetRecorder is an idempotent endpoint used to create or update
	// a recorder. Updating a recorder state can be achieved by calling
	// this endpoint
	SetRecorder(spec RecorderSpec)

	// Get the actual status of a recorder.
	GetRecorderStatus(id uuid.UUID) (RecorderStatus, error)

	// Collects the records associated with a recorder.
	CollectRecords(spec RecorderSpec) (Records, error)

	// Resets a VM:
	// - Terminate all recorder processes,
	// - Remove all network devices associated with a recorder.
	// - Delete all records
	ResetVM() error
}

func New() VMAgent {
	// return &vmagent{}
	panic("unimplemented")
}

type vmagent struct{}

/******************************************************************************
 * RunVMAgent
 *
 *
 ******************************************************************************/

// RunVMAgent runs an agent on a guest VM. The agent allows dynamically running
// and stopping recorder service on the guest VM.
// How to know which vm config must be used?
// - Expects a positional argument?
// - Derive it from hostname?
func RunVMAgent(cfg Config) error {
	panic("unimplemented")
}

type (
	RecorderState string // e.g. Running, Unschedulable, Stopped, Zombie.

	RecorderSpec struct {
		State RecorderState // The desired state of the recorder.
	}

	RecorderStatus struct {
		State RecorderState // the actual state of the recorder.
	}

	Records struct{}
)
