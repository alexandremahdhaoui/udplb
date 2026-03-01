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
	"net"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePacket_Valid(t *testing.T) {
	sid := uuid.MustParse("01020304-0506-0708-090a-0b0c0d0e0f10")

	pkt := make([]byte, 20)
	binary.LittleEndian.PutUint32(pkt[0:4], PacketPrefix)
	copy(pkt[4:20], sid[:])

	parsed, ok := ParsePacket(pkt)
	require.True(t, ok)
	assert.Equal(t, sid, parsed)
}

func TestParsePacket_ValidWithExtraPayload(t *testing.T) {
	sid := uuid.New()

	pkt := make([]byte, 100)
	binary.LittleEndian.PutUint32(pkt[0:4], PacketPrefix)
	copy(pkt[4:20], sid[:])
	// Extra bytes after the header should not affect parsing.

	parsed, ok := ParsePacket(pkt)
	require.True(t, ok)
	assert.Equal(t, sid, parsed)
}

func TestParsePacket_TooShort(t *testing.T) {
	// 19 bytes is one short of the minimum.
	pkt := make([]byte, 19)
	binary.LittleEndian.PutUint32(pkt[0:4], PacketPrefix)

	_, ok := ParsePacket(pkt)
	assert.False(t, ok)
}

func TestParsePacket_EmptyPacket(t *testing.T) {
	_, ok := ParsePacket(nil)
	assert.False(t, ok)

	_, ok = ParsePacket([]byte{})
	assert.False(t, ok)
}

func TestParsePacket_WrongPrefix(t *testing.T) {
	sid := uuid.New()
	pkt := make([]byte, 20)
	binary.LittleEndian.PutUint32(pkt[0:4], 0xDEADBEEF)
	copy(pkt[4:20], sid[:])

	_, ok := ParsePacket(pkt)
	assert.False(t, ok)
}

func TestStats_Record(t *testing.T) {
	stats := NewStats()

	sid1 := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	sid2 := uuid.MustParse("11111111-2222-3333-4444-555555555555")

	addr1 := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 1234}
	addr2 := &net.UDPAddr{IP: net.ParseIP("10.0.0.2"), Port: 5678}

	stats.Record(sid1, addr1)
	stats.Record(sid1, addr1)
	stats.Record(sid1, addr1)
	stats.Record(sid2, addr2)
	stats.Record(sid2, addr2)

	snapshot := stats.Snapshot()
	assert.Equal(t, 5, snapshot.TotalReceived)
	assert.Equal(t, 2, snapshot.UniqueSessions)
	assert.Equal(t, 3, snapshot.Sessions[sid1.String()])
	assert.Equal(t, 2, snapshot.Sessions[sid2.String()])
}

func TestStats_Record_NilAddr(t *testing.T) {
	stats := NewStats()
	sid := uuid.New()

	// Should not panic with nil address.
	stats.Record(sid, nil)

	snapshot := stats.Snapshot()
	assert.Equal(t, 1, snapshot.TotalReceived)
	assert.Equal(t, 1, snapshot.Sessions[sid.String()])
}

func TestStats_Snapshot_Empty(t *testing.T) {
	stats := NewStats()
	snapshot := stats.Snapshot()

	assert.Equal(t, 0, snapshot.TotalReceived)
	assert.Equal(t, 0, snapshot.UniqueSessions)
	assert.Empty(t, snapshot.Sessions)
	assert.NotEmpty(t, snapshot.Timestamp, "timestamp must be set")
}
