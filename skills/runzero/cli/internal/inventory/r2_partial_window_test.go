// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package inventory

import (
	"context"
	"testing"
)

// Class unrefreshed-entity-read-as-new (adversarial review of the 4.34.2
// reprint, executed on the built binary): after a full sync, a sync of only
// assets and services left vulnerabilities and software with one refresh,
// the baseline run itself, and diff and exposure-delta read every row of that
// one refresh as first seen, so unchanged findings and software were reported
// as new. An entity whose latest refresh is not newer than the baseline run
// changed nothing in the window: it reports no rows and is named stale.
func TestR2UnrefreshedEntityIsNotNew(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	asset := `[` + r1Asset("a1", "web01", "10.0.0.5", "Ubuntu") + `]`
	svc := `[` + r1Service("a1", "10.0.0.5", "svc-22", 22, "ssh") + `]`
	r1Sync(t, db, map[string]string{
		"/export/org/assets.json":          asset,
		"/export/org/services.json":        svc,
		"/export/org/vulnerabilities.json": `[` + r1Vuln("a1", "v-1", "CVE-2024-3094", "Critical") + `]`,
		"/export/org/software.json":        `[` + r1Software("a1", "sw-1", "openssl", "3.0.1") + `]`,
	}, nil, "")
	r1Sync(t, db, map[string]string{"/export/org/assets.json": asset, "/export/org/services.json": svc},
		[]string{"assets", "services"}, "")

	d, err := ExposureDelta(ctx, db, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.NewVulns) != 0 || len(d.NewServices) != 0 {
		t.Errorf("exposure-delta new vulns %+v, new services %+v; want none (nothing changed)", d.NewVulns, d.NewServices)
	}
	diff, err := Diff(ctx, db, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Software.Added) != 0 || len(diff.Assets.Added) != 0 || len(diff.Services.Added) != 0 {
		t.Errorf("diff added assets %v services %v software %v; want none", diff.Assets.Added, diff.Services.Added, diff.Software.Added)
	}
	if !sameSet(diff.StaleEntities, "software") {
		t.Errorf("diff stale = %v, want [software]", diff.StaleEntities)
	}
}
