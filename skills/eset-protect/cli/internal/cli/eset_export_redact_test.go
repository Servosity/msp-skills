// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// TestExportRedactsCredentialTier (handfixes.json: export-redacts-credential-tier):
// `export installers --output <file>` wrote each installer's enrollment
// downloadUrl to disk unredacted, although the mirror and the cache redact it
// and governance.md says those links are never written to disk.
func TestExportRedactsCredentialTier(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"installers":[{"uuid":"00000000-0000-0000-0000-000000000001","displayName":"Acme Agent","downloadUrl":"https://x.invalid/enroll?tok=SENTINELLINK"}],"nextPageToken":""}`)
	})
	for _, format := range []string{"jsonl", "json"} {
		file := filepath.Join(t.TempDir(), "installers."+format)
		if _, errOut, err := captureExecuteAll(t, "export", "installers", "--format", format, "--output", file); err != nil {
			t.Fatalf("export --format %s: %v\n%s", format, err, errOut)
		}
		got, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(got, []byte("SENTINELLINK")) {
			t.Fatalf("export --format %s wrote the enrollment link to disk: %s", format, got)
		}
		if !bytes.Contains(got, []byte("[redacted]")) || !bytes.Contains(got, []byte("Acme Agent")) {
			t.Fatalf("export --format %s lost the row or the redaction marker: %s", format, got)
		}
	}
}
