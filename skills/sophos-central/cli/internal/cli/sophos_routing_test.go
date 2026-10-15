// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"sophos-central-pp-cli/internal/client"
	"sophos-central-pp-cli/internal/config"
)

// fakeSophos plays the token endpoint, whoami, the tenant lookup and every
// regional API, recording what reached the wire.
type fakeSophos struct {
	idType string
	mu     sync.Mutex
	seen   []*http.Request
}

func (f *fakeSophos) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	f.seen = append(f.seen, req)
	f.mu.Unlock()
	body := `{}`
	switch {
	case req.URL.Host == "id.sophos.com":
		_ = req.ParseForm()
		if req.PostForm.Get("client_id") == "" {
			return respond(500, `{"errorCode":"SERVER_ERROR"}`), nil
		}
		body = `{"access_token":"tok","token_type":"bearer","expires_in":3600,"errorCode":"success"}`
	case req.URL.Path == "/whoami/v1":
		switch f.idType {
		case "tenant":
			body = `{"id":"T-SELF","idType":"tenant","apiHosts":{"global":"https://api.central.sophos.com","dataRegion":"https://api-eu02.central.sophos.com"}}`
		default:
			body = `{"id":"P-1","idType":"` + f.idType + `","apiHosts":{"global":"https://api.central.sophos.com"}}`
		}
	case strings.HasPrefix(req.URL.Path, "/partner/v1/tenants/"):
		id := strings.TrimPrefix(req.URL.Path, "/partner/v1/tenants/")
		host := "https://api-us03.central.sophos.com"
		if id == "EVIL" {
			host = "https://attacker.example.com"
		}
		body = `{"id":"` + id + `","apiHost":"` + host + `"}`
	}
	return respond(200, body), nil
}

func respond(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func (f *fakeSophos) last(t *testing.T, path string) *http.Request {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.seen) - 1; i >= 0; i-- {
		if f.seen[i].URL.Path == path {
			return f.seen[i]
		}
	}
	t.Fatalf("no request for %s reached the wire", path)
	return nil
}

func newRoutedClient(t *testing.T, idType, tenant string) (*client.Client, *fakeSophos) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv(sophosTenantEnv, "")
	sophosTenantFlag = tenant
	t.Cleanup(func() { sophosTenantFlag = "" })
	cfg := &config.Config{BaseURL: "https://api.central.sophos.com", ClientID: "cid-" + idType + tenant, ClientSecret: "secret", Path: filepath.Join(t.TempDir(), "config.toml")}
	c := client.New(cfg, 10*time.Second, 0)
	c.NoCache = true
	fake := &fakeSophos{idType: idType}
	c.HTTPClient.Transport = fake
	if err := installSophosRouting(c); err != nil {
		t.Fatal(err)
	}
	return c, fake
}

func TestSophosRoutingTenantCredential(t *testing.T) {
	c, fake := newRoutedClient(t, "tenant", "")
	if _, err := c.Get(context.Background(), "/endpoint/v1/endpoints", nil); err != nil {
		t.Fatal(err)
	}
	req := fake.last(t, "/endpoint/v1/endpoints")
	if req.URL.Host != "api-eu02.central.sophos.com" || req.Header.Get(headerTenantID) != "T-SELF" {
		t.Fatalf("tenant call routed to %s with X-Tenant-ID=%q", req.URL.Host, req.Header.Get(headerTenantID))
	}
	if _, err := c.Get(context.Background(), "/partner/v1/tenants", nil); err == nil || !strings.Contains(err.Error(), "partner account") {
		t.Fatalf("partner call with a tenant credential should fail clearly, got %v", err)
	}
}

func TestSophosRoutingPartnerCredential(t *testing.T) {
	c, fake := newRoutedClient(t, "partner", "")
	if _, err := c.Get(context.Background(), "/partner/v1/tenants", nil); err != nil {
		t.Fatal(err)
	}
	req := fake.last(t, "/partner/v1/tenants")
	if req.URL.Host != sophosGlobalHost || req.Header.Get(headerPartnerID) != "P-1" || req.Header.Get(headerTenantID) != "" {
		t.Fatalf("partner call: host=%s partner=%q tenant=%q", req.URL.Host, req.Header.Get(headerPartnerID), req.Header.Get(headerTenantID))
	}
	// No tenant selected: a regional call must refuse with guidance, not guess.
	if _, err := c.Get(context.Background(), "/endpoint/v1/endpoints", nil); err == nil || !strings.Contains(err.Error(), "--tenant") {
		t.Fatalf("regional call without tenant should ask for --tenant, got %v", err)
	}
	// Per-call tenant override (the fleet path) resolves that tenant's host.
	if _, err := c.GetWithHeaders(context.Background(), "/common/v1/alerts", nil, map[string]string{headerTenantID: "T-9"}); err != nil {
		t.Fatal(err)
	}
	req = fake.last(t, "/common/v1/alerts")
	if req.URL.Host != "api-us03.central.sophos.com" || req.Header.Get(headerTenantID) != "T-9" || req.Header.Get(headerPartnerID) != "" {
		t.Fatalf("fleet call: host=%s tenant=%q partner=%q", req.URL.Host, req.Header.Get(headerTenantID), req.Header.Get(headerPartnerID))
	}
}

