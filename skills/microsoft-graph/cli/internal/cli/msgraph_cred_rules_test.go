// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"microsoft-graph-pp-cli/internal/client"
	"microsoft-graph-pp-cli/internal/cliutil"
	"microsoft-graph-pp-cli/internal/cliutil/testenv"
	"microsoft-graph-pp-cli/internal/credkit"
)

// heldNow reports whether the credential kit withholds text from a file write.
func heldNow(t *testing.T, text string) bool {
	t.Helper()
	err := credkit.WriteFile(filepath.Join(t.TempDir(), "probe"), []byte(text), 0o600)
	if err != nil && !errors.Is(err, credkit.ErrWithheld) {
		t.Fatalf("probe write: %v", err)
	}
	return errors.Is(err, credkit.ErrWithheld)
}

// api-origin-pinned: a redirect to another host is refused before it is
// followed, an @odata.nextLink naming another host is fetched from the
// configured host (pull keeps only its path and query), and the bearer token
// never reaches the other host; the configured host still answers.
func TestOriginPinRefusesRedirectToAnotherHost(t *testing.T) {
	var hits, apiHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"value":[]}`))
	}))
	t.Cleanup(other.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiHits.Add(1)
		if r.Header.Get("Authorization") != "Bearer pinUnitToken-4c1e7a9b" {
			t.Errorf("the configured host got Authorization %q", r.Header.Get("Authorization"))
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/redirect"):
			http.Redirect(w, r, other.URL+"/v1.0/users", http.StatusTemporaryRedirect)
		case r.URL.Query().Get("$skiptoken") != "":
			_, _ = w.Write([]byte(`{"value":[{"id":"u2"}]}`))
		default:
			_, _ = w.Write([]byte(`{"value":[{"id":"u1"}],"@odata.nextLink":"` + other.URL + `/v1.0/users?$skiptoken=page2-7d1c9e"}`))
		}
	}))
	t.Cleanup(srv.Close)
	testenv.Isolate(t)
	t.Setenv("MICROSOFT_GRAPH_BASE_URL", srv.URL+"/v1.0")
	t.Setenv("MICROSOFT_GRAPH_TOKEN", "pinUnitToken-4c1e7a9b")
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get(context.Background(), "/redirect", nil)
	if err == nil || !strings.Contains(err.Error(), "leave the configured Microsoft Graph API host") {
		t.Fatalf("redirect: want the pin refusal, got %v", err)
	}
	items, err := graphGetAllPages(context.Background(), c, "/users", nil)
	if err != nil || len(items) != 2 {
		t.Fatalf("nextLink: want 2 items from the configured host, got %d (%v)", len(items), err)
	}
	if hits.Load() != 0 {
		t.Fatalf("the other host was contacted %d time(s)", hits.Load())
	}
	if apiHits.Load() == 0 {
		t.Fatal("the configured host was never asked")
	}
	if !heldNow(t, "x pinUnitToken-4c1e7a9b x") {
		t.Fatal("the configured token is not held")
	}
}

// The pin is the outermost transport and sends through the credential kit's
// outbound guard (client.OutboundGuard, credkit.Transport in the built
// programs: TestOutboundGuardIsTheKit), the response cache is off, and a
// second hook run does not wrap the chain again.
func TestGuardWrapsTheTransport(t *testing.T) {
	saved := client.OutboundGuard
	t.Cleanup(func() { client.OutboundGuard = saved })
	client.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return kitMarker{rt} }
	c := &client.Client{BaseURL: "https://graph.microsoft.com/v1.0", HTTPClient: &http.Client{}}
	guardClient(c)
	pin, ok := c.HTTPClient.Transport.(originPinTripper)
	if !ok {
		t.Fatalf("the origin pin is not outermost: %T", c.HTTPClient.Transport)
	}
	if m, ok := pin.next.(kitMarker); !ok || m.next != http.DefaultTransport {
		t.Fatalf("the pin sends through %T, not the outbound guard around the client's transport", pin.next)
	}
	if !c.NoCache {
		t.Fatal("the response cache is on")
	}
	guardClient(c)
	if again, ok := c.HTTPClient.Transport.(originPinTripper); !ok || again.next != pin.next {
		t.Fatal("a second guard run re-wrapped the transport")
	}
}

type kitMarker struct{ next http.RoundTripper }

func (k kitMarker) RoundTrip(r *http.Request) (*http.Response, error) { return k.next.RoundTrip(r) }

func TestNormalizedOriginRefusesNonASCIIAndFillsPorts(t *testing.T) {
	for raw, want := range map[string]string{
		"https://Graph.Microsoft.com/v1.0":   "https://graph.microsoft.com:443",
		"https://graph.microsoft.com:8443/x": "https://graph.microsoft.com:8443",
		"http://127.0.0.1/x":                 "http://127.0.0.1:80",
		"https://graph.mİcrosoft.com/v1.0/x": "",
	} {
		u, _ := url.Parse(raw)
		if got := normalizedOrigin(u); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
	got := pinnedOrigins("https://graph.microsoft.com/v1.0", "://bad")
	if len(got) != 1 || !got["https://graph.microsoft.com:443"] {
		t.Errorf("pinned origins: %v", got)
	}
}

// api-origin-pinned for 'auth login': the client secret is posted to the token
// endpoint only; a redirect is refused and never followed, and the minted
// token is held before anything prints.
func TestAuthLoginMintIsPinnedAndHeld(t *testing.T) {
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
	}))
	t.Cleanup(other.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/redirect/") {
			http.Redirect(w, r, other.URL+"/steal", http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"mintedGraphTokenUnit-91c2d3e4","expires_in":3599,"token_type":"Bearer"}`))
	}))
	t.Cleanup(srv.Close)
	saved := entraLoginURLFormat
	t.Cleanup(func() { entraLoginURLFormat = saved })

	entraLoginURLFormat = srv.URL + "/redirect/%s/oauth2/v2.0/token"
	_, _, err := mintClientCredentialsToken(context.Background(), 10*time.Second, "tenant-a", "app-id", "mintSecretUnit-4b8d2f6a", "https://graph.microsoft.com/.default")
	if !errors.Is(err, errTokenRedirectRefused) {
		t.Fatalf("redirect: want errTokenRedirectRefused, got %v", err)
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the redirect target was contacted %d time(s)", otherHits.Load())
	}

	entraLoginURLFormat = srv.URL + "/%s/oauth2/v2.0/token"
	tok, _, err := mintClientCredentialsToken(context.Background(), 10*time.Second, "tenant-a", "app-id", "mintSecretUnit-4b8d2f6a", "https://graph.microsoft.com/.default")
	if err != nil || tok != "mintedGraphTokenUnit-91c2d3e4" {
		t.Fatalf("mint: got %q, %v", tok, err)
	}
	if !heldNow(t, "x mintedGraphTokenUnit-91c2d3e4 y") {
		t.Error("the minted token is not held")
	}
}

