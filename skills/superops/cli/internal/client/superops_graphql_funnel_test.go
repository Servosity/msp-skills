// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"superops-pp-cli/internal/config"
)

// A GraphQL error arrives with HTTP 200, so it never took the HTTP error
// path's mask. Every error Query and Mutate return is shown somewhere (sync
// events, doctor, MCP results), so none may carry the credential in any form,
// and an access denial must still classify as one.
func TestGraphQLErrorsNeverCarryTheCredential(t *testing.T) {
	const token = "sop-test-token-4471b9"
	forms := []string{token, base64.StdEncoding.EncodeToString([]byte(token)), hex.EncodeToString([]byte(token))}
	for _, form := range forms {
		for _, code := range []string{"FORBIDDEN", "", "INTERNAL_SERVER_ERROR"} {
			e := map[string]any{"message": "token " + form + " rejected"}
			if code != "" {
				e["extensions"] = map[string]any{"code": code}
			}
			body, _ := json.Marshal(map[string]any{"errors": []any{e, map[string]any{"message": "Bearer " + form}}})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, string(body))
			}))
			c := New(&config.Config{BaseURL: srv.URL, SuperopsApiToken: token}, time.Second, 0)
			c.NoCache = true
			_, qerr := c.Query(context.Background(), `{ getStatusList { statusId } }`, nil)
			_, merr := c.Mutate(context.Background(), `mutation { a }`, nil)
			srv.Close()
			for op, err := range map[string]error{"Query": qerr, "Mutate": merr} {
				if err == nil {
					t.Fatalf("%s returned no error for a GraphQL error response", op)
				}
				text := err.Error()
				var denied *GraphQLAccessDeniedError
				if errors.As(err, &denied) {
					text += strings.Join(denied.Messages, " ") + strings.Join(denied.Codes, " ")
				}
				for _, f := range forms {
					if strings.Contains(text, f) {
						t.Fatalf("%s error carries the credential (%s, code %q): %s", op, f, code, text)
					}
				}
			}
		}
	}
	// Only a response whose every error is a denial is a denial; it still is.
	body := `{"errors":[{"message":"no","extensions":{"code":"FORBIDDEN"}}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, SuperopsApiToken: token}, time.Second, 0)
	c.NoCache = true
	_, err := c.Query(context.Background(), `{ a }`, nil)
	var denied *GraphQLAccessDeniedError
	if !errors.As(err, &denied) || len(denied.Codes) != 1 || denied.Codes[0] != "FORBIDDEN" {
		t.Fatalf("a FORBIDDEN response no longer classifies as an access denial: %v", err)
	}
}
