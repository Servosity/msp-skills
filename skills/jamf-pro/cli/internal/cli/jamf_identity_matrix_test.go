// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"jamf-pro-pp-cli/internal/client"
	"jamf-pro-pp-cli/internal/cliutil"
	"jamf-pro-pp-cli/internal/config"
	"jamf-pro-pp-cli/internal/fleet"
	"jamf-pro-pp-cli/internal/jamfauth"
)

// fakeJamf stands in for every Jamf Pro server at once: it answers by host,
// mints "tok|<host>|<client>" and records what each request carried.
type fakeJamf struct {
	mu   sync.Mutex
	recs []wireRec
}

type wireRec struct{ host, path, bearer, mintClient, mintSecret string }

func (f *fakeJamf) RoundTrip(r *http.Request) (*http.Response, error) {
	rec := wireRec{host: r.URL.Host, path: r.URL.Path}
	if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") {
		rec.bearer = strings.TrimPrefix(a, "Bearer ")
	}
	body := `{"totalCount":1,"results":[{"id":"1","name":"s","general":{"name":"m"}}]}`
	if r.URL.Path == "/api/v1/oauth/token" {
		raw, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(raw))
		rec.mintClient, rec.mintSecret = form.Get("client_id"), form.Get("client_secret")
		body = fmt.Sprintf(`{"access_token":"tok|%s|%s","expires_in":1200}`, r.URL.Host, rec.mintClient)
	}
	f.mu.Lock()
	f.recs = append(f.recs, rec)
	f.mu.Unlock()
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
}

func (f *fakeJamf) since(n int) []wireRec {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]wireRec(nil), f.recs[n:]...)
}

// activeJamf, when set, replaces the network under every client's
// identity-checked transport (the transport itself stays in place).
var activeJamf *fakeJamf

func init() {
	registerClientHook(func(c *client.Client) error {
		if activeJamf == nil || c.HTTPClient == nil {
			return nil
		}
		if t, ok := c.HTTPClient.Transport.(jamfTransport); ok {
			t.next = activeJamf
			c.HTTPClient.Transport = t
		} else {
			c.HTTPClient.Transport = activeJamf
		}
		return nil
	})
}

// phase is what one process legitimately acts as.
type phase struct {
	host, client, secret string
	pasted               string // token-only: token saved with auth set-token
	instance             string
	tokenURL             string // a token endpoint override
}

func (p phase) env(t *testing.T) {
	t.Helper()
	if p.instance != "" {
		t.Setenv(fleet.EntriesEnv, "acme=s-acme,globex=s-globex")
		return
	}
	t.Setenv("JAMF_PRO_SERVER", p.host)
	t.Setenv("JAMF_PRO_CLIENT_ID", p.client)
	t.Setenv("JAMF_PRO_CLIENT_SECRET", p.secret)
	if p.tokenURL != "" {
		t.Setenv("JAMF_PRO_CONFIG", writeTokenURLConfig(t, "", p.tokenURL))
	}
}

func (p phase) args(a ...string) []string {
	if p.instance == "" {
		return a
	}
	return append([]string{"--instance", p.instance}, a...)
}

var matrixSwitches = map[string][2]phase{
	"server":     {{host: "a.jamf.test", client: "c1", secret: "s-a"}, {host: "b.jamf.test", client: "c1", secret: "s-b"}},
	"client-id":  {{host: "a.jamf.test", client: "c1", secret: "s-1"}, {host: "a.jamf.test", client: "c2", secret: "s-2"}},
	"instance":   {{host: "acme.jamf.test", client: "c-acme", secret: "s-acme", instance: "acme"}, {host: "globex.jamf.test", client: "c-globex", secret: "s-globex", instance: "globex"}},
	"token-only": {{host: "a.jamf.test", pasted: "pasted-A"}, {host: "b.jamf.test"}},
	// A token endpoint on another host would receive the new client's secret.
	"token-url": {{host: "a.jamf.test", client: "c1", secret: "s-1"}, {host: "a.jamf.test", client: "c2", secret: "s-2", tokenURL: "https://evil.jamf.test/api/v1/oauth/token"}},
}

