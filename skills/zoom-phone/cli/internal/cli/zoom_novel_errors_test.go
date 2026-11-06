// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestZoomNovelErrorEnvelope(t *testing.T) {
	cases := []struct {
		name    string
		json    bool
		run     func(*cobra.Command) error
		wantOut string // "" = stdout must stay empty; "envelope" = {"error","code"}
	}{
		{"not found under json", true, func(*cobra.Command) error { return notFoundErr(errors.New("number not found")) }, "envelope"},
		{"usage error stays on stderr", true, func(*cobra.Command) error { return usageErr(errors.New("missing input")) }, ""},
		{"human mode prints nothing", false, func(*cobra.Command) error { return apiErr(errors.New("boom")) }, ""},
		{"already wrote results", true, func(c *cobra.Command) error {
			c.OutOrStdout().Write([]byte(`[{"ok":false}]`))
			return apiErr(errors.New("one account failed"))
		}, `[{"ok":false}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			flags := &rootFlags{asJSON: tc.json}
			var out bytes.Buffer
			cmd := &cobra.Command{Use: "x", RunE: func(c *cobra.Command, _ []string) error { return tc.run(c) }}
			cmd.SetOut(&out)
			zoomWrapJSONErrorEnvelope(cmd, flags)
			if err := cmd.RunE(cmd, nil); err == nil {
				t.Fatal("wrapper swallowed the error")
			}
			switch tc.wantOut {
			case "envelope":
				var env map[string]any
				if err := json.Unmarshal(out.Bytes(), &env); err != nil || env["code"] != float64(3) || env["error"] != "number not found" {
					t.Fatalf("stdout = %q; want error envelope with code 3", out.String())
				}
			default:
				if out.String() != tc.wantOut {
					t.Fatalf("stdout = %q; want %q", out.String(), tc.wantOut)
				}
			}
		})
	}
}

// Human mode must hand the command the real stdout, so isTerminal can see the
// *os.File and render tables on a TTY; --json mode may wrap it.
func TestZoomNovelErrorEnvelopeKeepsStdoutInHumanMode(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, asJSON := range []bool{false, true} {
		var seen any
		cmd := &cobra.Command{Use: "x", RunE: func(c *cobra.Command, _ []string) error {
			seen = c.OutOrStdout()
			return nil
		}}
		cmd.SetOut(f)
		zoomWrapJSONErrorEnvelope(cmd, &rootFlags{asJSON: asJSON})
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Fatal(err)
		}
		_, isFile := seen.(*os.File)
		if !asJSON && (!isFile || seen != any(f)) {
			t.Fatalf("human mode: command saw %T, want the original *os.File", seen)
		}
		if cmd.OutOrStdout() != any(f) {
			t.Fatalf("asJSON=%v: stdout not restored after the run, got %T", asJSON, cmd.OutOrStdout())
		}
	}
}
