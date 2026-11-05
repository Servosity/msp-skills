// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"encoding/json"
	"fmt"
	"testing"
)

// store-never-keeps-secrets: a record is stored with every value under a
// credential name redacted, at any depth, and its data and measures kept.
func TestStoredRecordKeepsNoCredentialField(t *testing.T) {
	in := `{"id":"u-1","password":"Pw-a7Rq2","client_secret":"Cs-x3Lm9","otp":123456,` +
		`"nested":{"accessToken":"At-k8Vn4","items":[{"refresh_token":"Rt-q5Zp1"}]},` +
		`"total_tokens":42,"token_type":"Bearer","remote_session":"https://assist.example.net/s/1","name":"HQ"}`
	var got map[string]any
	if err := json.Unmarshal(withoutStoredSecrets(json.RawMessage(in)), &got); err != nil {
		t.Fatal(err)
	}
	nested := got["nested"].(map[string]any)
	for name, v := range map[string]any{"password": got["password"], "client_secret": got["client_secret"], "otp": got["otp"],
		"accessToken": nested["accessToken"], "refresh_token": nested["items"].([]any)[0].(map[string]any)["refresh_token"]} {
		if v != redactedValue {
			t.Errorf("%s stored as %v, want %q", name, v, redactedValue)
		}
	}
	for name, want := range map[string]string{"id": "u-1", "total_tokens": "42", "token_type": "Bearer",
		"remote_session": "https://assist.example.net/s/1", "name": "HQ"} {
		if v := fmt.Sprint(got[name]); v != want {
			t.Errorf("%s stored as %v, want %v", name, v, want)
		}
	}
	if string(withoutStoredSecrets(json.RawMessage(`not json`))) != "{}" {
		t.Error("an undecodable record must be stored as {}")
	}
}