// credkit-wiring for 'auth login': the token request leaves through the
// credential kit, so a client secret the kit holds never goes over plain http
// (an httptest server here; in the built program, any host but
// login.microsoftonline.com, internal/credwire Origins). Without the kit's
// transport the server below receives the secret.
func TestAuthLoginSendsThroughTheKit(t *testing.T) {
	const secret = "kitMint" + "Secret-6e2a9c4b"
	var leaked atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), secret) {
			leaked.Add(1)
		}
		_, _ = w.Write([]byte(`{"access_token":"kitMintToken-3f8b1d7c","expires_in":3599}`))
	}))
	t.Cleanup(srv.Close)
	saved := entraLoginURLFormat
	t.Cleanup(func() { entraLoginURLFormat = saved })
	entraLoginURLFormat = srv.URL + "/%s/oauth2/v2.0/token"
	credkit.Hold("flag:--client-secret", secret)
	_, _, err := mintClientCredentialsToken(context.Background(), 10*time.Second, "tenant-a", "app-id", secret, "https://graph.microsoft.com/.default")
	if err == nil {
		t.Fatal("the mint succeeded: the held client secret went over plain http")
	}
	if leaked.Load() != 0 {
		t.Fatalf("the server received the held client secret %d time(s)", leaked.Load())
	}
}

// credkit-wiring: the startup hold of a run-profile store and the CLI's own
// refusal of a credential-named profile flag use one rule.
func TestCredentialProfileFlagMatchesTheCLI(t *testing.T) {
	names := []string{"client-secret", "client_secret", "API-Token", "password", "x-api-key", "apikey", "authorization",
		"cookie", "private-key", "credential-file", "select", "json", "base-url", "client-id", "top", "tenant"}
	for _, name := range names {
		if got, want := cliutil.CredentialProfileFlag(name), credentialLikeRunProfileFlag(name); got != want {
			t.Errorf("%q: cliutil.CredentialProfileFlag %v, credentialLikeRunProfileFlag %v", name, got, want)
		}
	}
}
