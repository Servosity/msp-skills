// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-fix (handfixes.json vault-secrets-withheld): end to end through the
// real `asset-passwords create` command. Before the fix, Hudu's validation
// error {"errors":{"password":["rejected value: <secret>"]}} passed the
// shape-based error filter (the secret sat inside an array, not directly
// under the key) and the command printed it.

package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProtectVaultCreateErrorCLIOutput(t *testing.T) {
	const sentinel = "SENTINEL-ERR"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/asset_passwords" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"errors":{"password":["rejected value: ` + sentinel + `"]}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	t.Cleanup(srv.Close)

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HUDU_HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("HUDU_BASE_URL", srv.URL)
	t.Setenv("HUDU_API_KEY", "fixture-token")
	t.Setenv("HUDU_NO_LOCAL_DATA", "")
	t.Setenv("PRINTING_PRESS_CLIENT_PROFILE", "")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_VERIFY_LIVE_HTTP", "")
	cfg := filepath.Join(home, "hudu-config.toml")
	if err := os.WriteFile(cfg, []byte("access_token = \"fixture-token\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HUDU_CONFIG", cfg)

	for _, args := range [][]string{
		{"asset-passwords", "create", "--company-id", "1", "--name", "vpn", "--password", "typed-by-operator"},
		{"asset-passwords", "create", "--company-id", "1", "--name", "vpn", "--password", "typed-by-operator", "--json"},
	} {
		var out bytes.Buffer
		root := RootCmd()
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(args)
		err := root.Execute()
		if err == nil {
			t.Fatalf("%v: want the 422 to fail the command, output: %s", args, out.String())
		}
		if !strings.Contains(err.Error(), "422") {
			t.Fatalf("%v: the error lost its status: %v", args, err)
		}
		if strings.Contains(err.Error(), sentinel) || strings.Contains(out.String(), sentinel) {
			t.Fatalf("%v printed the rejected value:\nerror: %v\noutput: %s", args, err, out.String())
		}
	}
}