// entry runs one entry point as phase p (other is the phase on the other
// side of the switch). refused reports that the entry must not send anything.
type entry struct {
	run     func(t *testing.T, p, other phase, second bool) error
	refused func(p phase, second bool) bool
	// noCredential reports a phase the entry leaves without any usable
	// credential (it may not authenticate, and must not borrow one).
	noCredential func(p, other phase, second bool) bool
}

var listArgs = []string{"computer-inventory", "list", "--json", "--data-source", "live", "--no-cache"}

var matrixEntries = map[string]entry{
	"auto-mint": {run: func(t *testing.T, p, _ phase, _ bool) error {
		_, err := runCLI(t, p.args(listArgs...)...)
		return err
	}},
	"auth-login": {
		// First process logs in with flags (the pair is saved) and uses it.
		// After a server switch the second process has no env pair, so only
		// the cached one could reach the new server; after a client switch
		// on the same server the new env pair is the switch.
		run: func(t *testing.T, p, other phase, second bool) error {
			if !second && p.client != "" {
				_, _ = runCLI(t, p.args("auth", "login", "--client-id", p.client, "--client-secret", p.secret)...)
				t.Setenv("JAMF_PRO_CLIENT_ID", "")
				t.Setenv("JAMF_PRO_CLIENT_SECRET", "")
			}
			if second && p.instance == "" && p.host != other.host {
				t.Setenv("JAMF_PRO_CLIENT_ID", "")
				t.Setenv("JAMF_PRO_CLIENT_SECRET", "")
			}
			_, err := runCLI(t, p.args(listArgs...)...)
			return err
		},
		noCredential: func(p, other phase, second bool) bool { return second && p.instance == "" && p.host != other.host },
	},
	"sync": {run: func(t *testing.T, p, _ phase, _ bool) error {
		_, err := runCLI(t, p.args("sync", "--resources", "computer_inventory", "--json")...)
		return err
	}},
	"mcp-startup": {run: func(t *testing.T, p, _ phase, _ bool) error {
		if err := mcpStart(t, p); err != nil {
			return err
		}
		cfg, err := config.Load("")
		if err != nil {
			return err
		}
		c := client.New(cfg, 5*time.Second, 0)
		c.NoCache = true
		if err := ApplyClientHooks(c); err != nil {
			return err
		}
		_, err = c.Get(context.Background(), "/api/v1/scripts", nil)
		return err
	}},
	"mcp-retarget": {
		// An MCP tool input that fills {server} after the client was built
		// (no sync guard on this path): the second process aims at another host.
		run: func(t *testing.T, p, other phase, second bool) error {
			if err := mcpStart(t, p); err != nil {
				return err
			}
			cfg, err := config.Load("")
			if err != nil {
				return err
			}
			c := client.New(cfg, 5*time.Second, 0)
			c.NoCache = true
			if err := ApplyClientHooks(c); err != nil {
				return err
			}
			if second {
				target := other.host
				if target == p.host {
					target = "evil.jamf.test"
				}
				if cfg.TemplateVars == nil {
					cfg.TemplateVars = map[string]string{}
				}
				c.BaseURL, cfg.BaseURL = "https://{server}", "https://{server}"
				cfg.TokenURL = "https://{server}/api/v1/oauth/token"
				cfg.TemplateVars["server"] = target
			}
			_, err = c.Get(context.Background(), "/api/v1/scripts", nil)
			return err
		},
		refused: func(_ phase, second bool) bool { return second },
	},
	"fleet-exec": {
		// The child primitive, started the way the fleet runner starts it:
		// the parent's identity handed down whole.
		run: func(t *testing.T, p, _ phase, _ bool) error {
			if p.instance != "" {
				id, err := jamfauth.ForInstance(p.instance, p.host, p.client)
				if err != nil {
					return err
				}
				t.Setenv(fleet.SelectEnv, p.instance)
				t.Setenv(jamfauth.IdentityEnv, id.Encode())
			}
			_, err := runCLI(t, "fleet", "exec", "--raw", "--req", fleet.Request{Path: "/api/v1/scripts"}.Encode(), "--json", "--no-input")
			return err
		},
		refused: func(p phase, _ bool) bool { return p.instance == "" }, // the raw child needs a pinned instance
	},
	"path-context": {
		// The second process tries to aim its client at the first one's server.
		run: func(t *testing.T, p, other phase, second bool) error {
			a := p.args("sync", "--resources", "computer_inventory", "--json")
			if second {
				a = append(a, "--path-context", "server="+other.host)
			}
			_, err := runCLI(t, a...)
			return err
		},
		refused: func(_ phase, second bool) bool { return second },
	},
}

