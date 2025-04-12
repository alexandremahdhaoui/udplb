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
package types

import (
	"io"
	"os"

	yaml "sigs.k8s.io/yaml/goyaml.v2"
)

// -------------------------------------------------------------------
// -- CONFIG
// -------------------------------------------------------------------

type BackendConfig struct {
	Enabled bool   `json:"enabled"`
	IP      string `json:"ip"`
	MAC     string `json:"mac"`
	Port    int    `json:"port"`
}

type Config struct {
	Ifname string `json:"ifname"`
	IP     string `json:"ip"`
	Port   uint16 `json:"port"`

	Backends []BackendConfig `json:"backends"`
}

// -------------------------------------------------------------------
// -- GetConfig
// -------------------------------------------------------------------

func GetConfig(filepath string) (Config, error) {
	var (
		b   []byte
		err error
	)

	if filepath == "-" { // read from stdin
		// TODO: unblock when reading from stdin
		b, err = io.ReadAll(os.Stdin)
	} else {
		b, err = os.ReadFile(os.Args[1])
	}

	if err != nil {
		return Config{}, err
	}

	out := Config{}
	if err := yaml.Unmarshal(b, &out); err != nil {
		return Config{}, err
	}

	return out, nil
}
