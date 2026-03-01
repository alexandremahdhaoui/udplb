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

package main

import (
	"encoding/binary"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildPacket_Format(t *testing.T) {
	sid := uuid.MustParse("01020304-0506-0708-090a-0b0c0d0e0f10")
	pkt := BuildPacket(sid, 0)

	require.Len(t, pkt, 20, "packet must be 4-byte prefix + 16-byte UUID")

	// Verify the 4-byte little-endian prefix matches 0x55554944.
	prefix := binary.LittleEndian.Uint32(pkt[0:4])
	assert.Equal(t, uint32(0x55554944), prefix)

	// Verify the raw prefix bytes: LE representation of 0x55554944 is [0x44, 0x49, 0x55, 0x55].
	assert.Equal(t, byte(0x44), pkt[0])
	assert.Equal(t, byte(0x49), pkt[1])
	assert.Equal(t, byte(0x55), pkt[2])
	assert.Equal(t, byte(0x55), pkt[3])

	// Verify UUID bytes match.
	var parsed uuid.UUID
	copy(parsed[:], pkt[4:20])
	assert.Equal(t, sid, parsed)
}

func TestBuildPacket_WithPayload(t *testing.T) {
	sid := uuid.New()
	payloadSize := 64
	pkt := BuildPacket(sid, payloadSize)

	assert.Len(t, pkt, 4+16+payloadSize)

	// Prefix must still be correct.
	prefix := binary.LittleEndian.Uint32(pkt[0:4])
	assert.Equal(t, uint32(0x55554944), prefix)

	// UUID must still be correct.
	var parsed uuid.UUID
	copy(parsed[:], pkt[4:20])
	assert.Equal(t, sid, parsed)

	// Payload bytes must be zeroed.
	for i := 20; i < len(pkt); i++ {
		assert.Equal(t, byte(0), pkt[i], "payload byte at offset %d must be zero", i)
	}
}

func TestGenerateSessions_Uniqueness(t *testing.T) {
	n := 50
	sessions := GenerateSessions(n)

	require.Len(t, sessions, n)

	seen := make(map[uuid.UUID]struct{}, n)
	for _, s := range sessions {
		_, exists := seen[s]
		assert.False(t, exists, "duplicate session UUID: %s", s)
		seen[s] = struct{}{}
	}
}

func TestGenerateSessions_Count(t *testing.T) {
	for _, n := range []int{1, 5, 100} {
		sessions := GenerateSessions(n)
		assert.Len(t, sessions, n)
	}
}
