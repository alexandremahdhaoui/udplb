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

import (
	"crypto/sha256"
	"encoding/binary"
	"time"

	"github.com/google/uuid"
)

type (
	RawData     = []byte
	RawWALEntry = WALEntry[RawData]
)

type WALEntryType int

const (
	DataWALEntryType WALEntryType = iota
	StateWALEntryType
)

type WALEntry[T any] struct {
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
	// Data is the data of type T.
	Data T

	// The timestamp ensures the proposed entries are consented in the right
	// order. The timestamp allows the observer WALEntries to process them in
	// order.
	Timestamp time.Time
	// WALName is the Id of a WAL used for multiplexing.
	// Its purpose is to simplify the dispatchment of entries and the lookup
	// of the actual type of data T.
	WALName string

	// ProposalHash is the hash of the proposal.
	// It's the hash of the binary encoded data of WALEntry excluding the
	// PreviousHash and Hash (not known at proposal time).
	ProposalHash [32]byte
	// PreviousHash is the Hash of the previous entry in the WAL.
	PreviousHash [32]byte
	// Hash of this WAL entry.
	Hash [32]byte
}

type Suggestion[T any] struct {
	Key  string
	Data T
}

// Creates a WALEntry[T] of type Standard.
func NewProposal[T any](
	key string,
	data T,
) Suggestion[T] {
	return Suggestion[T]{
		Key:  key,
		Data: data,
	}
}

// Create a new Entry from an accepted Proposal.
func TransformProposalIntoWALEntry[T any](
	walId uuid.UUID,
	previousHash [32]byte,
	proposal Suggestion[T],
) (WALEntry[T], error) {
	out := WALEntry[T]{
		PreviousHash: previousHash,
		Hash:         [32]byte{},
		Timestamp:    time.Now(),
		Key:          proposal.Key,
		Data:         proposal.Data,
	}

	buf := make([]byte, 0)
	if _, err := binary.Encode(buf, binary.LittleEndian, out); err != nil {
		return WALEntry[T]{}, nil
	}
	out.Hash = sha256.Sum256(buf)

	return out, nil
}

func RawWALEntryInto[T any](
	in RawWALEntry,
) (WALEntry[T], error) {
	out := WALEntry[T]{
		Hash:         in.Hash,
		PreviousHash: in.PreviousHash,
		Timestamp:    in.Timestamp,
		Key:          in.Key,
		WALName:      in.WALName,
		Data:         *new(T),
	}

	if _, err := binary.Decode(in.Data, binary.LittleEndian, &out.Data); err != nil {
		return WALEntry[T]{}, err
	}

	return out, nil
}
