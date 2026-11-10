// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): the credential
// classes of this connector, proved against the built binaries now that the
// shared credential kit replaces its own layer. A run holds what it was given,
// and the fake repeats it under names that do not read as credentials, so only a
// value held from its real source can be withheld.

package credwire

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"datto-rmm-pp-cli/internal/store"
)

var heldValues = []string{heldKey, heldSecret, heldMinted, heldToken}

func noHeldValue(t *testing.T, where, text string) {
	t.Helper()
	for _, v := range heldValues {
		if strings.Contains(text, v) {
			t.Errorf("%s carries the held value %s:\n%.1500s", where, v, text)
		}
	}
}

// echoAll is a handler whose every answer repeats each credential a run can hold
// in a field named note.
func echoAll(w http.ResponseWriter, r *http.Request, _ seen) bool {
	if r.URL.Path == "/auth/oauth/token" {
		return false
	}
	note := strings.Join(heldValues, " ")
	w.Header().Set("Content-Type", "application/json")
	rec := fmt.Sprintf(`{"id":1042,"uid":"site-1042","name":"HQ-Record","note":%q}`, note)
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v2/account/") {
		_, _ = w.Write([]byte(dattoList("sites", rec)))
		return true
	}
	_, _ = w.Write([]byte(rec))
	return true
}

// TestHeldValuesNeverPrintOrLand: with the API answering with every credential
// the run holds, no output and no file the run writes carries one, on either
// credential mode. The credential store is the one exception: the minted token
// is saved there on purpose, and it is 0600.
func TestHeldValuesNeverPrintOrLand(t *testing.T) {
	t.Parallel()
	for name, mode := range map[string]struct {
		creds []string
		held  []string
	}{
		"minted": {keyPair, []string{heldKey, heldSecret, heldMinted}},
		"token":  {tokenEnv, []string{heldToken}},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, echoAll, mode.creds...)
			audit := filepath.Join(e.home, "audit")
			check := func(where, text string) {
				t.Helper()
				for _, v := range mode.held {
					if strings.Contains(text, v) {
						t.Errorf("%s carries the held value %s:\n%.1500s", where, v, text)
					}
				}
			}
			runs := [][]string{
				{"account", "get-sites", "--json"},
				{"account", "get-sites"},
				{"account", "get-sites", "--csv"},
				{"account", "get-sites", "--plain"},
				{"account", "get-sites", "--json", "--audit-dir", audit},
				{"account", "get-sites", "--json", "--deliver", "file:" + filepath.Join(e.home, "out", "delivered.json")},
				{"site", "get", "site-1042", "--json"},
				{"site", "update", "site-1042", "--name", "HQ", "--json"},
				{"sync", "--resources", "account", "--json"},
				{"export", "account", "--format", "jsonl", "--output", filepath.Join(e.home, "out", "export.jsonl")},
				{"export", "account", "--format", "json"},
				{"doctor"},
				{"auth", "status"},
			}
			for _, v := range mode.held[:1] {
				runs = append(runs,
					[]string{"account", "get-sites", "--" + v},
					[]string{"site", "get", v, "--json"},
					[]string{"feedback", "noted the token " + v + " in a ticket"},
					[]string{"teach", "--query", "find sites " + v, "--resource", "site-1042", "--resource-type", "account", "--notes", "rotated " + v, "--no-validate"},
				)
			}
			for _, args := range runs {
				r := e.run(t, "", args...)
				check(fmt.Sprintf("the output of %v", args), r.out)
			}
			if len(e.api.all()) == 0 {
				t.Fatal("no request reached the fake, so this test proves nothing")
			}
			_, out := e.mcp(t,
				map[string]any{"name": "datto-rmm_read", "arguments": map[string]any{"endpoint_id": "account.get-sites"}},
				map[string]any{"name": "sql", "arguments": map[string]any{"query": "SELECT data FROM resources"}},
				map[string]any{"name": "datto-rmm_read", "arguments": map[string]any{"endpoint_id": "site.get", "params": map[string]any{"siteUid": mode.held[0]}}},
			)
			check("the MCP session", out)
			for path, body := range e.files(t) {
				if strings.HasPrefix(filepath.Base(path), "credentials") || synccpuStoreFile(path) { // synccpu: the local store is not checked
					continue
				}
				check("file "+path, body)
			}
		})
	}
}

