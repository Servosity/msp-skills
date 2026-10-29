// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package kaseyablackbox

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The MCP server's stderr runs through the output filter; a refusal printed
// just before exit must still reach the operator.
func TestMCPStartupRefusalReachesStderr(t *testing.T) {
	e := newEnv(t, "127.0.0.1:9", false)
	for _, args := range [][]string{
		{"--transport", "http"},
		{"--transport", "bogus"},
	} {
		cmd := exec.Command(bbMCP, args...)
		cmd.Env, cmd.Dir = e.env, e.home
		var errb bytes.Buffer
		cmd.Stderr = &errb
		done := make(chan error, 1)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			if err == nil {
				t.Fatalf("%v: want a non-zero exit", args)
			}
		case <-time.After(30 * time.Second):
			_ = cmd.Process.Kill()
			t.Fatalf("%v: did not exit", args)
		}
		want := map[string]string{"http": "KASEYA_VSA_MCP_HTTP_TOKEN", "bogus": "unknown --transport"}[args[1]]
		if !strings.Contains(errb.String(), want) {
			t.Fatalf("%v: stderr %q does not name %q", args, errb.String(), want)
		}
	}
}

// A plain-http base URL is served over HTTP/1.1; the Chrome transport used to
// send it to its h2-over-TLS path and fail every request.
func TestPlainHTTPBaseURLIsServed(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Data":[{"Id":7,"Name":"Northwind IT"}],"Meta":{"ResponseCode":200,"TotalCount":1}}`))
	}))
	t.Cleanup(srv.Close)
	e := newEnv(t, strings.TrimPrefix(srv.URL, "http://"), true)
	e.env = append(e.env, "KASEYA_VSA_BASE_URL="+srv.URL)
	out := e.run(t, "organizations", "list", "--json", "--data-source", "live")
	if hits.Load() == 0 || !strings.Contains(out, "Northwind IT") {
		t.Fatalf("want the plain-http server answered (hits=%d), got:\n%.1500s", hits.Load(), out)
	}
}
