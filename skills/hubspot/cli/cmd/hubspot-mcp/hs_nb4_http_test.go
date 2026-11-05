// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

// Next-batch round 2 (handfixes.json credkit-wiring): every response the HTTP
// transport serves passes the credential kit, a protocol error that quotes the
// request back included, in a JSON body and in an event stream, and a bearer
// refusal is written through the kit too.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hubspot-pp-cli/internal/credkit"
)

func TestNB4EveryHTTPResponseIsWithheld(t *testing.T) {
	const v = "nb4-http-secret-0001"
	const bearer = "nb4-http-bearer-ok"
	credkit.Hold("config:test", v)
	h := requireBearerAuthWithLimits(bearer, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sse" {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"error\":{\"code\":-32602,\"message\":\"handler not found for resource URI '" + v + "'\"}}\n\n"))
			w.(http.Flusher).Flush()
			return
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"handler not found for resource URI '` + v + `'"}}`))
	}), mcpHTTPMaxBodyBytes, time.Second)
	srv := httptest.NewServer(h)
	defer srv.Close()
	for _, path := range []string{"/json", "/sse"} {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
		req.Header.Set("Authorization", "Bearer "+bearer)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, rerr := resp.Body.Read(buf)
			b.Write(buf[:n])
			if rerr != nil {
				break
			}
		}
		_ = resp.Body.Close()
		if strings.Contains(b.String(), v) {
			t.Errorf("%s: the held value left the server: %s", path, b.String())
		}
	}
	resp, err := http.Post(srv.URL+"/json", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a request with no bearer got %d, want 401", resp.StatusCode)
	}
}
