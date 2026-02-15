//go:build e2e

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

package e2e

import (
	"os"
	"strconv"
)

// DefaultUDPTestPort is the default UDP port for socat connectivity tests.
const DefaultUDPTestPort = 12345

// UDPTestPort is the UDP port for e2e socat connectivity tests.
// Initialized from PORTALLOC_UDPLB_E2E_UDP env var with fallback to DefaultUDPTestPort.
var UDPTestPort = GetUDPTestPort()

// GetUDPTestPort returns the UDP test port.
// It reads from the PORTALLOC_UDPLB_E2E_UDP env var (set by forge testenv create via
// allocateOpenPort in forge.yaml) with a fallback to DefaultUDPTestPort for manual usage.
func GetUDPTestPort() int {
	if v := os.Getenv("PORTALLOC_UDPLB_E2E_UDP"); v != "" {
		port, err := strconv.Atoi(v)
		if err == nil {
			return port
		}
	}
	return DefaultUDPTestPort
}