// TestIdentityMatrix walks every entry point through every identity switch
// and asserts that nothing one identity owns (token, pasted token, client
// secret, local store) is ever used by the other.
func TestIdentityMatrix(t *testing.T) {
	for sw, ph := range matrixSwitches {
		for name, e := range matrixEntries {
			t.Run(sw+"/"+name, func(t *testing.T) { runMatrixCell(t, ph[0], ph[1], e) })
		}
	}
}

func runMatrixCell(t *testing.T, a, b phase, e entry) {
	dir := setupFleet(t, nil)
	home := t.TempDir()
	reg := &fleet.Registry{}
	for _, n := range []string{"acme", "globex"} {
		reg.Upsert(fleet.Instance{Name: n, Server: n + ".jamf.test", ClientID: "c-" + n})
	}
	if err := reg.Save(); err != nil {
		t.Fatal(err)
	}
	net := &fakeJamf{}
	activeJamf = net
	jamfLoginNet = net // 'auth login' mints through it
	t.Cleanup(func() {
		activeJamf = nil
		jamfLoginNet = nil
		_, _ = cliutil.SetHomeOverride("")
	})
	base := os.Environ()
	// Each phase is a fresh process: environment, pin and home reset.
	start := func(p phase) {
		os.Clearenv()
		for _, kv := range base {
			k, v, _ := strings.Cut(kv, "=")
			_ = os.Setenv(k, v)
		}
		t.Setenv("JAMF_PRO_HOME", home)
		t.Setenv(fleet.DirEnv, dir)
		for _, k := range []string{"JAMF_PRO_BASE_URL", "JAMF_PRO_TOKEN_URL", "JAMF_PRO_CONFIG", fleet.SelectEnv, jamfauth.IdentityEnv} {
			t.Setenv(k, "")
			_ = os.Unsetenv(k)
		}
		pinnedInstance, instanceSelection = "", ""
		jamfauth.ResetForTest()
		fleet.ReleaseHeldForTest()
		_, _ = cliutil.SetHomeOverride("")
		p.env(t)
		if p.pasted != "" {
			cmd := RootCmd()
			cmd.SetArgs([]string{"auth", "set-token"})
			cmd.SetIn(bytes.NewReader([]byte(p.pasted + "\n")))
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("set-token: %v", err)
			}
		}
	}
	// A refused phase must fail and send nothing. A token URL off the server
	// refuses every mint, so that switch's second phase is refused too.
	refused := func(p phase, second bool) bool {
		return (e.refused != nil && e.refused(p, second)) || p.tokenURL != ""
	}
	// A phase that holds a credential and is not refused must succeed and
	// reach its own server with its own credential: an entry that broke
	// after the switch would otherwise pass with nothing to inspect.
	check := func(p, other phase, second bool, err error, recs []wireRec) {
		t.Helper()
		usable := (p.client != "" || p.pasted != "" || p.instance != "") && !(e.noCredential != nil && e.noCredential(p, other, second))
		switch {
		case refused(p, second) && err == nil:
			t.Fatalf("phase %s should have been refused but succeeded", p.host)
		case !refused(p, second) && usable:
			if err != nil {
				t.Fatalf("phase %s failed: %v", p.host, err)
			}
			if !sawBearer(recs, p) {
				t.Fatalf("phase %s never reached its own server with its own credential: %+v", p.host, recs)
			}
		}
	}

	start(a)
	errA := e.run(t, a, b, false)
	storeA, _ := MCPStorePath()
	check(a, b, false, errA, net.since(0))

	mark := len(net.since(0))
	start(b)
	errB := e.run(t, b, a, true)
	storeB, _ := MCPStorePath()
	check(b, a, true, errB, net.since(mark))
	for _, r := range net.since(mark) {
		if refused(b, true) {
			t.Fatalf("a refused entry still sent %+v", r)
		}
		if r.host != b.host {
			t.Fatalf("after the switch a request went to %s (acting as %s): %+v", r.host, b.host, r)
		}
		if r.bearer != "" && r.bearer != "tok|"+b.host+"|"+b.client && r.bearer != b.pasted {
			t.Fatalf("after the switch %s received a credential it does not own: %q", r.host, r.bearer)
		}
		if r.path == "/api/v1/oauth/token" && (r.mintClient != b.client || r.mintSecret != b.secret) {
			t.Fatalf("after the switch a mint at %s carried client %q secret %q", r.host, r.mintClient, r.mintSecret)
		}
	}
	if storeA == storeB {
		t.Fatalf("both identities opened the same local store %s", storeA)
	}
}

