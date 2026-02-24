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
package types

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// WALEntryType constants
// ---------------------------------------------------------------------------

func TestWALEntryTypeConstants(t *testing.T) {
	t.Run("DataWALEntryType is zero", func(t *testing.T) {
		assert.Equal(t, WALEntryType(0), DataWALEntryType)
	})

	t.Run("StateWALEntryType is one", func(t *testing.T) {
		assert.Equal(t, WALEntryType(1), StateWALEntryType)
	})

	t.Run("types are distinct", func(t *testing.T) {
		assert.NotEqual(t, DataWALEntryType, StateWALEntryType)
	})
}

// ---------------------------------------------------------------------------
// NewProposal
// ---------------------------------------------------------------------------

func TestNewProposal(t *testing.T) {
	t.Run("creates suggestion with correct fields for string data", func(t *testing.T) {
		s := NewProposal("my-key", "my-data", PutCommand)
		assert.Equal(t, "my-key", s.Key)
		assert.Equal(t, "my-data", s.Data)
		assert.Equal(t, PutCommand, s.Verb)
	})

	t.Run("creates suggestion with correct fields for int data", func(t *testing.T) {
		s := NewProposal("counter-key", 42, AddCommand)
		assert.Equal(t, "counter-key", s.Key)
		assert.Equal(t, 42, s.Data)
		assert.Equal(t, AddCommand, s.Verb)
	})

	t.Run("creates suggestion with delete verb", func(t *testing.T) {
		s := NewProposal("delete-key", []byte{1, 2, 3}, DeleteCommand)
		assert.Equal(t, "delete-key", s.Key)
		assert.Equal(t, []byte{1, 2, 3}, s.Data)
		assert.Equal(t, DeleteCommand, s.Verb)
	})

	t.Run("creates suggestion with struct data", func(t *testing.T) {
		type testData struct {
			Value int
			Name  string
		}
		data := testData{Value: 10, Name: "test"}
		s := NewProposal("struct-key", data, AppendCommand)
		assert.Equal(t, "struct-key", s.Key)
		assert.Equal(t, data, s.Data)
		assert.Equal(t, AppendCommand, s.Verb)
	})

	t.Run("creates suggestion with empty key", func(t *testing.T) {
		s := NewProposal("", "data", PutCommand)
		assert.Equal(t, "", s.Key)
	})
}

// ---------------------------------------------------------------------------
// TransformProposalIntoWALEntry
// ---------------------------------------------------------------------------