// TestNonAPIHostsNeverCarryAHeldValue: the feedback endpoint and a --deliver
// webhook are not the API host, so a URL or body repeating a held value is never
// sent to them. The clean controls show each sender works.
func TestNonAPIHostsNeverCarryAHeldValue(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var got strings.Builder
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		fmt.Fprintf(&got, "%s\n%v\n%s\n", r.RequestURI, r.Header, b)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(sink.Close)
	seenBySink := func() string {
		mu.Lock()
		defer mu.Unlock()
		return got.String()
	}
	e := newEnv(t, echoAll, tokenEnv...)
	base := e.vars
	for _, ep := range []string{sink.URL + "/collect?token=" + heldToken, sink.URL + "/hooks/" + heldToken} {
		e.vars = append(append([]string{}, base...), "DATTO_RMM_FEEDBACK_ENDPOINT="+ep)
		noHeldValue(t, "feedback --send", e.run(t, "", "feedback", "ok", "--send").out)
	}
	e.vars = append(append([]string{}, base...), "DATTO_RMM_FEEDBACK_ENDPOINT="+sink.URL+"/clean-text")
	e.run(t, "", "feedback", "the token "+heldToken+" was in a ticket", "--send")
	e.vars = base
	e.run(t, "", "account", "get-sites", "--json", "--deliver", "webhook:"+sink.URL+"/deliver?key="+heldToken)
	e.run(t, "", "account", "get-sites", "--json", "--deliver", "webhook:"+sink.URL+"/deliver/"+heldToken)
	// echoAll's records carry the token, so this body must not go either.
	e.run(t, "", "account", "get-sites", "--json", "--deliver", "webhook:"+sink.URL+"/deliver-echo")
	if text := seenBySink(); strings.Contains(text, heldToken) {
		t.Fatalf("a non-API host received the token:\n%s", text)
	}

	clean := newEnv(t, nil, append([]string{"DATTO_RMM_FEEDBACK_ENDPOINT=" + sink.URL + "/collect"}, tokenEnv...)...)
	clean.run(t, "", "feedback", "clean", "--send")
	clean.run(t, "", "account", "get-sites", "--json", "--deliver", "webhook:"+sink.URL+"/deliver-clean")
	text := seenBySink()
	if !strings.Contains(text, "/collect\n") || !strings.Contains(text, "/deliver-clean\n") {
		t.Fatalf("a clean feedback or webhook endpoint received nothing, so this test proves nothing:\n%s", text)
	}
	if strings.Contains(text, heldToken) {
		t.Fatalf("a clean endpoint received the token:\n%s", text)
	}
}

// TestMCPChildNeverReceivesAHeldValue: teach is a mirrored tool whose CLI child
// writes notes to the local store. The server holds the token it minted, which the
// child does not (under DATTO_RMM_NO_CONFIG_WRITE it cannot even read it from
// disk), so the value must not cross into the child's arguments. The control call
// proves the child really writes.
func TestMCPChildNeverReceivesAHeldValue(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, append([]string{"DATTO_RMM_NO_CONFIG_WRITE=1"}, keyPair...)...)
	teach := func(notes string) map[string]any {
		return map[string]any{"name": "teach", "arguments": map[string]any{
			"query": "find site", "resource-type": "account", "resource": []any{"site-1042"}, "notes": notes, "no-validate": true}}
	}
	_, out := e.mcp(t,
		map[string]any{"name": "datto-rmm_read", "arguments": map[string]any{"endpoint_id": "account.get-sites"}},
		teach("control learning note"),
		teach("rotated "+heldMinted),
	)
	if len(e.mints.all()) == 0 {
		t.Fatalf("the server minted nothing, so this test proves nothing:\n%.2000s", out)
	}
	noHeldValue(t, "the MCP session", out)
	var control bool
	for path, body := range e.files(t) {
		control = control || strings.Contains(body, "control learning note")
		noHeldValue(t, "file "+path, body)
	}
	if !control {
		t.Fatalf("the control teach call wrote nothing, so this test proves nothing:\n%.2000s", out)
	}
}

