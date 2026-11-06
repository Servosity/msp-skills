// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
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

	"threecx-pp-cli/internal/client"
	"threecx-pp-cli/internal/cliutil"
	"threecx-pp-cli/internal/credkit"
	"threecx-pp-cli/internal/credwire"
)

// No in-process test runs credkit.Start, so the outbound guard would refuse every
// credentialed request, and a value held here would stay held for the rest of the
// test process. internal/threecxblackbox proves both on the built binaries.
func init() {
	client.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	cliutil.Hold = func(credkit.Source, ...string) {}
}

// api-origin-pinned: a request or redirect to another host never reaches it.
func TestOriginPinRefusesAnotherHost(t *testing.T) {
	var hits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	t.Cleanup(other.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/connect/token", http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	hc := &http.Client{Transport: originPinTripper{origin: normalizedOrigin(u), next: http.DefaultTransport}}
	if _, err := hc.Get(srv.URL + "/xapi/v1/SystemStatus"); err == nil || !strings.Contains(err.Error(), "leave the configured 3CX phone system") {
		t.Fatalf("want the pin refusal, got %v", err)
	}
	if _, err := hc.Get(other.URL + "/connect/token"); err == nil {
		t.Fatal("a direct request to another host was sent")
	}
	if hits.Load() != 0 {
		t.Fatalf("the other host was contacted %d time(s)", hits.Load())
	}
}

func TestNormalizedOriginRefusesNonASCIIAndFillsPorts(t *testing.T) {
	for raw, want := range map[string]string{
		"https://PBX.Example.com/xapi/v1": "https://pbx.example.com:443",
		"https://acme.3cx.us:5001/x":      "https://acme.3cx.us:5001",
		"http://10.0.0.5/xapi/v1":         "http://10.0.0.5:80",
		"https://İ.example.com/xapi/v1":   "",
	} {
		u, _ := url.Parse(raw)
		if got := normalizedOrigin(u); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
	if got := pinnedOrigin("https://{fqdn}/xapi/v1", map[string]string{"fqdn": "acme.3cx.us:5001"}); got != "https://acme.3cx.us:5001" {
		t.Errorf("template origin: %q", got)
	}
	if got := pinnedOrigin("https://{fqdn}/xapi/v1", nil); got != "" {
		t.Errorf("unresolved template must pin nothing, got %q", got)
	}
}

// response-credentials-held: a credential a response carries is held before
// any command reads the body, at any depth, by name or by 3CX's Concealed
// shape; masks, PINs and plain OData property values are not, and a minting
// function's unnamed value is. A listing has those members replaced, so it stays
// readable once the value is held; a token exchange is left whole for the client.
func TestResponseCredentialsHeld(t *testing.T) {
	var held []string
	hold := cliutil.Hold
	cliutil.Hold = func(_ credkit.Source, values ...string) { held = append(held, values...) }
	t.Cleanup(func() { cliutil.Hold = hold })
	const record = `{"access_token":"mintedTokUnitA1b2c3d4e5","nested":{"AuthPassword":{"Concealed":true,"Value":"concealedUnit-F6g7h8i9"}},"VMPIN":"48213","AuthPassword2":"**********","FirstName":"getNameNotSecretK2m4"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "Pbx.GenerateApiKey"):
			_, _ = w.Write([]byte(`{"value":"mintedKeyUnit-Q7w9e1r3"}`))
		case strings.Contains(r.URL.Path, "Pbx.GetVersionType"):
			_, _ = w.Write([]byte(`{"value":"plainVersionUnit-20.0.10"}`))
		default:
			_, _ = w.Write([]byte(record))
		}
	}))
	t.Cleanup(srv.Close)
	rt := responseHoldTripper{next: http.DefaultTransport}
	get := func(method, p, contentType string) string {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+p, nil)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		if len(body) == 0 {
			t.Fatalf("%s: body not handed on", p)
		}
		return string(body)
	}
	listing := get(http.MethodGet, "/xapi/v1/Users", "")
	for _, v := range []string{"mintedTokUnitA1b2c3d4e5", "concealedUnit-F6g7h8i9", "48213"} {
		if strings.Contains(listing, v) {
			t.Errorf("listing still carries %s: %s", v, listing)
		}
	}
	if !strings.Contains(listing, "getNameNotSecretK2m4") {
		t.Errorf("listing lost its record: %s", listing)
	}
	if exchange := get(http.MethodPost, "/connect/token", "application/x-www-form-urlencoded"); exchange != record {
		t.Errorf("the token exchange was changed: %s", exchange)
	}
	get(http.MethodGet, "/xapi/v1/ConferenceSettings/Pbx.GenerateApiKey()", "")
	get(http.MethodGet, "/xapi/v1/SystemStatus/Pbx.GetVersionType()", "")
	for _, v := range []string{"mintedTokUnitA1b2c3d4e5", "concealedUnit-F6g7h8i9", "mintedKeyUnit-Q7w9e1r3"} {
		if !slices.Contains(held, v) {
			t.Errorf("%s not held", v)
		}
	}
	for _, v := range []string{"getNameNotSecretK2m4", "**********", "48213", "plainVersionUnit-20.0.10"} {
		if slices.Contains(held, v) {
			t.Errorf("%s held, but it is not a credential this program holds", v)
		}
	}
}

// Every string flag named like a credential is declared to the kit, and every
// declared flag exists: a flag a reprint adds fails here until it is judged.
// PINs are redacted by name, never held (credwire.Declarations says why).
func TestDeclaredCredentialFlagsMatchTheCommands(t *testing.T) {
	looks := regexp.MustCompile(`(^|-)(password|secret|access-token|access-key|verifier|provision-link|provision-url|prov-link|vmpin|pin-number|voicemail-pin|api-key|license-key|private-key|key-grok)(-|$)`)
	declared := credwire.Declarations.CredentialFlags
	seen := map[string]bool{}
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		visit := func(f *pflag.Flag) {
			if f.Value.Type() != "string" {
				return
			}
			seen["--"+f.Name] = true
			if !strings.HasSuffix(f.Name, "-name") && looks.MatchString(f.Name) && !slices.Contains(declared, "--"+f.Name) {
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
