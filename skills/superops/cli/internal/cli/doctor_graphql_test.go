// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"superops-pp-cli/internal/cliutil/testenv"
)

// api.superops.ai is the vendor's real API host. Matching it as the "shipped
// placeholder" told every healthy default install to change its base URL.
func TestDoctorTreatsTheShippedHostAsReal(t *testing.T) {
	if doctorBaseURLIsShippedPlaceholder("https://api.superops.ai") {
		t.Fatal("api.superops.ai read as a placeholder")
	}
	if doctorBaseURLIsShippedPlaceholder("https://euapi.superops.ai") {
		t.Fatal("euapi.superops.ai read as a placeholder")
	}
	for _, p := range []string{"https://{tenant}.example/api", "https://acme.example/msp", "https://x.example.com"} {
		if !doctorBaseURLIsShippedPlaceholder(p) {
			t.Fatalf("%s not read as a placeholder", p)
		}
	}
}

// SuperOps serves only POST /msp. The probe must be a GraphQL query there, and
// a GraphQL error's message (which can echo what was sent) is never printed.
func TestDoctorProbesTheGraphQLEndpoint(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"healthy", 200, `{"data":{"getTicketList":{"listInfo":{"totalCount":3}}}}`, "valid"},
		{"denied", 200, `{"errors":[{"message":"token sop-test-token-4471b9 is not valid","extensions":{"code":"UNAUTHENTICATED"}}]}`, "ERROR rejected (GraphQL UNAUTHENTICATED"},
		{"other-error", 200, `{"errors":[{"message":"echo sop-test-token-4471b9","extensions":{"code":"<b>x</b>"}}]}`, "WARN not verified"},
		{"no-tenant", 400, ``, "SUPEROPS_SUBDOMAIN"},
		{"expired", 401, `{"error":"unauthorized"}`, "ERROR rejected (HTTP 401"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sawQuery bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/msp" {
					http.NotFound(w, r)
					return
				}
				b, _ := io.ReadAll(r.Body)
				var req struct {
					Query string `json:"query"`
				}
				if json.Unmarshal(b, &req) == nil && strings.Contains(req.Query, "getTicketList") {
					sawQuery = true
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			testenv.Isolate(t)
			t.Setenv("SUPEROPS_BASE_URL", srv.URL)
			t.Setenv("SUPEROPS_API_TOKEN", "sop-test-token-4471b9")
			out, _ := runSuperOpsCLI(t, "doctor", "--json")
			start := strings.Index(out, "{")
			if start < 0 {
				t.Fatalf("doctor printed no JSON:\n%s", out)
			}
			var report map[string]any
			if err := json.NewDecoder(strings.NewReader(out[start:])).Decode(&report); err != nil {
				t.Fatalf("doctor JSON: %v\n%s", err, out)
			}
			creds, _ := report["credentials"].(string)
			if !strings.HasPrefix(creds, tc.want) && !strings.Contains(creds, tc.want) {
				t.Fatalf("credentials = %q, want %q", creds, tc.want)
			}
			if tc.status == 200 && !sawQuery {
				t.Fatal("doctor never sent a GraphQL query to POST /msp")
			}
			if strings.Contains(out, "sop-test-token-4471b9") {
				t.Fatalf("doctor printed the credential:\n%s", out)
			}
		})
	}
}
