// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"zoom-phone-pp-cli/internal/cliutil/testenv"
)

// routingFakeHandler answers one path; it returns an HTTP status and a body
// (a raw JSON string or any value to encode).
type routingFakeHandler func(q url.Values) (int, any)

// routingFake is a spec-shaped Zoom Phone API stand-in mounted under /v2.
type routingFake struct {
	mu     sync.Mutex
	routes map[string]routingFakeHandler
	hits   []string
}

func (f *routingFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v2")
	f.mu.Lock()
	f.hits = append(f.hits, path+"?"+r.URL.Query().Encode())
	h := f.routes[path]
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if h == nil {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":404,"message":"Not found"}`))
		return
	}
	status, body := h(r.URL.Query())
	w.WriteHeader(status)
	if s, ok := body.(string); ok {
		_, _ = w.Write([]byte(s))
		return
	}
	_ = json.NewEncoder(w).Encode(body)
}

func (f *routingFake) hitList() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.hits...)
}

func (f *routingFake) countPrefix(prefix string) int {
	n := 0
	for _, h := range f.hitList() {
		if strings.HasPrefix(h, prefix) {
			n++
		}
	}
	return n
}

// routingStartFake starts the fake and points the CLI config at it.
func routingStartFake(t *testing.T, routes map[string]routingFakeHandler) *routingFake {
	t.Helper()
	testenv.Isolate(t)
	fake := &routingFake{routes: routes}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	t.Setenv("ZOOM_PHONE_BASE_URL", srv.URL+"/v2")
	for _, k := range []string{
		"ZOOM_PHONE_ACCOUNT_ID", "ZOOM_PHONE_CLIENT_ID", "ZOOM_PHONE_CLIENT_SECRET",
		"ZOOM_PHONE_TOKEN_URL", "PRINTING_PRESS_VERIFY", "PRINTING_PRESS_DOGFOOD",
	} {
		t.Setenv(k, "")
	}
	return fake
}

func routingStatic(body any) routingFakeHandler {
	return func(url.Values) (int, any) { return http.StatusOK, body }
}

// routingRun executes a novel command with --json semantics and returns
// stdout, stderr and the command error.
func routingRun(t *testing.T, build func(*rootFlags) *cobra.Command, args ...string) (string, string, error) {
	t.Helper()
	return routingRunFlags(t, &rootFlags{asJSON: true}, build, args...)
}

// routingRunFlags runs a command with caller-supplied root flags (for root
// persistent flags such as --allow-partial-failure). The commands under test
// force NoCache on their own client, so the flags leave the cache untouched.
func routingRunFlags(t *testing.T, flags *rootFlags, build func(*rootFlags) *cobra.Command, args ...string) (string, string, error) {
	t.Helper()
	cmd := build(flags)
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	err := cmd.Execute()
	return out.String(), errb.String(), err
}

func routingExitCode(err error) int {
	var ce *cliError
	if errors.As(err, &ce) {
		return ce.code
	}
	if err != nil {
		return 1
	}
	return 0
}
