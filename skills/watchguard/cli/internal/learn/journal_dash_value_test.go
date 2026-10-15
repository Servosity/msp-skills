// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package learn_test

import (
	"strings"
	"testing"

	"watchguard-pp-cli/internal/learn"
)

// hand-fix (handfixes.json journal-dash-value-not-a-flag, ported from
// connectsecure): pflag hands a value-taking flag the next token even when it
// starts with "-", so a pre-shared key like -SENTINELpsk12 is a value and must
// never become an argv_shape key in the on-disk learn journal.
func TestJournalDashValueIsNotAFlagName(t *testing.T) {
	isBool := func(name string) bool { return name == "json" }
	got := learn.JournalArgvShape([]string{"firebox-bovpn-tunnels", "create", "--psk", "-SENTINELpsk12", "--name", "t1", "--json"}, isBool)
	for k := range got {
		if strings.Contains(k, "SENTINEL") {
			t.Fatalf("PSK value journaled as a flag name: %v", got)
		}
	}
	if got["psk"] != "str" || got["name"] != "str" || got["json"] != "bool" {
		t.Fatalf("shape = %v", got)
	}
}
