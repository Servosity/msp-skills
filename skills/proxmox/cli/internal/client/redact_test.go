// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"strings"
	"testing"
)

func TestRedactSecretFields(t *testing.T) {
	in := `{"keep-last":3,"tokens":[{"tokenid":"a"}],"fingerprint":"AA","token":"s","nested":[{"freenas_password":"p","client-key":"k"}],"password-set":true}`
	got := string(redactSecretFields([]byte(in)))
	for _, leak := range []string{`"s"`, `"p"`, `"k"`} {
		if strings.Contains(got, leak) {
			t.Errorf("secret %s survived: %s", leak, got)
		}
	}
	for _, keep := range []string{`"tokenid":"a"`, `"fingerprint":"AA"`, `"keep-last":3`, `"password-set":true`} {
		if !strings.Contains(got, keep) {
			t.Errorf("%s lost: %s", keep, got)
		}
	}
	// Each secret shape alone in a body: the prefilter must not miss any.
	for _, body := range []string{`{"storage":"x","access_token":"S1"}`, `{"token":"S1"}`, `{"api_key":"S1"}`, `{"ssh-private-key":"S1"}`, `{"freenas_password":"S1"}`, `[{"client_secret":"S1"}]`} {
		if strings.Contains(string(redactSecretFields([]byte(body))), "S1") {
			t.Errorf("secret survived in %s", body)
		}
	}
	if plain := `{"storage":"local"}`; string(redactSecretFields([]byte(plain))) != plain {
		t.Error("a body with nothing to redact must be returned unchanged")
	}
}
