// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"synology-pp-cli/internal/cliutil/testenv"
)

// runCLIErr is runCLI that also returns the error text the CLI would print.
func runCLIErr(t *testing.T, args ...string) (stdout, errText string, code int) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	if err != nil {
		code = ExitCode(err)
		errText = err.Error()
	}
	return out.String(), errOut.String() + errText, code
}

// notDSMHost records everything every request carried.
func notDSMHost(t *testing.T, h http.HandlerFunc) (url string, seen func() string) {
	t.Helper()
	var mu sync.Mutex
	var log strings.Builder
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		log.WriteString(r.Method + " " + r.URL.String() + " ")
		for k, vs := range r.Header {
			log.WriteString(k + "=" + strings.Join(vs, ",") + ";")
		}
		log.WriteString(" " + string(b) + "\n")
		mu.Unlock()
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, func() string { mu.Lock(); defer mu.Unlock(); return log.String() }
}

// TestCredentialPathsNeverSendThePasswordToANonDSMHost drives every command
// that logs in (auth login, a live read, fleet add --check, fleet sync)
// against a host that is not a DSM WebAPI - a router's HTML page, JSON
// without DSM's envelope, a 404 - and requires that no request it received
// carried the password, and that the error says why. The same commands
// against a DSM stand-in still log in.
func TestCredentialPathsNeverSendThePasswordToANonDSMHost(t *testing.T) {
	const pw = "PWSENTINEL-9917"
	hosts := map[string]http.HandlerFunc{
		"html": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, "<html><body>router login</body></html>")
		},
		"json": func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"status":"ok","sid":"x","data":{"SYNO.API.Auth":{}}}`)
		},
		"404": http.NotFound,
	}
	for name, h := range hosts {
		t.Run(name, func(t *testing.T) {
			testenv.Isolate(t)
			url, seen := notDSMHost(t, h)
			t.Setenv("SYNOLOGY_ACCOUNT", "msp-admin")
			t.Setenv("SYNOLOGY_PASSWORD", pw)
			t.Setenv("SYNOLOGY_ROUTER_ACCOUNT", "msp-admin")
			t.Setenv("SYNOLOGY_ROUTER_PASSWORD", pw)
			t.Setenv("SYNOLOGY_BASE_URL", url)
			steps := [][]string{
				{"auth", "login", "--json"},
				{"system", "dsm", "--json"},
				{"fleet", "add", "router", "--url", url, "--check", "--json"},
				{"fleet", "add", "router", "--url", url, "--json"},
				{"fleet", "sync", "--nas", "router", "--json"},
			}
			for _, args := range steps {
				out, errText, code := runCLIErr(t, args...)
				joined := strings.Join(args, " ")
				if strings.Contains(seen(), pw) {
					t.Fatalf("%s sent the password to a %s host that is not DSM:\n%s", joined, name, seen())
				}
				if strings.Contains(joined, "--check") || args[0] != "fleet" || args[1] == "sync" {
					if code == 0 {
						t.Fatalf("%s against a %s host exited 0", joined, name)
					}
					if !strings.Contains(out+errText, "not a DSM WebAPI") && !strings.Contains(out+errText, "does not answer as a DSM WebAPI") {
						t.Fatalf("%s: the error does not say the host is not DSM:\nout=%s\nerr=%s", joined, out, errText)
					}
					if !strings.Contains(out+errText, "the password was not sent") {
						t.Fatalf("%s: the error does not say the password was not sent:\nout=%s\nerr=%s", joined, out, errText)
					}
				}
			}
			if strings.Contains(seen(), "POST") {
				t.Fatalf("a %s host that is not DSM received a POST:\n%s", name, seen())
			}
		})
	}

	t.Run("dsm stand-in still logs in", func(t *testing.T) {
		testenv.Isolate(t)
		srv := fakeABBNAS(t)
		t.Setenv("SYNOLOGY_BASE_URL", srv.URL)
		t.Setenv("SYNOLOGY_ACCOUNT", "msp-admin")
		t.Setenv("SYNOLOGY_PASSWORD", "s3cret")
		t.Setenv("SYNOLOGY_ACME_ACCOUNT", "msp-admin")
		t.Setenv("SYNOLOGY_ACME_PASSWORD", "s3cret")
		for _, args := range [][]string{
			{"auth", "login", "--json"},
			{"fleet", "add", "acme", "--url", srv.URL, "--check", "--json"},
			{"fleet", "sync", "--nas", "acme", "--json"},
		} {
			if out, errText, code := runCLIErr(t, args...); code != 0 {
				t.Fatalf("%v against a DSM: exit %d\nout=%s\nerr=%s", args, code, out, errText)
			}
		}
	})
}
