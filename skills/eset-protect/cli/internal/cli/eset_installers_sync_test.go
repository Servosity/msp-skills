// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

const installerLinkSentinel = "https://download.example/INSTALLER-URL-SENTINEL"

// homeHolds reports whether any file under dir contains needle.
func homeHolds(t *testing.T, dir, needle string) []string {
	t.Helper()
	var hits []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if b, rerr := os.ReadFile(path); rerr == nil && bytes.Contains(b, []byte(needle)) {
			hits = append(hits, path)
		}
		return nil
	})
	return hits
}

// TestDefaultSyncLeavesInstallerLinksOut guards installers-not-in-default-sync:
// installer rows carry downloadUrl, a Credential-tier link that installs and
// enrolls a device, so neither the default sync nor workflow archive fetch
// them, while `sync --resources installers` still mirrors the rows (with the
// link replaced, so it never reaches disk).
func TestDefaultSyncLeavesInstallerLinksOut(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	var mu sync.Mutex
	var paths []string
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/installers" {
			_, _ = io.WriteString(w, `{"installers":[{"uuid":"00000000-0000-0000-0000-0000000000b1","displayName":"Acme agent","downloadUrl":"`+installerLinkSentinel+`"}],"nextPageToken":""}`)
			return
		}
		_, _ = io.WriteString(w, `{"nextPageToken":""}`)
	})
	if slices.Contains(defaultSyncResources(), "installers") {
		t.Fatal("installers is in the default sync list")
	}
	if !slices.Contains(knownSyncResourceNames(), "installers") {
		t.Fatal("installers must stay syncable by name")
	}
	run := func(home string, args ...string) {
		cmd := RootCmd()
		var out lockedBuffer // sync workers write concurrently
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(append(args, "--json", "--rate-limit", "0", "--home", home))
		_ = cmd.Execute()
	}
	for _, argv := range [][]string{{"sync"}, {"workflow", "archive"}} {
		home := t.TempDir()
		mu.Lock()
		paths = nil
		mu.Unlock()
		run(home, argv...)
		mu.Lock()
		got := append([]string(nil), paths...)
		mu.Unlock()
		if !slices.Contains(got, "/v1/devices") {
			t.Fatalf("%v did not sync devices, so the check proves nothing: %v", argv, got)
		}
		if slices.Contains(got, "/v1/installers") {
			t.Fatalf("%v fetched /v1/installers by default", argv)
		}
		if hits := homeHolds(t, home, installerLinkSentinel); len(hits) > 0 {
			t.Fatalf("%v stored an installer download link: %v", argv, hits)
		}
	}
	// Named explicitly, installers still sync: the row is mirrored, but its
	// link is not (credential-tier-never-persisted).
	home := t.TempDir()
	run(home, "sync", "--resources", "installers")
	if hits := homeHolds(t, home, "Acme agent"); len(hits) == 0 {
		t.Fatal("sync --resources installers no longer mirrors installers")
	}
	if hits := homeHolds(t, home, installerLinkSentinel); len(hits) > 0 {
		t.Fatalf("sync --resources installers stored the download link: %v", hits)
	}
}
