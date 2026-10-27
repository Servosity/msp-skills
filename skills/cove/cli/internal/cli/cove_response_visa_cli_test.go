// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated), cove only: a raw endpoint command (devices
// get --visa) never shows a visa Cove's answer returned, in any encoding, on
// stdout, on stderr or in its error, whatever the answer's status or content
// type (Codex round 1, press 4.34.0 reprint). Hand-fix code:
// cove-short-credential-funnel.

package cli

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"cove-pp-cli/internal/cliutil"
)

var coveCLIB64Run = regexp.MustCompile(`[A-Za-z0-9+/_-]{8,}={0,2}`)

// coveCLILeaks reports whether text shows secret as written or inside any
// base64 or base64url run at any of its four alignments: an oracle written
// apart from the funnel it tests.
func coveCLILeaks(text, secret string) bool {
	if strings.Contains(text, secret) {
		return true
	}
	for _, run := range coveCLIB64Run.FindAllString(text, -1) {
		run = strings.TrimRight(run, "=")
		for off := 0; off < 4 && off < len(run); off++ {
			s := run[off:]
			for _, enc := range []*base64.Encoding{base64.RawStdEncoding, base64.RawURLEncoding} {
				for cut := len(s); cut > len(s)-4 && cut > 0; cut-- {
					if b, err := enc.DecodeString(s[:cut]); err == nil && strings.Contains(string(b), secret) {
						return true
					}
				}
			}
		}
	}
	return false
}

// coveRunCaptured runs the CLI with the backstops Execute wires, capturing
// Cobra's writers and the process's stdout and stderr.
func coveRunCaptured(t *testing.T, args ...string) (string, error) {
	t.Helper()
	outFile, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	errFile, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outFile, errFile
	root := newRootCmd(&rootFlags{})
	funnelCommandErrors(root)
	coveShortFunnelCommandErrors(root)
	var printed bytes.Buffer
	root.SetOut(&printed)
	root.SetErr(&printed)
	root.SetArgs(args)
	runErr := root.Execute()
	os.Stdout, os.Stderr = oldOut, oldErr
	_ = outFile.Close()
	_ = errFile.Close()
	stdout, _ := os.ReadFile(outFile.Name())
	stderr, _ := os.ReadFile(errFile.Name())
	return printed.String() + "\n" + string(stdout) + "\n" + string(stderr), runErr
}

// --dry-run prints the request it would send; the visa that request carries
// (--visa) is masked in that preview, never printed.
func TestCoveRawEndpointDryRunNeverShowsTheSentVisa(t *testing.T) {
	const visa = "sent-dry-visa-0123456789"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("a dry run reached the server: %s %s", r.Method, r.URL)
	}))
	t.Cleanup(srv.Close)
	fleetTestEnv(t, srv)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	shown, err := coveRunCaptured(t, "devices", "get", "--params-account-id", "7", "--visa", visa, "--dry-run")
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if coveCLILeaks(shown, visa) {
		t.Fatalf("the dry run shows the sent visa: %q", shown)
	}
	if !strings.Contains(shown, "GetAccountInfoById") {
		t.Fatalf("the dry run no longer previews the request body: %q", shown)
	}
}

func TestCoveRawEndpointCommandNeverShowsAReturnedVisa(t *testing.T) {
	esc := func(s string) string { return strings.ReplaceAll(s, "~", string(rune(92))) }
	for name, tc := range map[string]struct {
		status       int
		ct, answer   string
		visa         string
		wantErr      bool
		wantInOutput string
	}{
		// Codex round 1, finding 1 (exact input).
		"octet-stream 200": {200, "application/octet-stream", `{"visa":"new-visa-1234567890","result":{"result":{}}}`, "new-visa-1234567890", false, `"_pp_binary"`},
		// Codex round 1, finding 2 (exact input).
		"http 400":                {400, "application/json", `{"visa":"fresh-visa-1234567890","error":{"code":-32603,"data":13501,"message":"refused"}}`, "fresh-visa-1234567890", true, ""},
		"http 500 text/plain":     {500, "text/plain", `upstream failure: {"visa":"plain-cli-visa-0123456789","error":{"message":"boom"}}`, "plain-cli-visa-0123456789", true, ""},
		"escaped name 200 binary": {200, "application/octet-stream", esc(`{"~u0076isa":"esc-cli-visa-0123456789","result":{"result":{}}}`), "esc-cli-visa-0123456789", false, `"_pp_binary"`},
		"escaped name 400":        {400, "application/json", esc(`{"~u0076isa":"esc400-cli-visa-0123456789","error":{"code":-32603,"data":13501,"message":"refused"}}`), "esc400-cli-visa-0123456789", true, ""},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.ct)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.answer))
			}))
			t.Cleanup(srv.Close)
			fleetTestEnv(t, srv)
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			// No retry: a 5xx answer is reported at once.
			t.Setenv(cliutil.DogfoodEnvVar, "1")
			shown, err := coveRunCaptured(t, "devices", "get", "--params-account-id", "7", "--visa", "sent-cli-visa-0123456789", "--json")
			if tc.wantErr != (err != nil) {
				t.Fatalf("err=%v, want error %v; output=%q", err, tc.wantErr, shown)
			}
			if err != nil && coveCLILeaks(err.Error(), tc.visa) {
				t.Fatalf("the error shows the returned visa: %v", err)
			}
			if coveCLILeaks(shown, tc.visa) {
				t.Fatalf("the output shows the returned visa: %q", shown)
			}
			if tc.wantInOutput != "" && !strings.Contains(shown, tc.wantInOutput) {
				t.Fatalf("the answer was not printed: %q", shown)
			}
		})
	}
}
