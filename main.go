// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/mertdotcc/kubectl-cron/internal/cli"
)

var version string

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := cli.New(os.Stdin, os.Stdout, os.Stderr, time.Now, buildVersion()).ExecuteContext(ctx)
	if errors.Is(err, context.Canceled) {
		return
	}
	if err != nil && !errors.Is(err, cli.ErrFindings) {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
	}
	if err != nil {
		os.Exit(cli.ExitCode(err))
	}
}

func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "(devel)" {
		return strings.TrimPrefix(info.Main.Version, "v")
	}
	return ""
}
