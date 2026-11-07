// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (handfixes.json doctor-verdict-tracks-reality). Every credential
// probe that does not come back 2xx is shown as WARN or FAIL, so
// `doctor --fail-on warn` must exit nonzero for each of them. The HTTP 403
// verdict rendered yellow without a WARN prefix and slipped past the gate.
// (429 and 5xx are retried by the client and end as "ERROR not verified".)

package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"datto-saas-pp-cli/internal/client"
	"datto-saas-pp-cli/internal/cliutil/testenv"
	"datto-saas-pp-cli/internal/config"
)

func TestDoctorNonSuccessCredentialProbeTripsFailOnWarn(t *testing.T) {
	for _, status := range []int{401, 403, 404, 418} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			testenv.Isolate(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"message":"no"}`))
			}))
			defer srv.Close()
			t.Setenv("DATTO_SAAS_BASE_URL", srv.URL)
			t.Setenv("DATTO_SAAS_PUBLIC_KEY", "pub")
			t.Setenv("DATTO_SAAS_SECRET_KEY", "sec")
			cfg, err := config.Load("")
			if err != nil {
				t.Fatal(err)
			}
			c := client.New(cfg, 5*time.Second, 0)
			c.NoCache = true
			report := map[string]any{}
			doctorProbeCredentials(context.Background(), c, RootCmd(), "datto-saas-cli", report)
			v, _ := report["credentials"].(string)
			if !strings.Contains(v, fmt.Sprintf("HTTP %d", status)) {
				t.Fatalf("probe did not record HTTP %d: %q", status, v)
			}
			if err := doctorExitForFailOn("warn", report); err == nil {
				t.Fatalf("--fail-on warn must trip on a non-2xx credential probe, got exit 0 for %q", v)
			}
		})
	}
}

func TestDoctorScopeLimitedCredentialIsWarnNotError(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	t.Setenv("DATTO_SAAS_BASE_URL", srv.URL)
	t.Setenv("DATTO_SAAS_PUBLIC_KEY", "pub")
	t.Setenv("DATTO_SAAS_SECRET_KEY", "sec")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	report := map[string]any{}
	doctorProbeCredentials(context.Background(), client.New(cfg, 5*time.Second, 0), RootCmd(), "datto-saas-cli", report)
	v, _ := report["credentials"].(string)
	if !strings.HasPrefix(v, "WARN scope-limited (HTTP 403") {
		t.Fatalf("403 verdict must read WARN scope-limited, got %q", v)
	}
	if err := doctorExitForFailOn("error", report); err != nil {
		t.Fatalf("a 403 is a warning, not an error: %v (%q)", err, v)
	}
}
