// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophos-central-pp-cli/internal/cliutil"
	"sophos-central-pp-cli/internal/cliutil/testenv"
)

// SOPHOS_CENTRAL_NO_LOCAL_DATA=1 keeps no local copy of API responses. fleet
// refresh copies tenants, endpoints, alerts, health and licences into the
// store, so it is refused like sync, before any request and before the store
// file exists. --dry-run still previews.
func TestNoLocalDataRefusesFleetRefresh(t *testing.T) {
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv(noLocalDataEnvVar, "1")
	t.Setenv("SOPHOS_CENTRAL_CLIENT_ID", "cid")
	t.Setenv("SOPHOS_CENTRAL_CLIENT_SECRET", "secret")
	t.Setenv("SOPHOS_CENTRAL_BASE_URL", "http://127.0.0.1:1")
	t.Setenv("SOPHOS_CENTRAL_TOKEN_URL", "http://127.0.0.1:1/token")
	db := filepath.Join(t.TempDir(), "fleet.db")
	_, err := runRoot(t, "fleet", "refresh", "--db", db)
	if err == nil || !strings.Contains(err.Error(), noLocalDataEnvVar) {
		t.Fatalf("fleet refresh with %s=1: want a refusal naming the switch, got %v", noLocalDataEnvVar, err)
	}
	if _, serr := os.Stat(db); !os.IsNotExist(serr) {
		t.Fatalf("fleet refresh created the store while %s=1 (stat err %v)", noLocalDataEnvVar, serr)
	}
	if _, err := runRoot(t, "fleet", "refresh", "--db", db, "--dry-run"); err != nil {
		t.Fatalf("--dry-run must still preview: %v", err)
	}
}
