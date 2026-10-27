// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ninjaone-pp-cli/internal/cliutil/testenv"
)

// Class token-errors-funnelled-at-source, end to end: a token endpoint that
// echoes the client secret into its redirect target must not get the secret
// printed by any command, whether the command returns the error (auth login)
// or prints it itself as it goes (sync's sync_error events).

func redirectingTokenServer(secret string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/oauth/token") {
			http.Redirect(w, r, "https://"+secret+".example.invalid/token", http.StatusTemporaryRedirect)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
}

func runWithTokenRedirect(t *testing.T, secret string, args ...string) string {
	t.Helper()
	testenv.Isolate(t)
	srv := redirectingTokenServer(secret)
	defer srv.Close()
	home := t.TempDir()
	t.Setenv("NINJAONE_CONFIG", filepath.Join(home, "config.toml"))
	t.Setenv("NINJAONE_BASE_URL", srv.URL)
	t.Setenv("NINJAONE_TOKEN_URL", srv.URL+"/ws/oauth/token")
	t.Setenv("NINJAONE_CLIENT_ID", "redirect-client-id")
	t.Setenv("NINJAONE_CLIENT_SECRET", secret)
	t.Setenv("NINJAONE_NO_CONFIG_WRITE", "")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	root := newRootCmd(&rootFlags{})
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs(args)
	captured := captureProcessOutput(t, func() {
		if err := root.Execute(); err != nil {
			errBuf.WriteString("\nRETURNED: " + err.Error())
		}
	})
	return out.String() + errBuf.String() + captured
}

// captureProcessOutput returns what fn wrote to the process's own stdout and
// stderr (code that prints with os.Stderr directly, not through Cobra).
func captureProcessOutput(t *testing.T, fn func()) string {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = w, w
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()
	defer func() {
		os.Stdout, os.Stderr = oldOut, oldErr
	}()
	fn()
	_ = w.Close()
	<-done
	_ = r.Close()
	return buf.String()
}

func assertNoSecret(t *testing.T, label, text, secret string) {
	t.Helper()
	for _, form := range []string{secret, base64.StdEncoding.EncodeToString([]byte(secret))} {
		if strings.Contains(text, form) {
			t.Fatalf("%s printed the client secret (%q):\n%s", label, form, text)
		}
	}
}

func TestAuthLoginNeverPrintsTheSecretFromATokenTransportError(t *testing.T) {
	secret := "loginsecretz8x4c6v2b0"
	text := runWithTokenRedirect(t, secret, "auth", "login")
	if !strings.Contains(text, "RETURNED:") {
		t.Fatalf("auth login succeeded against a token endpoint that redirected off its origin:\n%s", text)
	}
	assertNoSecret(t, "auth login", text, secret)
}

func TestSyncEventsNeverPrintTheSecretFromATokenTransportError(t *testing.T) {
	secret := "syncsecretm3n5b7v9c1"
	text := runWithTokenRedirect(t, secret, "sync", "--resources", "devices", "--full", "--db", filepath.Join(t.TempDir(), "d.db"), "--json")
	if !strings.Contains(text, "sync_error") && !strings.Contains(text, "RETURNED:") {
		t.Fatalf("sync reported no failure for a token endpoint that redirected off its origin:\n%s", text)
	}
	assertNoSecret(t, "sync", text, secret)
}

// A supplied secret shorter than the funnel's decoded-view minimum (8) is still
// the operator's credential: the token-error path withholds it at any length.
func TestTokenTransportErrorsNeverPrintAShortSecret(t *testing.T) {
	secret := "s3cr3t"
	for _, args := range [][]string{
		{"auth", "login"},
		{"sync", "--resources", "devices", "--full", "--db", filepath.Join(t.TempDir(), "d.db"), "--json"},
	} {
		text := runWithTokenRedirect(t, secret, args...)
		if !strings.Contains(text, "RETURNED:") && !strings.Contains(text, "sync_error") {
			t.Fatalf("%v reported no failure:\n%s", args, text)
		}
		assertNoSecret(t, strings.Join(args[:1], " "), text, secret)
	}
}
