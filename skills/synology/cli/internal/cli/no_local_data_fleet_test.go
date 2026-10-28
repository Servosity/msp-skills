// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package cli

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// TestNoLocalDataRefusesFleetSync: SYNOLOGY_NO_LOCAL_DATA=1 promises no local
// copy of API data, and fleet sync's whole job is to save every NAS's answers
// to the fleet mirror. It is refused before any NAS is contacted, and no
// mirror file appears.
func TestNoLocalDataRefusesFleetSync(t *testing.T) {
	home := sweepIsolate(t)
	var hits atomic.Int32
	nas := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer nas.Close()
	if _, errText, code := runCLIErr(t, "fleet", "add", "acme", "--url", nas.URL, "--json"); code != 0 {
		t.Fatalf("fleet add acme: %d %s", code, errText)
	}
	t.Setenv("SYNOLOGY_ACME_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_ACME_PASSWORD", "PWNOLOCAL-5521")
	t.Setenv("SYNOLOGY_NO_LOCAL_DATA", "1")
	hits.Store(0)

	_, errText, code := runCLIErr(t, "fleet", "sync", "--json")
	if code == 0 || !strings.Contains(errText, "SYNOLOGY_NO_LOCAL_DATA=1") {
		t.Fatalf("fleet sync ran with SYNOLOGY_NO_LOCAL_DATA=1: exit %d %s", code, errText)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("fleet sync contacted the NAS %d times with SYNOLOGY_NO_LOCAL_DATA=1", n)
	}
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasPrefix(filepath.Base(p), "fleet.db") {
			t.Errorf("a fleet mirror was written: %s", p)
		}
		return nil
	})
}
