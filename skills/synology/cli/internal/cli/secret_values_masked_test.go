// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Pins HAND-FIX(dsm-mask-secret-values).

package cli

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"synology-pp-cli/internal/dsm"
	"synology-pp-cli/internal/dsm/dsmtest"
)

// TestSecretValuesNeverLeaveTheClient: login returned sid SID-LEAK-7731...;
// a later answer carrying that value (or the password, or the SynoToken) in
// any field - a user's description, JSON-escaped, percent-encoded in a URL -
// reached stdout, the response cache, the local store and export. Every
// successful answer is masked for every secret value the client holds
// before any of those.
func TestSecretValuesNeverLeaveTheClient(t *testing.T) {
	const (
		sid = "SID-LEAK-7731-abcdef"
		tok = "TOKLEAK-55667788"
		pw  = "PW-LEAK-99001122"
	)
	home := sweepIsolate(t)
	payload := map[string]string{
		"SYNO.Core.User/list": `{"total":3,"offset":0,"users":[` +
			`{"name":"alice","uid":1026,"description":"copied ` + sid + ` here"},` +
			`{"name":"bob","uid":1027,"description":"PW-LEAK-99001122 escaped"},` +
			`{"name":"carol","uid":1028,"description":"https://nas/?t=` + url.QueryEscape(url.QueryEscape(tok)) + `"}]}`,
		"SYNO.ActiveBackup.Setting/list": `{"settings":{"note":"session ` + sid + ` token ` + tok + `"}}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dsmtest.ServeAPIInfo(w, r) {
			return
		}
		_ = r.ParseForm()
		key := r.Form.Get("api") + "/" + r.Form.Get("method")
		switch {
		case key == "SYNO.API.Auth/login":
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"` + sid + `","synotoken":"` + tok + `"}}`))
		case payload[key] != "":
			_, _ = w.Write([]byte(`{"success":true,"data":` + payload[key] + `}`))
		default:
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":102}}`))
		}
	}))
	defer srv.Close()
	t.Setenv("SYNOLOGY_BASE_URL", srv.URL)
	t.Setenv("SYNOLOGY_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_PASSWORD", pw)

	leaks := func(where, text string) {
		t.Helper()
		for _, s := range []string{sid, tok, pw} {
			if strings.Contains(text, s) || strings.Contains(dsm.Decoded(text), s) || strings.Contains(text, url.QueryEscape(s)) {
				t.Errorf("%s carries secret %q", where, s) // the text may be binary (data.db): not quoted
			}
		}
	}
	out, errOut, code := sweepExecute(t, "abb", "settings", "--json")
	if code != 0 {
		t.Fatalf("abb settings: exit %d out=%s err=%s", code, out, errOut)
	}
	leaks("abb settings stdout", out)
	if !strings.Contains(out, dsm.ResponseMask) {
		t.Errorf("abb settings shows no mask: %s", out)
	}
	out, errOut, code = sweepExecute(t, "sync", "--resources", "user", "--json")
	if code != 0 {
		t.Fatalf("sync: exit %d %s %s", code, out, errOut)
	}
	out, errOut, code = sweepExecute(t, "export", "user")
	if code != 0 || !strings.Contains(out, "alice") {
		t.Fatalf("export: exit %d out=%s err=%s", code, out, errOut)
	}
	leaks("export stdout", out)
	files := 0
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(p, "stdout") || strings.HasSuffix(p, "stderr") {
			return nil
		}
		b, _ := os.ReadFile(p)
		if strings.Contains(string(b), "alice") || strings.Contains(string(b), "session ") {
			files++
		}
		if strings.Contains(filepath.Base(p), "session") {
			return nil // the CLI's own session cache holds the sid by design
		}
		leaks(p, string(b))
		return nil
	})
	if files == 0 {
		t.Fatal("nothing was stored or cached; the check proved nothing")
	}
}
