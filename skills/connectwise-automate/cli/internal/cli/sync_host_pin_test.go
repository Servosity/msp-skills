// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"connectwise-automate-pp-cli/internal/cliutil/testenv"
)

// The configured server is where the bearer token and the clientId go.
// `sync --path-context` (also an MCP tool argument) may fill path
// placeholders, but never a placeholder of the base URL itself: that would
// send both credentials to a host the operator never configured.
func TestSyncPathContextCannotMoveTheBaseURLHost(t *testing.T) {
	testenv.Isolate(t)
	var tenantHits, otherHits atomic.Int32
	serve := func(n *atomic.Int32) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		}))
	}
	tenant, other := serve(&tenantHits), serve(&otherHits)
	defer tenant.Close()
	defer other.Close()
	t.Setenv("CONNECTWISE_AUTOMATE_BASE_URL", "http://{server}/cwa/api/v1")
	t.Setenv("CONNECTWISE_AUTOMATE_SERVER", strings.TrimPrefix(tenant.URL, "http://"))
	t.Setenv("CONNECTWISE_AUTOMATE_TOKEN", "tenant-secret")
	t.Setenv("CONNECTWISE_AUTOMATE_CLIENT_ID", "11111111-2222-3333-4444-555555555555")

	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"sync", "--resources", "computers", "--json", "--path-context", "server=" + strings.TrimPrefix(other.URL, "http://")})
	err := cmd.Execute()
	if n := otherHits.Load(); n != 0 {
		t.Fatalf("--path-context server= sent %d request(s) with the credentials to another host", n)
	}
	if err == nil || !strings.Contains(err.Error(), "server") {
		t.Fatalf("want a refusal naming the base-URL placeholder, got err=%v out=%s", err, out.String())
	}
}