// TestMCPProtocolErrorsWithheld: the parser's error for a malformed request
// repeats what it was given, over stdio. The server's reply must not.
func TestMCPProtocolErrorsWithheld(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, tokenEnv...)
	_, out := e.mcp(t,
		map[string]any{"name": heldToken, "arguments": map[string]any{}},
		map[string]any{"name": "datto-rmm_read", "arguments": map[string]any{"endpoint_id": heldToken}},
	)
	noHeldValue(t, "the MCP session", out)
}

// TestPublicClientWordsStillPrint: the fixed OAuth client public-client:public is
// not a credential. The kit holds every Authorization header and both Basic parts
// it sees, so a Basic header set above it would make the word public a held value
// and withhold ordinary output for the rest of the run. The header is set beneath
// the kit, so records naming a public site print after a mint.
func TestPublicClientWordsStillPrint(t *testing.T) {
	t.Parallel()
	const rec = `{"id":7,"uid":"site-7","name":"public-library-public-client-branch"}`
	e := newEnv(t, func(w http.ResponseWriter, r *http.Request, _ seen) bool {
		if r.URL.Path == "/auth/oauth/token" {
			return false
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(dattoList("sites", rec)))
		return true
	}, keyPair...)
	r := e.run(t, "", "account", "get-sites", "--json")
	if got := e.mints.all(); len(got) != 1 {
		t.Fatalf("want one mint, got %+v\n%s", got, r.out)
	}
	if !strings.Contains(r.out, "public-library-public-client-branch") || strings.Contains(r.out, "withheld") {
		t.Fatalf("an ordinary record naming a public site was withheld:\n%s", r.out)
	}
}

// otherHost is a TLS server a redirect points at, counting what reaches it.
func otherHost(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n.Add(1)
		_, _ = w.Write([]byte(dattoList("sites")))
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

// TestRedirectNeverReachesAnotherHost: neither the API token nor the key pair
// follows a redirect to another origin, from the CLI or the MCP server.
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	t.Parallel()
	other, reached := otherHost(t)
	redirect := func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}
	t.Run("API request", func(t *testing.T) {
		e := newEnv(t, func(w http.ResponseWriter, r *http.Request, _ seen) bool {
			if r.URL.Path == "/auth/oauth/token" {
				return false
			}
			redirect(w, r)
			return true
		}, keyPair...)
		r := e.run(t, "", "system", "get-status", "--json")
		_, out := e.mcp(t, map[string]any{"name": "datto-rmm_read", "arguments": map[string]any{"endpoint_id": "system.get-status"}})
		if len(e.api.all()) < 2 {
			t.Fatalf("the CLI and MCP calls did not both reach the fake: %+v\n%s", e.api.all(), r.out)
		}
		if n := reached.Load(); n != 0 {
			t.Fatalf("the other host was reached %d time(s)\n%s", n, r.out)
		}
		if r.code == 0 {
			t.Fatalf("a refused redirect exited 0:\n%s", r.out)
		}
		noHeldValue(t, "the CLI output", r.out)
		noHeldValue(t, "the MCP session", out)
	})
	t.Run("token endpoint", func(t *testing.T) {
		e := newEnv(t, func(w http.ResponseWriter, r *http.Request, _ seen) bool {
			if r.URL.Path == "/auth/oauth/token" {
				redirect(w, r)
				return true
			}
			return false
		}, keyPair...)
		r := e.run(t, "", "account", "get-user", "--json")
		_, out := e.mcp(t, map[string]any{"name": "datto-rmm_read", "arguments": map[string]any{"endpoint_id": "account.get-user"}})
		if len(e.mints.all()) < 2 {
			t.Fatalf("the CLI and MCP mints did not both run: %+v\n%s", e.mints.all(), r.out)
		}
		if n := reached.Load(); n != 0 {
			t.Fatalf("the redirected mint reached the other host %d time(s)\n%s", n, r.out)
		}
		if got := e.api.all(); len(got) != 0 {
			t.Fatalf("API requests went out with no minted token: %+v", got)
		}
		noHeldValue(t, "the CLI output", r.out)
		noHeldValue(t, "the MCP session", out)
	})
}

