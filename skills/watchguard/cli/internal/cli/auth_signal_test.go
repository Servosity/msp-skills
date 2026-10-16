// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"errors"
	"io"
	"testing"

	"watchguard-pp-cli/internal/client"
)

func exitCodeOf(t *testing.T, err error) int {
	t.Helper()
	var ce *cliError
	if !errors.As(classifyAPIError(io.Discard, err, &rootFlags{}), &ce) {
		t.Fatalf("classifyAPIError did not return a typed error for %v", err)
	}
	return ce.code
}

// hand-fix (handfixes.json auth-hint-reads-body-not-path, ported from
// connectsecure): every Firebox user/group path and every AuthPoint path
// contains /authentication/, so the 400 auth heuristic read the path and
// classified a plain validation error as "auth is missing or invalid"
// (exit 4) on those commands only.
func TestAuthenticationPathIsNotAnAuthSignal(t *testing.T) {
	for _, path := range []string{
		"/rest/firebox/management/v1/configuration/{accountId}/authentication/users",
		"/rest/authpoint/authentication/v1/accounts/{accountId}/resources/{resourceId}/password",
	} {
		err := &client.APIError{Method: "GET", Path: path, StatusCode: 400, Body: `{"message":"Invalid query parameter 'limit'."}`}
		if code := exitCodeOf(t, err); code == 4 {
			t.Errorf("400 on %s classified as auth (exit 4)", path)
		}
	}
}

func TestValidationMessageIsNotAnAuthSignal(t *testing.T) {
	err := &client.APIError{Method: "POST", Path: "/rest/firebox/management/v1/configuration/{accountId}/authentication/users", StatusCode: 400, Body: `{"message":"Credential name already exists"}`}
	if code := exitCodeOf(t, err); code == 4 {
		t.Fatalf("validation error classified as auth (exit 4)")
	}
}

func TestAuthMessageKeepsTheAuthHint(t *testing.T) {
	for _, body := range []string{`{"message":"Authorization token is required"}`, `{"message":"Missing WatchGuard-API-Key header"}`} {
		err := &client.APIError{Method: "GET", Path: "/rest/firebox/management/v1/configuration/{accountId}/aliases", StatusCode: 400, Body: body}
		if code := exitCodeOf(t, err); code != 4 {
			t.Errorf("auth message %s lost its auth classification: exit %d", body, code)
		}
	}
}
