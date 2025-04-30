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

	"github.com/alexandremahdhaoui/tooling/pkg/flaterrors"
	bpfadapter "github.com/alexandremahdhaoui/udplb/internal/adapter/bpf"
	monitoradapter "github.com/alexandremahdhaoui/udplb/internal/adapter/monitor"
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

	name := "todo"
	instanceId := uuid.New()
	iface, _ := net.InterfaceByName("todo")
	ip := net.ParseIP("todo")
	port := uint16(12345)
	lookupTableSize := uint32(23)

	bpfProgram, manager, err := bpfadapter.New(instanceId, iface, ip, port, lookupTableSize)
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

	var (
		bsl monitoradapter.BackendSpecList
		bs  monitoradapter.BackendState
		ra  monitoradapter.RemoteAssignment
	)

	// move to controller
	var errs error
	bslCh, err := bsl.Watch()
	errs = flaterrors.Join(errs, err)
	bsCh, err := bs.Watch()
	errs = flaterrors.Join(errs, err)
	raCh, err := ra.Watch()
	errs = flaterrors.Join(errs, err)
	if errs != nil {
		errExit(errs)
	}
}

func fmtExit(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format, a...)
	os.Exit(1)
}

func errExit(err error) {
	fmt.Fprintf(os.Stderr, err.Error())
	os.Exit(1)
}
