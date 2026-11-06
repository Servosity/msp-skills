// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"connectsecure-pp-cli/internal/store"
)

// Hand-fix (handfixes.json: report-queries-own-partition), 4.35.1 reprint: the
// press renamed the per-asset registry dependent, so the registry command must
// read whatever partition sync's dependent for that path writes, not a fixed name.
func TestRegistryCommandReadsTheSyncDependentPartition(t *testing.T) {
	const path = "/r/report_queries/registry_problems_remediation_asset_details"
	name := ""
	for _, dep := range dependentResourceDefs() {
		if dep.PathTemplate == path {
			name = dep.Name
		}
	}
	if name == "" {
		t.Fatalf("no sync dependent for %s", path)
	}
	src, err := os.ReadFile("report_queries_registry-problems-remediation-asset-details.go")
	if err != nil {
		t.Fatal(err)
	}
	if want := `"auto", "` + name + `", path`; !strings.Contains(string(src), want) {
		t.Fatalf("registry command does not read the dependent's partition: want %s", want)
	}
}

// csTwoPCGetter answers with two clients' AD computers that share a name and carry no id.
type csTwoPCGetter struct{}

func (csTwoPCGetter) Get(context.Context, string, map[string]string) (json.RawMessage, error) {
	return json.RawMessage(`{"status":true,"data":[{"name":"RECEPTION","company_id":1,"object_guid":"g-pc-1"},{"name":"RECEPTION","company_id":2,"object_guid":"g-pc-2"}]}`), nil
}
func (csTwoPCGetter) RateLimit() float64 { return 0 }

// Hand-fix (handfixes.json: per-client-report-keys), 4.35.1 reprint: the flat
// report_queries resource now syncs /report_queries/ad_computers_view, whose rows
// have no id; keyed by name, two clients' RECEPTION collapsed into one row.
func TestFlatReportQueriesKeepsEveryClientsSameNamedComputer(t *testing.T) {
	prev := humanFriendly
	humanFriendly = false
	defer func() { humanFriendly = prev }()
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var events bytes.Buffer
	if res := syncResource(context.Background(), csTwoPCGetter{}, db, "report_queries", "", true, 0, false, true, nil, &events); res.Err != nil {
		t.Fatal(res.Err)
	}
	if n, _ := db.Count("report_queries"); n != 2 {
		t.Fatalf("stored %d AD computer rows, want 2 (events %s)", n, events.String())
	}
}
