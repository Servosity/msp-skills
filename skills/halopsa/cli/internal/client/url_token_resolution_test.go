// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"net/http"
	"strings"
	"testing"
)

// TestResolveTemplateURL_TokenEndpoint is the regression guard for issue #147:
// the OAuth token-mint/refresh paths used to POST a literal
// "https://{tenant}.{domain}/auth/token" because they never substituted the
// {placeholder} markers, so http.NewRequest rejected it with
// `invalid character "{" in host name`. ResolveTemplateURL now routes that URL
// through the same substitution buildURL applies to request URLs.
func TestResolveTemplateURL_TokenEndpoint(t *testing.T) {
	const raw = "https://{tenant}.{domain}/auth/token"

	// 1. With both vars set, placeholders resolve and the URL is requestable.
	vars := map[string]string{"tenant": "acme", "domain": "halopsa.com"}
	got, err := ResolveTemplateURL(raw, vars)
	if err != nil {
		t.Fatalf("ResolveTemplateURL returned error for resolvable URL: %v", err)
	}
	if want := "https://acme.halopsa.com/auth/token"; got != want {
		t.Fatalf("substitution mismatch: got %q want %q", got, want)
	}
	if strings.ContainsAny(got, "{}") {
		t.Fatalf("resolved URL still carries placeholder braces: %q", got)
	}
	if _, err := http.NewRequest(http.MethodPost, got, nil); err != nil {
		t.Fatalf("resolved URL is not requestable (the #147 symptom): %v", err)
	}

	// 2. With an unresolved var, the error names the env var to export rather
	//    than silently shipping a literal placeholder into the request.
	if _, err := ResolveTemplateURL(raw, map[string]string{"domain": "halopsa.com"}); err == nil {
		t.Fatal("expected a TemplateVarError when {tenant} is unresolved, got nil")
	} else if !strings.Contains(err.Error(), "HALOPSA_TENANT") {
		t.Fatalf("error should name the env var to export, got: %v", err)
	}
}
