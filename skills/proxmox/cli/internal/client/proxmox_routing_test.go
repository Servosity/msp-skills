// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: guards the Proxmox routing/TLS hand-fixes against a reprint that drops a hook.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"proxmox-pp-cli/internal/config"
)

func TestPBSTargetURL(t *testing.T) {
	cfg := func(host string) *config.Config {
		return &config.Config{TemplateVars: map[string]string{"pbs_host": host}}
	}
	cases := []struct {
		name, path, host, base, want string
		isPBS, wantErr               bool
	}{
		{"pve path untouched", "/nodes/pve1/qemu", "pbs1", "", "", false, false},
		{"bare host gets 8007", pbsSpecOrigin + "/api2/json/admin/datastore", "pbs1", "", "https://pbs1:8007/api2/json/admin/datastore", true, false},
		{"host:port kept", pbsSpecOrigin + "/api2/json/ping", "10.0.0.5:9007", "", "https://10.0.0.5:9007/api2/json/ping", true, false},
		{"full origin", pbsSpecOrigin + "/api2/json/ping", "https://pbs.example.net", "", "https://pbs.example.net:8007/api2/json/ping", true, false},
		{"path in host rejected", pbsSpecOrigin + "/api2/json/ping", "https://pbs1/evil", "", "", true, true},
		{"unset errors on remote PVE override", pbsSpecOrigin + "/api2/json/ping", "pbs_host_placeholder", "https://pve1:8006/api2/json", "", true, true},
		{"unset errors on templated default", pbsSpecOrigin + "/api2/json/ping", "", "https://{host}:8006/api2/json", "", true, true},
		{"profile without PBS never routes", pbsSpecOrigin + "/api2/json/ping", PBSHostNone, "http://127.0.0.1:5555", "", true, true},
		{"unset errors on loopback base outside the harness", pbsSpecOrigin + "/api2/json/ping", "", "http://127.0.0.1:5555", "", true, true},
	}
	for _, tc := range cases {
		t.Setenv("PRINTING_PRESS_VERIFY", "")
		t.Setenv("PRINTING_PRESS_DOGFOOD", "")
		got, isPBS, err := pbsTargetURL(tc.path, cfg(tc.host), tc.base)
		if isPBS != tc.isPBS || (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("%s: got (%q, %v, %v), want (%q, %v, err=%v)", tc.name, got, isPBS, err, tc.want, tc.isPBS, tc.wantErr)
		}
	}
}

func TestPBSTargetURLHarnessMock(t *testing.T) {
	t.Setenv("PRINTING_PRESS_VERIFY", "1")
	got, isPBS, err := pbsTargetURL(pbsSpecOrigin+"/api2/json/ping", &config.Config{}, "http://127.0.0.1:5555")
	if !isPBS || err != nil || got != "http://127.0.0.1:5555/api2/json/ping" {
		t.Fatalf("verify harness mock: got (%q, %v, %v)", got, isPBS, err)
	}
	none := &config.Config{TemplateVars: map[string]string{"pbs_host": PBSHostNone}}
	if _, _, err := pbsTargetURL(pbsSpecOrigin+"/api2/json/ping", none, "http://127.0.0.1:5555"); err == nil {
		t.Fatal("a profile without PBS must not use the harness fallback")
	}
	if _, _, err := pbsTargetURL(pbsSpecOrigin+"/api2/json/ping", &config.Config{}, "https://pve1:8006/api2/json"); err == nil {
		t.Fatal("a remote base must never stand in for PBS, even under the harness")
	}
}

func TestPBSDisplayURL(t *testing.T) {
	cfg := &config.Config{TemplateVars: map[string]string{"pbs_host": "pbs1"}}
	if got := pbsDisplayURL(pbsSpecOrigin+"/api2/json/ping", cfg, ""); got != "https://pbs1:8007/api2/json/ping" {
		t.Fatalf("display = %q", got)
	}
	if got := pbsDisplayURL("/nodes/pve1/qemu", cfg, ""); got != "/nodes/pve1/qemu" {
		t.Fatalf("PVE path changed: %q", got)
	}
}

