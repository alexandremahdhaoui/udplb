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
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
)

// PacketPrefix is the 4-byte magic prefix for udplb packets.
// Must match the traffic generator and BPF program.
const PacketPrefix uint32 = 0x55554944

// MinPacketSize is the minimum valid packet size: 4-byte prefix + 16-byte UUID.
const MinPacketSize = 20

// ParsePacket extracts the session UUID from a raw udplb packet.
// Returns the UUID and true if the packet is valid, or uuid.Nil and false otherwise.
func ParsePacket(data []byte) (uuid.UUID, bool) {
	if len(data) < MinPacketSize {
		return uuid.Nil, false
	}

	prefix := binary.LittleEndian.Uint32(data[0:4])
	if prefix != PacketPrefix {
		return uuid.Nil, false
	}

	var sid uuid.UUID
	copy(sid[:], data[4:20])
	return sid, true
}

// Stats tracks per-session receive counts and source IPs. Safe for concurrent use.
type Stats struct {
	mu            sync.Mutex
	TotalReceived int
	Sessions      map[uuid.UUID]int
	SourceIPs     map[string]struct{}
}

// NewStats creates an initialized Stats instance.
func NewStats() *Stats {
	return &Stats{
		Sessions:  make(map[uuid.UUID]int),
		SourceIPs: make(map[string]struct{}),
	}
}

// Record adds a received packet to the stats.
func (s *Stats) Record(sessionID uuid.UUID, sourceAddr *net.UDPAddr) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.TotalReceived++
	s.Sessions[sessionID]++
	if sourceAddr != nil {
		s.SourceIPs[sourceAddr.IP.String()] = struct{}{}
	}
}

// Snapshot returns a JSON-serializable snapshot of current stats.
func (s *Stats) Snapshot() StatsReport {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessions := make(map[string]int, len(s.Sessions))
	for sid, count := range s.Sessions {
		sessions[sid.String()] = count
	}

	return StatsReport{
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		TotalReceived:  s.TotalReceived,
		UniqueSessions: len(s.Sessions),
		Sessions:       sessions,
	}
}

// StatsReport is the JSON output format for periodic and final reports.
type StatsReport struct {
	Timestamp      string         `json:"timestamp"`
	TotalReceived  int            `json:"totalReceived"`
	UniqueSessions int            `json:"uniqueSessions"`
	Sessions       map[string]int `json:"sessions"`
}

func main() {
	listen := flag.String("listen", ":8080", "Listen address")
	reportInterval := flag.Duration("report-interval", 5*time.Second, "How often to print stats")
	flag.Parse()

	addr, err := net.ResolveUDPAddr("udp", *listen)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: resolving listen address: %v\n", err)
		os.Exit(1)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: listening on UDP: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()

	stats := NewStats()

	// Set up signal handling.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start periodic reporting.
	reportTicker := time.NewTicker(*reportInterval)
	defer reportTicker.Stop()

	doneCh := make(chan struct{})

	// Report goroutine.
	go func() {
		for {
			select {
			case <-reportTicker.C:
				printReport(stats.Snapshot())
			case <-doneCh:
				return
			}
		}
	}()

	// Receive goroutine.
	go func() {
		buf := make([]byte, 65535)
		for {
			n, remoteAddr, err := conn.ReadFromUDP(buf)
			if err != nil {
				// Check if the connection was closed (normal shutdown).
				select {
				case <-doneCh:
					return
				default:
				}
				fmt.Fprintf(os.Stderr, "warning: read error: %v\n", err)
				continue
			}

			sid, ok := ParsePacket(buf[:n])
			if !ok {
				continue
			}

			stats.Record(sid, remoteAddr)

			// Echo the packet back to the sender.
			if remoteAddr != nil {
				_, _ = conn.WriteToUDP(buf[:n], remoteAddr)
			}
		}
	}()

	// Wait for signal.
	<-sigCh
	close(doneCh)
	_ = conn.Close()

	// Print final summary.
	fmt.Fprintln(os.Stderr, "received signal, shutting down")
	printReport(stats.Snapshot())
}

func printReport(report StatsReport) {
	data, err := json.Marshal(report)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: encoding report: %v\n", err)
		return
	}
	fmt.Println(string(data))
}
