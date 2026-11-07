// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (handfixes.json credkit-wiring).

package cli

import (
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"sonicwall-pp-cli/internal/cliutil"
	"sonicwall-pp-cli/internal/credkit"
)

type stubRT func(*http.Request) (*http.Response, error)

func (f stubRT) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestTokenExchangeCredentialIsHeld: every token exchange's minted credential
// is held before the client reads the body, which still arrives intact. The
// built binary's proof is internal/credwire TestMintedSessionTokenIsHeld.
func TestTokenExchangeCredentialIsHeld(t *testing.T) {
	minted := "nsmMinted-" + "Hq3Rw8Kz5Lp1"
	code := "cscCode-" + "Mn4Bv6Cx8Za2"
	for _, tc := range []struct {
		name, method, path, body string
		wantHeld                 []string
		wantStatus               int
	}{
		{"refresh", http.MethodPost, "/api/manager/auth/refreshSessionToken", `{"status":{"success":true,"info":[{"message":"` + minted + `"}]}}`, []string{minted}, 200},
		{"sso, keys in another case", http.MethodPost, "/api/manager/auth/sso", `{"Status":{"Success":true,"Info":[{"Message":" ` + minted + ` "}]}}`, []string{minted}, 200},
		{"sso, no success field", http.MethodPost, "/api/manager/auth/sso", `{"status":{"info":[{"message":"` + minted + `"}]}}`, []string{minted}, 200},
		{"access code", http.MethodPost, "/api/generate-cscAccessCode", `{"content":{"accessCode":"` + code + `"}}`, []string{code}, 200},
		{"numeric access code", http.MethodPost, "/api/generate-cscAccessCode", `{"content":{"accessCode":123456789012}}`, []string{"123456789012"}, 200},
		{"error envelope", http.MethodPost, "/api/manager/auth/sso", `{"status":{"success":false,"info":[{"message":"Invalid access code supplied"}]}}`, nil, 401},
		{"not JSON", http.MethodPost, "/api/manager/auth/sso", `token=` + minted, nil, 422},
		{"not an exchange", http.MethodGet, "/api/manager/auth/sso", `{"status":{"success":true,"info":[{"message":"` + minted + `"}]}}`, nil, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var held []string
			prev := cliutil.Hold
			cliutil.Hold = func(_ credkit.Source, values ...string) { held = append(held, values...) }
			defer func() { cliutil.Hold = prev }()
			status := tc.wantStatus
			if status == 422 {
				status = 200
			}
			rt := sonicOSRedactTransport{base: stubRT(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			req, _ := http.NewRequest(tc.method, "https://nsm.example.test"+tc.path, nil)
			resp, err := rt.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != tc.wantStatus {
				t.Errorf("status %d, want %d (%s)", resp.StatusCode, tc.wantStatus, got)
			}
			for _, w := range tc.wantHeld {
				if !slices.Contains(held, w) {
					t.Errorf("%q not held (held %q)", w, held)
				}
				if !strings.Contains(string(got), w) {
					t.Errorf("the caller lost %q: %s", w, got)
				}
			}
			if tc.wantHeld == nil && len(held) > 0 {
				t.Errorf("held %q from a body that mints nothing", held)
			}
			if tc.wantStatus == 422 && strings.Contains(string(got), minted) {
				t.Errorf("an unreadable exchange body reached the caller: %s", got)
			}
		})
	}
}