// TestRejectedMintPrintsNoSecret: a refused mint, whose answer repeats what it was
// sent, is reported without the key or the secret, and no API request follows.
func TestRejectedMintPrintsNoSecret(t *testing.T) {
	t.Parallel()
	const wrong = "dwrongaa-bbbb-nnnn-bbbb"
	e := newEnv(t, nil, "DATTO_RMM_API_KEY="+heldKey, "DATTO_RMM_API_SECRET_KEY="+wrong)
	r := e.run(t, "", "account", "get-user", "--json")
	_, out := e.mcp(t, map[string]any{"name": "datto-rmm_read", "arguments": map[string]any{"endpoint_id": "account.get-user"}})
	if len(e.mints.all()) < 2 {
		t.Fatalf("the CLI and MCP mints did not both run: %+v\n%s", e.mints.all(), r.out)
	}
	if got := e.api.all(); len(got) != 0 {
		t.Fatalf("API requests went out after a refused mint: %+v", got)
	}
	if r.code == 0 {
		t.Fatalf("a refused mint exited 0:\n%s", r.out)
	}
	for _, text := range []string{r.out, out} {
		if strings.Contains(text, wrong) || strings.Contains(text, heldKey) {
			t.Fatalf("the refusal repeats the credential:\n%s", text)
		}
	}
}

// TestAuthLoginMintsOnlyAtItsHost: auth login mints once at the configured host,
// sends nothing anywhere else, stores the token and never the pair, and the token
// serves the next run without the pair.
func TestAuthLoginMintsOnlyAtItsHost(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, keyPair...)
	r := e.run(t, "", "auth", "login")
	if got := e.proxy.all(); len(got) != 0 {
		t.Fatalf("auth login sent to another host: %+v\n%s", got, r.out)
	}
	if got := e.mints.all(); len(got) != 1 {
		t.Fatalf("want one mint at the configured host, got %+v\n%s", got, r.out)
	}
	noHeldValue(t, "auth login", r.out)
	var stored bool
	for path, body := range e.files(t) {
		if strings.HasPrefix(filepath.Base(path), "credentials") {
			stored = stored || strings.Contains(body, heldMinted)
			for _, v := range []string{heldKey, heldSecret} {
				if strings.Contains(body, v) {
					t.Errorf("the credential store keeps the environment credential %s", v)
				}
			}
			continue
		}
		noHeldValue(t, "file "+path, body)
	}
	if !stored {
		t.Fatalf("auth login stored no token:\n%s", r.out)
	}
	e.without("DATTO_RMM_API_KEY")
	e.without("DATTO_RMM_API_SECRET_KEY")
	r = e.run(t, "", "account", "get-sites", "--json")
	if !strings.Contains(r.out, "HQ-Record") {
		t.Fatalf("the read after login fetched nothing:\n%s", r.out)
	}
	if got := e.mints.all(); len(got) != 1 {
		t.Fatalf("the read after login minted again: %+v", got)
	}
	got := e.api.all()
	if len(got) == 0 || got[len(got)-1].Header.Get("Authorization") != "Bearer "+heldMinted {
		t.Fatalf("the read after login did not carry the saved token: %+v", got)
	}
	noHeldValue(t, "the read after login", r.out)
}

