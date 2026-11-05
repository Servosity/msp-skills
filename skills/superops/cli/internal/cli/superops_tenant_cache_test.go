// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"superops-pp-cli/internal/cliutil/testenv"
)

// getAnsweringTenants answers the GET the generated list commands send, per
// CustomerSubDomain, and counts the requests each tenant received. The real
// MSP API serves GraphQL on POST /msp; the response cache only ever stores a
// successful GET over HTTPS, so this stands in for any host that answers one.
type getAnsweringTenants struct {
	mu    sync.Mutex
	calls map[string]int
}

func (g *getAnsweringTenants) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tenant := r.Header.Get("CustomerSubDomain")
	g.mu.Lock()
	g.calls[tenant]++
	g.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"getAlertList": map[string]any{
		"nodes": []any{map[string]any{"alertId": tenant + "-1", "message": tenant + "-alert"}},
	}}})
}

func (g *getAnsweringTenants) count(tenant string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls[tenant]
}

// The on-disk response cache is a local store too. Its key folded the base URL
// and the credential but not CustomerSubDomain, so tenant B's read within the
// cache TTL was answered from tenant A's cached body without a request: the
// server never saw tenant B at all.
func TestResponseCacheIsNeverReadAsAnotherTenant(t *testing.T) {
	fake := &getAnsweringTenants{calls: map[string]int{}}
	srv := httptest.NewTLSServer(fake)
	t.Cleanup(srv.Close)
	testenv.Isolate(t)
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUPEROPS_CA_CERT", caPath)
	t.Setenv("SUPEROPS_BASE_URL", srv.URL)
	t.Setenv("SUPEROPS_API_TOKEN", reviewToken)

	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-a")
	for i := 0; i < 2; i++ {
		out, err := runSuperOpsCLI(t, "alerts", "--json", "--data-source", "live")
		if err != nil || !strings.Contains(out, "acme-a-alert") {
			t.Fatalf("tenant A alerts (err %v):\n%s", err, out)
		}
	}
	if got := fake.count("acme-a"); got != 1 {
		t.Fatalf("tenant A's repeat read made %d requests, want 1: the response cache is not in play", got)
	}

	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-b")
	out, err := runSuperOpsCLI(t, "alerts", "--json", "--data-source", "live")
	if err != nil {
		t.Fatalf("tenant B alerts: %v\n%s", err, out)
	}
	if strings.Contains(out, "acme-a-alert") || !strings.Contains(out, "acme-b-alert") || fake.count("acme-b") != 1 {
		t.Fatalf("tenant B's live read was answered from tenant A's cache (tenant B requests %d):\n%s", fake.count("acme-b"), out)
	}
}
