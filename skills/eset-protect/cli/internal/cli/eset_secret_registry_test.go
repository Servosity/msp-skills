// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/credvariants"
	"eset-protect-pp-cli/internal/cliutil/testenv"
	"eset-protect-pp-cli/internal/secrets"
)

// tokenEcho is a fake ESET token service that answers every request with the
// status and JSON fields the test last set.
type tokenEcho struct {
	mu     sync.Mutex
	status int
	fields map[string]string
}

func (e *tokenEcho) set(status int, fields map[string]string) {
	e.mu.Lock()
	e.status, e.fields = status, fields
	e.mu.Unlock()
}

func (e *tokenEcho) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	e.mu.Lock()
	status, fields := e.status, e.fields
	e.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(fields)
}

func withTokenEcho(t *testing.T) *tokenEcho {
	t.Helper()
	echo := &tokenEcho{status: http.StatusBadRequest}
	srv := httptest.NewServer(echo)
	t.Cleanup(srv.Close)
	esetTokenURLOverride = srv.URL
	t.Cleanup(func() { esetTokenURLOverride = "" })
	return echo
}

// TestTokenServiceErrorsNeverShowASecret (Codex round 6, P1 #1): the token
// service's error and error_description reached every error, doctor row
// and MCP result unmasked, so a service that echoes the submitted password
// ({"error":"invalid_grant","error_description":"Rejected password SECRET"})
// disclosed it. For random passwords and refresh tokens, echoed in every
// spelling, in error_description and as the error code itself, at 400, 401,
// 500 and a 200 without a token: no token error lets the oracle recover the
// secret (handfixes.json: secret-registry-masks-every-boundary). The secret
// is registered only by the request that submits it.
func TestTokenServiceErrorsNeverShowASecret(t *testing.T) {
	echo := withTokenEcho(t)
	r := credvariants.NewRand(31)
	for trial := 0; trial < 24; trial++ {
		secret := credvariants.RandomSecret(r)
		form := url.Values{"grant_type": {"password"}, "username": {"api@msp.example"}, "password": {secret}}
		if trial%3 == 2 {
			form = url.Values{"grant_type": {"refresh_token"}, "refresh_token": {secret}}
		}
		for _, f := range credvariants.SecretForms(secret) {
			for _, a := range []struct {
				status int
				fields map[string]string
			}{
				{http.StatusBadRequest, map[string]string{"error": "invalid_grant", "error_description": "Rejected password " + f.Text}},
				{http.StatusUnauthorized, map[string]string{"error": f.Text}},
				{http.StatusInternalServerError, map[string]string{"error": "server_error", "error_description": f.Text + " could not be checked"}},
				{http.StatusOK, map[string]string{"token_type": "Bearer", "error_description": "no token for " + f.Text}},
				{http.StatusBadRequest, map[string]string{"error": "invalid_request", "error_description": strings.Repeat("padding ", 40) + f.Text}},
			} {
				echo.set(a.status, a.fields)
				_, err := esetRequestToken(context.Background(), "eu", form)
				if err == nil {
					t.Fatalf("HTTP %d %v returned no error", a.status, a.fields)
				}
				if credvariants.SecretRecoverable([]byte(err.Error()), secret) {
					t.Fatalf("trial %d, form %s, HTTP %d: the token error shows the secret\nsecret %q\nfields %q\n   err %v", trial, f.Name, a.status, secret, a.fields, err)
				}
			}
		}
	}
}

