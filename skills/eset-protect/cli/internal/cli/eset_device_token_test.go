// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

const (
	deviceTokenSentinel = "XX-DEVICE-TOKEN-SENTINEL-XX"
	tokenTestDevice     = "00000000-0000-0000-0000-000000000001"
	tokenTestGroup      = "00000000-0000-0000-0000-0000000000a1"
)

// deviceTokenFakeESET serves every device-returning ESET Connect endpoint
// with a device that carries deviceToken = deviceTokenSentinel.
func deviceTokenFakeESET(t *testing.T) {
	t.Helper()
	device := func() map[string]any {
		return map[string]any{"uuid": tokenTestDevice, "displayName": "FRONTDESK-01", "parentGroupUuid": tokenTestGroup, "deviceToken": deviceTokenSentinel, "lastSyncTime": "2026-10-18T10:00:00Z"}
	}
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		var body any
		switch r.URL.Path {
		case "/v1/devices", "/v1/device_groups/" + tokenTestGroup + "/devices":
			body = map[string]any{"devices": []any{device()}, "nextPageToken": ""}
		case "/v1/devices:batchGet":
			body = map[string]any{"devices": []any{device()}}
		case "/v1/devices:batchImport":
			body = map[string]any{"devices": []any{device()}, "errors": []any{}}
		case "/v1/devices/" + tokenTestDevice:
			body = map[string]any{"device": device()}
		case "/v1/device_groups":
			body = map[string]any{"deviceGroups": []any{map[string]any{"uuid": tokenTestGroup, "displayName": "Acme", "linkedEntityType": "LINKED_ENTITY_TYPE_COMPANY"}}, "nextPageToken": ""}
		default:
			body = map[string]any{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	})
}

func runTokenRoot(t *testing.T, args ...string) string {
	t.Helper()
	cmd := RootCmd()
	var out, errOut lockedBuffer // sync workers write concurrently
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v: %v\n%s%s", args, err, out.String(), errOut.String())
	}
	return out.String() + errOut.String()
}

// filesHolding lists every file under root whose raw bytes contain needle
// (the SQLite database, its WAL, the response cache, config).
func filesHolding(t *testing.T, root, needle string) []string {
	t.Helper()
	var hits []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		if b, err := io.ReadAll(f); err == nil && bytes.Contains(b, []byte(needle)) {
			hits = append(hits, path)
		}
		return nil
	})
	return hits
}

// TestDeviceTokenNeverStoredOrPrinted: deviceToken is a per-device credential
// (handfixes.json: device-token-redacted-at-decode). It must not land in the
// mirror (resources, typed tables, FTS) or in any command's output.
func TestDeviceTokenNeverStoredOrPrinted(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	deviceTokenFakeESET(t)
	home := t.TempDir()

	runTokenRoot(t, "--home", home, "sync", "--resources", "devices,device-groups", "--json")
	if hits := filesHolding(t, home, deviceTokenSentinel); len(hits) != 0 {
		t.Fatalf("sync wrote the device token to disk: %v", hits)
	}

	for _, args := range [][]string{
		{"devices", "list", "--json"},
		{"devices", "list", "--json", "--data-source", "live"},
		{"devices", "list", "--json", "--data-source", "local"},
		{"devices", "get", tokenTestDevice, "--json"},
		{"devices-batch-get", "--devices-uuids", tokenTestDevice, "--json"},
		{"device-groups", "devices", "groups-list-member", tokenTestGroup, "--json"},
		{"devices-batch-import", "--devices", "[]", "--parent-group-uuid", tokenTestGroup, "--json"},
		{"devices", "profile", "FRONTDESK-01", "--json"},
		{"search", "FRONTDESK", "--data-source", "local", "--json"},
	} {
		out := runTokenRoot(t, append([]string{"--home", home}, args...)...)
		if strings.Contains(out, deviceTokenSentinel) {
			t.Errorf("%v printed the device token:\n%s", args, out)
		}
		if strings.Contains(out, "FRONTDESK-01") || strings.Contains(out, tokenTestDevice) {
			continue
		}
		t.Errorf("%v returned no device at all, so the check proves nothing:\n%s", args, out)
	}
	if hits := filesHolding(t, home, deviceTokenSentinel); len(hits) != 0 {
		t.Fatalf("a read command cached the device token on disk: %v", hits)
	}
}