// TestMalformedConfigNeverPrintsItsCredential: startup ordering. A config file the
// parser refuses repeats the file's text in its error, which is printed after
// credkit.Start, so the kit has read it token by token by then.
func TestMalformedConfigNeverPrintsItsCredential(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"bad line":      "rmm_api_key = \"" + heldKey + "\"\nthis is = = not toml " + heldSecret + "\n",
		"bad value":     "rmm_api_secret_key = " + heldSecret + "\n",
		"open string":   "rmm_token = \"" + heldToken + "\n",
		"wrong type":    "rmm_api_key = [\"" + heldKey + "\", \"" + heldSecret + "\"]\n",
		"unknown field": "password = \"" + heldSecret + "\"\nx y z\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, nil)
			writeFile(t, filepath.Join(e.home, "dr", "config", "config.toml"), body)
			noHeldValue(t, "the CLI", e.run(t, "", "account", "get-sites", "--json").out)
			noHeldValue(t, "the CLI", e.run(t, "", "doctor").out)
			_, out := e.mcp(t)
			noHeldValue(t, "the MCP server", out)
			cfg := writeFile(t, filepath.Join(e.home, "other.toml"), body)
			noHeldValue(t, "the CLI with --config", e.run(t, "", "--config", cfg, "version").out)
		})
	}
}

// TestConfigRefusalNeverPrintsItsCredential: startup ordering. A config file other
// users can change, naming another Datto host, is refused while it loads, and the
// refusal repeats its base_url, whose userinfo carries a password only that file
// has and whose path repeats an environment credential. The kit has read the file
// and the environment before the refusal prints.
func TestConfigRefusalNeverPrintsItsCredential(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("this case rests on a loose file mode; Windows has no Unix mode bits")
	}
	const inURL = "dinurlaa-dddd-oooo-dddd"
	for name, base := range map[string]string{
		"password only the file has": "https://ro:" + inURL + "@other.example.com/api",
		"environment credential":     "https://other.example.com/" + heldSecret,
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, nil, keyPair...)
			cfg := writeFile(t, filepath.Join(e.home, "loose.toml"), fmt.Sprintf("base_url = %q\n", base))
			if err := os.Chmod(cfg, 0o644); err != nil {
				t.Fatal(err)
			}
			e.without("DATTO_RMM_BASE_URL")
			e.vars = append(e.vars, "DATTO_RMM_CONFIG="+cfg)
			r := e.run(t, "", "version")
			if !strings.Contains(r.out, "refused") && !strings.Contains(r.out, "withheld") {
				t.Fatalf("the CLI printed nothing for the loose config, so this test proves nothing:\n%.1500s", r.out)
			}
			_, mcp := e.mcp(t, map[string]any{"name": "datto-rmm_read", "arguments": map[string]any{"endpoint_id": "system.get-status"}})
			for where, out := range map[string]string{"the CLI": r.out, "the MCP server": mcp} {
				if strings.Contains(out, inURL) || strings.Contains(out, heldSecret) {
					t.Fatalf("%s printed a credential before the kit held it:\n%.1500s", where, out)
				}
			}
			if got := e.mints.all(); len(got) != 0 {
				t.Errorf("the refused route minted: %+v", got)
			}
		})
	}
}

// TestSyncOfASiteWithAProxyPasswordCompletes: a site's proxy password is a
// declared response field, so the kit refuses any SQL statement carrying it. One
// such site must not fail the sync of every site: the store replaces the field
// before the statement, the sync completes, and the store never has the password.
func TestSyncOfASiteWithAProxyPasswordCompletes(t *testing.T) {
	t.Parallel()
	const proxyPassword = "dproxyss-tttt-rrrr-tttt"
	body := dattoList("sites",
		fmt.Sprintf(`{"id":1,"uid":"site-1","name":"Plain-Site","proxySettings":{"host":"proxy.example","username":"svc","password":%q}}`, proxyPassword),
		`{"id":2,"uid":"site-2","name":"Other-Site"}`)
	e := newEnv(t, func(w http.ResponseWriter, r *http.Request, _ seen) bool {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
		return true
	}, tokenEnv...)
	r := e.run(t, "", "sync", "--resources", "account", "--json")
	if r.code != 0 {
		t.Fatalf("sync failed on a site with a proxy password (exit %d):\n%s", r.code, r.out)
	}
	if strings.Contains(r.out, proxyPassword) {
		t.Fatalf("sync printed the proxy password:\n%s", r.out)
	}
	for path, content := range e.files(t) {
		if strings.Contains(content, proxyPassword) {
			t.Errorf("%s keeps the proxy password", path)
		}
	}
	got := e.run(t, "", "search", "Plain-Site", "--data-source", "local", "--json")
	if !strings.Contains(got.out, "Plain-Site") {
		t.Fatalf("the synced site is not in the store:\n%s", got.out)
	}
	if strings.Contains(got.out, proxyPassword) {
		t.Fatalf("the store returned the proxy password:\n%s", got.out)
	}
}

