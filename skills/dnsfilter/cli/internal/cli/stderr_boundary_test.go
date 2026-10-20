// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil/testenv"
)

// A non-JSON API error body carrying OSC 52, BEL, CSI and C1 is printed on
// stderr with every control escaped (the Cobra error line goes through the
// same writer as every direct stderr write).
func TestAPIErrorBodyOnStderrIsEscaped(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, "bad \x1b]52;c;cHduZWQ=\x07 thing \u009b31m \x1b[2J end")
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
	t.Setenv("DNSFILTER_API_KEY", "fixture-key")
	t.Setenv("NO_COLOR", "1")
	_, stderr, err := executeArgs(t, "organizations", "index")
	if err == nil {
		t.Fatal("a 422 succeeded")
	}
	if strings.ContainsAny(stderr, "\x1b\x07\u009b") {
		t.Fatalf("raw control reached stderr: %q", stderr)
	}
	if !strings.Contains(stderr, `\u001b]52;c;cHduZWQ=\u0007`) {
		t.Fatalf("error body missing or not escaped on stderr: %q", stderr)
	}
}

// Every stderr write in the CLI's own packages goes through the escaping
// writer; a direct os.Stderr write (a reprint restores hundreds) bypasses it.
// sync.go's "\r" progress redraws use cliutil.RawStderr() on purpose.
func TestNoDirectStderrWrites(t *testing.T) {
	direct := regexp.MustCompile(`\bos\.Stderr\b`)
	for _, dir := range []string{".", "../client", "../config", "../cliutil", "../store", "../learn", "../platform", "../../cmd/dnsfilter-mcp", "../../cmd/dnsfilter-cli"} {
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		for _, f := range files {
			base := filepath.Base(f)
			if strings.HasSuffix(base, "_test.go") || base == "termwriter.go" {
				continue
			}
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			for i, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "//") {
					continue
				}
				if direct.MatchString(line) {
					t.Errorf("%s:%d writes to os.Stderr directly; use stderrW() / cliutil.Stderr(): %s", f, i+1, strings.TrimSpace(line))
				}
			}
		}
	}
}
