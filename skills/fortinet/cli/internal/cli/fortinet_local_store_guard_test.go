// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"fortinet-pp-cli/internal/cliutil/testenv"
)

// The local store holds only the default VDOM. A live read scoped to --vdom
// must not be written into it, and a local read scoped to --vdom (explicit or
// as the network fallback) must refuse rather than serve another VDOM's row.
func TestVDOMScopedReadsBypassLocalStore(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		name := strings.TrimPrefix(r.URL.Path, "/api/v2/cmdb/firewall/address/")
		if name == r.URL.Path {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		vdom := r.URL.Query().Get("vdom")
		if vdom == "" {
			vdom = "root"
		}
		_, _ = fmt.Fprintf(w, `{"results":[{"name":%q,"subnet":"10.0.0.0 255.0.0.0","comment":"from %s"}],"vdom":%q,"status":"success"}`, name, vdom, vdom)
	}))
	t.Setenv("FORTINET_BASE_URL", srv.URL+"/api/v2")
	t.Setenv("FORTINET_API_TOKEN", "tok")

	// Scoped live read: answered live, not cached.
	if out, err := runCLI(t, "addresses", "get", "shared", "--vdom", "client-a", "--insecure", "--json"); err != nil || !strings.Contains(out, "from client-a") {
		t.Fatalf("live scoped read: err=%v\n%s", err, out)
	}
	if out, err := runCLI(t, "addresses", "get", "shared", "--data-source", "local", "--json"); err == nil {
		t.Fatalf("a --vdom client-a live read was written into the default-VDOM store:\n%s", out)
	}
	// Scoped local read refuses, whatever is cached.
	if out, err := runCLI(t, "addresses", "get", "shared", "--vdom", "client-b", "--data-source", "local", "--json"); err == nil || !strings.Contains(err.Error()+out, "could not apply vdom") {
		t.Fatalf("scoped local read not refused: err=%v\n%s", err, out)
	}
	if out, err := runCLI(t, "addresses", "list", "--vdom", "client-b", "--data-source", "local", "--json"); err == nil || !strings.Contains(err.Error()+out, "could not apply vdom") {
		t.Fatalf("scoped local list not refused: err=%v\n%s", err, out)
	}

	// Unscoped live read still writes through, and a name with a space is
	// found offline under its raw name.
	if out, err := runCLI(t, "addresses", "get", "HQ LAN", "--insecure", "--json"); err != nil || !strings.Contains(out, "from root") {
		t.Fatalf("live unscoped read: err=%v\n%s", err, out)
	}
	if out, err := runCLI(t, "addresses", "get", "HQ LAN", "--data-source", "local", "--json"); err != nil || !strings.Contains(out, "from root") {
		t.Fatalf("offline read of a cached name with a space: err=%v\n%s", err, out)
	}

	// Network fallback: with the API gone, a scoped read must not fall back
	// to the cached default-VDOM row of the same name.
	srv.Close()
	out, err := runCLI(t, "addresses", "get", "HQ LAN", "--vdom", "client-b", "--insecure", "--json")
	if err == nil || strings.Contains(out, "from root") || !strings.Contains(err.Error()+out, "could not apply vdom") {
		t.Fatalf("scoped read fell back to the default-VDOM cache: err=%v\n%s", err, out)
	}
}

// sync must refuse a vdom passed through its free-form parameter flags before
// it reads anything: the rows would land in the default-VDOM store and a
// --full reconcile would prune against another VDOM's IDs.
func TestSyncRefusesVDOMScopedParams(t *testing.T) {
	testenv.Isolate(t)
	requests := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"name":"shared","subnet":"10.0.0.0 255.0.0.0"}],"status":"success"}`))
	}))
	defer srv.Close()
	t.Setenv("FORTINET_BASE_URL", srv.URL+"/api/v2")
	t.Setenv("FORTINET_API_TOKEN", "tok")
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--param", "vdom=client-a"}, "default VDOM"},
		{[]string{"--global-param", "VDOM=client-a"}, "default VDOM"},
		{[]string{"--resource-param", "addresses:vdom=client-a"}, "default VDOM"},
		// A pass that can return fewer rows would prune the rest of the table.
		{[]string{"--param", "filter=name==office"}, "never change which rows come back"},
		{[]string{"--resource-param", "addresses:key=name", "--resource-param", "addresses:pattern=office"}, "never change which rows come back"},
		{[]string{"--param", "count=1"}, "never change which rows come back"},
		{[]string{"--global-param", "start=5"}, "never change which rows come back"},
		{[]string{"--param", "format=comment"}, "never change which rows come back"},
		{[]string{"--param", "exclude-default-values=1"}, "never change which rows come back"},
		{[]string{"--param", "action=schema"}, "never change which rows come back"},
	} {
		argv := append([]string{"sync", "--resources", "addresses", "--full", "--insecure"}, c.args...)
		out, err := runCLI(t, argv...)
		if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: err=%v exit=%d\n%s", c.args, err, ExitCode(err), out)
		}
	}
	if requests != 0 {
		t.Fatalf("a refused sync still sent %d request(s)", requests)
	}
	// An unscoped sync, or one with a property-only parameter, is unchanged.
	if out, err := runCLI(t, "sync", "--resources", "addresses", "--insecure", "--param", "with_meta=1", "--param", "datasource=1"); err != nil {
		t.Fatalf("unscoped sync: %v\n%s", err, out)
	}
	if requests == 0 {
		t.Fatal("unscoped sync sent no request")
	}
}

// Every monitor command shares the resource type "monitor" and every settings
// command "settings": a cached row from one endpoint must never answer
// another, explicitly or as the network fallback.
func TestSharedLocalTablesNeverAnswerLocally(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/monitor/") {
			_, _ = w.Write([]byte(`{"results":[{"name":"vpn-cert","valid_to":1900000000}],"status":"success"}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":{"status":"enable","server":"203.0.113.50","name":"setting","id":"setting"},"status":"success"}`))
	}))
	t.Setenv("FORTINET_BASE_URL", srv.URL+"/api/v2")
	t.Setenv("FORTINET_API_TOKEN", "tok")
	for _, pair := range [][2][]string{
		{{"monitor", "certificates"}, {"monitor", "ha-checksums"}},
		{{"settings", "fortianalyzer"}, {"settings", "syslog"}},
	} {
		if out, err := runCLI(t, append(pair[0], "--insecure", "--json")...); err != nil {
			t.Fatalf("live %v: %v\n%s", pair[0], err, out)
		}
		out, err := runCLI(t, append(pair[1], "--data-source", "local", "--json")...)
		if err == nil || strings.Contains(out, "vpn-cert") || strings.Contains(out, "203.0.113.50") || !strings.Contains(err.Error()+out, "share one local table") {
			t.Errorf("%v answered from another endpoint's cached rows: err=%v\n%s", pair[1], err, out)
		}
	}
	srv.Close()
	out, err := runCLI(t, "monitor", "ha-checksums", "--insecure", "--json")
	if err == nil || strings.Contains(out, "vpn-cert") || !strings.Contains(err.Error()+out, "share one local table") {
		t.Fatalf("network fallback served another endpoint's rows: err=%v\n%s", err, out)
	}
}

