// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"datto-rmm-pp-cli/internal/store"
)

// runDattoAt runs the root command with a fixed home, so several commands in
// one test share one default store.
func runDattoAt(t *testing.T, home string, env map[string]string, args ...string) (string, string, error) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("DATTO_RMM_HOME", home)
	os.Unsetenv("PRINTING_PRESS_VERIFY")
	for _, k := range []string{"DATTO_RMM_TOKEN", "DATTO_RMM_API_KEY", "DATTO_RMM_API_SECRET_KEY", "DATTO_RMM_PLATFORM",
		"DATTO_RMM_API_URL", "DATTO_RMM_CONFIG", "DATTO_RMM_NO_CONFIG_WRITE"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	var out, errb bytes.Buffer
	root := newRootCmd(&rootFlags{})
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errb.String(), err
}

// Codex review r1 class "wrong local partition" (datto-rmm handfix
// local-partition-follows-the-path): every generated account command named
// the one `account` partition, which sync fills with /v2/account/sites (the
// partition every fleet analytic reads as sites). A default (auto) live read
// of /v2/account/devices, /v2/account/variables or /v2/account/components
// cached its documented records there, and a local read of devices returned
// the synced sites. A sync partition now serves and receives only the records
// of its own path.
func TestSyncPartitionHoldsOnlyItsOwnPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		page := `{"pageDetails":{"count":1,"totalCount":1,"prevPageUrl":null,"nextPageUrl":null},`
		switch r.URL.Path {
		case "/api/v2/account/sites":
			_, _ = w.Write([]byte(page + `"sites":[{"id":11,"uid":"site-acme","name":"Acme Site","devicesStatus":{"numberOfDevices":1,"numberOfOnlineDevices":1,"numberOfOfflineDevices":0}}]}`))
		case "/api/v2/account/devices":
			_, _ = w.Write([]byte(page + `"devices":[{"id":21,"uid":"dev-pc1","hostname":"PC1-HOST","siteUid":"site-acme","siteName":"Acme Site"}]}`))
		case "/api/v2/account/variables":
			_, _ = w.Write([]byte(page + `"variables":[{"id":31,"name":"ProxyHost","value":"proxy.local","masked":false}]}`))
		case "/api/v2/account/components":
			_, _ = w.Write([]byte(page + `"components":[{"id":41,"uid":"comp-1","name":"Disk Cleanup","categoryCode":"script","credentialsRequired":false}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	home := t.TempDir()
	env := map[string]string{"DATTO_RMM_BASE_URL": srv.URL + "/api", "DATTO_RMM_TOKEN": "partitionToken-4c4c"}
	if _, stderr, err := runDattoAt(t, home, env, "sync", "--resources", "account,account-devices", "--full", "--json"); err != nil {
		t.Fatalf("sync: %v\n%s", err, stderr)
	}
	for _, args := range [][]string{
		{"account", "get-user-devices", "--json"},
		{"account", "get-variables", "--json"},
		{"account", "get-components", "--json"},
	} {
		if _, stderr, err := runDattoAt(t, home, env, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, stderr)
		}
	}
	writeMutationResponseToStore(context.Background(), "account", json.RawMessage(`{"id":32,"name":"NewVar","value":"v","masked":false}`), "")

	db, err := store.Open(defaultDBPath("datto-rmm-cli"))
	if err != nil {
		t.Fatal(err)
	}
	sites, err := loadFleetSites(context.Background(), db)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 || sites[0].UID != "site-acme" {
		names := []string{}
		for _, s := range sites {
			names = append(names, s.UID+"/"+s.Name)
		}
		t.Errorf("the sites partition holds records of other paths: %v", names)
	}

	stdout, stderr, err := runDattoAt(t, home, env, "account", "get-user-devices", "--data-source", "local", "--json")
	if err != nil {
		t.Fatalf("local devices: %v\n%s", err, stderr)
	}
	if strings.Contains(stdout, "Acme Site\",\"devicesStatus") || strings.Contains(stdout, "site-acme\",\"name") || !strings.Contains(stdout, "PC1-HOST") {
		t.Errorf("a local read of /v2/account/devices did not return the synced devices:\n%s", stdout)
	}
}
