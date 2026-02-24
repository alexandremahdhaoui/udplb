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
package monitoradapter

import (
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/alexandremahdhaoui/udplb/internal/util"

	"github.com/google/uuid"
)

// TODO: Define & Implement LiveConfig
// - List&Watch kubernetes resource definitions such as Service type loadbalancer.

// TODO: define what's in a types.LiveConfig or in types.LiveSpec.

// -- LBSpec
// The LB spec or config cannot be updated at runtime.

var _ types.Watcher[BackendSpecMap] = &backendSpecList{}

type BackendSpecMap = map[uuid.UUID]types.BackendSpec

type backendSpecList struct {
	specMap     BackendSpecMap
	watcherMux  *util.WatcherMux[BackendSpecMap]
	doneCh      chan struct{}
	terminateCh chan struct{}
	dispatched  bool
	mu          *sync.Mutex
}

// NewBackendSpecList parses config.Backends into a BackendSpecMap and
// returns a watcher that dispatches the map on the first Watch call.
func NewBackendSpecList(config types.Config, watcherMux *util.WatcherMux[BackendSpecMap]) *backendSpecList {
	specMap := make(BackendSpecMap)

	for _, bc := range config.Backends {
		if !bc.Enabled {
			continue
		}

		ip := net.ParseIP(bc.IP)
		if ip == nil {
			slog.Warn("skipping backend with invalid IP",
				"ip", bc.IP, "port", bc.Port)
			continue
		}

		mac, err := net.ParseMAC(bc.MAC)
		if err != nil {
			slog.Warn("skipping backend with invalid MAC",
				"mac", bc.MAC, "ip", bc.IP, "port", bc.Port, "err", err)
			continue
		}

		id := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(fmt.Sprintf("%s:%d", bc.IP, bc.Port)))
		specMap[id] = types.BackendSpec{
			IP:      ip,
			Port:    bc.Port,
			MacAddr: mac,
			State:   types.StateAvailable,
		}
	}

	return &backendSpecList{
		specMap:     specMap,
		watcherMux:  watcherMux,
		doneCh:      make(chan struct{}),
		terminateCh: make(chan struct{}),
		dispatched:  false,
		mu:          &sync.Mutex{},
	}
}

func (b *backendSpecList) Watch() (<-chan BackendSpecMap, func()) {
	ch, cancel := b.watcherMux.Watch(util.NoFilter)

	b.mu.Lock()
	if !b.dispatched {
		b.dispatched = true
		specMap := b.specMap
		go func() {
			b.watcherMux.Dispatch(specMap)
		}()
	}
	b.mu.Unlock()

	return ch, cancel
}

func (b *backendSpecList) Close() error {
	close(b.terminateCh)
	_ = b.watcherMux.Close()
	close(b.doneCh)
	return nil
}

func (b *backendSpecList) Done() <-chan struct{} {
	return b.doneCh
}
