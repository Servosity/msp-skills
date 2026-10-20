// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "secret-fields-never-persisted" entry.

package cli

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"synology-pp-cli/internal/dsm/dsmtest"
)

// TestSyncAndLiveReadsNeverPersistSecretFields drives the real pipeline: sync
// stores folder/group/log/package/user rows in data.db and live reads write
// through to it and to the HTTP response cache. A NAS answer carrying a
// credential-named field (passwd, enc_passwd, repo_password, smtp_password)
// must not reach any file under the CLI's home; the live output still shows
// what DSM returned.
func TestSyncAndLiveReadsNeverPersistSecretFields(t *testing.T) {
	const sentinel = "SENTINEL-SECRET-7731"
	home := sweepIsolate(t)
	payload := map[string]string{
		"SYNO.Core.User/list":            `{"total":1,"users":[{"name":"alice","uid":1026,"passwd":"` + sentinel + `"}]}`,
		"SYNO.Core.Share/list":           `{"total":1,"shares":[{"name":"ActiveBackupforBusiness","enc_passwd":"` + sentinel + `"}]}`,
		"SYNO.Core.Group/list":           `{"total":1,"groups":[{"name":"administrators","gid":101}]}`,
		"SYNO.Core.Package/list":         `{"total":1,"packages":[{"id":"ActiveBackup","name":"Active Backup for Business"}]}`,
		"SYNO.Backup.Task/list":          `{"task_list":[{"task_id":1,"name":"Offsite","repo_password":"` + sentinel + `"}]}`,
		"SYNO.ActiveBackup.Setting/list": `{"settings":{"smtp_password":"` + sentinel + `"}}`,
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

	if _, errOut, code := sweepExecute(t, "sync", "--resources", "folder,group,package,user", "--json"); code != 0 {
		t.Fatalf("sync exited %d: %s", code, errOut)
	}
	for _, args := range [][]string{{"hyperbackup", "tasks", "--json"}, {"abb", "settings", "--json"}} {
		out, errOut, code := sweepExecute(t, args...)
		if code != 0 {
			t.Fatalf("%v exited %d: %s", args, code, errOut)
		}
		if !strings.Contains(out, sentinel) {
			t.Fatalf("%v live output no longer shows what DSM returned: %s", args, out)
		}
	}
	found := 0
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(p)
		if strings.Contains(string(b), sentinel) {
			t.Errorf("%s carries a NAS secret", p)
		}
		if strings.Contains(string(b), "alice") {
			found++
		}
		return nil
	})
	if found == 0 {
		t.Fatal("sync stored nothing; the check proved nothing")
	}
}
