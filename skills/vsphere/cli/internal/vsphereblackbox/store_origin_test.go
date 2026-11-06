// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package vsphereblackbox

import (
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// namedVCenter is a fake vCenter whose only VM is vm-1021 named vmName. Two of
// them reproduce one MoRef naming different machines on two vCenters.
func namedVCenter(t *testing.T, vmName string) *server {
	t.Helper()
	return vmListVCenter(t, vmName, http.StatusOK, fmt.Sprintf(`[{"vm":"vm-1021","name":%q,"power_state":"POWERED_ON","cpu_count":2,"memory_size_MiB":4096}]`, vmName))
}

// vmListVCenter answers GET /api/vcenter/vm with status and body; vm-1021's
// detail is named vmName.
func vmListVCenter(t *testing.T, vmName string, status int, body string) *server {
	t.Helper()
	sv := &server{quiet: true}
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		sv.mu.Lock()
		sv.sent = append(sv.sent, r.Method+" "+p)
		sv.mu.Unlock()
		w.Header().Set("X-Ratelimit-Remaining", "1000000")
		w.Header().Set("X-Ratelimit-Reset", strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && p == "/api/session":
			if _, _, ok := r.BasicAuth(); !ok {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error_type":"UNAUTHENTICATED"}`))
				return
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, "%q", bbAPISess)
			return
		case p == viRoot+"/ServiceInstance/ServiceInstance/content":
			_, _ = w.Write([]byte(`{"about":{"name":"VMware VirtualCenter Server","version":"8.0.2","apiVersion":"8.0.2.0","instanceUuid":"vc-uuid"},"sessionManager":{"value":"SessionManager"}}`))
			return
		case r.Method == http.MethodPost && p == viRoot+"/SessionManager/SessionManager/Login":
			w.Header().Set("vmware-api-session-id", bbVISess)
			_, _ = w.Write([]byte(`{"key":"k1","userName":"ro"}`))
			return
		}
		if strings.HasPrefix(p, "/api/") && r.Header.Get("vmware-api-session-id") != bbAPISess {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error_type":"UNAUTHENTICATED"}`))
			return
		}
		switch {
		case p == "/api/vcenter/vm":
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		case p == "/api/vcenter/vm/vm-1021":
			fmt.Fprintf(w, `{"name":%q,"power_state":"POWERED_ON","cpu":{"count":2},"memory":{"size_MiB":4096}}`, vmName)
		case isList(p):
			_, _ = w.Write([]byte(`[]`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	sv.url = srv.URL
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	sv.caFile = ca
	return sv
}

// atBase is e with VSPHERE_BASE_URL replaced by base (the trust root kept).
func atBase(e *bbEnv, base string) *bbEnv {
	var kept []string
	for _, kv := range e.env {
		if !strings.HasPrefix(kv, "VSPHERE_BASE_URL=") {
			kept = append(kept, kv)
		}
	}
	return &bbEnv{home: e.home, env: append(kept, "VSPHERE_BASE_URL="+base)}
}

// Class store-shared-across-vcenters: every local store is the store of the
// vCenter the run points at, with or without a credential. A first run with no
// credential left a shared data.db that the credentialed sync at A then wrote,
// and a run at B (whose saved credential is refused) read A's VM as its own.
func TestLocalStoreIsTheStoreOfTheEffectiveVCenter(t *testing.T) {
	t.Parallel()
	const aName, bName = "ACME-DC01", "GLOBEX-FS01"
	a, b := namedVCenter(t, aName), namedVCenter(t, bName)
	e := newEnv(t, a, false)

	// No credential yet: each of these may fail, and each opened the store.
	runCode(t, e, "sync", "--json")
	runCode(t, e, "learnings", "list", "--json")
	runCode(t, e, "search", "anything", "--json")
	runCode(t, e, "teach", "--query", "domain controller", "--resource", "vm-1021", "--resource-type", "vm", "--no-validate")

	e.run(t, "auth", "set-credentials", "ro@a.local", "ClientA-Pass-1111-aaaa")
	if out, code := runCode(t, e, "sync", "--json"); code != 0 {
		t.Fatalf("the control failed: sync at A rc=%d:\n%.1200s", code, out)
	}
	if out := e.run(t, "vm", "get", "vm-1021", "--data-source", "local", "--json"); !strings.Contains(out, aName) {
		t.Fatalf("the control failed: A's own store has no %s:\n%.1200s", aName, out)
	}
	// The same vCenter spelled another way is the same store.
	for _, spelling := range []string{a.url + "/", strings.Replace(a.url, "https://", "HTTPS://", 1)} {
		if out := atBase(e, spelling).run(t, "vm", "get", "vm-1021", "--data-source", "local", "--json"); !strings.Contains(out, aName) {
			t.Errorf("%s is A, and its store has no %s:\n%.1200s", spelling, aName, out)
		}
	}

	atB := &bbEnv{home: e.home, env: serverEnv(e, b)}
	for _, args := range [][]string{
		{"vm", "get", "vm-1021", "--data-source", "local", "--json"},
		{"vm", "list", "--data-source", "local", "--json"},
		{"search", aName, "--json"},
		{"learnings", "list", "--json"},
	} {
		if out, _ := runCode(t, atB, args...); strings.Contains(out, aName) {
			t.Errorf("at B, %v reported A's VM:\n%.1200s", args, out)
		}
	}
	if out := atB.mcpCalls(t, []map[string]any{
		{"name": "sql", "arguments": map[string]any{"query": "SELECT data FROM resources"}},
		{"name": "search", "arguments": map[string]any{"query": aName}},
	}); strings.Contains(out, aName) {
		t.Errorf("at B, MCP sql or search reported A's VM:\n%.1500s", out)
	}

	// The environment variant: the credential is in the environment, and the
	// run that falls back to local data points at a vCenter that does not answer.
	env := newEnv(t, a, false)
	runCode(t, env, "sync", "--json")
	withPw := &bbEnv{home: env.home, env: append(append([]string(nil), env.env...), "VSPHERE_USERNAME=ro@a.local", "VSPHERE_PASSWORD=ClientA-Pass-1111-aaaa")}
	if out, code := runCode(t, withPw, "sync", "--json"); code != 0 {
		t.Fatalf("the control failed: sync at A with the environment password rc=%d:\n%.1200s", code, out)
	}
	down := atBase(env, "https://127.0.0.1:1")
	for _, args := range [][]string{
		{"vm", "get", "vm-1021", "--json"},
		{"vm", "list", "--json"},
	} {
		if out, _ := runCode(t, down, args...); strings.Contains(out, aName) {
			t.Errorf("at an unreachable vCenter, %v reported A's VM:\n%.1200s", args, out)
		}
	}
}

// The store fallback with no home directory at all (HOME, VSPHERE_HOME,
// VSPHERE_DATA_DIR and XDG_DATA_HOME unset) is scoped by origin too: two
// complete configs for two vCenters, run from one directory, never share a
// store (Codex more-pass round 1).
func TestStoreFallbackWithNoHomeIsScoped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.UserHomeDir reads USERPROFILE on Windows; the no-home fallback is the Unix path")
	}
	t.Parallel()
	a, b := namedVCenter(t, "ACME-DC01"), namedVCenter(t, "GLOBEX-FS01")
	dir := t.TempDir()
	cfg := func(name string, sv *server) string {
		p := filepath.Join(dir, name)
		body := fmt.Sprintf("base_url = %q\nca_cert = %q\nusername = %q\npassword = %q\n", sv.url, sv.caFile, bbUser, bbPassword)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	aCfg, bCfg := cfg("A.toml", a), cfg("B.toml", b)
	run := func(args ...string) (string, error) {
		cmd := exec.Command(bbCLI, args...)
		cmd.Env, cmd.Dir = []string{"PATH=" + os.Getenv("PATH")}, dir
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := run("--config", aCfg, "--no-learn", "sync", "--resources", "vm"); err != nil {
		t.Logf("sync at A: %v\n%s", err, out)
	}
	out, _ := run("--config", bCfg, "--no-learn", "vm", "list", "--data-source", "local", "--json")
	if strings.Contains(out, "ACME-DC01") {
		t.Fatalf("a run at B read A's store:\n%.1500s", out)
	}
	// Control: A's own local read still finds what A synced.
	if out, _ := run("--config", aCfg, "--no-learn", "vm", "list", "--data-source", "local", "--json"); !strings.Contains(out, "ACME-DC01") {
		t.Errorf("A's local read lost A's sync (the control did not run):\n%.1500s", out)
	}
}