func TestSophosRoutingFlagTenantAndHostAllowList(t *testing.T) {
	c, fake := newRoutedClient(t, "partner", "T-7")
	if c.Config.Headers[headerTenantID] != "T-7" {
		t.Fatal("--tenant must ride in cfg.Headers so the response cache key includes it")
	}
	if _, err := c.Get(context.Background(), "/endpoint/v1/endpoints", nil); err != nil {
		t.Fatal(err)
	}
	if req := fake.last(t, "/endpoint/v1/endpoints"); req.URL.Host != "api-us03.central.sophos.com" {
		t.Fatalf("flag tenant routed to %s", req.URL.Host)
	}
	// A tenant whose apiHost is not a Sophos host never receives the token.
	_, err := c.GetWithHeaders(context.Background(), "/endpoint/v1/endpoints", nil, map[string]string{headerTenantID: "EVIL"})
	if err == nil || !strings.Contains(err.Error(), "no usable Sophos apiHost") {
		t.Fatalf("non-Sophos apiHost must be refused, got %v", err)
	}
	for _, r := range fake.seen {
		if r.URL.Host == "attacker.example.com" {
			t.Fatal("request reached a non-Sophos host")
		}
	}
}

func TestSophosRoutingMockBaseURLPassthrough(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	sophosTenantFlag = "T-1"
	t.Cleanup(func() { sophosTenantFlag = "" })
	cfg := &config.Config{BaseURL: "http://127.0.0.1:9999"}
	c := client.New(cfg, time.Second, 0)
	before := c.HTTPClient.Transport
	if err := installSophosRouting(c); err != nil {
		t.Fatal(err)
	}
	if c.HTTPClient.Transport != before {
		t.Fatal("a non-Sophos BaseURL (verify mock) must not get the routing transport")
	}
	if c.Config.Headers[headerTenantID] != "T-1" {
		t.Fatal("tenant header should still be set for mock servers")
	}
}

func TestSophosContextFileRoundTrip(t *testing.T) {
	var f sophosContextFile
	data := `{"identity":{"id":"P","idType":"partner"},"fetched_at":"2026-01-01T00:00:00Z","tenant_hosts":{"a":"api-us01.central.sophos.com"}}`
	if err := json.Unmarshal([]byte(data), &f); err != nil || f.TenantHosts["a"] == "" {
		t.Fatalf("context file decode: %v %+v", err, f)
	}
}

func TestSophosRoutingGlobalTenantAPIs(t *testing.T) {
	c, fake := newRoutedClient(t, "partner", "")
	if _, err := c.Get(context.Background(), "/licenses/v1/licenses/firewalls", nil); err != nil {
		t.Fatal(err)
	}
	req := fake.last(t, "/licenses/v1/licenses/firewalls")
	if req.URL.Host != sophosGlobalHost || req.Header.Get(headerPartnerID) != "P-1" || req.Header.Get(headerTenantID) != "" {
		t.Fatalf("partner licence call: host=%s partner=%q tenant=%q", req.URL.Host, req.Header.Get(headerPartnerID), req.Header.Get(headerTenantID))
	}
	if _, err := c.GetWithHeaders(context.Background(), "/audit/v1/audit-events", nil, map[string]string{headerTenantID: "T-2"}); err != nil {
		t.Fatal(err)
	}
	req = fake.last(t, "/audit/v1/audit-events")
	if req.URL.Host != sophosGlobalHost || req.Header.Get(headerTenantID) != "T-2" || req.Header.Get(headerPartnerID) != "" {
		t.Fatalf("tenant audit call: host=%s tenant=%q partner=%q", req.URL.Host, req.Header.Get(headerTenantID), req.Header.Get(headerPartnerID))
	}
}
