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
package util

import (
	"encoding/binary"
	"math"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// ParseIPV4ToUint32
// ---------------------------------------------------------------------------

func TestParseIPV4ToUint32_ValidIPs(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want uint32
	}{
		{
			name: "loopback",
			ip:   "127.0.0.1",
			want: binary.NativeEndian.Uint32(net.ParseIP("127.0.0.1").To4()),
		},
		{
			name: "all zeros",
			ip:   "0.0.0.0",
			want: binary.NativeEndian.Uint32(net.ParseIP("0.0.0.0").To4()),
		},
		{
			name: "broadcast",
			ip:   "255.255.255.255",
			want: binary.NativeEndian.Uint32(net.ParseIP("255.255.255.255").To4()),
		},
		{
			name: "private class A",
			ip:   "10.0.0.1",
			want: binary.NativeEndian.Uint32(net.ParseIP("10.0.0.1").To4()),
		},
		{
			name: "private class C",
			ip:   "192.168.1.100",
			want: binary.NativeEndian.Uint32(net.ParseIP("192.168.1.100").To4()),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseIPV4ToUint32(tt.ip)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseIPV4ToUint32_InvalidIPs(t *testing.T) {
	tests := []struct {
		name string
		ip   string
	}{
		{name: "empty string", ip: ""},
		{name: "garbage text", ip: "not-an-ip"},
		{name: "ipv6 address", ip: "::1"},
		{name: "partial ip", ip: "192.168.1"},
		{name: "too many octets", ip: "1.2.3.4.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseIPV4ToUint32(tt.ip)
			assert.Error(t, err)
		})
	}
}

// ---------------------------------------------------------------------------
// NetIPv4ToUint32
// ---------------------------------------------------------------------------

func TestNetIPv4ToUint32(t *testing.T) {
	tests := []struct {
		name string
		ip   net.IP
	}{
		{name: "loopback", ip: net.ParseIP("127.0.0.1")},
		{name: "all zeros", ip: net.ParseIP("0.0.0.0")},
		{name: "broadcast", ip: net.ParseIP("255.255.255.255")},
		{name: "private", ip: net.ParseIP("10.1.2.3")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NetIPv4ToUint32(tt.ip)
			expected := binary.NativeEndian.Uint32(tt.ip.To4())
			assert.Equal(t, expected, got)
		})
	}
}

// ---------------------------------------------------------------------------
// ParseIEEE802MAC
// ---------------------------------------------------------------------------

func TestParseIEEE802MAC_Valid(t *testing.T) {
	tests := []struct {
		name string
		mac  string
		want [6]uint8
	}{
		{
			name: "colon separated lowercase",
			mac:  "aa:bb:cc:dd:ee:ff",
			want: [6]uint8{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
		},
		{
			name: "colon separated uppercase",
			mac:  "AA:BB:CC:DD:EE:FF",
			want: [6]uint8{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
		},
		{
			name: "all zeros",
			mac:  "00:00:00:00:00:00",
			want: [6]uint8{0, 0, 0, 0, 0, 0},
		},
		{
			name: "hyphen separated",
			mac:  "01-23-45-67-89-ab",
			want: [6]uint8{0x01, 0x23, 0x45, 0x67, 0x89, 0xab},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseIEEE802MAC(tt.mac)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseIEEE802MAC_Invalid(t *testing.T) {
	tests := []struct {
		name string
		mac  string
	}{
		{name: "empty string", mac: ""},
		{name: "garbage", mac: "not-a-mac"},
		{name: "too short", mac: "aa:bb:cc"},
		{name: "invalid hex", mac: "gg:hh:ii:jj:kk:ll"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseIEEE802MAC(tt.mac)
			assert.Error(t, err)
		})
	}
}

func TestParseIEEE802MAC_EUI64ReturnsError(t *testing.T) {
	// EUI-64 has 8 bytes -- net.ParseMAC accepts it but it is not IEEE 802
	// (6 bytes). The function must reject it.
	_, err := ParseIEEE802MAC("01:23:45:67:89:ab:cd:ef")
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// ValidateUint16Port
// ---------------------------------------------------------------------------

func TestValidateUint16Port_Valid(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{name: "min valid port 1000", port: 1000},
		{name: "common port 8080", port: 8080},
		{name: "max uint16", port: math.MaxUint16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NoError(t, ValidateUint16Port(tt.port))
		})
	}
}

func TestValidateUint16Port_Invalid(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{name: "zero", port: 0},
		{name: "negative", port: -1},
		{name: "below 1000", port: 999},
		{name: "above max uint16", port: math.MaxUint16 + 1},
		{name: "very large", port: 1_000_000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUint16Port(tt.port)
			assert.ErrorIs(t, err, ErrInvalidUDPLBPort)
		})
	}
}