func TestPBSAuthValue(t *testing.T) {
	if got := pbsAuthValue("root@pam!cli:abc"); got != "PBSAPIToken=root@pam!cli:abc" {
		t.Fatalf("bare token: %q", got)
	}
	if got := pbsAuthValue("PBSAPIToken=root@pam!cli:abc"); got != "PBSAPIToken=root@pam!cli:abc" {
		t.Fatalf("prefixed token doubled: %q", got)
	}
}

// End to end through Client.Get: PBS routing + PBS token scheme + pinned self-signed TLS.
func TestPBSRequestPinnedTLS(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":"pong"}`))
	}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "https://")
	newClient := func(pbsPins ...string) *Client {
		cfg := &config.Config{
			BaseURL:            "https://{host}:8006/api2/json",
			TemplateVars:       map[string]string{"host": "localhost", "pbs_host": addr},
			ProxmoxPbsApiToken: "root@pam!cli:s3cret",
		}
		c := New(cfg, 5*time.Second, 0)
		c.NoCache = true
		if err := c.SetServicePins(nil, pbsPins); err != nil {
			t.Fatal(err)
		}
		return c
	}
	t.Setenv("PRINTING_PRESS_DOGFOOD", "1") // no retries on the expected TLS failures
	if _, err := newClient().Get(context.Background(), pbsSpecOrigin+"/api2/json/ping", nil); err == nil {
		t.Fatal("self-signed cert accepted without a pin")
	}
	if _, err := newClient(strings.Repeat("AB:", 31)+"AB").Get(context.Background(), pbsSpecOrigin+"/api2/json/ping", nil); err == nil {
		t.Fatal("wrong pin accepted")
	}
	if _, err := newClient(CertFingerprint(srv.Certificate())).Get(context.Background(), pbsSpecOrigin+"/api2/json/admin/datastore", nil); err != nil {
		t.Fatalf("pinned request failed: %v", err)
	}
	if gotPath != "/api2/json/admin/datastore" || gotAuth != "PBSAPIToken=root@pam!cli:s3cret" {
		t.Fatalf("server saw path=%q auth=%q", gotPath, gotAuth)
	}
}

// A PBS pin must never vouch for the PVE endpoint (and vice versa): httptest
// servers share one certificate, so the PVE server below presents exactly the
// pinned PBS certificate and must still be rejected.
func TestPinsAreBoundToTheirService(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	pve, pbs := httptest.NewTLSServer(ok), httptest.NewTLSServer(ok)
	defer pve.Close()
	defer pbs.Close()
	t.Setenv("PRINTING_PRESS_DOGFOOD", "1")
	cfg := &config.Config{
		BaseURL:            pve.URL + "/api2/json",
		TemplateVars:       map[string]string{"pbs_host": strings.TrimPrefix(pbs.URL, "https://")},
		ProxmoxApiToken:    "root@pam!cli=s3cret",
		ProxmoxPbsApiToken: "root@pam!cli:s3cret",
	}
	c := New(cfg, 5*time.Second, 0)
	c.NoCache = true
	fp := CertFingerprint(pbs.Certificate())
	if err := c.SetServicePins(nil, []string{fp}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), pbsSpecOrigin+"/api2/json/admin/datastore", nil); err != nil {
		t.Fatalf("PBS with its own pin failed: %v", err)
	}
	if _, err := c.Get(context.Background(), "/nodes", nil); err == nil {
		t.Fatal("the PBS pin authorized the PVE endpoint")
	}
	if err := c.SetServicePins([]string{fp}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/nodes", nil); err != nil {
		t.Fatalf("PVE with its own pin failed: %v", err)
	}
	if _, err := c.Get(context.Background(), pbsSpecOrigin+"/api2/json/admin/datastore", nil); err == nil {
		t.Fatal("the PVE pin authorized the PBS endpoint")
	}
}

func TestPBSOriginRequiresHTTPS(t *testing.T) {
	if _, err := pbsOrigin("http://pbs1:8007"); err == nil {
		t.Fatal("plain http PBS origin accepted")
	}
	if got, err := pbsOrigin("pbs1"); err != nil || got != "https://pbs1:8007" {
		t.Fatalf("pbsOrigin(pbs1) = %q, %v", got, err)
	}
}

