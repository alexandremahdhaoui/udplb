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
	"errors"
	"time"

	"github.com/alexandremahdhaoui/tooling/pkg/flaterrors"
)

type (
	Hash = [32]byte

	RawData     = []byte
	RawWALEntry = WALEntry[RawData]
)

var (
	NilHash = Hash([32]byte{})

	ErrEntryCannotBeFoundInWalLinkedList = errors.New("entry cannot be found in wal linked list")
	ErrGettingNextEntryFromWalLinkedList = errors.New("getting next entry from wal linked list")
	ErrMalformedWalLinkedList            = errors.New("malformed wal linked list")
)

// [Context from dvds implementation]
//
// The returned type of the wal channel must be a linked list:
//
// Solution A:
//   - The wal ch returns a struct holding a map[walEntryHash]WALEntry
//   - It also holds the hash of the first and last entries to allow quick
//     access to head and tail
//
// Solution B:
//   - types.WALEntry[T] must hold pointers to previous and next entry.
//   - We also want a map[walEntryHash]*walEntry
//   - dvds can hold:
//   - walEntryHash of the last successfully executed WALEntry
//   - state machine
//   - PROBLEM: cloning the linked list is costly because it requires
//     deep copying all entries and update all pointers of the
//     linked list.
type WalLinkedList[T any] struct {
	head, tail Hash
	m          map[Hash]WALEntry[T]
}

func (s WalLinkedList[T]) TailHash() Hash {
	return s.tail
}

func (s WalLinkedList[T]) GetNext(hash Hash) (WALEntry[T], error) {
	curr, err := s.Get(hash)
	if err != nil {
		return WALEntry[T]{}, flaterrors.Join(err, ErrGettingNextEntryFromWalLinkedList)
	}

	if curr.Hash == NilHash {
		// Edge case where prev is not the tail and next hash is nil.
		// -> ll must be validated before sent, but handling
		// the edge case here ensure we don't get strange
		// errors.
		return WALEntry[T]{}, errors.Join(
			ErrMalformedWalLinkedList,
			ErrGettingNextEntryFromWalLinkedList,
		)
	}

	out, err := s.Get(curr.NextHash)
	if err != nil {
		return WALEntry[T]{}, flaterrors.Join(err, ErrGettingNextEntryFromWalLinkedList)
	}

	return out, nil
}

func (s WalLinkedList[T]) Get(hash Hash) (WALEntry[T], error) {
	out, ok := s.m[hash]
	if !ok {
		return WALEntry[T]{}, ErrEntryCannotBeFoundInWalLinkedList
	}
	return out, nil
}

func (s WalLinkedList[T]) Contains(hash Hash) bool {
	_, ok := s.m[hash]
	return ok
}

func NewWALEntry[T any](
	previousHash Hash,
	key string,
	command StateMachineCommand,
	obj T,
) (WALEntry[T], error) {
	out := WALEntry[T]{
		PreviousHash: previousHash,
		Timestamp:    time.Now(),
		Key:          key,
		Command:      command,
		Object:       obj,

		// Field that MUST NOT be set before computing hash.
		Hash:     Hash{},
		NextHash: Hash{},
		WALId:    "", // TODO:
	}

	buf := make([]byte, 0)
	if _, err := binary.Encode(buf, binary.LittleEndian, out); err != nil {
		return WALEntry[T]{}, nil
	}
	out.Hash = sha256.Sum256(buf)

	return out, nil
}

type WALEntry[T any] struct {
	// Hash of this WAL entry.
	// Obviously: it MUST NOT participate to the hash of this entry.
	Hash Hash
	// Hash of the previous entry in the WAL.
	PreviousHash Hash
	// Hash of the next entry in the WAL.
	// NOTE: NextHash MUST NOT participate to the hash of this entry.
	// NextHash's purposes is to build a linked list.
	NextHash Hash

	// The timestamp ensures the proposed entries are consented in the right
	// order. The timestamp allows the observer WALEntries to process them in
	// order.
	Timestamp time.Time

	// WALId is the Id of a WAL. This Id serves the purpose of looking up the actual
	// type T. This type is then use to decode `types.RawWALProposal.Data` by a
	// multiplexer.
	// NOTE: WALId MUST NOT participate to the hash of this entry.
	// This is a helper field
	WALId string

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

	// Command is the command that must be executed on the underlying state machine
	Command StateMachineCommand

	// Object is the data of type T.
	Object T
}
