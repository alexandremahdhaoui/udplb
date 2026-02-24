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
	"net"
	"time"
)

/*******************************************************************************
 * Protocol
 *
 ******************************************************************************/

// Protocol abstracts network transport for the cluster.
type Protocol interface {
	// Listen starts listening on the given address.
	// Returns a Listener for receiving data.
	Listen(addr string) (Listener, error)
	// Dial connects to a remote address for sending data.
	Dial(addr string) (Conn, error)
}

/*******************************************************************************
 * Listener
 *
 ******************************************************************************/

// Listener receives incoming data.
// net.PacketConn satisfies this interface directly.
type Listener interface {
	// ReadFrom reads a packet and returns the data and source address.
	ReadFrom(buf []byte) (n int, addr net.Addr, err error)
	// Close closes the listener.
	Close() error
	// LocalAddr returns the listener's local address.
	LocalAddr() net.Addr
	// SetReadDeadline sets the deadline for future ReadFrom calls.
	SetReadDeadline(t time.Time) error
}

/*******************************************************************************
 * Conn
 *
 ******************************************************************************/

// Conn sends data to a remote address.
// net.Conn satisfies this interface directly.
type Conn interface {
	// Write sends data to the remote address.
	Write(data []byte) (int, error)
	// Close closes the connection.
	Close() error
	// RemoteAddr returns the remote address.
	RemoteAddr() net.Addr
}

/*******************************************************************************
 * Compile-time interface satisfaction checks
 *
 ******************************************************************************/

// net.PacketConn satisfies Listener; net.Conn satisfies Conn.
// These are verified at compile time via the assertions below.
var (
	_ Listener = (net.PacketConn)(nil)
	_ Conn     = (net.Conn)(nil)
)

/*******************************************************************************
 * udpProtocol
 *
 ******************************************************************************/

type udpProtocol struct{}

// NewUDPProtocol returns a Protocol that uses UDP transport.
func NewUDPProtocol() Protocol {
	return &udpProtocol{}
}

// Listen opens a UDP listener on the given address.
// The returned Listener is a net.PacketConn, which satisfies Listener directly.
func (u *udpProtocol) Listen(addr string) (Listener, error) {
	return net.ListenPacket("udp", addr)
}

// Dial opens a UDP connection to the given address.
// The returned Conn is a net.Conn, which satisfies Conn directly.
func (u *udpProtocol) Dial(addr string) (Conn, error) {
	return net.Dial("udp", addr)
}
