package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"hook-security-pp-cli/internal/client"
	"hook-security-pp-cli/internal/config"
)

// A 403 proves the key was accepted but cannot read the probe endpoint. Doctor
// must say so in a form --fail-on warn can see; an unprefixed verdict passes
// the warn gate and reads as healthy to a script.
func TestDoctorScopeLimitedCredentialIsAWarning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := client.New(&config.Config{BaseURL: srv.URL, HookSecurityApiKey: "fixture-" + "doctor-key"}, 5*time.Second, 0)
	report := map[string]any{}
	doctorProbeCredentials(context.Background(), c, &cobra.Command{Use: "root"}, "hook-security-cli", report)

	got, _ := report["credentials"].(string)
	if !strings.HasPrefix(got, "WARN scope-limited") {
		t.Fatalf("credentials verdict = %q, want a WARN scope-limited prefix", got)
	}
	if err := doctorExitForFailOn("warn", report); err == nil {
		t.Fatal("--fail-on warn must fail on a scope-limited credential")
	}
	if err := doctorExitForFailOn("error", report); err != nil {
		t.Fatalf("--fail-on error must not fail on a warning: %v", err)
	}
}
