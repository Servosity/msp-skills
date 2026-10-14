// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json journal-dash-value-not-a-flag.

package learn_test

import (
	"strings"
	"testing"

	"connectsecure-pp-cli/internal/learn"
)

// TestJournalDashValueIsNotAFlagName: pflag hands a value-taking flag the
// next token even when it starts with "-", so a password like -SENTINELpw12
// is a value and must never become an argv_shape key in the journal.
func TestJournalDashValueIsNotAFlagName(t *testing.T) {
	isBool := func(name string) bool { return name == "json" }
	got := learn.JournalArgvShape([]string{"scheduler", "create", "--data-password", "-SENTINELpw12", "--json"}, isBool)
	for k := range got {
		if strings.Contains(k, "SENTINEL") {
			t.Fatalf("password value journaled as a flag name: %v", got)
		}
	}
	if got["data-password"] != "redacted" || got["json"] != "bool" {
		t.Fatalf("shape = %v", got)
	}
}
