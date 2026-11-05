// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"compress/zlib"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"cisco-umbrella-pp-cli/internal/client"
	"cisco-umbrella-pp-cli/internal/cliutil"
	"cisco-umbrella-pp-cli/internal/cliutil/testenv"
	"cisco-umbrella-pp-cli/internal/config"
	"cisco-umbrella-pp-cli/internal/credkit"
	"cisco-umbrella-pp-cli/internal/credwire"
)

// No in-process test runs credkit.Start, so the outbound guard would refuse every
// credentialed request, and a value held here would stay held for the rest of the
// test process. internal/umbrellablackbox proves both on the built binaries.
func init() {
	client.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	cliutil.Hold = func(credkit.Source, ...string) {}
}

// recordHolds swaps the hold seam for a recorder for one test. Tests that use it
// do not run in parallel.
func recordHolds(t *testing.T) *[]string {
	t.Helper()
	var got []string
	prior := cliutil.Hold
	cliutil.Hold = func(_ credkit.Source, values ...string) { got = append(got, values...) }
	t.Cleanup(func() { cliutil.Hold = prior })
	return &got
}

// api-origin-pinned: a redirect to another host never reaches it, and no
// credential is sent there; the configured API and token hosts still work.
func TestOriginPinRefusesRedirectToAnotherHost(t *testing.T) {
	var hits, tokenHits atomic.Int32
	held := recordHolds(t)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(other.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/v2/token" {
			tokenHits.Add(1)
			_, _ = w.Write([]byte(`{"access_token":"pinUnitMintedTok-4c1e","expires_in":3600}`))
			return
		}
		http.Redirect(w, r, other.URL+"/deployments/v2/networks", http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	testenv.Isolate(t)
	t.Setenv("CISCO_UMBRELLA_BASE_URL", srv.URL)
	t.Setenv("CISCO_UMBRELLA_TOKEN_URL", srv.URL+"/auth/v2/token")
	t.Setenv("CISCO_UMBRELLA_API_KEY", "pin-unit-key")
	t.Setenv("CISCO_UMBRELLA_API_SECRET", "pin-unit-secret")
	t.Setenv("CISCO_UMBRELLA_ORG_ID", "")
	flags := rootFlags{}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get(context.Background(), "/deployments/v2/networks", nil)
	if hits.Load() != 0 {
		t.Fatalf("the redirect target was contacted %d time(s)", hits.Load())
	}
	if tokenHits.Load() != 1 {
		t.Fatalf("the configured token host was minted against %d time(s), want 1", tokenHits.Load())
	}
	if err == nil || !strings.Contains(err.Error(), "leave the configured Umbrella API host") {
		t.Fatalf("want the pin refusal, got %v", err)
	}
	if !slices.Contains(*held, "pinUnitMintedTok-4c1e") {
		t.Fatal("the minted token is not held")
	}
}

func TestNormalizedOriginRefusesNonASCIIAndFillsPorts(t *testing.T) {
	for raw, want := range map[string]string{
		"https://API.Umbrella.com/deployments": "https://api.umbrella.com:443",
		"https://api.umbrella.com:8443/x":      "https://api.umbrella.com:8443",
		"http://127.0.0.1/x":                   "http://127.0.0.1:80",
		"https://İ.umbrella.com/x":             "",
	} {
		u, _ := url.Parse(raw)
		if got := normalizedOrigin(u); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
	got := pinnedOrigins("https://api.umbrella.com", "https://api.umbrella.com/auth/v2/token", "://bad")
	if len(got) != 1 || !got["https://api.umbrella.com:443"] {
		t.Errorf("pinned origins: %v", got)
	}
}

// response-credentials-held: a credential a response carries is held before
// any command reads the body, deflate-encoded or not, at any depth; masks and
// names that are not credentials are not.
func TestResponseCredentialsHeldAndReplacedByName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var z bytes.Buffer
			zw := zlib.NewWriter(&z)
			_, _ = zw.Write([]byte(`{"key":{"id":"k1","name":"getNameNotSecretA7z1","clientId":"mintedKeyUnitDeflate77","clientSecret":"mintedSecretUnitDeflate88"}}`))
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "deflate")
			_, _ = w.Write(z.Bytes())
			return
		}
		_, _ = w.Write([]byte(`{"keyId":"AKIAUNIT","secretAccessKey":"s3SecretUnitF6g7h8i9j0","currentKeyId":"bucketKey` + `NotAwsShaped01","nested":{"psk":"tunnelPskUnitA1b2c3"},"password":"**********","name":"getNameNotSecretK2m4"}`))
	}))
	t.Cleanup(srv.Close)
	got := recordHolds(t)
	rt := responseHoldTripper{next: http.DefaultTransport}
	bodies := ""
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req, _ := http.NewRequest(method, srv.URL+"/admin/v2/apiKeys", nil)
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		bodies += string(body)
	}
	for _, v := range []string{"s3SecretUnitF6g7h8i9j0", "bucketKeyNotAwsShaped01", "tunnelPskUnitA1b2c3", "mintedKeyUnitDeflate77", "mintedSecretUnitDeflate88"} {
		if !slices.Contains(*got, v) {
			t.Errorf("%s not held: %q", v, *got)
		}
		if strings.Contains(bodies, v) {
			t.Errorf("%s not replaced by name:\n%s", v, bodies)
		}
	}
	for _, v := range []string{"getNameNotSecretK2m4", "getNameNotSecretA7z1", "**********", "AKIAUNIT", "k1"} {
		if slices.Contains(*got, v) {
			t.Errorf("%s held, but it is not a credential", v)
		}
	}
	for _, v := range []string{"getNameNotSecretK2m4", "getNameNotSecretA7z1", "[redacted]"} {
		if !strings.Contains(bodies, v) {
			t.Errorf("the body lost %s:\n%s", v, bodies)
		}
	}
}

