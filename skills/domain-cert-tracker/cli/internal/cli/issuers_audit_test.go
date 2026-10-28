// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"strings"
	"testing"

	"domain-cert-tracker-pp-cli/internal/cliutil/testenv"
	"domain-cert-tracker-pp-cli/internal/store"
	"domain-cert-tracker-pp-cli/internal/tracker"
)

// TestNovelIssuersAuditHelpWires smoke-tests that the issuers audit command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelIssuersAuditHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"issuers", "audit", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("issuers audit --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "audit"} {
		if !strings.Contains(help, want) {
			t.Fatalf("issuers audit --help missing %q in output:\n%s", want, help)
		}
	}
}

// Every status the audit can return, and the names_source values, are
// documented in its help (the MCP tool and agents read the same text).
func TestIssuersAuditHelpNamesEveryStatus(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"issuers", "audit", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	for _, want := range []string{tracker.IssuerNotInCAA, tracker.IssuerNamesPartial, tracker.IssuerCAAUnverified, tracker.IssuerUnmapped, tracker.IssuerNew,
		tracker.IssuerRoutine, store.NamesFromCertificate, store.NamesFromCrtSh, "totals"} {
		if !strings.Contains(help, want) {
			t.Errorf("issuers audit --help does not mention %q", want)
		}
	}
}