func TestEnvelopeSidecarsUnwrap(t *testing.T) {
	got := string(unwrapResponseEnvelope([]byte(`{"data":[{"upid":"x"}],"total":1}`)))
	if got != `[{"upid":"x"}]` {
		t.Fatalf("total sidecar not unwrapped: %s", got)
	}
	if got := string(unwrapResponseEnvelope([]byte(`{"data":{"a":1},"digest":"d"}`))); got != `{"a":1}` {
		t.Fatalf("digest sidecar not unwrapped: %s", got)
	}
	if got := string(unwrapResponseEnvelope([]byte(`{"data":1,"other":2}`))); got != `{"data":1,"other":2}` {
		t.Fatalf("unknown sibling key must not be unwrapped: %s", got)
	}
}

func TestPBSPathsNeverUsePVEToken(t *testing.T) {
	var gotAuth string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()
	t.Setenv("PRINTING_PRESS_DOGFOOD", "1")
	cfg := &config.Config{
		BaseURL:            "https://{host}:8006/api2/json",
		TemplateVars:       map[string]string{"host": "localhost", "pbs_host": strings.TrimPrefix(srv.URL, "https://")},
		ProxmoxApiToken:    "root@pam!pve=PVE-SECRET",
		ProxmoxPbsApiToken: "root@pam!pbs:PBS-SECRET",
	}
	c := New(cfg, 5*time.Second, 0)
	c.NoCache = true
	if err := c.SetServicePins(nil, []string{CertFingerprint(srv.Certificate())}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), PBSPath("/status/not-in-the-spec"), nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(gotAuth, "PVE-SECRET") || gotAuth != "PBSAPIToken=root@pam!pbs:PBS-SECRET" {
		t.Fatalf("PBS path outside the route table sent %q", gotAuth)
	}
}

// The PVE base is checked after {host} resolves: PROXMOX_HOST naming the PBS
// endpoint must be refused, not slip past as an unparsed template.
func TestServiceOriginCheckResolvesTemplate(t *testing.T) {
	c := New(&config.Config{BaseURL: "https://{host}:8006/api2/json",
		TemplateVars: map[string]string{"host": "pbs1", "pbs_host": "pbs1:8006"}}, time.Second, 0)
	if err := c.CheckServiceOrigins(); err == nil {
		t.Fatal("templated PVE base resolving to the PBS endpoint accepted")
	}
	c.Config.TemplateVars["pbs_host"] = "pbs1:8007"
	if err := c.CheckServiceOrigins(); err != nil {
		t.Fatalf("distinct ports refused: %v", err)
	}
}

// An absolute URL on the configured PBS host goes to PBS with the PBS token; a
// PBS-shaped path that goes to PVE carries no PBS token.
func TestAuthFollowsDestination(t *testing.T) {
	var pbsAuth, pveAuth string
	pbs := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pbsAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer pbs.Close()
	pve := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pveAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer pve.Close()
	cfg := &config.Config{
		BaseURL:            pve.URL + "/api2/json",
		TemplateVars:       map[string]string{"pbs_host": strings.TrimPrefix(pbs.URL, "https://")},
		ProxmoxApiToken:    "root@pam!pve=PVE-SECRET",
		ProxmoxPbsApiToken: "root@pam!pbs:PBS-SECRET",
	}
	c := New(cfg, 5*time.Second, 0)
	c.NoCache = true
	if err := c.SetServicePins([]string{CertFingerprint(pve.Certificate())}, []string{CertFingerprint(pbs.Certificate())}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), pbs.URL+"/api2/json/admin/datastore", nil); err != nil {
		t.Fatal(err)
	}
	if pbsAuth != "PBSAPIToken=root@pam!pbs:PBS-SECRET" || pveAuth != "" {
		t.Fatalf("absolute PBS URL: pbs got %q, pve got %q", pbsAuth, pveAuth)
	}
	pbsAuth, pveAuth = "", ""
	if _, err := c.Get(context.Background(), "/api2/json/admin/datastore", nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(pveAuth, "PBS-SECRET") || pbsAuth != "" {
		t.Fatalf("PBS-shaped relative path: pve got %q, pbs got %q", pveAuth, pbsAuth)
	}
}

