// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactDryRunBody(t *testing.T) {
	v, ok := redactDryRunBody([]byte(`{"primaryEmail":"jane@example.com","password":"Hunter2!","name":{"givenName":"Jane"},"keys":[{"refreshToken":"r1","id":7}],"suspended":false}`))
	if !ok {
		t.Fatal("JSON body must parse")
	}
	out, _ := json.Marshal(v)
	s := string(out)
	for _, leaked := range []string{"Hunter2!", `"r1"`} {
		if strings.Contains(s, leaked) {
			t.Fatalf("secret leaked into the dry-run preview: %s", s)
		}
	}
	for _, kept := range []string{"jane@example.com", "Jane", `"id":7`, `"suspended":false`} {
		if !strings.Contains(s, kept) {
			t.Fatalf("non-secret field lost: %s", s)
		}
	}
	v, _ = redactDryRunBody([]byte(`{"changePasswordAtNextLogin":true,"pageToken":"p2","customerAuthToken":"xfer-secret"}`))
	out, _ = json.Marshal(v)
	s = string(out)
	if !strings.Contains(s, `"changePasswordAtNextLogin":true`) || !strings.Contains(s, `"pageToken":"p2"`) || strings.Contains(s, "xfer-secret") {
		t.Fatalf("booleans and paging cursors stay visible, transfer tokens do not: %s", s)
	}
	if _, ok := redactDryRunBody([]byte("not json")); ok {
		t.Fatal("a non-JSON body is not printed")
	}
}
