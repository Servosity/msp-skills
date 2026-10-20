// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-read-only-redacts-before-bounding" entry.

package redact

import (
	"encoding/json"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
)

// TestFinalWithholdsUndecodablePreview: a bounded preview is a JSON string
// holding the first bytes of a body, cut mid-record, so it never decodes. If
// it still names a credential field with a value (even one cut at the
// truncation point, or behind escaped quotes) the string is replaced; a
// preview whose credential fields already carry the placeholder, a cut
// placeholder, or a boolean flag is kept.
func TestFinalWithholdsUndecodablePreview(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	envelope := func(preview string) string {
		b, _ := json.Marshal(map[string]any{"_pp_truncated": true, "original_bytes": 105597, "preview": preview})
		return string(b)
	}
	leaky := []string{
		`{"data": [{"id": 1, "name": "Site 0001", "secret_key": "NETSECRET-0001-a8f3"}, {"id": 2, "name": "Site 0002", "secret_key": "NETSEC`,
		`{"data": [{"id": 1, "secret_key": "NETSECRET-0001-a8f3"}, {"id": 2, "name": "Site 0002", "secret_key": "NETSEC`,
		`{\"data\": [{\"id\": 2, \"name\": \"Site 0002\", \"secret_key\": \"NETSECRET-0002`,
		`{"data": [{"id": 2, "pin": 4938`,
		"id,name,secret_key\n1,Site 0001,NETSECRET-0001\n2,Site 00",
	}
	for _, p := range leaky {
		got := Final(envelope(p))
		if strings.Contains(got, "NETSEC") || strings.Contains(got, "4938") {
			t.Fatalf("undecodable preview leaked: %s", got)
		}
		if !strings.Contains(got, `"original_bytes":105597`) {
			t.Fatalf("envelope metadata lost: %s", got)
		}
	}
	kept := []string{
		`{"data": [{"id": 1, "name": "Site 0001", "secret_key": "` + WithheldFieldPlaceholder + `", "has_secret_key": true}, {"id": 2, "name": "Site 0002", "secret_key": "[withhe`,
		`{"data": [{"id": 1, "name": "Site 0001", "secret_key": `,
	}
	for _, p := range kept {
		if got := Final(envelope(p)); !strings.Contains(got, "Site 0001") {
			t.Fatalf("redacted preview was withheld: %s", got)
		}
	}
}

// TestRawRedactsCompleteRecordsInTruncatedText: Raw runs before bounding on a
// capped stdout, so the complete records inside a truncated container are
// redacted one by one and never withheld as a block.
func TestRawRedactsCompleteRecordsInTruncatedText(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	in := `{"data":[{"id":1,"name":"Site 0001","secret_key":"NETSECRET-0001-a8f3"},{"id":2,"name":"Site 0002","secret_key":"NETSECRET-0002-a8f3"},{"id":3,"na`
	got := Raw(in)
	if strings.Contains(got, "NETSECRET-0001") || strings.Contains(got, "NETSECRET-0002") {
		t.Fatalf("complete records were not redacted: %s", got)
	}
	if !strings.Contains(got, "Site 0001") || !strings.Contains(got, `{"id":3,"na`) {
		t.Fatalf("Raw dropped data it should keep: %s", got)
	}
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	if Raw(in) != in || Final(in) != in {
		t.Fatal("default mode must not rewrite text")
	}
}
