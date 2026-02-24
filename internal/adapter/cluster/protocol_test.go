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
package clusteradpater

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUDPProtocol(t *testing.T) {
	p := NewUDPProtocol()
	assert.NotNil(t, p)
}

func TestUDPProtocol_Listen(t *testing.T) {
	p := NewUDPProtocol()
	listener, err := p.Listen("127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	addr := listener.LocalAddr()
	require.NotNil(t, addr)
	assert.NotEmpty(t, addr.String())
}

func TestUDPProtocol_Dial(t *testing.T) {
	p := NewUDPProtocol()

	// Start a listener first so we have a valid address to dial.
	listener, err := p.Listen("127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	conn, err := p.Dial(listener.LocalAddr().String())
	require.NoError(t, err)
	defer conn.Close()

	assert.NotNil(t, conn.RemoteAddr())
}

func TestUDPProtocol_RoundTrip(t *testing.T) {
	p := NewUDPProtocol()

	// Set up a listener on a random port.
	listener, err := p.Listen("127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	// Dial the listener address.
	conn, err := p.Dial(listener.LocalAddr().String())
	require.NoError(t, err)
	defer conn.Close()

	// Write data via the Conn.
	payload := []byte("hello udp")
	n, err := conn.Write(payload)
	require.NoError(t, err)
	assert.Equal(t, len(payload), n)

	// Read data via the Listener.
	buf := make([]byte, 1024)
	n, addr, err := listener.ReadFrom(buf)
	require.NoError(t, err)
	assert.Equal(t, payload, buf[:n])
	assert.NotNil(t, addr)
}

func TestUDPProtocol_Close(t *testing.T) {
	p := NewUDPProtocol()

	listener, err := p.Listen("127.0.0.1:0")
	require.NoError(t, err)

	conn, err := p.Dial(listener.LocalAddr().String())
	require.NoError(t, err)

	assert.NoError(t, conn.Close())
	assert.NoError(t, listener.Close())
}
