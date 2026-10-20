// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/backup-radar/handfixes.json
// under the "radar-human-output-scrub-terminal" entry.

package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintRadarTableScrubsTerminalControls(t *testing.T) {
	var out bytes.Buffer
	evil := "DC01\x1b]52;c;VEVTVA==\x07\tX\u009b2J"
	if err := printRadarTable(&out, []string{"client", "device\x07"}, [][]string{{"Acme\x1b[31m", evil}}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, bad := range []string{"\x1b", "\x07", "\u009b"} {
		if strings.Contains(got, bad) {
			t.Fatalf("control character %q survived in %q", bad, got)
		}
	}
	if !strings.Contains(got, "DC01") || !strings.Contains(got, "Acme") || !strings.Contains(got, "X") {
		t.Fatalf("visible text lost: %q", got)
	}
	// The embedded tab must not open a new column.
	if strings.Count(strings.Split(strings.TrimSpace(got), "\n")[1], "DC01") != 1 || strings.Contains(got, "DC01\t") {
		t.Fatalf("tab inside a cell changed the column layout: %q", got)
	}
	if scrubRadarText("a\r\nb") != "a  b" {
		t.Fatalf("newlines must become spaces: %q", scrubRadarText("a\r\nb"))
	}
}
