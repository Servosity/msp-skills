// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestNovelTicketNoteHelpWires smoke-tests that the ticket note command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelTicketNoteHelpWires(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"ticket", "note", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("ticket note --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "note"} {
		if !strings.Contains(help, want) {
			t.Fatalf("ticket note --help missing %q in output:\n%s", want, help)
		}
	}
}

func TestTicketNoteDryRunValidatesRealPayloads(t *testing.T) {
	for _, tt := range []struct {
		name       string
		args       []string
		wantErr    string
		wantOutput []string
	}{
		{name: "bare verify probe"},
		{name: "zero id", args: []string{"0", "--body", "hello"}, wantErr: "positive integer"},
		{name: "non integer id", args: []string{"abc", "--body", "hello"}, wantErr: "must be an integer"},
		{name: "missing body", args: []string{"42"}, wantErr: "--body is required"},
		{name: "valid payload", args: []string{"42", "--body", "hello"}, wantOutput: []string{`"ticket_id": 42`, `"body": "hello"`}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			flags := &rootFlags{dryRun: true, asJSON: true}
			cmd := newNovelTicketNoteCmd(flags)
			cmd.SetArgs(tt.args)
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			err := cmd.Execute()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			for _, want := range tt.wantOutput {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("output missing %q:\n%s", want, out.String())
				}
			}
		})
	}
}
