// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"bytes"
	"strings"
	"testing"
)

const persistLinkSentinel = "PERSIST-LINK-SENTINEL"

// TestRedactForPersistenceCoversEveryCredentialShape pins the rule behind
// credential-tier-never-persisted: every Credential-tier value ESET returns
// (installer downloadUrl, the GPO/SCCM file, a mobile enrollment link uri,
// deviceToken) is replaced at any depth, in any key case, through \u
// escapes and duplicate keys; a body that is not one JSON value is not
// persisted at all; a body without one comes back byte for byte.
func TestRedactForPersistenceCoversEveryCredentialShape(t *testing.T) {
	for _, c := range []struct{ name, scope, body string }{
		{"installer list", "installers", `{"installers":[{"uuid":"b1","displayName":"Acme","downloadUrl":"https://download.example/` + persistLinkSentinel + `"}],"nextPageToken":""}`},
		{"installer row", "installers", `{"uuid":"b1","downloadUrl":"https://download.example/` + persistLinkSentinel + `"}`},
		{"installer create", "https://{region}.installer-management.eset.systems/v1/installers", `{"installer":{"uuid":"b1","downloadUrl":"https://download.example/` + persistLinkSentinel + `"}}`},
		{"gpo file", "gpo-sccm-files-generate", `{"gpoSccmFile":"` + persistLinkSentinel + `"}`},
		{"enrollment map", "mobile-devices-batch-get-enrollment-links", `{"enrollmentLinks":{"00000000-0000-0000-0000-000000000001":{"uri":"https://enroll.example/` + persistLinkSentinel + `","expireTime":"2026-10-20T00:00:00Z"}}}`},
		{"enrollment row", "mobile_devices_batch_get_enrollment_links", `{"id":"00000000-0000-0000-0000-000000000001","uri":"https://enroll.example/` + persistLinkSentinel + `"}`},
		{"device token", "devices", `{"uuid":"d1","deviceToken":"` + persistLinkSentinel + `"}`},
		{"key case", "installers", `{"DownloadURL":"https://download.example/` + persistLinkSentinel + `"}`},
		{"escaped key", "installers", `{"download\u0055rl":"https://download.example/` + persistLinkSentinel + `"}`},
		{"kelvin key", "devices", `{"deviceTo\u212aen":"` + persistLinkSentinel + `"}`},
		{"literal kelvin", "detections_v2", "{\"deviceTo\u212aen\":\"" + persistLinkSentinel + "\"}"},
		{"long s key", "gpo-sccm-files-generate", "{\"gpo\u017fccmFile\":\"" + persistLinkSentinel + "\"}"},
		{"object value", "mobile-devices-batch-get-enrollment-links", `{"uri":{"href":"https://enroll.example/` + persistLinkSentinel + `"}}`},
		{"array of rows", "installers", `[{"downloadUrl":"https://download.example/` + persistLinkSentinel + `"},{"downloadUrl":"https://download.example/` + persistLinkSentinel + `2"}]`},
		{"escaped html link", "installers", `{"downloadUrl":"https://download.example/a?b=1\u0026c=` + persistLinkSentinel + `"}`},
		{"duplicate key", "installers", `{"downloadUrl":"https://download.example/` + persistLinkSentinel + `","downloadUrl":""}`},
		{"duplicate null", "mobile-devices-batch-get-enrollment-links", `{"uri":"` + persistLinkSentinel + `","uri":null}`},
		{"duplicate marked", "devices", `{"deviceToken":"` + persistLinkSentinel + `","deviceToken":"[redacted]"}`},
	} {
		name, body := c.name, c.body
		got, ok := PersistForm(c.scope, []byte(body))
		if !ok {
			t.Errorf("%s: a body that is one JSON value was refused", name)
			continue
		}
		if dec, _ := DecodeEscapesToFixedPoint(got); bytes.Contains(dec, []byte(persistLinkSentinel)) {
			t.Errorf("%s: a Credential-tier value survived: %s", name, got)
		}
		if !bytes.Contains(got, []byte(PersistRedactedValue)) && !strings.HasPrefix(name, "duplicate") {
			t.Errorf("%s: no %q marker: %s", name, PersistRedactedValue, got)
		}
		if !HoldsPersistCredential(c.scope, []byte(body)) {
			t.Errorf("%s: HoldsPersistCredential = false for a body carrying a link", name)
		}
		if HoldsPersistCredential(c.scope, got) {
			t.Errorf("%s: the redacted body still reports a credential: %s", name, got)
		}
	}
	// Round 3 (Codex P1): a body that is not exactly one JSON value is never
	// persisted. More() used to read "{}]" as end of input.
	for name, body := range map[string]string{
		"codex concatenation": `{}]{"deviceToken":"` + persistLinkSentinel + `"}`,
		"two values":          `{}{"downloadUrl":"` + persistLinkSentinel + `"}`,
		"not one JSON doc":    `{"uuid":"b1","downloadUrl":"https://download.example/` + persistLinkSentinel + `"} trailing`,
		"truncated":           `{"installers":[{"downloadUrl":"https://download.example/` + persistLinkSentinel + `"`,
		"unterminated value":  `{"downloadUrl":"https://download.example/` + persistLinkSentinel,
		"spaced raw":          `{"gpoSccmFile" :  "` + persistLinkSentinel + `", "x": `,
		"escaped truncated":   `{"x":[{"download\u0055rl":"https://download.example/` + persistLinkSentinel + `"`,
		"no key at all":       `not json`,
		"empty":               ``,
	} {
		for _, scope := range []string{"installers", "detections_v2", ""} {
			if got, ok := PersistForm(scope, []byte(body)); ok {
				t.Errorf("%s (%s): a body that is not one JSON value was persisted: %q", name, scope, got)
			}
			if !HoldsPersistCredential(scope, []byte(body)) {
				t.Errorf("%s (%s): HoldsPersistCredential = false for a body that may not be written", name, scope)
			}
		}
	}
	for name, body := range map[string]string{
		"device":         `{"uuid":"d1","displayName":"FRONTDESK-01","security":"during uri-like words","size":12345678901234567890}`,
		"null link":      `{"downloadUrl":null,"uuid":"b1"}`,
		"empty link":     `{"downloadUrl":"","uuid":"b1"}`,
		"already marked": `{"downloadUrl":"[redacted]","uuid":"b1"}`,
		"detection url":  `{"objectUrl":"https://evil.example/x","releaseNotesUrl":"https://vendor.example/n"}`,
		"escaped name":   `{"displayName":"Caf\u00e9 <b>","note":"uri"}`,
		"spaced":         " {\"uuid\":\"b1\"}\n",
	} {
		got, ok := PersistForm("installers", []byte(body))
		if !ok || !bytes.Equal(got, []byte(body)) {
			t.Errorf("%s: a body with nothing to hide changed (ok %v):\n got %s\nwant %s", name, ok, got, body)
		}
		if HoldsPersistCredential("installers", []byte(body)) {
			t.Errorf("%s: HoldsPersistCredential = true for a body with nothing to hide", name)
		}
	}
	// Numbers survive the re-encode exactly.
	got, _ := PersistForm("installers", []byte(`{"n":12345678901234567890,"downloadUrl":"x"}`))
	if !strings.Contains(string(got), "12345678901234567890") {
		t.Fatalf("re-encode lost number precision: %s", got)
	}
}

