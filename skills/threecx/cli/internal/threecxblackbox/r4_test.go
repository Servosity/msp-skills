// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (handfixes.json credkit-wiring): the two classes the third review
// budget stopped on, each from its failing input.

package threecxblackbox

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestR4ProfileHeldWhereItIsRead(t *testing.T) {
	const secret = "PbxProfilePass-84zQ"
	ps, e := honestEnv(t, nil)
	fleetDir := filepath.Join(e.home, "fleet-root")
	pin := e.with("THREECX_FLEET_DIR=" + fleetDir)
	pin.run(t, "fleet", "add", "acme", "--fqdn", ps.host, "--client-id", "901", "--env-prefix", "THREECX_ACME")
	pin.run(t, "--pbx", "acme", "system-status", "get", "--json", "--data-source", "live")
	var homes []string
	_ = filepath.WalkDir(fleetDir, func(path string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() && filepath.Base(filepath.Dir(filepath.Dir(path))) == "pbx" && filepath.Base(filepath.Dir(path)) == "acme" {
			homes = append(homes, path)
		}
		return nil
	})
	if len(homes) != 1 {
		t.Fatalf("homes %v", homes)
	}
	writePrivate(t, filepath.Join(homes[0], "config", "profiles.json"), `{"profiles":{"p":{"name":"p","values":{"auth-password":"`+secret+`"}}}}`)
	if out := pin.run(t, "--pbx", "acme", "profile", "show", "p", "--json"); strings.Contains(out, secret) {
		t.Fatalf("control: printed without the override:\n%.500s", out)
	}
	over := pin.with("THREECX_CONFIG_DIR=" + filepath.Join(e.home, "other"))
	if out := over.run(t, "--pbx", "acme", "profile", "show", "p", "--json"); strings.Contains(out, secret) {
		t.Fatalf("LEAK: printed under THREECX_CONFIG_DIR:\n%.500s", out)
	}
}

func TestR4MCPRefusesARedirectToAStateChangingGET(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/connect/token":
			fmt.Fprint(w, `{"access_token":"redirTok-1a2b3c4d","expires_in":3600,"token_type":"Bearer"}`)
		case r.URL.Path == "/xapi/v1/Users":
			http.Redirect(w, r, "/xapi/v1/SystemStatus/Pbx.GetRequestHelpLink(grantPeriodDays=365)", http.StatusFound)
		default:
			fmt.Fprint(w, `{"value":[]}`)
		}
	}))
	defer srv.Close()
	a := echoServer(t)
	host := strings.TrimPrefix(srv.URL, "https://")
	e := newEnv(t, a, true).without("THREECX_FQDN=", "THREECX_FQDN="+host)
	out := e.mcpCalls(t, []map[string]any{{"name": "export", "arguments": map[string]any{"resource": "users"}}})
	mu.Lock()
	defer mu.Unlock()
	if strings.Contains(strings.Join(seen, "\n"), "GetRequestHelpLink") {
		t.Fatalf("LEAK: the grant was sent with no confirm: %v\n%.500s", seen, out)
	}
	if !strings.Contains(strings.Join(seen, "\n"), "/xapi/v1/Users") {
		t.Fatalf("control: never reached the server: %v\n%.800s", seen, out)
	}
}
