// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package logs

import (
	"io"
	"strings"
	"testing"
)

func TestCopyLinesPreservesFragmentsAndRejectsOversizedLines(t *testing.T) {
	input := strings.Repeat("x", 65536) + "\nlast fragment"
	var out strings.Builder
	if err := copyLines(strings.NewReader(input), func(v string) error { out.WriteString(v); return nil }); err != nil || out.String() != input {
		t.Fatalf("%v len=%d", err, out.Len())
	}
	if err := copyLines(strings.NewReader(strings.Repeat("x", 1024*1024+1)), func(string) error { return nil }); err == nil {
		t.Fatal("unbounded line accepted")
	}
	if err := copyLines(strings.NewReader("line\n"), func(string) error { return io.ErrClosedPipe }); err != io.ErrClosedPipe {
		t.Fatal(err)
	}
}
