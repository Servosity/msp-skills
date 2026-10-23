// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestCoveDoctorLoginVerdict: the doctor login probe reports Cove's verdict,
// by error number only, and never the server's text (a server can reflect the
// password back). Hand-owned (handfixes.json: doctor-verifies-cove-login).
func TestCoveDoctorLoginVerdict(t *testing.T) {
	t.Setenv("COVE_PARTNER", "Acme (admin@example.test)")
	t.Setenv("COVE_USERNAME", "api-user")
	t.Setenv("COVE_PASSWORD", "reflected-password-credential")
	for _, tc := range []struct {
		name, status, body, want string
	}{
		{"accepted", "200", `{"jsonrpc":"2.0","id":"jsonrpc","visa":"visa-1","result":{"result":{"PartnerId":7}}}`, "ok (JSON-RPC Login accepted)"},
		{"refused", "200", `{"jsonrpc":"2.0","id":"jsonrpc","error":{"code":-32603,"data":2100,"message":"Unknown partner/username reflected-password-credential"}}`, "ERROR Login refused (Cove error 2100)"},
		{"not-cove", "500", `<html>proxy error for reflected-password-credential</html>`, "ERROR Login failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var sawLogin bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				sawLogin = strings.Contains(string(body), `"method":"Login"`) && strings.Contains(string(body), "reflected-password-credential")
				if tc.status == "500" {
					w.WriteHeader(http.StatusInternalServerError)
				}
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()
			got := coveDoctorLogin(context.Background(), srv.URL, 5*time.Second)
			if !sawLogin {
				t.Fatal("the probe did not send a JSON-RPC Login with the configured credentials")
			}
			if !strings.HasPrefix(got, tc.want) {
				t.Fatalf("verdict = %q, want prefix %q", got, tc.want)
			}
			if strings.Contains(got, "reflected-password-credential") {
				t.Fatalf("verdict carries server text: %q", got)
			}
		})
	}
}
