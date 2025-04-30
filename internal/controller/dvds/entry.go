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
package dvds

import "time"

type WALEntry[T any] struct {
	// Hash of this WAL entry.
	Hash [32]byte
	// Hash of the previous entry in the WAL.
	PreviousHash [32]byte

	// The timestamp ensures the proposed entries are consented in the right
	// order. The timestamp allows the observer WALEntries to process them in
	// order.
	Timestamp time.Time

	// The key ensures that duplicated proposals from 2 or more nodes does not
	// create a duplicate WAL entry.
	//
	// It also provides the capability to automatically consent entries that
	// shares the same `Key` and satisfies the condition described below.
	//
	// - Let d the `Data` of a ConsensusEntry.
	// - Let D a time duration reasonably chosen (e.g. 200ms or 1s).
	// - Let N a variable holding a Node ID.
	// - Let seq[n, K, d, D, N] a sequence of `n` ConsensusEntries sharing
	//   the same "Key" K; with any `Data` d; during duration D; proposed by
	//   node N; ordered by ascending Timestamp.
	// - Let n' <= n.
	// - Let seq[n', K, d, N] a subset of n' CONSECUTIVE elements of
	//   seq[n, K, d, D, N] ordered by ascending Timestamp.
	//
	// Condition: For all seq[n', K, d, N] such as all d are equal in value, then
	// all nodes N are considered giving their consent to the first WALProposal of
	// seq[n', K, d, N]. All subsequent proposals are discarded.
	Key string

	// WALId is the Id of a WAL. This Id servers the purpose of looking up the actual
	// type T. This type is then use to decode `types.RawWALProposal.Data` by a
	// multiplexer.
	WALId string

	// Data is the data of type T.
	Data T
}
