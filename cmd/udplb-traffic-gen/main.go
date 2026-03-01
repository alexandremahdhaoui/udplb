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
	"syscall"
	"time"

	"github.com/google/uuid"
)

// PacketPrefix is the 4-byte magic prefix for udplb packets.
// Matches the BPF definition: (0x55 << 24) + (0x55 << 16) + (0x49 << 8) + 0x44
// Written in little-endian byte order to match x86/BPF memory layout.
const PacketPrefix uint32 = 0x55554944

// SessionStats tracks per-session packet counts.
type SessionStats struct {
	Sent int `json:"sent"`
}

// Report is the final JSON output printed on completion.
type Report struct {
	TotalSent int                      `json:"totalSent"`
	Duration  string                   `json:"duration"`
	Sessions  map[string]*SessionStats `json:"sessions"`
}

// GenerateSessions creates n unique session UUIDs.
func GenerateSessions(n int) []uuid.UUID {
	sessions := make([]uuid.UUID, n)
	for i := 0; i < n; i++ {
		sessions[i] = uuid.New()
	}
	return sessions
}

// BuildPacket constructs a udplb packet: [4-byte prefix LE][16-byte UUID][optional payload].
func BuildPacket(sessionID uuid.UUID, payloadSize int) []byte {
	buf := make([]byte, 4+16+payloadSize)
	binary.LittleEndian.PutUint32(buf[0:4], PacketPrefix)
	copy(buf[4:20], sessionID[:])
	return buf
}

func main() {
	target := flag.String("target", "", "Target IP:port (e.g., 10.100.0.200:12345)")
	sessions := flag.Int("sessions", 10, "Number of unique sessions to generate")
	rate := flag.Int("rate", 100, "Packets per second")
	duration := flag.Duration("duration", 10*time.Second, "How long to send traffic")
	payloadSize := flag.Int("payload-size", 0, "Extra payload bytes after the session header")
	flag.Parse()

	if *target == "" {
		fmt.Fprintln(os.Stderr, "error: --target is required")
		flag.Usage()
		os.Exit(1)
	}

	if *sessions <= 0 {
		fmt.Fprintln(os.Stderr, "error: --sessions must be > 0")
		os.Exit(1)
	}

	if *rate <= 0 {
		fmt.Fprintln(os.Stderr, "error: --rate must be > 0")
		os.Exit(1)
	}

	addr, err := net.ResolveUDPAddr("udp", *target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: resolving target address: %v\n", err)
		os.Exit(1)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: dialing UDP: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()

	sessionIDs := GenerateSessions(*sessions)

	// Pre-build packets for each session.
	packets := make([][]byte, len(sessionIDs))
	for i, sid := range sessionIDs {
		packets[i] = BuildPacket(sid, *payloadSize)
	}

	// Initialize per-session stats.
	report := &Report{
		Sessions: make(map[string]*SessionStats, len(sessionIDs)),
	}
	for _, sid := range sessionIDs {
		report.Sessions[sid.String()] = &SessionStats{}
	}

	// Set up signal handling.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Send packets at the configured rate using a ticker.
	interval := time.Duration(float64(time.Second) / float64(*rate))
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	timer := time.NewTimer(*duration)
	defer timer.Stop()

	start := time.Now()
	idx := 0

	for {
		select {
		case <-ticker.C:
			sessionIdx := idx % len(sessionIDs)
			_, err := conn.Write(packets[sessionIdx])
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: send error: %v\n", err)
			} else {
				report.TotalSent++
				report.Sessions[sessionIDs[sessionIdx].String()].Sent++
			}
			idx++

		case <-timer.C:
			report.Duration = time.Since(start).Round(time.Millisecond).String()
			printReport(report)
			return

		case <-sigCh:
			report.Duration = time.Since(start).Round(time.Millisecond).String()
			printReport(report)
			return
		}
	}
}

func printReport(report *Report) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "error: encoding report: %v\n", err)
	}
}
