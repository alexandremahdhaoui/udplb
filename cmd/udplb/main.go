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
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/alexandremahdhaoui/udplb/internal/controller"
	"github.com/alexandremahdhaoui/udplb/internal/types"
)

const usage = `USAGE:
	%s <config file path>
`

func main() {
	if len(os.Args) != 2 {
		fmtExit(usage, os.Args[0])
	}

	config, err := types.GetConfig(os.Args[1])
	if err != nil {
		errExit(err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	ctrl, err := controller.New(config)
	if err != nil {
		errExit(err)
	}

	if err := ctrl.Run(ctx); err != nil {
		errExit(err)
	}

	slog.Info("udplb started", "ifname", config.Ifname, "ip", config.IP, "port", config.Port)

	// Block until signal.
	<-ctx.Done()
	slog.Info("shutting down...")

	if err := ctrl.Close(); err != nil {
		slog.Error("shutdown error", "err", err)
	}
}

func fmtExit(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format, a...)
	os.Exit(1)
}

func errExit(err error) {
	fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(1)
}
