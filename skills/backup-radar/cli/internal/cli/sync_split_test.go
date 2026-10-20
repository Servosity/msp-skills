// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"reflect"
	"testing"
	"time"

	"backup-radar-pp-cli/internal/client"
	"backup-radar-pp-cli/internal/cliutil"
	"backup-radar-pp-cli/internal/config"
)

// HAND-FIX(sync-skips-keyless-tier): exactly one key skips the other
// generation; unknown names stay (they fail as unknown); a refused stored v3
// credential counts as present so the sync still fails loudly.
func TestSyncSplitByKey(t *testing.T) {
	all := []string{"backups", "devices", "health", "devcies"}
	v3 := client.New(&config.Config{BackupRadarApiKey: "k3"}, time.Second, 0)
	none := client.New(&config.Config{}, time.Second, 0)
	refused := client.New(&config.Config{CredentialRefusals: []cliutil.CredentialRefusal{{}}}, time.Second, 0)

	t.Setenv("BACKUP_RADAR_V2_API_KEY", "")
	if kept, skipped, need := syncSplitByKey(v3, all); !reflect.DeepEqual(kept, []string{"devices", "health", "devcies"}) || !reflect.DeepEqual(skipped, []string{"backups"}) || need != "BACKUP_RADAR_V2_API_KEY" {
		t.Errorf("v3 only: kept=%v skipped=%v need=%q", kept, skipped, need)
	}
	if kept, skipped, _ := syncSplitByKey(none, all); !reflect.DeepEqual(kept, all) || skipped != nil {
		t.Errorf("no keys must not skip (the missing-credential error explains): kept=%v skipped=%v", kept, skipped)
	}

	t.Setenv("BACKUP_RADAR_V2_API_KEY", "k2")
	if kept, skipped, need := syncSplitByKey(none, all); !reflect.DeepEqual(kept, []string{"backups", "devcies"}) || !reflect.DeepEqual(skipped, []string{"devices", "health"}) || need != "BACKUP_RADAR_API_KEY" {
		t.Errorf("v2 only: kept=%v skipped=%v need=%q", kept, skipped, need)
	}
	if kept, skipped, _ := syncSplitByKey(refused, all); !reflect.DeepEqual(kept, all) || skipped != nil {
		t.Errorf("a refused stored v3 key must still run and fail: kept=%v skipped=%v", kept, skipped)
	}
}
