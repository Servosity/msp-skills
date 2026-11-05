// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

// Codex review round 1 (appdirect 4.34.2 reprint) class
// credential-sent-to-another-host: the token URL the client ID and secret are
// posted to came from somewhere the operator did not configure. A base URL
// the resolver could not derive an origin from (userinfo, no host) fell back
// to AppDirect's own marketplace, and a base_url from a configuration file
// other users can change (which Load refuses to send credentials to) still
// chose the token URL `auth login` posts the pair to. Each test was written to
// fail on the code before the fix.

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"appdirect-pp-cli/internal/config"
)

// The token URL is the configured one, or one on the configured base URL's
// own origin, or none at all: never another host.
func TestR1TokenURLNeverLeavesTheConfiguredHost(t *testing.T) {
	for _, c := range []struct {
		name     string
		cfg      *config.Config
		wantHost string // "" = no token URL at all
	}{
		{"base URL with a user", &config.Config{BaseURL: "https://user@tenant.example.net/api"}, ""},
		{"base URL with a user and password", &config.Config{BaseURL: "https://user:pw@tenant.example.net/api"}, ""},
		{"base URL with no host name", &config.Config{BaseURL: "https://:8443/api"}, ""},
		{"base URL that does not parse", &config.Config{BaseURL: "https://tenant.example.net:bad/api"}, ""},
		{"base URL with another scheme", &config.Config{BaseURL: "ftp://tenant.example.net/api"}, ""},
		{"base URL with a non-ASCII host", &config.Config{BaseURL: "https://tenаnt.example.net/api"}, ""},
		{"scheme-less base URL with a user", &config.Config{BaseURL: "user@tenant.example.net/api"}, ""},
		{"token URL with a user", &config.Config{BaseURL: "https://tenant.example.net/api", TokenURL: "https://u:p@login.example.net/oauth2/token"}, ""},
		{"token URL with another scheme", &config.Config{BaseURL: "https://tenant.example.net/api", TokenURL: "file:///etc/passwd"}, ""},
		{"token URL with no host", &config.Config{BaseURL: "https://tenant.example.net/api", TokenURL: "/oauth2/token"}, ""},
		{"a marketplace's own base URL", &config.Config{BaseURL: "https://tenant.example.net/api"}, "tenant.example.net"},
		{"a marketplace on a port", &config.Config{BaseURL: "https://tenant.example.net:8443/api"}, "tenant.example.net:8443"},
		{"an explicit token URL", &config.Config{BaseURL: "https://tenant.example.net/api", TokenURL: "https://login.example.net/oauth2/token"}, "login.example.net"},
		{"the default marketplace", &config.Config{BaseURL: "https://marketplace.appdirect.com/api"}, "marketplace.appdirect.com"},
	} {
		got := ResolveAppDirectTokenURL(c.cfg)
		if c.wantHost == "" {
			if got != "" {
				t.Errorf("%s: token URL %q, want none (nothing to post the client secret to)", c.name, got)
			}
			continue
		}
		u, err := url.Parse(got)
		if err != nil || u.Host != c.wantHost || u.Path != "/oauth2/token" && c.cfg.TokenURL == "" {
			t.Errorf("%s: token URL %q, want https://%s/oauth2/token", c.name, got, c.wantHost)
		}
	}
}

// A configuration file other users can change may not choose where the
// client secret is posted: Load refuses its credential route, and the token
// URL follows that refusal (auth login, the mint and the refresh all resolve
// through ResolveAppDirectTokenURL).
func TestR1UntrustedConfigNeverChoosesTheTokenHost(t *testing.T) {
	dir := isolateCredentialStore(t)
	path := filepath.Join(dir, "loose", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("base_url = \"https://collector.example.net/api\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o666); err != nil { // other users can change it
		t.Fatal(err)
	}
	t.Setenv("APPDIRECT_CLIENT_ID", "looseClientId-5d1e")
	t.Setenv("APPDIRECT_CLIENT_SECRET", "looseClientSecret-8a3c")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.CredentialRouteBlocked() {
		t.Skip("this platform does not report loose permissions")
	}
	if got := ResolveAppDirectTokenURL(cfg); got != "" {
		t.Fatalf("token URL %q: a file other users can change chose where the client secret is posted (want none: Load refused that route)", got)
	}
	// The mint refuses rather than sending the pair anywhere.
	c := New(cfg, 0, 0)
	c.HTTPClient.Transport = r1NoNetwork{t}
	if err := c.mintClientCredentials(t.Context(), "looseClientId-5d1e", "looseClientSecret-8a3c"); err == nil || !strings.Contains(err.Error(), "token URL") {
		t.Fatalf("mint with no token URL: err = %v, want a refusal naming the token URL", err)
	}
}

// r1NoNetwork fails the test on any request: nothing may be sent.
type r1NoNetwork struct{ t *testing.T }

func (n r1NoNetwork) RoundTrip(req *http.Request) (*http.Response, error) {
	n.t.Errorf("a request was sent: %s %s", req.Method, req.URL.Redacted())
	return nil, errors.New("no network in this test")
}
