// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import "testing"

func TestRedactSecretFields(t *testing.T) {
	in := []byte(`{"data":[{"id":"p1","as_uninstall_password":"x","n":1e400}],"secret_token":"y"}`)
	out, removed := RedactSecretFields(in)
	if !removed || string(out) != `{"data":[{"id":"p1","n":1e400}]}` {
		t.Fatalf("got %s removed=%v", out, removed)
	}
	clean := []byte(`{"id":"p1","policy_secret_set":true}`)
	if out, removed := RedactSecretFields(clean); removed || string(out) != string(clean) {
		t.Fatalf("clean input changed: %s", out)
	}
	if ContainsSecretField([]byte(`not json`)) {
		t.Fatal("invalid JSON reported as secret")
	}
}