// TestPersistRedactionIsScopedBySchema (round 3, adversarial P2): uri and
// downloadUrl are Credential tier only in the enrollment-link and installer
// responses. A v2 detection's triggeringEvent.data (a free-form object) and a
// policy configuration keep theirs, byte for byte, while deviceToken is
// redacted in every scope.
func TestPersistRedactionIsScopedBySchema(t *testing.T) {
	for _, c := range []struct {
		scope string
		want  []string
	}{
		{"installers", []string{"deviceToken", "downloadUrl"}},
		{"https://{region}.installer-management.eset.systems/v1/installers/{installerUuid}", []string{"deviceToken", "downloadUrl"}},
		{"gpo-sccm-files-generate", []string{"deviceToken", "gpoSccmFile"}},
		{"https://{region}.installer-management.eset.systems/v1/gpo-sccm-files:generate", []string{"deviceToken", "downloadUrl", "gpoSccmFile"}},
		{"mobile-devices-batch-get-enrollment-links", []string{"deviceToken", "uri"}},
		{"https://{region}.mobile-device-management.eset.systems/v1/mobile-devices:batchGetEnrollmentLinks", []string{"deviceToken", "uri"}},
		{"devices", []string{"deviceToken"}},
		{"detections_v2", []string{"deviceToken"}},
		{"policies", []string{"deviceToken"}},
		{"", PersistCredentialKeys()},
	} {
		if got := PersistCredentialKeysFor(c.scope); strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("PersistCredentialKeysFor(%q) = %v, want %v", c.scope, got, c.want)
		}
	}
	forensic := map[string]string{
		"detections_v2":    `{"uuid":"d1","triggeringEvent":{"type":"web","data":{"uri":"https://evil.example/LEGIT-EVENT-URI","downloadUrl":"https://evil.example/payload.exe"}}}`,
		"policies":         `{"uuid":"p1","configuration":{"@type":"x","updates":{"uri":"https://update.example/","downloadUrl":"https://mirror.example/"}}}`,
		"detection-groups": `{"uuid":"g1","triggeringEvent":{"data":{"URI":"https://evil.example/2"}}}`,
	}
	for scope, body := range forensic {
		got, ok := PersistForm(scope, []byte(body))
		if !ok || !bytes.Equal(got, []byte(body)) || HoldsPersistCredential(scope, []byte(body)) {
			t.Errorf("%s: forensic data was altered (ok %v): %s", scope, ok, got)
		}
	}
	token := `{"triggeringEvent":{"data":{"device":{"deviceToken":"` + persistLinkSentinel + `"}}}}`
	if got, ok := PersistForm("detections_v2", []byte(token)); !ok || bytes.Contains(got, []byte(persistLinkSentinel)) {
		t.Errorf("deviceToken must be redacted in every scope: %s", got)
	}
}
