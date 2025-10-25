package types_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"testing"
	"time"

	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

type TestData struct {
	Value string
}

func TestNewProposal(t *testing.T) {
	t.Run("should create a new proposal", func(t *testing.T) {
		key := "test_key"
		data := TestData{Value: "test data"}
		proposal := types.NewProposal(key, data)
		assert.NotNil(t, proposal)
		assert.Equal(t, key, proposal.Key)
		assert.Equal(t, data, proposal.Data)
	})
}

func TestTransformProposalIntoWALEntry(t *testing.T) {
	t.Run("should transform a proposal into a WAL entry", func(t *testing.T) {
		walID := uuid.New()
		previousHash := sha256.Sum256([]byte("previous hash"))
		key := "test_key"
		data := TestData{Value: "test data"}
		proposal := types.NewProposal(key, data)
		entry, err := types.TransformProposalIntoWALEntry(walID, previousHash, proposal)
		assert.NoError(t, err)
		assert.NotNil(t, entry)
		assert.Equal(t, previousHash, entry.PreviousHash)
		assert.WithinDuration(t, time.Now(), entry.Timestamp, time.Second)
		assert.NotEqual(t, [32]byte{}, entry.Hash)
	})
}

func TestRawWALEntryInto(t *testing.T) {
	t.Run("should decode a raw WAL entry", func(t *testing.T) {
		key := "test_key"
		data := TestData{Value: "test data"}

		// Encode the data to RawData
		var dataBuf bytes.Buffer
		err := gob.NewEncoder(&dataBuf).Encode(data)
		assert.NoError(t, err)

		rawEntry := types.RawWALEntry{
			Key:          key,
			Data:         dataBuf.Bytes(),
			Timestamp:    time.Now(),
			WALName:      "test_wal",
			ProposalHash: sha256.Sum256([]byte("proposal")),
			PreviousHash: sha256.Sum256([]byte("previous")),
			Hash:         sha256.Sum256([]byte("hash")),
		}

		decodedEntry, err := types.RawWALEntryInto[TestData](rawEntry)
		assert.NoError(t, err)
		assert.NotNil(t, decodedEntry)
		assert.Equal(t, rawEntry.Key, decodedEntry.Key)
		assert.Equal(t, data, decodedEntry.Data)
	})
}
