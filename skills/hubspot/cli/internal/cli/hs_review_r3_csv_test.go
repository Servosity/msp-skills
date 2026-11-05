// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Codex round 1 P2 (publish lane, 2026-10-24): a novel command's --csv output
// neutralizes spreadsheet formulas the same way the generated writer does.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestR3NovelCSVNeutralizesFormulas(t *testing.T) {
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	f := &rootFlags{csv: true}
	if err := f.printTabular(cmd, []string{"title", "amount"}, [][]string{{"=1+1", "-5"}, {"@SUM(A1)", "10"}}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, bad := range []string{"\n=1+1,", "\n@SUM(A1),"} {
		if strings.Contains(got, bad) {
			t.Fatalf("formula cell left live in CSV output:\n%s", got)
		}
	}
	if !strings.Contains(got, "'=1+1") || !strings.Contains(got, ",-5") {
		t.Fatalf("want a quoted formula and an untouched number:\n%s", got)
	}
}
