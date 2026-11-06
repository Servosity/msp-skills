// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"encoding/json"
	"strings"
	"testing"
)

// A documented shape: EngineProps SMBAUTH-* values hold WinSMBAuth as a JSON string
// (comet-go-sdk cometsdk.go). Its password is redacted in place, so the record still
// prints instead of the whole answer being withheld.
func TestRedactReachesJSONInsideAString(t *testing.T) {
	const pw = "smbSharePass-5d1e7b"
	in := map[string]any{"EngineProps": map[string]any{
		"SMBAUTH-1": `{"SharePath":"\\\\nas\\share","Username":"svc","Password":"` + pw + `","PasswordFormat":0}`,
		"NOTE":      `{"Plain":"kept as written"  }`,
		"TEXT":      "not json",
	}}
	out := RedactSecretFields(in)
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), pw) {
		t.Fatalf("password still in the record: %s", b)
	}
	props := out.(map[string]any)["EngineProps"].(map[string]any)
	var smb map[string]any
	if err := json.Unmarshal([]byte(props["SMBAUTH-1"].(string)), &smb); err != nil {
		t.Fatalf("the value is no longer JSON: %v", err)
	}
	if smb["Password"] != RedactedValue || smb["Username"] != "svc" {
		t.Fatalf("wrong redaction: %v", smb)
	}
	if props["NOTE"] != `{"Plain":"kept as written"  }` || props["TEXT"] != "not json" {
		t.Fatalf("a value with no credential was rewritten: %v", props)
	}
}
