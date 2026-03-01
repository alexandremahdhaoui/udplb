//go:build integration

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

package bpfadapter

import (
	"encoding/binary"
	"net"
	"testing"

	"github.com/alexandremahdhaoui/udplb/internal/util"
	"github.com/cilium/ebpf/rlimit"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// XDP return codes from linux/bpf.h.
const (
	xdpAborted  = 0
	xdpDrop     = 1
	xdpPass     = 2
	xdpTx       = 3
	xdpRedirect = 4
)

// Test constants.
var (
	lbIP   = net.ParseIP("10.0.0.1")
	lbPort = uint16(12345)

	clientMAC = net.HardwareAddr{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}
	lbMAC     = net.HardwareAddr{0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f}
	clientIP  = net.ParseIP("10.0.0.100")

	be0MAC = [6]uint8{0xaa, 0xbb, 0xcc, 0x00, 0x00, 0x01}
	be1MAC = [6]uint8{0xaa, 0xbb, 0xcc, 0x00, 0x00, 0x02}
	be2MAC = [6]uint8{0xaa, 0xbb, 0xcc, 0x00, 0x00, 0x03}
	be0IP  = net.ParseIP("10.0.0.10")
	be1IP  = net.ParseIP("10.0.0.11")
	be2IP  = net.ParseIP("10.0.0.12")

	bePort = uint16(8080)

	// udplbPacketPrefix matches UDPLB_PACKET_PREFIX (0x55554944) in udplb_kern_helpers.c.
	udplbPacketPrefix = uint32(0x55554944)
)

// -------------------------------------------------------------------
// -- Test helpers
// -------------------------------------------------------------------

// loadTestProgram loads the XDP program with the given config and returns the kernel objects.
func loadTestProgram(t *testing.T, ip net.IP, port uint16, lookupTableSize uint32) *udplbObjects {
	t.Helper()

	require.NoError(t, rlimit.RemoveMemlock())

	spec, err := loadUdplb()
	require.NoError(t, err)

	cfg := udplbConfigT{
		Ip:              util.NetIPv4ToUint32(ip),
		Port:            port,
		LookupTableSize: lookupTableSize,
		Mac:             [6]uint8(lbMAC),
	}
	require.NoError(t, spec.Variables["config"].Set(cfg))

	objs := new(udplbObjects)
	require.NoError(t, spec.LoadAndAssign(objs, nil))
	t.Cleanup(func() { _ = objs.Close() })

	return objs
}

// populateBackends writes backend specs into the active backend list map and
// populates the lookup table so hash_modulo(session_id) maps to a backend index.
func populateBackends(t *testing.T, objs *udplbObjects, backends []udplbBackendSpecT, lookupTableSize uint32) {
	t.Helper()

	// Write backends to backend_list_a (active_pointer defaults to 0 → reads from _a).
	for i, be := range backends {
		idx := uint32(i)
		require.NoError(t, objs.BackendListA.Put(idx, be))
	}

	// Populate lookup_table_a: distribute slots round-robin across backends.
	for i := uint32(0); i < lookupTableSize; i++ {
		backendIdx := i % uint32(len(backends))
		require.NoError(t, objs.LookupTableA.Put(i, backendIdx))
	}
}

// buildPacket constructs a raw Ethernet frame containing an IPv4/UDP packet
// with the UDPLB payload (4-byte prefix + 16-byte session UUID).
func buildPacket(srcMAC, dstMAC net.HardwareAddr, srcIP, dstIP net.IP, srcPort, dstPort uint16, sessionID uuid.UUID) []byte {
	const (
		ethLen  = 14
		ipLen   = 20
		udpLen  = 8
		dataLen = 20 // 4-byte prefix + 16-byte UUID
	)

	pkt := make([]byte, ethLen+ipLen+udpLen+dataLen)

	// -- Ethernet header
	copy(pkt[0:6], dstMAC)
	copy(pkt[6:12], srcMAC)
	binary.BigEndian.PutUint16(pkt[12:14], 0x0800) // ETH_P_IP

	// -- IPv4 header
	ip := pkt[ethLen:]
	ip[0] = 0x45                                                      // version=4, IHL=5
	binary.BigEndian.PutUint16(ip[2:4], uint16(ipLen+udpLen+dataLen)) // total length
	ip[8] = 64                                                        // TTL
	ip[9] = 17                                                        // IPPROTO_UDP
	copy(ip[12:16], srcIP.To4())
	copy(ip[16:20], dstIP.To4())

	// -- UDP header
	udp := pkt[ethLen+ipLen:]
	binary.BigEndian.PutUint16(udp[0:2], srcPort)
	binary.BigEndian.PutUint16(udp[2:4], dstPort)
	binary.BigEndian.PutUint16(udp[4:6], uint16(udpLen+dataLen))

	// -- UDPLB payload
	data := pkt[ethLen+ipLen+udpLen:]
	binary.LittleEndian.PutUint32(data[0:4], udplbPacketPrefix)
	copy(data[4:20], sessionID[:])

	return pkt
}

// makeBackendSpec creates a udplbBackendSpecT from IP, port, and MAC.
func makeBackendSpec(id uuid.UUID, ip net.IP, port uint16, mac [6]uint8) udplbBackendSpecT {
	be := udplbBackendSpecT{
		Ip:   util.NetIPv4ToUint32(ip),
		Port: port,
		Mac:  mac,
	}
	copy(be.Id[:], id[:])
	return be
}

// extractDstMAC reads the destination MAC from a raw Ethernet frame.
func extractDstMAC(pkt []byte) net.HardwareAddr {
	return net.HardwareAddr(pkt[0:6])
}

// extractSrcMAC reads the source MAC from a raw Ethernet frame.
func extractSrcMAC(pkt []byte) net.HardwareAddr {
	return net.HardwareAddr(pkt[6:12])
}

// extractDstIP reads the destination IP from the IPv4 header in a raw frame.
func extractDstIP(pkt []byte) net.IP {
	return net.IP(pkt[30:34]) // 14 (eth) + 16 (ip offset to daddr)
}

// extractDstPort reads the UDP destination port from a raw frame.
func extractDstPort(pkt []byte) uint16 {
	return binary.BigEndian.Uint16(pkt[36:38]) // 14 (eth) + 20 (ip) + 2 (udp offset to dst)
}

// -------------------------------------------------------------------
// -- Tests: packets that should NOT be load-balanced (XDP_PASS)
// -------------------------------------------------------------------

func TestXDP_WrongDestIP_ReturnsPass(t *testing.T) {
	objs := loadTestProgram(t, lbIP, lbPort, 7)

	pkt := buildPacket(clientMAC, lbMAC, clientIP, net.ParseIP("10.0.0.99"), 54321, lbPort, uuid.New())

	ret, _, err := objs.Udplb.Test(pkt)
	require.NoError(t, err)
	assert.Equal(t, uint32(xdpPass), ret, "packet to wrong dest IP should pass through")
}

func TestXDP_WrongDestPort_ReturnsPass(t *testing.T) {
	objs := loadTestProgram(t, lbIP, lbPort, 7)

	pkt := buildPacket(clientMAC, lbMAC, clientIP, lbIP, 54321, 9999, uuid.New())

	ret, _, err := objs.Udplb.Test(pkt)
	require.NoError(t, err)
	assert.Equal(t, uint32(xdpPass), ret, "packet to wrong dest port should pass through")
}

func TestXDP_NonUDP_ReturnsPass(t *testing.T) {
	objs := loadTestProgram(t, lbIP, lbPort, 7)

	pkt := buildPacket(clientMAC, lbMAC, clientIP, lbIP, 54321, lbPort, uuid.New())
	// Overwrite IP protocol from UDP (17) to TCP (6).
	pkt[14+9] = 6

	ret, _, err := objs.Udplb.Test(pkt)
	require.NoError(t, err)
	assert.Equal(t, uint32(xdpPass), ret, "TCP packet should pass through")
}

func TestXDP_NonIPv4_ReturnsPass(t *testing.T) {
	objs := loadTestProgram(t, lbIP, lbPort, 7)

	pkt := buildPacket(clientMAC, lbMAC, clientIP, lbIP, 54321, lbPort, uuid.New())
	// Overwrite ethertype from IPv4 (0x0800) to IPv6 (0x86DD).
	binary.BigEndian.PutUint16(pkt[12:14], 0x86DD)

	ret, _, err := objs.Udplb.Test(pkt)
	require.NoError(t, err)
	assert.Equal(t, uint32(xdpPass), ret, "IPv6 packet should pass through")
}

func TestXDP_WrongPrefix_ReturnsPass(t *testing.T) {
	objs := loadTestProgram(t, lbIP, lbPort, 7)

	pkt := buildPacket(clientMAC, lbMAC, clientIP, lbIP, 54321, lbPort, uuid.New())
	// Overwrite UDPLB prefix with garbage.
	binary.LittleEndian.PutUint32(pkt[42:46], 0xDEADBEEF)

	ret, _, err := objs.Udplb.Test(pkt)
	require.NoError(t, err)
	assert.Equal(t, uint32(xdpPass), ret, "packet with wrong prefix should pass through")
}

func TestXDP_TruncatedPacket_ReturnsPass(t *testing.T) {
	objs := loadTestProgram(t, lbIP, lbPort, 7)

	// Build a valid packet and truncate it before the UDPLB payload.
	pkt := buildPacket(clientMAC, lbMAC, clientIP, lbIP, 54321, lbPort, uuid.New())
	pkt = pkt[:42] // cut off UDPLB payload (eth + ip + udp = 42 bytes)

	ret, _, err := objs.Udplb.Test(pkt)
	require.NoError(t, err)
	assert.Equal(t, uint32(xdpPass), ret, "truncated packet should pass through")
}

// -------------------------------------------------------------------
// -- Tests: packets that SHOULD be load-balanced (XDP_TX)
// -------------------------------------------------------------------

func TestXDP_ValidPacket_ReturnsTX(t *testing.T) {
	const lookupTableSize = uint32(7)

	objs := loadTestProgram(t, lbIP, lbPort, lookupTableSize)

	be0ID := uuid.New()
	backends := []udplbBackendSpecT{
		makeBackendSpec(be0ID, be0IP, bePort, be0MAC),
	}
	populateBackends(t, objs, backends, lookupTableSize)

	sessionID := uuid.New()
	pkt := buildPacket(clientMAC, lbMAC, clientIP, lbIP, 54321, lbPort, sessionID)

	ret, out, err := objs.Udplb.Test(pkt)
	require.NoError(t, err)
	assert.Equal(t, uint32(xdpTx), ret, "valid UDPLB packet should be forwarded via XDP_TX")

	// Verify destination MAC was rewritten to backend MAC.
	gotMAC := extractDstMAC(out)
	assert.Equal(t, net.HardwareAddr(be0MAC[:]), gotMAC, "dst MAC should be rewritten to backend MAC")

	// Verify source MAC was rewritten to LB interface MAC.
	gotSrcMAC := extractSrcMAC(out)
	assert.Equal(t, lbMAC, gotSrcMAC, "src MAC should be rewritten to LB interface MAC")

	// Verify destination IP was rewritten to backend IP.
	gotIP := extractDstIP(out)
	assert.True(t, be0IP.Equal(gotIP), "dst IP should be rewritten to backend IP, got %s", gotIP)

	// Verify destination port was rewritten to backend port.
	gotPort := extractDstPort(out)
	assert.Equal(t, bePort, gotPort, "dst port should be rewritten to backend port")
}

func TestXDP_SessionAffinity(t *testing.T) {
	const lookupTableSize = uint32(7)

	objs := loadTestProgram(t, lbIP, lbPort, lookupTableSize)

	be0ID := uuid.New()
	be1ID := uuid.New()
	be2ID := uuid.New()
	backends := []udplbBackendSpecT{
		makeBackendSpec(be0ID, be0IP, bePort, be0MAC),
		makeBackendSpec(be1ID, be1IP, bePort, be1MAC),
		makeBackendSpec(be2ID, be2IP, bePort, be2MAC),
	}
	populateBackends(t, objs, backends, lookupTableSize)

	// Send the same session 50 times; all should hit the same backend.
	sessionID := uuid.New()
	var firstMAC net.HardwareAddr

	for i := 0; i < 50; i++ {
		pkt := buildPacket(clientMAC, lbMAC, clientIP, lbIP, 54321, lbPort, sessionID)
		ret, out, err := objs.Udplb.Test(pkt)
		require.NoError(t, err)
		require.Equal(t, uint32(xdpTx), ret, "iteration %d: should return XDP_TX", i)

		gotMAC := extractDstMAC(out)
		gotSrcMAC := extractSrcMAC(out)
		assert.Equal(t, lbMAC, gotSrcMAC,
			"iteration %d: src MAC should be LB interface MAC", i)
		if i == 0 {
			firstMAC = gotMAC
			t.Logf("Session %s assigned to backend MAC %s", sessionID, firstMAC)
		} else {
			assert.Equal(t, firstMAC, gotMAC,
				"iteration %d: session affinity broken, expected %s got %s", i, firstMAC, gotMAC)
		}
	}
}

func TestXDP_MultiSessionDistribution(t *testing.T) {
	const lookupTableSize = uint32(7)

	objs := loadTestProgram(t, lbIP, lbPort, lookupTableSize)

	be0ID := uuid.New()
	be1ID := uuid.New()
	be2ID := uuid.New()
	backends := []udplbBackendSpecT{
		makeBackendSpec(be0ID, be0IP, bePort, be0MAC),
		makeBackendSpec(be1ID, be1IP, bePort, be1MAC),
		makeBackendSpec(be2ID, be2IP, bePort, be2MAC),
	}
	populateBackends(t, objs, backends, lookupTableSize)

	// Send 100 unique sessions and count which backend each hits.
	macToCount := make(map[string]int)
	for i := 0; i < 100; i++ {
		sessionID := uuid.New()
		pkt := buildPacket(clientMAC, lbMAC, clientIP, lbIP, 54321, lbPort, sessionID)

		ret, out, err := objs.Udplb.Test(pkt)
		require.NoError(t, err)
		require.Equal(t, uint32(xdpTx), ret, "session %d: should return XDP_TX", i)

		gotMAC := extractDstMAC(out)
		macToCount[gotMAC.String()]++
	}

	t.Logf("Distribution across backends: %v", macToCount)

	// With 3 backends and 100 sessions, each backend should receive at least 1 session.
	// Hash distribution won't be perfect but should not be degenerate.
	assert.Len(t, macToCount, 3, "all 3 backends should receive at least 1 session")
	for mac, count := range macToCount {
		assert.Greater(t, count, 0, "backend %s received 0 sessions", mac)
		assert.Less(t, count, 80, "backend %s received %d/100 sessions (>80%% is degenerate)", mac, count)
	}
}

func TestXDP_PacketRewrite_Checksums(t *testing.T) {
	const lookupTableSize = uint32(7)

	objs := loadTestProgram(t, lbIP, lbPort, lookupTableSize)

	be0ID := uuid.New()
	backends := []udplbBackendSpecT{
		makeBackendSpec(be0ID, be0IP, bePort, be0MAC),
	}
	populateBackends(t, objs, backends, lookupTableSize)

	sessionID := uuid.New()
	pkt := buildPacket(clientMAC, lbMAC, clientIP, lbIP, 54321, lbPort, sessionID)

	ret, out, err := objs.Udplb.Test(pkt)
	require.NoError(t, err)
	require.Equal(t, uint32(xdpTx), ret)

	// Verify IP checksum is non-zero (was recomputed by XDP program).
	ipCsum := binary.BigEndian.Uint16(out[24:26]) // 14 (eth) + 10 (ip checksum offset)
	assert.NotZero(t, ipCsum, "IP checksum should be recomputed after rewrite")

	// Verify the IP header checksum is valid by computing it ourselves.
	ipHeader := make([]byte, 20)
	copy(ipHeader, out[14:34])
	// Zero out checksum field for verification.
	ipHeader[10] = 0
	ipHeader[11] = 0
	var sum uint32
	for i := 0; i < 20; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(ipHeader[i : i+2]))
	}
	for sum > 0xffff {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	computedCsum := uint16(^sum)
	assert.Equal(t, computedCsum, ipCsum, "IP checksum should be valid")
}
