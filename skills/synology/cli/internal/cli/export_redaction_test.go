// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"synology-pp-cli/internal/dsm/dsmtest"
)

// TestExportNeverWritesSecretFields: export --output wrote DSM answers as
// they came (user passwd, share enc_passwd), unlike the store and the
// response cache. Every exported record passes the same RedactSecretFields
// funnel: secret-named values read [redacted], everything else is kept.
func TestExportNeverWritesSecretFields(t *testing.T) {
	const sentinel = "SENTINEL-SECRET-7731"
	sweepIsolate(t)
	payload := map[string]string{
		"SYNO.Core.User/list":  `{"total":1,"offset":0,"users":[{"name":"alice","uid":1026,"passwd":"` + sentinel + `","password":"` + sentinel + `"}]}`,
		"SYNO.Core.Share/list": `{"total":1,"offset":0,"shares":[{"name":"homes","enc_passwd":"` + sentinel + `"}]}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dsmtest.ServeAPIInfo(w, r) {
			return
		}
		_ = r.ParseForm()
		key := r.Form.Get("api") + "/" + r.Form.Get("method")
		switch {
		case key == "SYNO.API.Auth/login":
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"sid-1","synotoken":"tok-1"}}`))
		case payload[key] != "":
			_, _ = w.Write([]byte(`{"success":true,"data":` + payload[key] + `}`))
		default:
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":102}}`))
		}
	}))
	defer srv.Close()
	t.Setenv("SYNOLOGY_BASE_URL", srv.URL)
	t.Setenv("SYNOLOGY_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_PASSWORD", "s3cretpw")

	dir := t.TempDir()
	for _, c := range []struct {
		args []string
		file string
		keep string
	}{
		{[]string{"export", "user", "--output", filepath.Join(dir, "users.jsonl")}, "users.jsonl", "alice"},
		{[]string{"export", "folder", "--format", "json", "--output", filepath.Join(dir, "shares.json")}, "shares.json", "homes"},
		{[]string{"export", "user", "--no-cache"}, "", "alice"},
	} {
		out, errOut, code := sweepExecute(t, c.args...)
		if code != 0 {
			t.Fatalf("%v: exit %d %s", c.args, code, errOut)
		}
		written := out
		if c.file != "" {
			b, err := os.ReadFile(filepath.Join(dir, c.file))
			if err != nil {
				t.Fatal(err)
			}
			written = string(b)
		}
		if strings.Contains(written, sentinel) || !strings.Contains(written, "[redacted]") || !strings.Contains(written, c.keep) {
			t.Fatalf("%v wrote: %s", c.args, written)
		}
	}
}