func TestTransformProposalIntoWALEntry(t *testing.T) {
	t.Run("sets WALName from argument", func(t *testing.T) {
		proposal := NewProposal[uint32]("key", 42, PutCommand)
		prevHash := [32]byte{0x01}

		entry, err := TransformProposalIntoWALEntry("test-wal", prevHash, proposal, PutCommand)
		require.NoError(t, err)
		assert.Equal(t, "test-wal", entry.WALName)
	})

	t.Run("works with string data", func(t *testing.T) {
		proposal := NewProposal("key", "string-data", PutCommand)
		prevHash := [32]byte{}

		entry, err := TransformProposalIntoWALEntry("wal", prevHash, proposal, PutCommand)
		require.NoError(t, err)
		assert.Equal(t, "string-data", entry.Data)
	})

	t.Run("sets PreviousHash from argument", func(t *testing.T) {
		proposal := NewProposal[uint32]("key", 100, AddCommand)
		prevHash := [32]byte{0xAA, 0xBB, 0xCC}

		entry, err := TransformProposalIntoWALEntry("wal-name", prevHash, proposal, AddCommand)
		require.NoError(t, err)
		assert.Equal(t, prevHash, entry.PreviousHash)
	})

	t.Run("copies Key from proposal", func(t *testing.T) {
		proposal := NewProposal[uint32]("my-unique-key", 1, DeleteCommand)
		entry, err := TransformProposalIntoWALEntry("w", [32]byte{}, proposal, DeleteCommand)
		require.NoError(t, err)
		assert.Equal(t, "my-unique-key", entry.Key)
	})

	t.Run("copies Data from proposal", func(t *testing.T) {
		proposal := NewProposal[uint32]("k", 999, PutCommand)
		entry, err := TransformProposalIntoWALEntry("w", [32]byte{}, proposal, PutCommand)
		require.NoError(t, err)
		assert.Equal(t, uint32(999), entry.Data)
	})

	t.Run("uses verb argument not proposal verb", func(t *testing.T) {
		proposal := NewProposal[uint32]("k", 1, PutCommand)
		entry, err := TransformProposalIntoWALEntry("w", [32]byte{}, proposal, DeleteCommand)
		require.NoError(t, err)
		assert.Equal(t, DeleteCommand, entry.Verb)
	})

	t.Run("sets timestamp close to now", func(t *testing.T) {
		before := time.Now()
		proposal := NewProposal[uint32]("k", 0, PutCommand)
		entry, err := TransformProposalIntoWALEntry("w", [32]byte{}, proposal, PutCommand)
		after := time.Now()
		require.NoError(t, err)
		assert.False(t, entry.Timestamp.Before(before))
		assert.False(t, entry.Timestamp.After(after))
	})

	t.Run("computes non-zero hash", func(t *testing.T) {
		proposal := NewProposal[uint32]("k", 42, PutCommand)
		entry, err := TransformProposalIntoWALEntry("w", [32]byte{}, proposal, PutCommand)
		require.NoError(t, err)
		assert.NotEqual(t, [32]byte{}, entry.Hash)
	})

	t.Run("different inputs produce different hashes", func(t *testing.T) {
		p1 := NewProposal[uint32]("k", 1, PutCommand)
		p2 := NewProposal[uint32]("k", 2, PutCommand)

		e1, err := TransformProposalIntoWALEntry("w", [32]byte{}, p1, PutCommand)
		require.NoError(t, err)
		e2, err := TransformProposalIntoWALEntry("w", [32]byte{}, p2, PutCommand)
		require.NoError(t, err)

		assert.NotEqual(t, e1.Hash, e2.Hash)
	})
}

// ---------------------------------------------------------------------------
// RawWALEntryInto
// ---------------------------------------------------------------------------