// Rule, not list: any resource type that generated commands with different
// endpoints share must be in sharedLocalTables, or a regenerated command
// would silently start answering from another endpoint's cached rows.
func TestSharedLocalTablesCoverEveryCommand(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	call := regexp.MustCompile(`resolve\w*Read\w*\(cmd\.Context\(\), c, flags, (?:"(?:auto|local|live)", )?"([^"]+)"`)
	ppPath := regexp.MustCompile(`"pp:path": "([^"]+)"`)
	trailingParam := regexp.MustCompile(`(/\{[^}]+\})+$`)
	endpoints := map[string]map[string]bool{}
	commands := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		rt, p := call.FindSubmatch(src), ppPath.FindSubmatch(src)
		if rt == nil || p == nil {
			continue
		}
		commands++
		coll := trailingParam.ReplaceAllString(string(p[1]), "")
		if endpoints[string(rt[1])] == nil {
			endpoints[string(rt[1])] = map[string]bool{}
		}
		endpoints[string(rt[1])][coll] = true
	}
	if commands < 50 {
		t.Fatalf("scanned only %d generated read commands; the pattern no longer matches the generated code", commands)
	}
	var missing []string
	for rt, colls := range endpoints {
		if len(colls) > 1 && !sharedLocalTables[rt] {
			missing = append(missing, rt)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("resource types shared by commands with different endpoints but not in sharedLocalTables: %v", missing)
	}
	for rt := range sharedLocalTables {
		if len(endpoints[rt]) < 2 {
			t.Errorf("sharedLocalTables lists %q, which no longer spans several endpoints", rt)
		}
	}
}

// The local store is per device: with the same token configured, pointing
// FORTINET_BASE_URL at another FortiGate must not read the first one's rows,
// explicitly or as the network fallback.
func TestLocalStoreIsPerDevice(t *testing.T) {
	testenv.Isolate(t)
	serve := func(who string) *httptest.Server {
		return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"results":[{"name":"shared","subnet":"10.0.0.0 255.0.0.0","comment":"from %s"}],"status":"success"}`, who)
		}))
	}
	a := serve("device-a")
	defer a.Close()
	t.Setenv("FORTINET_API_TOKEN", "same-token")
	t.Setenv("FORTINET_BASE_URL", a.URL+"/api/v2")
	if out, err := runCLI(t, "addresses", "get", "shared", "--insecure", "--json"); err != nil || !strings.Contains(out, "from device-a") {
		t.Fatalf("live read of device A: err=%v\n%s", err, out)
	}
	if out, err := runCLI(t, "addresses", "get", "shared", "--data-source", "local", "--json"); err != nil || !strings.Contains(out, "from device-a") {
		t.Fatalf("device A's own store: err=%v\n%s", err, out)
	}
	// Same token, another FortiGate that is not reachable.
	t.Setenv("FORTINET_BASE_URL", "https://127.0.0.1:1/api/v2")
	for _, args := range [][]string{
		{"addresses", "get", "shared", "--data-source", "local", "--json"},
		{"addresses", "get", "shared", "--insecure", "--json"},
	} {
		if out, err := runCLI(t, args...); err == nil || strings.Contains(out, "from device-a") {
			t.Errorf("%v answered with device A's rows under device B: err=%v\n%s", args, err, out)
		}
	}
}

func TestStoreScopeKeysOnOriginAndNeverAdoptsCredentialOnlyStores(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("FORTINET_API_TOKEN", "same-token")
	scope := func(base string) (string, string) {
		t.Setenv("FORTINET_BASE_URL", base)
		configureDefaultDBScope("")
		h, legacy, _ := currentDefaultDBScope()
		return h, legacy
	}
	a, legacyA := scope("https://192.0.2.1/api/v2")
	b, _ := scope("https://198.51.100.1/api/v2")
	a2, _ := scope("HTTPS://192.0.2.1/api/v2/")
	if a == "" || a == b {
		t.Fatalf("store scope does not separate devices sharing a token: a=%q b=%q", a, b)
	}
	if a != a2 {
		t.Errorf("one device got two store scopes from URL spelling: %q vs %q", a, a2)
	}
	if legacyA != "" {
		t.Errorf("a credential-only store would be adopted into a device scope (legacy hash %q)", legacyA)
	}
}