// A token exchange is left whole: the client needs the token it just minted.
func TestTokenExchangeIsHeldAndLeftWhole(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"exchangeTokenUnit-9c1e","token_type":"bearer","expires_in":3600}`))
	}))
	t.Cleanup(srv.Close)
	got := recordHolds(t)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/auth/v2/token", strings.NewReader("grant_type=client_credentials"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := responseHoldTripper{next: http.DefaultTransport}.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "exchangeTokenUnit-9c1e") {
		t.Fatalf("the exchange lost its token: %s", body)
	}
	if !slices.Contains(*got, "exchangeTokenUnit-9c1e") {
		t.Errorf("the minted token was not held: %q", *got)
	}
	if slices.Contains(*got, "bearer") {
		t.Errorf("the word bearer was held: %q", *got)
	}
}

// Every credential a client is configured with is held when the client is built,
// the minted token and the built Authorization header among them.
func TestGuardHoldsEveryConfiguredCredential(t *testing.T) {
	got := recordHolds(t)
	holdConfiguredCredentials(&config.Config{CiscoUmbrellaApiKey: "cfgKeyUnit-1", CiscoUmbrellaApiSecret: "cfgSecretUnit-2", AccessToken: "cfgTokenUnit-3", AuthHeaderVal: "Bearer cfgHeaderUnit-4"})
	for _, v := range []string{"cfgKeyUnit-1", "cfgSecretUnit-2", "cfgTokenUnit-3", "Bearer cfgHeaderUnit-4"} {
		if !slices.Contains(*got, v) {
			t.Errorf("%s not held: %q", v, *got)
		}
	}
}

// The token client 'auth login' builds is pinned like the API client: a token URL
// naming the configured hosts works, a redirect off them never leaves.
func TestTokenClientIsPinned(t *testing.T) {
	var hits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	t.Cleanup(other.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/auth/v2/token", http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	hc := &http.Client{Transport: http.DefaultTransport}
	guardTokenClient(hc, &config.Config{BaseURL: srv.URL, TokenURL: srv.URL + "/auth/v2/token"})
	_, err := hc.Get(srv.URL + "/auth/v2/token")
	if err == nil || !strings.Contains(err.Error(), "leave the configured Umbrella API host") {
		t.Fatalf("want the pin refusal, got %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("the redirect target was contacted %d time(s)", hits.Load())
	}
	if _, err := hc.Get(other.URL + "/auth/v2/token"); err == nil || hits.Load() != 0 {
		t.Fatalf("a request to another host was not refused (err %v, hits %d)", err, hits.Load())
	}
}

// Class 3 and the flag half of class credential-field-not-recognized: every flag
// that looks like a credential is declared, and every declared flag exists. The
// JSON object flags (--client-authentication-parameters) are not: their members are
// held by name when the body is built, and a whole JSON value would hold "secret".
func TestDeclaredCredentialFlagsMatchTheCommands(t *testing.T) {
	looks := regexp.MustCompile(`(^|-)(password|passwd|secret|psk-secret|access-token|refresh-token|token|client-id|api-secret|private-key|passphrase)(-|$)`)
	declared := credwire.Declarations.CredentialFlags
	seen := map[string]bool{}
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		visit := func(f *pflag.Flag) {
			if f.Value.Type() != "string" {
				return
			}
			seen["--"+f.Name] = true
			if looks.MatchString(f.Name) && !slices.Contains(declared, "--"+f.Name) {
				t.Errorf("%s --%s looks like a credential and is not in credwire.Declarations.CredentialFlags", c.CommandPath(), f.Name)
			}
		}
		c.Flags().VisitAll(visit)
		c.PersistentFlags().VisitAll(visit)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(RootCmd())
	for _, f := range declared {
		if !seen[f] {
			t.Errorf("%s is declared but no command has it", f)
		}
	}
}

// Class 3: every nonempty value of a declared flag is held, however it is spelled
// and whatever characters it is made of: --flag value, --flag=value, a value that
// starts with a hyphen, one that is nothing but hyphens. An org id beside it is not.
func TestArgvCredentialsHeld(t *testing.T) {
	args := []string{"auth", "login", "--client-id", "argvKeyUnit-5d2e8a", "--client-secret", "-argvSecretUnit-7c1e9a", "--org", "1234567",
		"users", "create", "--password=argvPassUnit-3b5d7f", "--password", "-----", "--psk-secret=---", "--email", "argvmail@example.com"}
	var values []string
	for _, f := range credwire.Declarations.CredentialFlags {
		values = append(values, credkit.FlagValues(args, f)...)
	}
	for _, v := range []string{"argvKeyUnit-5d2e8a", "-argvSecretUnit-7c1e9a", "argvPassUnit-3b5d7f", "-----", "---"} {
		if !slices.Contains(values, v) {
			t.Errorf("%s not found on the command line: %q", v, values)
		}
	}
	for _, v := range []string{"1234567", "argvmail@example.com"} {
		if slices.Contains(values, v) {
			t.Errorf("%s held, but it is not a credential", v)
		}
	}
}