// sawBearer reports whether p's own server received p's own credential.
func sawBearer(recs []wireRec, p phase) bool {
	for _, r := range recs {
		if r.host == p.host && r.path != "/api/v1/oauth/token" && (r.bearer == "tok|"+p.host+"|"+p.client || (p.pasted != "" && r.bearer == p.pasted)) {
			return true
		}
	}
	return false
}

// mcpStart runs the MCP server's startup for phase p, always, as the real
// server does, and checks what it promises: one identity established and
// exported for mirrored commands, and a client for any other client id
// refused within the same execution.
func mcpStart(t *testing.T, p phase) error {
	t.Helper()
	if p.instance != "" {
		t.Setenv(fleet.SelectEnv, p.instance)
	}
	if err := EstablishMCPIdentity(); err != nil {
		return err
	}
	est, ok := jamfauth.Established()
	if !ok {
		t.Fatalf("MCP startup for %s established no identity", p.host)
	}
	exported, err := jamfauth.Decode(os.Getenv(jamfauth.IdentityEnv))
	if err != nil || exported != est {
		t.Fatalf("MCP startup must export its identity: %+v vs %+v (%v)", exported, est, err)
	}
	if est.Host() != p.host {
		t.Fatalf("MCP startup acts as %s, want %s", est.Server(), p.host)
	}
	// A configuration that now names another client is refused.
	prevID, prevSecret := os.Getenv("JAMF_PRO_CLIENT_ID"), os.Getenv("JAMF_PRO_CLIENT_SECRET")
	_ = os.Setenv("JAMF_PRO_CLIENT_ID", "switched-client")
	_ = os.Setenv("JAMF_PRO_CLIENT_SECRET", "switched-secret")
	cfg, lerr := config.Load("")
	if lerr == nil {
		if ApplyClientHooks(client.New(cfg, time.Second, 0)) == nil {
			t.Fatalf("an MCP execution acting as %q must refuse a client for another client id", est.ClientID())
		}
	}
	_ = os.Setenv("JAMF_PRO_CLIENT_ID", prevID)
	_ = os.Setenv("JAMF_PRO_CLIENT_SECRET", prevSecret)
	return nil
}