// Equivalent spellings of one endpoint compare equal (leading-zero ports,
// case, IPv6 compression), so the PVE base cannot alias the PBS host.
func TestOriginCanonicalization(t *testing.T) {
	for _, base := range []string{"https://pbs1:08007/api2/json", "https://PBS1:8007/api2/json", "https://pbs1.:8007/api2/json"} {
		c := New(&config.Config{BaseURL: base, TemplateVars: map[string]string{"pbs_host": "pbs1:8007"}}, time.Second, 0)
		if err := c.CheckServiceOrigins(); err == nil {
			t.Errorf("%s aliases the PBS host but was accepted", base)
		}
	}
	c := New(&config.Config{BaseURL: "https://[fd00:0:0::1]:8007/api2/json", TemplateVars: map[string]string{"pbs_host": "[fd00::1]:8007"}}, time.Second, 0)
	if err := c.CheckServiceOrigins(); err == nil {
		t.Error("expanded IPv6 literal aliases the PBS host but was accepted")
	}
}

// Auth follows routing exactly: the bare placeholder origin is routed to PBS,
// so it uses the PBS scheme; a PBS-shaped path to a remote base under the
// harness carries no PBS token.
func TestAuthSchemeMatchesRouting(t *testing.T) {
	c := New(&config.Config{BaseURL: "https://pve1:8006/api2/json", TemplateVars: map[string]string{"pbs_host": "pbs1"}}, time.Second, 0)
	if got := c.proxmoxAuthScheme("GET", pbsSpecOrigin); got != "PbsApiToken" {
		t.Errorf("bare placeholder origin: scheme %q", got)
	}
	t.Setenv("PRINTING_PRESS_DOGFOOD", "1")
	remote := New(&config.Config{BaseURL: "https://pve1:8006/api2/json", TemplateVars: map[string]string{}}, time.Second, 0)
	if got := remote.proxmoxAuthScheme("GET", "/api2/json/admin/datastore"); got == "PbsApiToken" {
		t.Error("harness exception sent the PBS scheme to a remote base")
	}
}

// http:// to the PBS endpoint still dials PBS; plain http PVE bases are refused
// outside the harness; invalid ports never compare equal.
func TestServiceOriginSchemeAndPorts(t *testing.T) {
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	for _, base := range []string{"http://pbs1:8007/api2/json", "http://pve1:8006/api2/json"} {
		c := New(&config.Config{BaseURL: base, TemplateVars: map[string]string{"pbs_host": "pbs1"}}, time.Second, 0)
		if err := c.CheckServiceOrigins(); err == nil {
			t.Errorf("%s accepted", base)
		}
	}
	a, _ := url.Parse("https://pbs1:65536/")
	if sameEndpoint(a, a) {
		t.Error("an invalid port compared equal to itself")
	}
}

// Plain http is allowed only to a loopback IP literal (a local stub never
// leaves the machine). A loopback name, any other host, and a loopback base
// that aliases the PBS endpoint stay refused.
func TestPlainHTTPOnlyToLoopbackLiterals(t *testing.T) {
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	for base, ok := range map[string]bool{
		"http://127.0.0.1:8006/api2/json": true,
		"http://[::1]:8006/api2/json":     true,
		"http://localhost:8006/api2/json": false,
		"http://10.0.0.5:8006/api2/json":  false,
		"http://127.0.0.1:8007/api2/json": false, // the PBS endpoint below
	} {
		c := New(&config.Config{BaseURL: base, TemplateVars: map[string]string{"pbs_host": "127.0.0.1:8007"}}, time.Second, 0)
		if err := c.CheckServiceOrigins(); (err == nil) != ok {
			t.Errorf("%s: err = %v, want accepted=%v", base, err, ok)
		}
	}
}
