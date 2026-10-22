// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/mertdotcc/kubectl-cron/internal/cli"
)

var version string

func main() {
	err := cli.New(os.Stdin, os.Stdout, os.Stderr, time.Now, buildVersion()).Execute()
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
