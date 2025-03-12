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
	"errors"
	"fmt"
	"log/slog"
	"os"

	"sigs.k8s.io/yaml"
)

const usage = `USAGE:
	%s <COMMAND> <CONFIG FILE PATH>

	Available commands:
	- setup
	- teardown
	- run
`

type NetdevType string

const (
	NetdevTypeTap    NetdevType = "tap"
	NetdevTypeBridge NetdevType = "bridge"
)

type NetdevSpec struct {
	Name string     `json:"name"`
	IP   string     `json:"ip"`
	Type NetdevType `json:"type"`
	Link string     `json:"link,omitempty"` // e.g. a tap can set link with br0 (ip l set tap0 master br0)
}

type UDPLBSpec struct{}

type VMResourceSpec struct {
	CPUs int    `json:"cpus"`
	RAM  string `json:"ram"`
}

type VMSpec struct {
	Name    string       `json:"name"`
	Netdevs []NetdevSpec `json:"netdevs"`
	// TODO: Recorders must be dynamically set up??
	// Recorders []RecorderSpec `json:"recorders"`
	Resources VMResourceSpec `json:"resources"`
	UDPLB     UDPLBSpec      `json:"udplb"`
}

type Config struct {
	VMs         []VMSpec     `json:"vms"`
	HostNetdevs []NetdevSpec `json:"hostNetdevs"`
}

// Idea: Add support for redirecting packet to a != network interface.
//       -> to redirect packet to a dedicated internal network.
//       For now we XDP_PASS and let the network stack figure it out?

/******************************************************************************
 * main
 *
 *
 ******************************************************************************/

func main() {
	cfg, err := GetConfig()
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	// First implement the NoVM test.
	// Then we might try out the test using VMs.

	switch os.Args[2] {
	case "setup":
		err = Setup(cfg)
	case "teardown":
		err = Teardown(cfg)
	case "run":
		err = Run(cfg)
	case "vm-agent":
		err = RunVMAgent(cfg)
	}

	if err != nil {
		slog.Error(err.Error())
		slog.Info("Tearing down environment")
		_ = Teardown(cfg)
		os.Exit(1)
	}
}

/******************************************************************************
 * Teardown
 *
 *
 ******************************************************************************/

func Teardown(cfg Config) error {
	return nil
}

/******************************************************************************
 * Helpers
 *
 *
 ******************************************************************************/

func GetConfig() (Config, error) {
	if len(os.Args) != 3 {
		fmt.Printf(usage, os.Args[0])
		return Config{}, errors.New("incorrect arguments")
	}

	b, err := os.ReadFile(os.Args[1])
	if err != nil {
		return Config{}, err
	}

	cfg := Config{}
	if err = yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}
