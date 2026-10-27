// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Codex review round 1, class "credentials sent over plaintext HTTP": the raw
// generated endpoint commands carry the Cove session visa in the JSON-RPC body
// (--visa), so the plaintext guard must treat a body visa as a credential.
func TestPlaintextGuardTreatsACoveBodyVisaAsACredential(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","id":"jsonrpc","visa":"visa-0123456789abcdef","method":"GetServerInfo"}`)
	if !bodyHasPassword(body, "application/json") {
		t.Fatal("a JSON-RPC body carrying a visa is not recognised as carrying a credential")
	}
	u, _ := url.Parse("http://api.backup.management/jsonapi")
	req := &http.Request{Method: http.MethodPost, URL: u, Header: http.Header{"Content-Type": {"application/json"}}}
	if err := plaintextCredentialError(req.URL, requestCarriesCredential(req, nil, "", body)); !errors.Is(err, ErrPlaintextCredential) {
		t.Fatalf("visa body over plaintext http was not refused: %v", err)
	}
	if bodyHasPassword([]byte(`{"visa":""}`), "application/json") {
		t.Fatal("an empty visa is not a credential")
	}
	if !strings.EqualFold("visa", "VISA") || !isPasswordField("Visa") {
		t.Fatal("the visa field name must match case-insensitively")
	}
}