// TestTokenErrorShowsOnlyKnownOAuthCodes: the error code is shown only when
// it is a known OAuth2 or ESET code; any other value is server text and is
// never shown, whether or not it holds a registered secret.
func TestTokenErrorShowsOnlyKnownOAuthCodes(t *testing.T) {
	echo := withTokenEcho(t)
	form := url.Values{"grant_type": {"password"}, "username": {"api@msp.example"}, "password": {"pw-7"}}
	for _, tc := range []struct {
		code, want, never string
	}{
		{"invalid_grant", "invalid_grant", ""},
		{"INVALID_USERNAME_OR_PASSWORD", "invalid_username_or_password", ""},
		{"pw-7 is wrong", "unrecognized error code", "pw-7"},
		{"see https://evil.example/x", "unrecognized error code", "evil.example"},
	} {
		echo.set(http.StatusBadRequest, map[string]string{"error": tc.code})
		_, err := esetRequestToken(context.Background(), "eu", form)
		if err == nil || !strings.Contains(err.Error(), tc.want) || (tc.never != "" && strings.Contains(err.Error(), tc.never)) {
			t.Fatalf("error code %q: got %v, want %q and never %q", tc.code, err, tc.want, tc.never)
		}
	}
}

// TestMintedTokensAreRegistered (Codex round 6, P1 #2): every access and
// refresh token the token service returns is a secret of the process from
// that moment, success or not, so an API body that later echoes it (the
// 401 re-mint's new bearer above all) is masked.
func TestMintedTokensAreRegistered(t *testing.T) {
	echo := withTokenEcho(t)
	form := url.Values{"grant_type": {"password"}, "username": {"api@msp.example"}, "password": {"right-password"}}
	for i, status := range []int{http.StatusOK, http.StatusBadRequest} {
		access := "minted-access-" + string(rune('A'+i)) + "-9F3K2Q7W1Z"
		refresh := "minted-refresh-" + string(rune('A'+i)) + "-4H8J6L0P2R"
		echo.set(status, map[string]string{"access_token": access, "refresh_token": refresh, "token_type": "Bearer", "error": "invalid_grant"})
		_, _ = esetRequestToken(context.Background(), "eu", form)
		got := secrets.Mask("Rejected credential " + access + " and " + url.QueryEscape(refresh))
		if strings.Contains(got, "minted-") {
			t.Fatalf("HTTP %d: a token the service returned was not registered: %q", status, got)
		}
	}
}

// TestDoctorNeverShowsASecret: doctor rows carry token-service and API text
// and configuration (base_url); every string in the report is masked before
// it is printed or gated on, in --json and in the human report. The base URL
// row is masked by doctor alone: it is configuration, not server text.
func TestDoctorNeverShowsASecret(t *testing.T) {
	r := credvariants.NewRand(43)
	for trial := 0; trial < 3; trial++ {
		secret := credvariants.RandomSecret(r)
		for fi, f := range credvariants.SecretForms(secret) {
			if fi%3 != trial%3 {
				continue
			}
			for _, human := range []bool{false, true} {
				testenv.Isolate(t)
				respondClearHarness(t)
				echo := withTokenEcho(t)
				echo.set(http.StatusBadRequest, map[string]string{"error": "invalid_grant", "error_description": "Rejected password " + f.Text})
				api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusForbidden)
					_, _ = w.Write([]byte(`{"message":"Rejected credential ` + f.Text + `"}`))
				}))
				t.Setenv("ESET_PROTECT_REGION", "eu")
				t.Setenv("ESET_PROTECT_USERNAME", "api@msp.example")
				t.Setenv("ESET_PROTECT_PASSWORD", secret)
				t.Setenv("ESET_PROTECT_TOKEN", "")
				t.Setenv("ESET_PROTECT_BASE_URL", api.URL+"/p/"+url.PathEscape(f.Text))
				out := runDoctorText(t, human)
				api.Close()
				if credvariants.SecretRecoverable([]byte(out), secret) {
					t.Fatalf("trial %d, form %s (human %v): doctor shows the secret\nsecret %q\n%s", trial, f.Name, human, secret, out)
				}
			}
		}
	}
}

func runDoctorText(t *testing.T, human bool) string {
	t.Helper()
	cmd := RootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	args := []string{"doctor"}
	if !human {
		args = append(args, "--json")
	}
	cmd.SetArgs(args)
	_ = cmd.Execute()
	return out.String() + errOut.String()
}

