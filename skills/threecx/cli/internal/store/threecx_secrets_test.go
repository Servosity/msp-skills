// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"encoding/json"
	"strings"
	"testing"
)

// store-never-keeps-secrets: every merge redacts credential members by name, a
// PIN among them (never held, so nothing else would stop it), at any depth. A
// record that cannot be decoded is not kept.
func TestStoreRedactsCredentialMembersByName(t *testing.T) {
	got := string(withoutStoredSecrets(json.RawMessage(`{"Name":"rec","Count":12345678901234567890,"PinNumber":"86420975","Nested":{"AuthPassword":"storeUnitPw-1"}}`)))
	for _, v := range []string{"86420975", "storeUnitPw-1"} {
		if strings.Contains(got, v) {
			t.Errorf("kept %s: %s", v, got)
		}
	}
	if !strings.Contains(got, `"rec"`) || !strings.Contains(got, "12345678901234567890") {
		t.Errorf("lost the record or a number's spelling: %s", got)
	}
	if got := string(withoutStoredSecrets(json.RawMessage(`{not json`))); got != "{}" {
		t.Errorf("undecodable record stored as %s", got)
	}
}
