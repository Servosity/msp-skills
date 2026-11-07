// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"strings"
	"testing"

	"afi-pp-cli/internal/syncer"

	"github.com/spf13/cobra"
)

// afi handfix fleet-sync-snapshot-truth: a report over the store names every
// list the last fleet-sync could not read in full, fleet-wide ones always and a
// tenant's own when that tenant is read, so "every resource is protected" over
// a failed protections read never reads as complete.
func TestEveryStoreReportNamesTheIncompleteSync(t *testing.T) {
	db, path := newTestStore(t)
	seed(t, db, "tenants", "ten1", `{"id":"ten1","name":"Contoso","org_id":"org1"}`)
	seed(t, db, "resources", "res1", `{"id":"res1","tenant_id":"ten1","name":"Jane","kind":"user","external_id":"x1"}`)
	seed(t, db, "protections", "prot1", `{"id":"prot1","resource_id":"res1","policy_id":"pol1","tenant_id":"ten1"}`)
	seed(t, db, syncer.GapResourceType, "ten1:protections", `{"gap_scope":"ten1","tenant_id":"ten1","list":"protections","reason":"429 too many requests"}`)
	seed(t, db, syncer.GapResourceType, "ten2:archives", `{"gap_scope":"ten2","tenant_id":"ten2","list":"archives","reason":"503 unavailable"}`)
	seed(t, db, syncer.GapResourceType, "fleet:org org1 tenants", `{"gap_scope":"fleet","list":"org org1 tenants","reason":"stopped at --max-pages=50 with more pages remaining"}`)

	type report struct {
		Note       string   `json:"note"`
		Incomplete []string `json:"incomplete"`
	}
	for _, tc := range []struct {
		name string
		cmd  func(*rootFlags) *cobra.Command
		args []string
		want int // ten1 + fleet = 2; every tenant = 3
	}{
		{"coverage-gaps", newNovelCoverageGapsCmd, nil, 3},
		{"coverage-gaps --tenant", newNovelCoverageGapsCmd, []string{"--tenant", "ten1"}, 2},
		{"fleet-health", newNovelFleetHealthCmd, nil, 3},
		{"backup-stale --tenant", newNovelBackupStaleCmd, []string{"--tenant", "ten1"}, 2},
		{"reconcile-licenses", newNovelReconcileLicensesCmd, nil, 3},
		{"tenant-scorecard", newNovelTenantScorecardCmd, []string{"ten1"}, 2},
		{"resolve", newNovelResolveCmd, []string{"x1"}, 3},
	} {
		out, err := runNovel(t, tc.cmd, append([]string{"--db", path}, tc.args...)...)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		view := decodeView[report](t, out)
		if len(view.Incomplete) != tc.want || !strings.Contains(view.Note, "did not read") {
			t.Errorf("%s: incomplete %v, note %q (want %d gaps named)", tc.name, view.Incomplete, view.Note, tc.want)
		}
		if strings.Contains(strings.Join(view.Incomplete, "\n"), "ten2") && tc.want == 2 {
			t.Errorf("%s: another tenant's gap named: %v", tc.name, view.Incomplete)
		}
	}
}
