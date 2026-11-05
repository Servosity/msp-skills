// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "secret-fields-never-persisted" entry.

package cliutil

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactSecretFields(t *testing.T) {
	in := []byte(`{"users":[{"name":"alice","passwd":"S1","password_last_change":17000,"passwd_never_expire":true,"empty_password":""}],` +
		`"settings":{"smtp-password":"S2","SMTPPassword":"S3","credentials":{"user":"x","value":"S4"},"api_key":"S5","sid":"S6","did":"S7","private_key":"S8"},` +
		`"task":{"name":"File Server","token_count":3,"secret":null}}`)
	out := RedactSecretFields(in)
	for _, s := range []string{"S1", "S2", "S3", "S4", "S5", "S6", "S7", "S8"} {
		if bytes.Contains(out, []byte(`"`+s+`"`)) {
			t.Fatalf("secret %s survived: %s", s, out)
		}
	}
	var back map[string]any
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("redacted output is not JSON: %v", err)
	}
	for _, keep := range []string{`"name":"alice"`, `"password_last_change":17000`, `"passwd_never_expire":true`, `"token_count":3`, `"name":"File Server"`} {
		if !strings.Contains(string(out), keep) {
			t.Fatalf("redaction dropped non-secret data %s: %s", keep, out)
		}
	}
	clean := []byte(`{"name":"alice",  "uid":1026}`)
	if got := RedactSecretFields(clean); !bytes.Equal(got, clean) {
		t.Fatalf("input without secrets changed: %q", got)
	}
	if got := RedactSecretFields([]byte("not json passwd=x")); string(got) != "not json passwd=x" {
		t.Fatalf("non-JSON changed: %q", got)
	}
}
