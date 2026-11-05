// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// End-to-end class test for the 4.34.2 reprint's credential fix (orchestrator
// group A, credential-output): the built binary, its real stdout and stderr,
// and every file it writes. Each case failed on the round-2 tree.

package main

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"netsapiens-pp-cli/internal/cliutil/testenv"
)

const apiKey = "DifferentApiKey55443322"

var (
	buildOnce sync.Once
	builtBin  string
	buildErr  error
)

func cliBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "netsapiens-chokepoint-")
		if err != nil {
			buildErr = err
			return
		}
		builtBin = filepath.Join(dir, "netsapiens-cli")
		if runtime.GOOS == "windows" {
			builtBin += ".exe"
		}
		out, err := exec.Command("go", "build", "-o", builtBin, ".").CombinedOutput()
		if err != nil {
			buildErr = err
			builtBin = string(out)
		}
	})
	if buildErr != nil {
		t.Fatalf("build: %v\n%s", buildErr, builtBin)
	}
	return builtBin
}

type run struct{ stdout, stderr string }

func runCLI(t *testing.T, env []string, args ...string) run {
	t.Helper()
	cmd := exec.Command(cliBinary(t), args...)
	cmd.Env = append(os.Environ(), env...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	_ = cmd.Run()
	return run{out.String(), errb.String()}
}

func shown(text string, secrets ...string) string {
	for _, s := range secrets {
		for _, f := range []string{s, base64.StdEncoding.EncodeToString([]byte(s))} {
			if strings.Contains(text, f) {
				return f
			}
		}
	}
	return ""
}

func filesUnder(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			data, _ := os.ReadFile(p)
			files[p] = string(data)
		}
		return nil
	})
	return files
}

func fakeVendor(t *testing.T, status int, body string) []string {
	t.Helper()
	srv, trust := vendorTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	return append(trust, "NETSAPIENS_BASE_URL="+srv.URL, "NETSAPIENS_API_KEY="+apiKey, "NETSAPIENS_RETRY_MAX=0")
}

func TestClassNoCredentialReachesAnyOutput(t *testing.T) {
	cliBinary(t) // before Isolate: the build needs the real module and build caches
	home := testenv.Isolate(t)
	b64Key := base64.StdEncoding.EncodeToString([]byte(apiKey))
	cases := []struct {
		name    string
		status  int
		body    string
		args    []string
		secrets []string
	}{
		{"vendor credential field on stdout (Domain.email-send-smtp-password)", 200,
			`[{"domain":"acme.example","email-send-smtp-password":"SmtpSecret55443322"}]`,
			[]string{"domains", "list", "--json", "--no-cache", "--data-source", "live"}, []string{"SmtpSecret55443322"}},
		{"vendor credential named in error text", 400,
			`{"code":400,"message":"device-sip-registration-password=SipSecret55443322"}`,
			[]string{"domains", "list", "--json", "--no-cache", "--data-source", "live"}, []string{"SipSecret55443322"}},
		{"operator key echoed base64 in a 2xx field", 200,
			`[{"domain":"acme.example","description":"` + b64Key + `"}]`,
			[]string{"domains", "list", "--json", "--no-cache", "--data-source", "live"}, []string{apiKey}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := runCLI(t, fakeVendor(t, tc.status, tc.body), tc.args...)
			if f := shown(r.stdout+r.stderr, tc.secrets...); f != "" {
				t.Fatalf("%q reached the terminal:\nstdout=%s\nstderr=%s", f, r.stdout, r.stderr)
			}
		})
	}
	// A credential the operator typed into local text (no API call at all): the
	// learn store, the journal and every later output withhold it.
	t.Run("operator key in local text", func(t *testing.T) {
		env := []string{"NETSAPIENS_API_KEY=" + apiKey}
		r1 := runCLI(t, env, "teach", "--query", "find "+apiKey, "--resource-type", "domains", "--resource", "acme.example")
		r2 := runCLI(t, env, "learnings", "list", "--json")
		if f := shown(r1.stdout+r1.stderr+r2.stdout+r2.stderr, apiKey); f != "" {
			t.Fatalf("%q reached the terminal:\n%s%s%s%s", f, r1.stdout, r1.stderr, r2.stdout, r2.stderr)
		}
	})
	for p, data := range filesUnder(t, home) {
		if f := shown(data, apiKey, "SmtpSecret55443322", "SipSecret55443322"); f != "" {
			t.Errorf("%s holds a credential (%q); only the credential store may", p, f)
		}
	}
}
