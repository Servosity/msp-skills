// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
	"eset-protect-pp-cli/internal/store"
)

const (
	persistInstallerLink = "https://download.example/INSTALLER-LINK-PERSIST-SENTINEL"
	persistEnrollLink    = "https://enroll.example/ENROLL-LINK-PERSIST-SENTINEL"
	persistGPOText       = "GPO-SCCM-FILE-PERSIST-SENTINEL"
	persistInstallerUUID = "00000000-0000-0000-0000-0000000000b1"
	// persistForensicURI sits in a v2 detection's free-form
	// triggeringEvent.data: forensic data, not a credential.
	persistForensicURI = "https://evil.example/LEGIT-EVENT-URI-R3"
	persistDetectionV2 = "00000000-0000-0000-0000-0000000000d2"
)

func persistFakeESET(t *testing.T) {
	t.Helper()
	installer := `{"uuid":"` + persistInstallerUUID + `","displayName":"Acme agent","downloadUrl":"` + persistInstallerLink + `","operatingSystemFamilyId":1}`
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/installers" && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"installers":[`+installer+`],"nextPageToken":"","totalSize":1}`)
		case r.URL.Path == "/v1/installers" && r.Method == http.MethodPost:
			_, _ = io.WriteString(w, `{"installer":`+installer+`}`)
		case r.URL.Path == "/v1/installers/"+persistInstallerUUID:
			_, _ = io.WriteString(w, `{"installer":`+installer+`}`)
		case r.URL.Path == "/v1/gpo-sccm-files:generate":
			_, _ = io.WriteString(w, `{"gpoSccmFile":"`+base64.StdEncoding.EncodeToString([]byte(persistGPOText))+`"}`)
		case r.URL.Path == "/v1/mobile-devices:batchGetEnrollmentLinks":
			_, _ = io.WriteString(w, `{"enrollmentLinks":{"00000000-0000-0000-0000-000000000001":{"uri":"`+persistEnrollLink+`","expireTime":"2026-10-20T00:00:00Z"}}}`)
		case r.URL.Path == "/v2/detections":
			_, _ = io.WriteString(w, `{"detections":[{"uuid":"`+persistDetectionV2+`","displayName":"Web threat blocked","triggeringEvent":{"type":"web","data":{"processName":"chrome.exe","uri":"`+persistForensicURI+`"}}}],"nextPageToken":""}`)
		case r.URL.Path == "/v1/devices":
			_, _ = io.WriteString(w, `{"devices":[{"uuid":"00000000-0000-0000-0000-000000000001","displayName":"FRONTDESK-01"}],"nextPageToken":""}`)
		default:
			_, _ = io.WriteString(w, `{"nextPageToken":""}`)
		}
	})
}

// TestCredentialTierNeverPersisted guards credential-tier-never-persisted end
// to end: every command that receives a Credential-tier value (installers
// list and get with the default --data-source auto, in JSON and plain;
// installers create; sync --resources installers; the GPO/SCCM file; mobile
// enrollment links) prints it live (export installers prints the marker), and after each one
// no byte under --home (the SQLite mirror and its WAL, the response cache)
// holds it. The mirror still keeps the installer row, with the marker.
func TestCredentialTierNeverPersisted(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	persistFakeESET(t)
	home := t.TempDir()
	gpoB64 := base64.StdEncoding.EncodeToString([]byte(persistGPOText))
	sentinels := []string{persistInstallerLink, persistEnrollLink, gpoB64, persistGPOText}
	for _, step := range []struct {
		argv []string
		live string
	}{
		{[]string{"installers", "list", "--json"}, persistInstallerLink},
		{[]string{"installers", "list", "--plain"}, persistInstallerLink},
		{[]string{"installers", "list", "--csv"}, persistInstallerLink},
		{[]string{"installers", "get", persistInstallerUUID, "--json"}, persistInstallerLink},
		{[]string{"installers", "list", "--json", "--no-cache"}, persistInstallerLink},
		{[]string{"installers", "create", "--installer-display-name", "x", "--installer-operating-system-family-id", "1", "--json", "--yes"}, persistInstallerLink},
		{[]string{"sync", "--resources", "installers", "--json"}, ""},
		{[]string{"sync", "--resources", "installers,devices", "--full", "--json"}, ""},
		// export-redacts-credential-tier: an export is a file on disk, so
		// it carries the marker, never the link, even on stdout.
		{[]string{"export", "installers", "--format", "jsonl"}, `"downloadUrl":"[redacted]"`},
		{[]string{"gpo-sccm-files-generate", "--json", "--yes"}, gpoB64},
		{[]string{"mobile-devices-batch-get-enrollment-links", "--device-uuids", "00000000-0000-0000-0000-000000000001", "--json", "--yes"}, persistEnrollLink},
		{[]string{"installers", "list", "--json"}, persistInstallerLink},
		{[]string{"sync", "--resources", "detections_v2", "--json"}, ""},
		{[]string{"search", "Web threat blocked", "--data-source", "local", "--json"}, persistForensicURI},
	} {
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(append(step.argv, "--rate-limit", "0", "--home", home))
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v\n%s", step.argv, err, out.String())
		}
		if step.live != "" && !strings.Contains(out.String(), step.live) {
			t.Fatalf("%v: live output must still carry the Credential-tier value: %s", step.argv, out.String())
		}
		for _, s := range sentinels {
			if hits := homeHolds(t, home, s); len(hits) > 0 {
				t.Fatalf("%v wrote a Credential-tier value to disk (%s): %v", step.argv, s, hits)
			}
		}
	}
	// The mirror still holds the installer row, link replaced.
	db, err := store.OpenWithContext(t.Context(), filepath.Join(home, "data", filepath.Base(defaultDBPath("eset-protect-cli"))))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	row, err := db.Get("installers", persistInstallerUUID)
	if err != nil {
		t.Fatalf("the installer row must stay in the mirror: %v", err)
	}
	if !strings.Contains(string(row), `"downloadUrl":"[redacted]"`) || !strings.Contains(string(row), "Acme agent") {
		t.Fatalf("mirrored installer row: %s", row)
	}
	// Round 3 (adversarial P2): redaction is scoped by schema, so the v2
	// detection's triggeringEvent.data.uri is in the mirror as ESET sent it.
	det, err := db.Get("detections_v2", persistDetectionV2)
	if err != nil || !strings.Contains(string(det), persistForensicURI) {
		t.Fatalf("the v2 detection must keep its triggeringEvent URI in the mirror: %s %v", det, err)
	}
}
