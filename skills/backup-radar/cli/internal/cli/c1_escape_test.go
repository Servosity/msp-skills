// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"testing"

	"backup-radar-pp-cli/internal/termio"
)

func TestEscapingWriterIsNotATerminal(t *testing.T) {
	var out bytes.Buffer
	if isTerminal(termio.New(&out)) {
		t.Fatal("a wrapped buffer must not read as a terminal")
	}
}
