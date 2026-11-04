// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"ninjaone-pp-cli/internal/cliutil/testenv"
)

// Hand-fix (#496): the custom-fields reports declare updatedAfter as a string,
// and the 4.34 reprint sent the RFC3339 sync checkpoint in it. NinjaOne parses
// it as a number and answers HTTP 400 ("For input string"), so every
// incremental sync failed those four reports. They must sync whole.
func TestNinjaCustomFieldsReportsSyncWholeOnEveryRun(t *testing.T) {
	testenv.Isolate(t)
	f := &ninjaFake{}
	const total = 3
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		if v := r.URL.Query().Get("updatedAfter"); v != "" {
			if _, err := strconv.ParseFloat(v, 64); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, `{"resultCode":"FAILURE","errorMessage":"For input string: \"%s\""}`, v)
				return
			}
		}
		// Row shapes per spec.yaml: NodeAttributes, NodeAttributesDetailed (entityId +
		// entityType + a fields array; deviceId is writeOnly), ScopedAttributes(Detailed).
		row := map[string]string{
			"/v2/queries/custom-fields":                 `{"deviceId":%d,"fields":{"AssetTag":"A-1"}}`,
			"/v2/queries/custom-fields-detailed":        `{"entityId":%d,"entityType":"NODE","fields":[{"name":"AssetTag","value":"A-1"}]}`,
			"/v2/queries/scoped-custom-fields":          `{"scope":"NODE","entityId":%d,"fields":{"AssetTag":"A-1"}}`,
			"/v2/queries/scoped-custom-fields-detailed": `{"scope":"NODE","entityId":%d,"fields":[{"name":"AssetTag","value":"A-1"}]}`,
		}[r.URL.Path]
		if row == "" {
			http.NotFound(w, r)
			return
		}
		var rows []string
		for i := 1; i <= total; i++ {
			rows = append(rows, fmt.Sprintf(row, i))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"cursor":{"name":"SESSION_TOKEN","offset":0,"count":"%d","expires":"2026-01-01T00:00:00Z"},"results":[%s]}`,
			total, strings.Join(rows, ","))
	}))
	defer srv.Close()

	resources := []string{
		"queries-custom-fields", "queries-custom-fields-detailed",
		"queries-scoped-custom-fields", "queries-scoped-custom-fields-detailed",
	}
	dbPath := filepath.Join(t.TempDir(), "data.db")
	// Run 1 writes the checkpoint; run 2 is the incremental sync that failed.
	for run := 1; run <= 2; run++ {
		runNinjaSyncRaw(t, srv.URL, dbPath, "--resources", strings.Join(resources, ","))
	}
	if f.count() < 2*len(resources) {
		t.Fatalf("expected both runs to request every report, got %v", f.requests)
	}
	for _, req := range f.requests {
		if strings.Contains(req, "updatedAfter=") {
			t.Fatalf("sync sent a watermark NinjaOne rejects: %s", req)
		}
	}
	for _, res := range resources {
		if n := storedRows(t, dbPath, res); n != total {
			t.Errorf("%s: mirror holds %d rows after the second sync, want %d", res, n, total)
		}
	}
}
