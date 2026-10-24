// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/cipp/handfixes.json under the
// "fanout-save-honors-no-local-data" entry.

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFanoutSaveRefusedWithNoLocalData: CIPP_NO_LOCAL_DATA=1 promises no local
// copy of API data, so fanout --save must refuse before it opens the store.
func TestFanoutSaveRefusedWithNoLocalData(t *testing.T) {
	_, db := startFakeCIPP(t, map[string][]string{
		"/ListUsers": {`[{"id":"u1","userPrincipalName":"a@t.example"}]`},
	})
	t.Setenv("CIPP_NO_LOCAL_DATA", "1")
	_, err := runCLI(t, "fanout", "--endpoint", "/ListUsers", "--tenant", "t.example", "--save", "--db", db, "--no-cache")
	if err == nil || !strings.Contains(err.Error(), "CIPP_NO_LOCAL_DATA") {
		t.Fatalf("fanout --save ran with CIPP_NO_LOCAL_DATA=1: err=%v", err)
	}
	if _, statErr := os.Stat(db); statErr == nil {
		t.Fatalf("fanout --save wrote %s with CIPP_NO_LOCAL_DATA=1", filepath.Base(db))
	}
	if _, err := runCLI(t, "fanout", "--endpoint", "/ListUsers", "--tenant", "t.example", "--resume", "--no-cache"); err == nil || !strings.Contains(err.Error(), "CIPP_NO_LOCAL_DATA") {
		t.Fatalf("fanout --resume ran with CIPP_NO_LOCAL_DATA=1: err=%v", err)
	}
	// A live read without --save still works.
	if _, err := runCLI(t, "fanout", "--endpoint", "/ListUsers", "--tenant", "t.example", "--no-cache"); err != nil {
		t.Fatalf("a live fanout without --save must still run: %v", err)
	}
}