// TestPlatformRoutesCredentialsToItsHost: DATTO_RMM_PLATFORM chooses the host the
// credential goes to, so the kit must take that host as the allowed origin, and
// a credential never goes to the default platform instead.
func TestPlatformRoutesCredentialsToItsHost(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, append([]string{"DATTO_RMM_PLATFORM=zinfandel"}, tokenEnv...)...)
	e.without("DATTO_RMM_BASE_URL")
	r := e.run(t, "", "account", "get-sites", "--json")
	got := e.proxy.all()
	if len(got) == 0 {
		t.Fatalf("a credentialed request never left for the platform's host; the kit refused it:\n%s", r.out)
	}
	for _, s := range got {
		if !strings.Contains(s.Host, "zinfandel") {
			t.Errorf("a request left for %s, not the platform's host", s.Host)
		}
	}
	if strings.Contains(r.out, "withheld") {
		t.Errorf("the kit refused the platform's own host:\n%s", r.out)
	}
}

// TestDiscoveredConfigCannotPullACredential: a discovered config file naming
// another host cannot widen where the environment's credential goes: the explicit
// environment base URL wins.
func TestDiscoveredConfigCannotPullACredential(t *testing.T) {
	t.Parallel()
	other, reached := otherHost(t)
	e := newEnv(t, nil, tokenEnv...)
	writeFile(t, filepath.Join(e.home, "dr", "config", "config.toml"), fmt.Sprintf("base_url = %q\n", other.URL+"/api"))
	r := e.run(t, "", "account", "get-sites", "--json")
	if n := reached.Load(); n != 0 {
		t.Fatalf("the discovered config's host received the credential %d time(s)\n%s", n, r.out)
	}
	if len(e.api.all()) != 1 {
		t.Fatalf("the environment's base URL did not win: %+v\n%s", e.api.all(), r.out)
	}
}

// TestStoredTokenFileIsPrivate: the token store is 0600.
func TestStoredTokenFileIsPrivate(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no Unix mode bits")
	}
	e := newEnv(t, nil, keyPair...)
	if r := e.run(t, "", "auth", "login"); r.code != 0 {
		t.Fatalf("auth login failed:\n%s", r.out)
	}
	var found bool
	for path := range e.files(t) {
		if !strings.HasPrefix(filepath.Base(path), "credentials") {
			continue
		}
		found = true
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s is %o", path, info.Mode().Perm())
		}
	}
	if !found {
		t.Fatal("auth login left no credential store")
	}
}

// TestDeclarationCopiesMatch keeps the two literals the kit's checker makes
// separate the same as the Declarations they repeat.
func TestDeclarationCopiesMatch(t *testing.T) {
	t.Parallel()
	if !slices.Equal(profileCredentialFlags, Declarations.CredentialFlags) {
		t.Errorf("profileCredentialFlags %v, Declarations.CredentialFlags %v", profileCredentialFlags, Declarations.CredentialFlags)
	}
	if !slices.Equal(store.SecretResponseFields, Declarations.ResponseFields) {
		t.Errorf("store.SecretResponseFields %v, Declarations.ResponseFields %v", store.SecretResponseFields, Declarations.ResponseFields)
	}
}

