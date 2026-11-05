// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"compress/zlib"
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

	"prtg-pp-cli/internal/client"
	"prtg-pp-cli/internal/cliutil"
	"prtg-pp-cli/internal/credkit"
	"prtg-pp-cli/internal/credwire"
)

// No in-process test runs credkit.Start, so the outbound guard would refuse every
// credentialed request, and a value held here would stay held for the rest of the
// test process. internal/prtgblackbox proves both on the built binaries.
func init() {
	client.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	cliutil.Hold = func(credkit.Source, ...string) {}
}

// recordHolds records what is held for one test.
func recordHolds(t *testing.T) *[]string {
	t.Helper()
	var held []string
	prior := cliutil.Hold
	cliutil.Hold = func(_ credkit.Source, values ...string) { held = append(held, values...) }
	t.Cleanup(func() { cliutil.Hold = prior })
	return &held
}

// api-origin-pinned: a redirect to another host never reaches it, and the
// core's key is never sent there.
func TestOriginPinRefusesRedirectToAnotherHost(t *testing.T) {
	var hits atomic.Int32
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"api":"x"}`))
	}))
	t.Cleanup(other.Close)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/api/v2/version", http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	pinCore(t, srv)
	bodies, err := runRaw(t, "--req", "/version")
	if err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 0 {
		t.Fatalf("the redirect target was contacted %d time(s)", hits.Load())
	}
	if len(bodies) != 1 || !strings.Contains(string(bodies[0]), "leave the configured PRTG core") {
		t.Fatalf("want the pin refusal, got %s", bodies)
	}
}

// api-origin-pinned: a same-origin redirect is refused too, so the
// credential the generated redirect hook re-attaches never follows it.
func TestOriginPinRefusesSameOriginRedirect(t *testing.T) {
	var captured atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/capture" {
			captured.Add(1)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		http.Redirect(w, r, "/capture", http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	pinCore(t, srv)
	bodies, err := runRaw(t, "--req", "/version")
	if err != nil {
		t.Fatal(err)
	}
	if captured.Load() != 0 {
		t.Fatalf("the same-origin redirect was followed %d time(s)", captured.Load())
	}
	if len(bodies) != 1 || !strings.Contains(string(bodies[0]), "redirect") {
		t.Fatalf("want the redirect refusal, got %s", bodies)
	}
}

func TestNormalizedOriginRefusesNonASCIIAndFillsPorts(t *testing.T) {
	for raw, want := range map[string]string{
		"https://PRTG.Example.com/api/v2": "https://prtg.example.com:443",
		"https://prtg.example.com:1616/x": "https://prtg.example.com:1616",
		"http://10.0.0.5/api/v2":          "http://10.0.0.5:80",
		"https://İ.example.com/api/v2":    "",
	} {
		u, _ := url.Parse(raw)
		if got := normalizedOrigin(u); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
	if got := pinnedOrigin("https://{server}/api/v2", map[string]string{"server": "core.acme.example:1616"}); got != "https://core.acme.example:1616" {
		t.Errorf("template origin: %q", got)
	}
	if got := pinnedOrigin("https://{server}/api/v2", nil); got != "" {
		t.Errorf("unresolved template must pin nothing, got %q", got)
	}
}

// response-credentials-held: a credential a response carries is held before
// any command reads the body, deflate-encoded or not, at any depth; PRTG's
// masks and short values under credential-like names are not.
func TestResponseCredentialsHeld(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `{"token":"mintedTokUnitA1b2c3d4e5","nested":{"license_key":"licKeyUnitF6g7h8i9j0"},"windowsloginpassword":"**********","token_type":"Bearer","name":"getNameNotSecretK2m4"}`
		if r.Method == http.MethodPost {
			var z bytes.Buffer
			zw := zlib.NewWriter(&z)
			_, _ = zw.Write([]byte(`{"id":"k1","key":"mintedKeyUnitDeflate77"}`))
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "deflate")
			_, _ = w.Write(z.Bytes())
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	held := recordHolds(t)
	rt := responseHoldTripper{next: http.DefaultTransport}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req, _ := http.NewRequest(method, srv.URL+"/api/v2/x", nil)
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		if body, _ := io.ReadAll(resp.Body); len(body) == 0 {
			t.Fatalf("%s: body not handed on", method)
		}
	}
	for _, v := range []string{"mintedTokUnitA1b2c3d4e5", "licKeyUnitF6g7h8i9j0", "mintedKeyUnitDeflate77"} {
		if !slices.Contains(*held, v) {
			t.Errorf("%s not held", v)
		}
	}
	for _, v := range []string{"getNameNotSecretK2m4", "**********"} {
		if slices.Contains(*held, v) {
			t.Errorf("%s held, but it is not a credential", v)
		}
	}
}

// state-changing-get-confirms: fleet exec and the raw child refuse a GET
// that changes state, in every spelling of its path.
func TestCleanAPIPathRefusesStateChangingGET(t *testing.T) {
	for _, p := range []string{"/session", "session", "/api/v2/session", "/Session", "/session/"} {
		if _, err := cleanAPIPath(p); err == nil || !strings.Contains(err.Error(), "changes PRTG state") {
			t.Errorf("%q: want refusal, got %v", p, err)
		}
	}
	if _, err := cleanAPIPath("/sensors"); err != nil {
		t.Errorf("/sensors refused: %v", err)
	}
}

// Every string flag named like a credential is declared to the kit, and every
// declared flag exists: a flag a reprint adds fails here until it is judged.
func TestDeclaredCredentialFlagsMatchTheCommands(t *testing.T) {
	looks := regexp.MustCompile(`(^|-)(password|passhash|secret|token|api-key|apikey|key)($|-)`)
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

// A mask is a vendor's way of hiding a setting in a response. What the operator
// supplies is never one: a password of eight asterisks, in a tool argument or a
// request body, is held like any other (codex round 1, operator-mask).
func TestOperatorValueShapedLikeAMaskIsHeld(t *testing.T) {
	held := recordHolds(t)
	args := map[string]any{"endpoint_id": "probes.update", "params": map[string]any{"id": "1",
		"windowsconnection": map[string]any{"windowsloginpassword": "********"}}}
	cliutil.HoldOperatorValues(args)
	cliutil.HoldRequestCredentials(map[string]string{"passhash": "[redacted]"}, []byte(`{"linuxloginpassword":"\u2022\u2022\u2022\u2022"}`))
	for _, want := range []string{"********", "[redacted]", "\u2022\u2022\u2022\u2022"} {
		if !slices.Contains(*held, want) {
			t.Errorf("operator value %q was not held; held %q", want, *held)
		}
	}
	*held = nil
	cliutil.HoldCredentialFields([]byte(`{"windowsloginpassword":"********","snmpauthpass":""}`))
	if len(*held) != 0 {
		t.Errorf("a response mask was held: %q", *held)
	}
}
