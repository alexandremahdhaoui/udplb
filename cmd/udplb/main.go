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
	"context"
	"fmt"
	"net"
	"os"
	"time"

	bpfadapter "github.com/alexandremahdhaoui/udplb/internal/adapter/bpf"
	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/alexandremahdhaoui/udplb/internal/util"
	"github.com/google/uuid"
)

// TODO: end to end test with qemu vms or so.
// TODO: init a context w/ ifname & process name & use slog.{Info,Error}Context instead.

const usage = `USAGE:
	%s <config file path>
`

func main() {
	if len(os.Args) != 2 {
		fmtExit(usage, os.Args[0])
	}

	ctx := context.TODO()

	// TODO: these values should come from config
	_ = "todo" // name placeholder
	instanceId := uuid.New()
	iface, _ := net.InterfaceByName("todo")
	ip := net.ParseIP("todo")
	port := uint16(12345)
	lookupTableSize := uint32(23)

	// Create assignment watcher mux for the BPF manager
	assignmentWatcherMux := util.NewWatcherMux[types.Assignment](100, util.NewDispatchFuncWithTimeout[types.Assignment](time.Second))

	bpfProgram, manager, err := bpfadapter.New(instanceId, iface, ip, port, lookupTableSize, assignmentWatcherMux)
	if err != nil {
		errExit(err)
	}

	// move to controller
	if err := bpfProgram.Run(ctx); err != nil {
		errExit(err)
	}

	// move to controller
	if err := manager.Run(ctx); err != nil {
		errExit(err)
	}

	// TODO: move monitor adapter integration to controller
	// var (
	// 	bl monitoradapter.backendSpecList
	// 	bs monitoradapter.backendState
	// 	ra monitoradapter.remoteAssignment
	// )
	// blCh, blCancel := bl.Watch()
	// bsCh, bsCancel := bs.Watch()
	// raCh, raCancel := ra.Watch()
}

func fmtExit(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format, a...)
	os.Exit(1)
}

func errExit(err error) {
	fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(1)
}
