// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package learn

import (
	"strings"
	"testing"
)

// journal-credential-hardening: argv_shape records only flag names the command
// declares, a value never lands, and a harness session label is hashed under its own
// prefix.
func TestJournalCredentialHardening(t *testing.T) {
	const pasted = "PastedKeyAsAFlagName9f3e"
	isBool := func(string) bool { return false }
	isDeclared := func(name string) bool { return name == "limit" }
	shape := JournalArgvShape([]string{"mailboxes", "list", "--" + pasted, "plainvalue", "--limit", "5", "--" + pasted + "=two"}, isBool, isDeclared)
	for name, class := range shape {
		if strings.Contains(name, pasted) || strings.Contains(class, pasted) {
			t.Errorf("argv_shape recorded a typed flag name: %q=%q", name, class)
		}
	}
	if _, ok := shape["limit"]; !ok {
		t.Errorf("a declared flag was lost: %v", shape)
	}
	if _, ok := shape[journalUndeclaredFlag]; !ok {
		t.Errorf("an undeclared flag left no placeholder: %v", shape)
	}

	const session = "HarnessSessionLabel7Qk2"
	t.Setenv("EXCHANGE_ONLINE_LEARN_SESSION", session)
	got := JournalSessionKey()
	if strings.Contains(got, session) || !strings.HasPrefix(got, "o:") {
		t.Errorf("session id %q; want the label hashed under o:", got)
	}
}
