// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"synology-pp-cli/internal/dsm/dsmtest"
)

// TestAbbTrendReportsUnknownDurationsAsNull drives the finding end to end: a
// mirror with one recent completed backup lacking time_end made
// `abb trend --since 1d --json` report median_duration_sec 0, p90 0 and
// overrun false with no note. Through the real sync and report pipeline
// (--json and --agent compaction) they are null with an unknown note.
func TestAbbTrendReportsUnknownDurationsAsNull(t *testing.T) {
	sweepIsolate(t)
	at := time.Now().Add(-3 * time.Hour).Unix()
	body := map[string]string{
		"SYNO.API.Info/query":   `{"SYNO.ActiveBackup.Task":{"path":"entry.cgi","minVersion":1,"maxVersion":2}}`,
		"SYNO.Core.System/info": `{"model":"DS1621+","firmware_ver":"DSM 7.2.2"}`,
		"SYNO.ActiveBackup.Task/list": fmt.Sprintf(`{"tasks":[{"task_id":5,"task_name":"Office PCs","backup_type":2,"next_trigger_time":%d,
  "last_result":{"result_id":590,"status":2,"job_action":1,"time_start":%d},"devices":[]}]}`, time.Now().Add(12*time.Hour).Unix(), at),
		"SYNO.ActiveBackup.Log/list_result":                    fmt.Sprintf(`{"results":[{"result_id":590,"task_id":5,"task_name":"Office PCs","status":2,"job_action":1,"time_start":%d}]}`, at),
		"SYNO.ActiveBackup.Overview/list_device_transfer_size": `{"device_list":[]}`,
		"SYNO.Storage.CGI.Storage/load_info":                   `{"volumes":[{"id":"volume_1","vol_path":"/volume1","status":"normal","size":{"total":"1000","used":"10"}}],"storagePools":[],"disks":[]}`,
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
		case key == "SYNO.API.Auth/logout":
			_, _ = w.Write([]byte(`{"success":true}`))
		case body[key] != "":
			_, _ = w.Write([]byte(`{"success":true,"data":` + body[key] + `}`))
		default:
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":102}}`))
		}
	}))
	defer srv.Close()
	t.Setenv("SYNOLOGY_ACME_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_ACME_PASSWORD", "s3cret")
	if _, errText, code := runCLIErr(t, "fleet", "add", "acme", "--url", srv.URL, "--json"); code != 0 {
		t.Fatalf("fleet add: %d %s", code, errText)
	}
	if out, errText, code := runCLIErr(t, "fleet", "sync", "--json"); code != 0 {
		t.Fatalf("fleet sync: %d %s %s", code, out, errText)
	}
	for _, mode := range []string{"--json", "--agent"} {
		out, errText, code := runCLIErr(t, "abb", "trend", "--since", "1d", mode)
		if code != 0 {
			t.Fatalf("abb trend %s: exit %d %s %s", mode, code, out, errText)
		}
		var rows []map[string]any
		if err := json.Unmarshal([]byte(out), &rows); err != nil {
			var env struct {
				Results []map[string]any `json:"results"`
			}
			if json.Unmarshal([]byte(out), &env) == nil {
				rows = env.Results
			}
		}
		if len(rows) != 1 {
			t.Fatalf("abb trend %s: %s", mode, out)
		}
		row := rows[0]
		for _, k := range []string{"median_duration_sec", "p90_duration_sec", "overrun"} {
			if v, present := row[k]; mode == "--json" && (!present || v != nil) {
				t.Errorf("abb trend %s: %s = %v (present %v), want null", mode, k, v, present)
			} else if present && v != nil {
				t.Errorf("abb trend %s: %s = %v, want null or omitted", mode, k, v)
			}
		}
		if note, _ := row["note"].(string); !strings.Contains(note, "unknown") {
			t.Errorf("abb trend %s: no unknown note: %s", mode, out)
		}
	}
}
