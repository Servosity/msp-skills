// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"domain-cert-tracker-pp-cli/internal/client"
	"domain-cert-tracker-pp-cli/internal/config"
)

// doctor is healthy only when base_url answers RDAP /help with a JSON object;
// a web page, a 404 or a 401 is a negative verdict on both rows.
func TestRDAPDoctorVerdict(t *testing.T) {
	for _, c := range []struct {
		status        int
		body, ctype   string
		apiOK, credOK bool
	}{
		{200, `{"rdapConformance":["rdap_level_0"]}`, "application/rdap+json", true, true},
		{200, `<html>app</html>`, "text/html", false, false},
		{200, `null`, "application/json", false, false},
		{200, `{"error":"backend unavailable"}`, "application/json", false, false},
		{404, `{"error":"not found"}`, "application/json", false, false},
		{401, `{"error":"unauthorized"}`, "application/json", false, false},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", c.ctype)
			w.WriteHeader(c.status)
			_, _ = w.Write([]byte(c.body))
		}))
		t.Setenv("DOMAIN_CERT_TRACKER_BASE_URL", srv.URL) // the operator's stand-in on loopback
		cl := client.New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
		if err := ApplyClientHooks(cl); err != nil {
			t.Fatal(err)
		}
		cl.NoCache = true
		api, cred := rdapDoctorVerdict(context.Background(), cl)
		srv.Close()
		positive := func(s string) bool {
			return !strings.HasPrefix(s, "invalid") && !strings.HasPrefix(s, "refused") && !strings.HasPrefix(s, "rejected") && !strings.HasPrefix(s, "skipped") && !strings.HasPrefix(s, "unreachable")
		}
		if positive(api) != c.apiOK || positive(cred) != c.credOK {
			t.Errorf("HTTP %d %s: api=%q credentials=%q", c.status, c.ctype, api, cred)
		}
	}
}
