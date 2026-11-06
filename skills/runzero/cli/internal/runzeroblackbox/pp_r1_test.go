// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package runzeroblackbox

// Codex round 1 of the proofpoint 4.34.2 reprint review (the shared
// credential layer this connector carries), on the built binaries.

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Class r1-declared-credential-held: with RUNZERO_CONFIG naming a file that
// is not TOML (Codex's input: spec.yaml), configuration loading fails before
// it holds anything, so the startup holder must read the declared
// credential (RUNZERO_API_KEY, bare and as its "Bearer <key>" header) itself,
// or the unknown-command error prints it. A stored key is held the same way
// whatever the configuration file does.
func TestPPR1KeyHeldWhenTheConfigurationFails(t *testing.T) {
	t.Parallel() // its own fake runZero and home

	const key, storedKey = "CTQXJ7429RPZN81KLCafedcb00", "OTStoredKeyR1-0c7a446b9e2d" // gitleaks:allow synthetic test fixture
	h := t.TempDir()
	yaml := filepath.Join(h, "spec.yaml")
	if err := os.WriteFile(yaml, []byte("openapi: 3.0.0\ninfo:\n  title: runZero\npaths: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	out.WriteString(runWith(t, cleanEnv(h, "RUNZERO_CONFIG="+yaml, "RUNZERO_API_KEY="+key), key))
	out.WriteString(runWith(t, cleanEnv(h, "RUNZERO_CONFIG="+yaml, "RUNZERO_API_KEY="+key), "org", key+"x"))
	out.WriteString(runWith(t, cleanEnv(h, "RUNZERO_CONFIG="+yaml, "RUNZERO_API_KEY=Bearer "+key), "Bearer "+key))
	s := t.TempDir()
	out.WriteString(runWithIn(t, cleanEnv(s), storedKey+"\n", "auth", "set-token"))
	broken := filepath.Join(s, "spec.yaml")
	_ = os.WriteFile(broken, []byte("openapi: 3.0.0\n"), 0o600)
	out.WriteString(runWith(t, cleanEnv(s, "RUNZERO_CONFIG="+broken), storedKey))
	out.WriteString(runWith(t, cleanEnv(s, "RUNZERO_CONFIG="+broken), "account", "get-key", "--no-such-flag", storedKey))
	// The unknown-command error repeats the key, so the kit withholds it
	// whole: its notice is the case having run.
	if !strings.Contains(out.String(), "unknown command") && !strings.Contains(out.String(), kitNotice("stderr")) {
		t.Fatalf("the unknown-command case did not run:\n%s", out.String())
	}
	for _, v := range []string{key, storedKey} {
		if strings.Contains(out.String(), v) {
			t.Errorf("printed %q:\n%s", v, out.String())
		}
	}
}

// Class r1-ioc-url-and-evidence-whole (the shared layer's proofpoint input):
// a URL a response carries is data an MSP acts on (a service's HTTP URL and
// redirect, a scan target), and it printed with every credential-named query
// value and userinfo password redacted. On runZero: an asset's and a
// service's URLs print whole, live, from the local store after a sync and an
// inventory sync, and over MCP after a text answer in the same process, and
// the key never prints. The fake repeats no credential (the kit withholds a
// whole answer that does, which TestNoCredentialInAnyOutput proves), and
// serves TLS (the kit sends no held value over plain http).
func TestPPR1IOCDataPrintsWhole(t *testing.T) {
	t.Parallel()                            // its own fake runZero and home: runs beside the other long batteries
	const key = "CTIocKeyR1-7e2c90b4d1a3f5" // gitleaks:allow synthetic test fixture
	const pageURL = "https://hq-portal.example/login?token=click-tracking-123&sig=AbC123"
	const userinfoURL = "http://admin:portal-default@10.20.30.40:8080/manage"
	service := fmt.Sprintf(`{"id":"asset-r1","names":["hq-r1"],"addresses":["10.20.30.40"],"service_id":"svc-r1","service_asset_id":"asset-r1",`+
		`"service_address":"10.20.30.40","service_transport":"tcp","service_port":443,"service_protocol":"http",`+
		`"attributes":{"http.url":%q,"http.redirect":%q}}`, pageURL, userinfoURL)
	bodies := map[string]string{
		apiBase + "/export/org/services.json": `[` + service + `]`,
		apiBase + "/org/services":             `[` + service + `]`,
		apiBase + "/export/org/assets.json": `[{"id":"asset-r1","names":["hq-r1"],"addresses":["10.20.30.40"],` +
			`"attributes":{"http.url":"` + pageURL + `"}}]`,
	}
	origin, ca := tlsFake(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimSuffix(r.URL.Path, "/")
		if strings.HasSuffix(p, ".csv") {
			w.Header().Set("Content-Type", "text/csv")
			fmt.Fprintf(w, "address,port,url\n10.20.30.40,443,%s&b=2\n", pageURL)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if b, ok := bodies[p]; ok {
			fmt.Fprint(w, b)
			return
		}
		fmt.Fprint(w, `[]`)
	}))
	h := t.TempDir()
	env := cleanEnv(h, "RUNZERO_BASE_URL="+origin+apiBase, "RUNZERO_CA_CERT="+ca, "RUNZERO_API_KEY="+key)
	db := filepath.Join(h, "rz.db")
	live := runWith(t, env, "org", "get-services", "--json")
	runWith(t, env, "sync", "--db", db, "--json")
	runWith(t, env, "inventory", "sync", "--db", db, "--json")
	local := runWith(t, env, "inventory", "list", "services", "--db", db, "--json")
	export := runWith(t, env, "runzero-export", "services-json", "--json")
	// One MCP session: a CSV answer first (its body read as a form could hold
	// the URL's query values for the whole process), then the JSON answer.
	mcp := (&bbEnv{home: h, env: append(env, "RUNZERO_CLI_PATH="+bbCLI)}).mcpPhases(t,
		[]map[string]any{read("runzero-export.services-csv", nil)},
		[]map[string]any{read("org.get-services", nil)})
	for name, c := range map[string]struct {
		out  string
		want []string
	}{
		"live service":             {live, []string{pageURL, userinfoURL}},
		"inventory list (local)":   {local, []string{pageURL}},
		"services export":          {export, []string{pageURL, userinfoURL}},
		"MCP after a text answer":  {mcp, []string{"click-tracking-123", "portal-default"}},
		"MCP after a text answer2": {mcp, []string{"hq-portal.example/login"}},
	} {
		for _, w := range c.want {
			if !strings.Contains(c.out, w) && !strings.Contains(c.out, strings.ReplaceAll(w, "&", `\u0026`)) {
				t.Errorf("%s: %q does not print whole:\n%s", name, w, c.out)
			}
		}
		if strings.Contains(c.out, key) {
			t.Errorf("%s printed the key:\n%s", name, c.out)
		}
	}
}