// TestStoredTokenNeverPrints: a token piped to auth set-token is held from the pipe
// (the parser reads the command line first, and its errors repeat what it was
// given), and from the store on every later run.
func TestStoredTokenNeverPrints(t *testing.T) {
	t.Parallel()
	e := newEnv(t, echoAll)
	r := e.run(t, heldToken+"\n", "auth", "set-token")
	if r.code != 0 {
		t.Fatalf("auth set-token failed:\n%s", r.out)
	}
	noHeldValue(t, "auth set-token", r.out)
	// A fresh home, so nothing but the pipe can have held it: the parser refuses
	// the flag and repeats it before the command reads the pipe.
	fresh := newEnv(t, nil)
	noHeldValue(t, "a refused set-token", fresh.run(t, heldToken+"\n", "auth", "set-token", "--"+heldToken).out)
	r = e.run(t, "", "account", "get-sites", "--json")
	if got := e.api.all(); len(got) != 1 || got[0].Header.Get("Authorization") != "Bearer "+heldToken {
		t.Fatalf("the stored token did not reach the API: %+v\n%s", got, r.out)
	}
	noHeldValue(t, "a read echoing the stored token", r.out)
}

// TestStdinBodyPasswordNeverPrints: a --stdin body's password field is held before
// anything prints, previews and --dry-run included, while the body's ordinary
// fields still preview.
func TestStdinBodyPasswordNeverPrints(t *testing.T) {
	t.Parallel()
	const password = "dstdinpw-eeee-ffff-eeee"
	body := `{"name":"Stdin-Site","proxySettings":{"host":"proxy.example","password":"` + password + `"}}`
	e := newEnv(t, nil, tokenEnv...)
	for _, args := range [][]string{
		{"site", "create", "--stdin", "--dry-run"},
		{"site", "create", "--stdin", "--dry-run", "--json"},
		{"site", "create", "--stdin", "--no-such-flag", password},
	} {
		r := e.run(t, body, args...)
		if strings.Contains(r.out, password) {
			t.Errorf("%v printed the body's password:\n%s", args, r.out)
		}
	}
	if got := e.api.all(); len(got) != 0 {
		t.Fatalf("a dry run sent: %+v", got)
	}
}

// TestCredentialNeverGoesOverPlainHTTP: a base URL naming plain http gets no
// credential. Any request that left would reach the recording proxy.
func TestCredentialNeverGoesOverPlainHTTP(t *testing.T) {
	t.Parallel()
	for name, creds := range map[string][]string{"minted": keyPair, "token": tokenEnv} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, nil, append([]string{"DATTO_RMM_BASE_URL=http://datto.example/api"}, creds...)...)
			r := e.run(t, "", "account", "get-sites", "--json")
			for _, s := range e.proxy.all() {
				if strings.Contains(fmt.Sprint(s.Header), heldToken) || strings.Contains(s.Body, heldSecret) || strings.Contains(fmt.Sprint(s.Header), heldMinted) {
					t.Fatalf("a credential went over plain http: %+v\n%s", s, r.out)
				}
			}
			if r.code == 0 {
				t.Fatalf("a run against a plain http base URL exited 0:\n%s", r.out)
			}
		})
	}
}

// TestErrorFieldFailsASinglePageRead: a populated error field on an HTTP 200 is a
// failed read on a list read with no --all too, which reads one page and returns
// before the page walk's check. Datto documents the field on activity log pages.
// The same page with no error field still succeeds.
func TestErrorFieldFailsASinglePageRead(t *testing.T) {
	t.Parallel()
	body := `{"pageDetails":{"nextPageUrl":null},"activities":[],"error":"database timeout"}`
	e := newEnv(t, func(w http.ResponseWriter, r *http.Request, _ seen) bool {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
		return true
	}, tokenEnv...)
	reads := [][]string{
		{"activity-logs", "--data-source", "live", "--plain"},
		{"activity-logs", "--data-source", "live", "--json"},
		{"activity-logs", "--data-source", "live", "--csv"},
	}
	for _, args := range reads {
		r := e.run(t, "", args...)
		if len(e.api.all()) == 0 {
			t.Fatalf("%v never reached the fake:\n%s", args, r.out)
		}
		if r.code == 0 {
			t.Errorf("%v exited 0 on a response that reports a failure:\n%s", args, r.out)
		}
	}
	body = `{"pageDetails":{"nextPageUrl":null},"activities":[],"error":null}`
	for _, args := range reads {
		if r := e.run(t, "", args...); r.code != 0 {
			t.Errorf("%v failed on a clean page (exit %d):\n%s", args, r.code, r.out)
		}
	}
}
