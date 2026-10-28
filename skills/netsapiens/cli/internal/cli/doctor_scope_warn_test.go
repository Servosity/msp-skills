// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"netsapiens-pp-cli/internal/cliutil/testenv"
)

// Codex publish round 2 input: otherwise healthy config, /apikeys/~ answers
// 403 Permission Denied. Human output said WARN; --fail-on warn exited 0, and
// the verdict claimed the credential was accepted, which a 403 does not prove.
func TestDoctorScopeLimitedIsAWarning(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":403,"message":"Permission Denied"}`))
	}))
	defer srv.Close()
	t.Setenv("NETSAPIENS_BASE_URL", srv.URL)
	t.Setenv("NETSAPIENS_API_KEY", "ExampleKey-1234567890")

	// doctor may exit non-zero for other sections; only the credentials row is judged.
	out, _, _ := runFleet(t, "doctor", "--json")
	verdict, _ := decodeObject(t, out)["credentials"].(string)
	if !strings.Contains(verdict, "403") {
		t.Fatalf("the probe did not report the 403: %q\n%s", verdict, out)
	}
	if strings.Contains(verdict, "accepted") {
		t.Fatalf("a 403 does not prove the credential authenticated: %q", verdict)
	}
	if err := doctorExitForFailOn("warn", map[string]any{"credentials": verdict}); err == nil {
		t.Fatalf("doctor --fail-on warn must trip on %q", verdict)
	}
}
