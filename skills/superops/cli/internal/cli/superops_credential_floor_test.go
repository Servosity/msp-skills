// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): a credential too short to catch in every
// encoding is never sent (superops-credential-floor).

package cli

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The round-1 input: a short base URL password (or API token) and a GraphQL
// error, answered with HTTP 200, that echoes it base64- and hex-encoded. The
// configuration is refused, nothing is sent, and no form of it is shown.
func TestShortCredentialIsNeverSentOrShown(t *testing.T) {
	const short = "Zq7x"
	forms := []string{
		base64.StdEncoding.EncodeToString([]byte(short)),
		base64.RawStdEncoding.EncodeToString([]byte(short)),
		hex.EncodeToString([]byte(short)),
	}
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"errors": []any{map[string]any{
			"message": "credential check failed for " + strings.Join(forms, " / "),
		}}})
	}))
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")
	for _, tc := range []struct{ name, base, token string }{
		{"base-url-password", "http://svc:" + short + "@" + host, "sop-floor-token-91ab"},
		{"base-url-username-only", "http://" + short + "@" + host, "sop-floor-token-91ab"},
		{"api-token", "http://" + host, short},
	} {
		t.Run(tc.name, func(t *testing.T) {
			urlSecretEnv(t, tc.base, tc.token, true)
			before := requests
			stdout, stderr, err := runCLIProcess(t, "raw", "query", urlSecretRawQuery, "--json")
			all := stdout + stderr + fmt.Sprint(err)
			for _, form := range forms {
				if strings.Contains(strings.ToLower(all), strings.ToLower(form)) {
					t.Errorf("raw query shows %q:\n%s", form, all)
				}
			}
			if !strings.Contains(all, "credential refused") || requests != before {
				t.Errorf("want the credential refused before any request (%d sent):\n%s", requests-before, all)
			}
		})
	}
}
