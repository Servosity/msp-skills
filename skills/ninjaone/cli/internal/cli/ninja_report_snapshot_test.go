// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"ninjaone-pp-cli/internal/cliutil/testenv"
	"ninjaone-pp-cli/internal/store"
)

// Class report-row-identity: a /v2/queries/* report is a snapshot. Every complete,
// unfiltered walk from page one (`sync --full` or an ordinary `sync`) replaces the
// partition, so a row NinjaOne no longer returns (an uninstalled title, the version an
// upgrade replaced, a row stored under an earlier key) does not stay in software-audit's
// answer, and no row stored under the shipped key (deviceId + name) stays beside its stamped
// successor, so an upgraded store never holds one install twice. Software rows are keyed by
// position, so --no-prune does not apply to them (a whole walk replaces them anyway) and a
// filtered walk is refused before any request, storing nothing.
func TestFullSyncReplacesAReportSnapshot(t *testing.T) {
	current := `{"deviceId":7,"name":"Acme Agent","version":"2","productCode":"A"}`
	for _, tc := range []struct {
		name    string
		args    []string
		want    int
		refused bool
	}{
		{"full", []string{"--full"}, 1, false},
		{"ordinary", nil, 1, false},
		{"full-no-prune-does-not-apply", []string{"--full", "--no-prune"}, 1, false},
		{"ordinary-no-prune-does-not-apply", []string{"--no-prune"}, 1, false},
		{"filtered-full-is-refused", []string{"--full", "--resource-param", "queries-software:df=org=1"}, 3, true},
		{"filtered-ordinary-is-refused", []string{"--resource-param", "queries-software:df=org=1"}, 3, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v2/queries/software" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"cursor":{"name":"C","offset":0,"count":1},"results":[%s]}`, current)
			}))
			defer srv.Close()
			dbPath := filepath.Join(t.TempDir(), "data.db")
			db, err := store.OpenWithContext(context.Background(), dbPath)
			if err != nil {
				t.Fatal(err)
			}
			for id, row := range map[string]string{
				"7\x00Acme Agent":                               `{"deviceId":7,"name":"Acme Agent"}`,            // shipped key (deviceId + name)
				"7\x00Old App\x00version=1":                     `{"deviceId":7,"name":"Old App","version":"1"}`, // keys no current row computes
				"7\x00Acme Agent\x00version=1\x00productCode=A": `{"deviceId":7,"name":"Acme Agent","version":"1","productCode":"A"}`,
			} {
				if err := db.Upsert("queries-software", id, json.RawMessage(row)); err != nil {
					t.Fatal(err)
				}
			}
			db.Close()
			if tc.refused {
				stdout, stderr, err := runReportCmd(t, srv.URL, append([]string{"sync", "--db", dbPath, "--json", "--resources", "queries-software"}, tc.args...)...)
				if err == nil || !strings.Contains(stdout+stderr, "syncs only whole") {
					t.Fatalf("a filtered sync of an ordinal report was not refused (err %v):\n%s%s", err, stdout, stderr)
				}
			} else {
				runNinjaSyncRaw(t, srv.URL, dbPath, append([]string{"--resources", "queries-software"}, tc.args...)...)
			}
			if n := storedRows(t, dbPath, "queries-software"); n != tc.want {
				t.Fatalf("queries-software holds %d rows after %v, want %d", n, tc.args, tc.want)
			}
		})
	}
}
