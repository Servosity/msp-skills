// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bitdefender-pp-cli/internal/client"
	"bitdefender-pp-cli/internal/config"
)

// doctor-verdict-tracks-reality: the credential verdict is positive only when
// Control Center answered getApiKeyDetails with a JSON-RPC result.
func TestDoctorProbeCredentials(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		positive bool
	}{
		{"result", 200, `{"jsonrpc":"2.0","id":"doctor","result":{"enabledApis":["network"]}}`, true},
		{"rpc error", 200, `{"jsonrpc":"2.0","id":"doctor","error":{"code":-32001,"message":"Invalid API key"}}`, false},
		{"null result", 200, `{"jsonrpc":"2.0","id":"doctor","result":null}`, false},
		{"401", 401, `{"jsonrpc":"2.0","id":"doctor","error":{"code":-32001,"message":"Invalid API key"}}`, false},
		{"404", 404, `not found`, false},
		{"html", 200, `<!doctype html><html></html>`, false},
		{"empty object", 200, `{}`, false},
		{"list envelope", 200, `{"data":[],"items":[],"results":[]}`, false},
		{"405", 405, `method not allowed`, false},
		{"429", 429, `{"jsonrpc":"2.0","id":"doctor","error":{"code":-32004,"message":"Too many requests"}}`, false},
	}
	for _, tc := range cases {
		var gotAuth, gotMethod string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			if r.URL.Path == "/api/v1.0/jsonrpc/general" {
				gotMethod = "general"
			}
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		c := &client.Client{Config: &config.Config{BaseURL: srv.URL + "/api"}, HTTPClient: srv.Client()}
		report := map[string]any{}
		doctorProbeCredentials(context.Background(), c, "Basic dGVzdC1zZWNyZXQ6", report)
		srv.Close()
		v, _ := report["credentials"].(string)
		positive := strings.HasPrefix(v, "valid")
		if positive != tc.positive {
			t.Errorf("%s: credentials = %q, positive %v, want %v", tc.name, v, positive, tc.positive)
		}
		if gotAuth != "Basic dGVzdC1zZWNyZXQ6" || gotMethod != "general" {
			t.Errorf("%s: probe sent auth %q to %q", tc.name, gotAuth, gotMethod)
		}
	}
}