func TestRawWALEntryInto(t *testing.T) {
	t.Run("decodes fixed-size data from RawWALEntry", func(t *testing.T) {
		// Encode a uint32 value into raw bytes.
		var val uint32 = 12345
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, val)

		now := time.Now()
		raw := RawWALEntry{
			Key:          "test-key",
			Data:         buf,
			Verb:         PutCommand,
			Timestamp:    now,
			WALName:      "my-wal",
			ProposalHash: [32]byte{0x01},
			PreviousHash: [32]byte{0x02},
			Hash:         [32]byte{0x03},
		}

		result, err := RawWALEntryInto[uint32](raw)
		require.NoError(t, err)

		assert.Equal(t, uint32(12345), result.Data)
		assert.Equal(t, "test-key", result.Key)
		assert.Equal(t, PutCommand, result.Verb)
		assert.Equal(t, now, result.Timestamp)
		assert.Equal(t, "my-wal", result.WALName)
		assert.Equal(t, [32]byte{0x01}, result.ProposalHash)
		assert.Equal(t, [32]byte{0x02}, result.PreviousHash)
		assert.Equal(t, [32]byte{0x03}, result.Hash)
	})

	t.Run("decodes uint64 data", func(t *testing.T) {
		var val uint64 = 0xDEADBEEFCAFEBABE
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, val)

		raw := RawWALEntry{
			Key:  "u64-key",
			Data: buf,
			Verb: AddCommand,
		}

		result, err := RawWALEntryInto[uint64](raw)
		require.NoError(t, err)
		assert.Equal(t, val, result.Data)
	})

	t.Run("decodes int32 data", func(t *testing.T) {
		var val int32 = -42
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, uint32(val))

		raw := RawWALEntry{
			Key:  "i32-key",
			Data: buf,
			Verb: SubtractCommand,
		}

		result, err := RawWALEntryInto[int32](raw)
		require.NoError(t, err)
		assert.Equal(t, val, result.Data)
	})

	t.Run("returns error for insufficient data", func(t *testing.T) {
		// Provide only 2 bytes when uint32 needs 4.
		raw := RawWALEntry{
			Key:  "short-key",
			Data: []byte{0x01, 0x02},
			Verb: PutCommand,
		}

		_, err := RawWALEntryInto[uint32](raw)
		assert.Error(t, err)
	})

	t.Run("returns error for empty data", func(t *testing.T) {
		raw := RawWALEntry{
			Key:  "empty-key",
			Data: []byte{},
			Verb: PutCommand,
		}

		_, err := RawWALEntryInto[uint32](raw)
		assert.Error(t, err)
	})

	t.Run("returns error for nil data", func(t *testing.T) {
		raw := RawWALEntry{
			Key:  "nil-key",
			Data: nil,
			Verb: PutCommand,
		}

		_, err := RawWALEntryInto[uint32](raw)
		assert.Error(t, err)
	})

	t.Run("preserves all metadata fields", func(t *testing.T) {
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, 0)

		proposalHash := [32]byte{0xAA, 0xBB}
		prevHash := [32]byte{0xCC, 0xDD}
		hash := [32]byte{0xEE, 0xFF}
		ts := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

		raw := RawWALEntry{
			Key:          "meta-key",
			Data:         buf,
			Verb:         DeleteCommand,
			Timestamp:    ts,
			WALName:      "meta-wal",
			ProposalHash: proposalHash,
			PreviousHash: prevHash,
			Hash:         hash,
		}

		result, err := RawWALEntryInto[uint32](raw)
		require.NoError(t, err)

		assert.Equal(t, "meta-key", result.Key)
		assert.Equal(t, DeleteCommand, result.Verb)
		assert.Equal(t, ts, result.Timestamp)
		assert.Equal(t, "meta-wal", result.WALName)
		assert.Equal(t, proposalHash, result.ProposalHash)
		assert.Equal(t, prevHash, result.PreviousHash)
		assert.Equal(t, hash, result.Hash)
	})
}

// ---------------------------------------------------------------------------
// WALEntry struct
// ---------------------------------------------------------------------------

func TestWALEntry(t *testing.T) {
	t.Run("zero value has expected defaults", func(t *testing.T) {
		var entry WALEntry[uint32]
		assert.Equal(t, "", entry.Key)
		assert.Equal(t, uint32(0), entry.Data)
		assert.Equal(t, StateMachineCommand(""), entry.Verb)
		assert.True(t, entry.Timestamp.IsZero())
		assert.Equal(t, "", entry.WALName)
		assert.Equal(t, [32]byte{}, entry.ProposalHash)
		assert.Equal(t, [32]byte{}, entry.PreviousHash)
		assert.Equal(t, [32]byte{}, entry.Hash)
	})
}

// ---------------------------------------------------------------------------
// Suggestion struct
// ---------------------------------------------------------------------------

func TestSuggestion(t *testing.T) {
	t.Run("fields are accessible", func(t *testing.T) {
		s := Suggestion[string]{
			Key:  "k",
			Data: "d",
			Verb: PutCommand,
		}
		assert.Equal(t, "k", s.Key)
		assert.Equal(t, "d", s.Data)
		assert.Equal(t, PutCommand, s.Verb)
	})
}

// ---------------------------------------------------------------------------
// Type aliases
// ---------------------------------------------------------------------------

func TestTypeAliases(t *testing.T) {
	t.Run("RawData is []byte", func(t *testing.T) {
		var rd RawData = []byte{1, 2, 3}
		assert.Equal(t, []byte{1, 2, 3}, rd)
	})

	t.Run("RawWALEntry is WALEntry[[]byte]", func(t *testing.T) {
		var entry RawWALEntry
		entry.Data = []byte{4, 5, 6}
		assert.Equal(t, []byte{4, 5, 6}, entry.Data)
	})
}