// TestDoctorMaskKeepsTheRowVerdict: a row withheld whole keeps the verdict
// word it had, so --fail-on and the indicator never turn a failure into OK.
func TestDoctorMaskKeepsTheRowVerdict(t *testing.T) {
	const fixture = "verdict-fixture-5T7Y9U"
	secrets.Register(fixture)
	// Every byte percent-encoded: no fixed form matches, so the backstop
	// withholds each row whole and only the verdict word can survive.
	var nested strings.Builder
	for i := 0; i < len(fixture); i++ {
		fmt.Fprintf(&nested, "%%%02X", fixture[i])
	}
	report := map[string]any{
		"auth":        "ERROR rejected: " + nested.String(),
		"credentials": "WARN scope-limited " + nested.String(),
		"api":         "unreachable: " + nested.String(),
		"cache":       map[string]any{"detail": "x " + fixture},
	}
	esetMaskDoctorReport(report)
	for key, want := range map[string]string{"auth": doctorVerdictFail, "credentials": doctorVerdictWarn, "api": doctorVerdictFail} {
		got, _ := report[key].(string)
		if !strings.Contains(got, secrets.Withheld) {
			t.Fatalf("%s = %q: the fixture must be withheld whole", key, got)
		}
		if strings.Contains(got, "5T7Y9U") || doctorRowVerdict(got) != want {
			t.Fatalf("%s = %q: verdict %s, want %s and no fixture", key, got, doctorRowVerdict(got), want)
		}
	}
	if d, _ := report["cache"].(map[string]any)["detail"].(string); strings.Contains(d, fixture) {
		t.Fatalf("a nested section kept the fixture: %q", d)
	}
}

// TestTerminalWriterMasksRegisteredSecrets: the stdout/stderr funnel is the
// last line. Text echoing a registered secret in any spelling, written in
// pieces cut anywhere (a bufio or tabwriter flush can split a value), comes
// out masked once its line completes or the writer is flushed; complete
// lines are written at once.
func TestTerminalWriterMasksRegisteredSecrets(t *testing.T) {
	r := credvariants.NewRand(53)
	for trial := 0; trial < 40; trial++ {
		secret := credvariants.RandomSecret(r)
		secrets.Register(secret)
		for _, f := range credvariants.SecretForms(secret) {
			in := []byte("first line\n" + credvariants.RandomContext(r) + " " + f.Text + "\n{\"message\":\"" + f.Text + "\"}\nlast " + f.Text)
			var out bytes.Buffer
			w := &c1EscapeWriter{w: &out}
			cuts := []int{r.Intn(len(in) + 1), r.Intn(len(in) + 1), r.Intn(len(in) + 1)}
			prev := 0
			for _, c := range append(sortedInts(cuts), len(in)) {
				if c < prev {
					continue
				}
				_, _ = w.Write(in[prev:c])
				prev = c
			}
			if !strings.HasPrefix(out.String(), "first line\n") {
				t.Fatalf("a complete line was held back: %q", out.String())
			}
			_ = w.Flush()
			if credvariants.SecretRecoverable(out.Bytes(), secret) {
				t.Fatalf("trial %d, form %s, cuts %v: the writer let the secret out\nsecret %q\n   out %q", trial, f.Name, cuts, secret, out.String())
			}
		}
	}
}

func sortedInts(xs []int) []int {
	out := append([]int(nil), xs...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// TestDeliverMasksTextPayloads: a --deliver file payload leaves the process
// like stdout and is masked; a binary download envelope stays byte-exact.
func TestDeliverMasksTextPayloads(t *testing.T) {
	const fixture = "deliver-fixture-8N2B6V"
	secrets.Register(fixture)
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	if err := Deliver(DeliverSink{Scheme: "file", Target: path}, []byte(`{"message":"Rejected `+url.QueryEscape(fixture)+`"}`+"\n"), false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || credvariants.SecretRecoverable(got, fixture) {
		t.Fatalf("the delivered file holds the fixture (%v): %q", err, got)
	}
}
