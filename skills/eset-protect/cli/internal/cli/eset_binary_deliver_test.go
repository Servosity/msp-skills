// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
	"eset-protect-pp-cli/internal/secrets"
)

// TestBinaryDeliverIsNotMasked (handfixes.json: binary-delivery-byte-exact):
// handleBinaryResponseDelivery unwrapped the binary envelope and handed the
// raw bytes to Deliver, which then saw no envelope and ran text masking over
// the quarantine sample. A sample holding a registered secret was rewritten
// while the receipt reported the original byte count.
func TestBinaryDeliverIsNotMasked(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	const secret = "registered-secret-value-0001"
	secrets.Register(secret)
	t.Cleanup(secrets.Reset)
	payload := append([]byte("PK\x03\x04"), []byte("sample bytes "+secret+" \\u0072\\\\u0065 end\xff\xfe")...)
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/v1/quarantined-objects:") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(payload)
	})
	file := filepath.Join(t.TempDir(), "sample.zip")
	if _, errOut, err := captureExecuteAll(t, "quarantined-objects-download", "--deliver", "file:"+file); err != nil {
		t.Fatalf("--deliver file: %v\n%s", err, errOut)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("--deliver file altered the quarantine bytes:\n got %q\nwant %q", got, payload)
	}
}
