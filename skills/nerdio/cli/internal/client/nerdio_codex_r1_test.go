// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

// Codex review round 1 (nerdio 4.34.2 reprint) class
// credential-sent-to-another-host, and nerdio handfix
// token-url-from-operator-only: the token URL the client ID and secret are
// posted to comes only from what the operator configured (NERDIO_TOKEN_URL or
// token_url in a trusted config file), never from the base URL, the shipped
// placeholder tenant, or a configuration file other users can change (which
// Load refuses to send credentials to).

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"nerdio-pp-cli/internal/config"
)

// The token URL is the configured one or none at all: never another host,
// never derived from the NMM instance's base URL (the token lives on the
// operator's Entra tenant, another host), never the placeholder tenant the
// press baked in as the default.
func TestR1TokenURLNeverLeavesTheConfiguredHost(t *testing.T) {
	const entra = "https://login.microsoftonline.com/7c1e9a3f-2b4d-4e6a-8c1f-3a5b7d9e1f2a/oauth2/v2.0/token"
	for _, c := range []struct {
		name string
		cfg  *config.Config
		want string // "" = no token URL at all
	}{
		{"no configuration", nil, ""},
		{"an NMM instance's base URL alone", &config.Config{BaseURL: "https://nmm.contoso.com"}, ""},
		{"an NMM instance on a port", &config.Config{BaseURL: "https://nmm.contoso.com:8443"}, ""},
		{"the shipped placeholder instance", &config.Config{BaseURL: "https://nmm.example.com"}, ""},
		{"the shipped placeholder tenant", &config.Config{BaseURL: "https://nmm.contoso.com", TokenURL: "https://login.microsoftonline.com/YOUR_TENANT_ID/oauth2/v2.0/token"}, ""},
		{"the placeholder tenant in another case", &config.Config{BaseURL: "https://nmm.contoso.com", TokenURL: "https://login.microsoftonline.com/your_tenant_id/oauth2/v2.0/token"}, ""},
		{"blank token URL", &config.Config{BaseURL: "https://nmm.contoso.com", TokenURL: "   "}, ""},
		{"token URL with a user", &config.Config{BaseURL: "https://nmm.contoso.com", TokenURL: "https://u@login.microsoftonline.com/t/oauth2/v2.0/token"}, ""},
		{"token URL with a user and password", &config.Config{BaseURL: "https://nmm.contoso.com", TokenURL: "https://u:p@login.microsoftonline.com/t/oauth2/v2.0/token"}, ""},
		{"token URL with another scheme", &config.Config{BaseURL: "https://nmm.contoso.com", TokenURL: "file:///etc/passwd"}, ""},
		{"token URL with no host", &config.Config{BaseURL: "https://nmm.contoso.com", TokenURL: "/oauth2/v2.0/token"}, ""},
		{"token URL with no host name", &config.Config{BaseURL: "https://nmm.contoso.com", TokenURL: "https://:8443/t/oauth2/v2.0/token"}, ""},
		{"token URL that does not parse", &config.Config{BaseURL: "https://nmm.contoso.com", TokenURL: "https://login.microsoftonline.com:bad/t"}, ""},
		{"token URL with a non-ASCII host", &config.Config{BaseURL: "https://nmm.contoso.com", TokenURL: "https://login.microsoftonlіne.com/t/oauth2/v2.0/token"}, ""},
		{"scheme-less token URL", &config.Config{BaseURL: "https://nmm.contoso.com", TokenURL: "login.microsoftonline.com/t/oauth2/v2.0/token"}, ""},
		{"opaque token URL", &config.Config{BaseURL: "https://nmm.contoso.com", TokenURL: "https:login.microsoftonline.com/t"}, ""},
		{"the operator's Entra tenant", &config.Config{BaseURL: "https://nmm.contoso.com", TokenURL: entra}, entra},
		{"the operator's Entra tenant, scheme in capitals", &config.Config{BaseURL: "https://nmm.contoso.com", TokenURL: "HTTPS://login.microsoftonline.com/7c1e9a3f-2b4d-4e6a-8c1f-3a5b7d9e1f2a/oauth2/v2.0/token"}, entra},
		{"the token URL with no base URL", &config.Config{TokenURL: entra}, entra},
	} {
		if got := ResolveNerdioTokenURL(c.cfg); got != c.want {
			t.Errorf("%s: token URL %q, want %q", c.name, got, c.want)
		}
	}
}

// NERDIO_OAUTH_SCOPE is the scope, exactly as the operator set it (trimmed),
// or none: never api://<client id>/.default, the caller's own application
// with the AADSTS500011 prefix the press defaulted to.
func TestScopeIsTheOperatorsOnly(t *testing.T) {
	for _, c := range []struct{ env, want string }{
		{"", ""},
		{"   ", ""},
		{"4a7c1e2b-9d3f-4b6a-8c1e-2f5a7d9b3c6e/.default", "4a7c1e2b-9d3f-4b6a-8c1e-2f5a7d9b3c6e/.default"},
		{"  4a7c1e2b-9d3f-4b6a-8c1e-2f5a7d9b3c6e/.default\n", "4a7c1e2b-9d3f-4b6a-8c1e-2f5a7d9b3c6e/.default"},
	} {
		t.Setenv("NERDIO_OAUTH_SCOPE", c.env)
		if got := ResolveNerdioScope(); got != c.want {
			t.Errorf("NERDIO_OAUTH_SCOPE=%q: scope %q, want %q", c.env, got, c.want)
		}
		if got := resolveClientCredentialsScope("scope-client-9e2b"); got != c.want || strings.Contains(got, "api://") {
			t.Errorf("NERDIO_OAUTH_SCOPE=%q: the mint asks for %q, want %q", c.env, got, c.want)
		}
	}
}

// A configuration file other users can change may not choose where the
// client secret is posted, though it names a token_url: Load refuses its
// credential route, and the token URL follows that refusal (auth login, the
// mint and the refresh all resolve through ResolveNerdioTokenURL). The scope
// is set, so the refusal is the token URL's.
func TestR1UntrustedConfigNeverChoosesTheTokenHost(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no group or other permission bits, so no config file reads as loose")
	}
	dir := isolateCredentialStore(t)
	path := filepath.Join(dir, "loose", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	loose := "base_url = \"https://collector.example.net\"\ntoken_url = \"https://collector.example.net/t/oauth2/v2.0/token\"\n"
	if err := os.WriteFile(path, []byte(loose), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o666); err != nil { // other users can change it
		t.Fatal(err)
	}
	t.Setenv("NERDIO_CLIENT_ID", "looseClientId-5d1e")
	t.Setenv("NERDIO_CLIENT_SECRET", "looseClientSecret-8a3c")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.CredentialRouteBlocked() {
		t.Skip("this platform does not report loose permissions")
	}
	if got := ResolveNerdioTokenURL(cfg); got != "" {
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
