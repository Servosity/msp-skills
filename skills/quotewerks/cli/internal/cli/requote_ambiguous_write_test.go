// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored test for requote.go and qw_common.go (hand-written files a reprint keeps).

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"quotewerks-pp-cli/internal/cliutil/testenv"
)

// TestRequoteAmbiguousLineIsFlagged: a line POST that QuoteWerks commits but
// whose response is lost must be reported as possibly written, and the advice
// must be to check before adding, never a blind 'items add'. A line QuoteWerks
// rejected with a 4xx is reported as not written.
func TestRequoteAmbiguousLineIsFlagged(t *testing.T) {
	testenv.Isolate(t)
	fake := newFakeQW()
	fake.failPart["SVC-1"] = true // definitive 400 rejection
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/DocumentItems") {
			body, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(body))
			if strings.Contains(string(body), `"X1"`) {
				fake.ServeHTTP(httptest.NewRecorder(), r)
				conn, _, _ := w.(http.Hijacker).Hijack() // committed, then the connection drops
				_ = conn.Close()
				return
			}
		}
		fake.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("QUOTEWERKS_BASE_URL", srv.URL)
	t.Setenv("QUOTEWERKS_API_KEY", "qww_test_key")

	cmd := RootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{"requote", "1042", "--only", "X1,SVC-1", "--confirm", "--json", "--no-cache"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("requote with failed lines exited 0: %s", out.String())
	}
	if !strings.Contains(err.Error(), "docs get ") || strings.Contains(err.Error(), "items list --doc") {
		t.Fatalf("partial-failure advice does not say to check first: %v", err)
	}
	var res struct {
		Failures []requoteFailure `json:"line_failures"`
	}
	if jerr := json.Unmarshal(out.Bytes(), &res); jerr != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", jerr, out.String())
	}
	got := map[string]bool{}
	for _, f := range res.Failures {
		got[f.Part] = f.MayHaveBeenWritten
	}
	if len(got) != 2 || !got["X1"] || got["SVC-1"] {
		t.Fatalf("may_have_been_written = %v, want X1 true (connection dropped after commit), SVC-1 false (400)", got)
	}
}

// TestCreateWithUnusableResponseMayHaveCommitted: a create that got a success
// status but no usable resource (a null body, an HTML page that error
// classification rewrites as an auth failure) may exist, so the marker must
// survive classification.
func TestCreateWithUnusableResponseMayHaveCommitted(t *testing.T) {
	testenv.Isolate(t)
	for _, tc := range []struct{ name, contentType, body string }{
		{"null data", "application/vnd.api+json", `{"data":null}`},
		{"html unauthorized", "text/html", `<html><head><title>Unauthorized</title></head><body>Please sign in</body></html>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(201)
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(srv.Close)
			t.Setenv("QUOTEWERKS_BASE_URL", srv.URL)
			t.Setenv("QUOTEWERKS_API_KEY", "qww_test_key")
			flags := &rootFlags{timeout: 5e9}
			c, err := flags.newClient()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := qwWrite(t.Context(), c, "POST", "DocumentItems", "", map[string]any{"Description": "x"}); !errors.Is(err, errWriteMayHaveCommitted) {
				t.Fatalf("err = %v, want errWriteMayHaveCommitted", err)
			}
		})
	}
}
