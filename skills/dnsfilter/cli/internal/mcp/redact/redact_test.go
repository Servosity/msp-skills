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
		// A tenant string that only starts like JSON still gets the plain rule.
		"{ Rack: 19\" cabinet\nUninstall PIN for \"HQ\" laptops: NETSECRET-77",
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
		`{"data": [{"id": 1, "name": "Site 0001", "secret_key": "` + WithheldFieldPlaceholder + `", "has_secret_key": true}, {"id": 2, "name": "Site 0002", "secret_key": null, "x": "y`,
		`{"data": [{"id": 1, "name": "Site 0001", "secret_key": `,
	}
	for _, p := range kept {
		if got := Final(envelope(p)); !strings.Contains(got, "Site 0001") {
			t.Fatalf("redacted preview was withheld: %s", got)
		}
	}
	// A preview holding an ordinary value that reads like a qualifier (it is
	// text a tenant can shape) loses its preview field, never the envelope.
	ordinary := `{"data": [{"id": 1, "name": "Site 0001"}, {"id": 7, "name": "Office of the Secretary of State", "status": "active"}, {"id": 9, "name": "Site`
	if got := Final(envelope(ordinary)); !strings.Contains(got, `"original_bytes":105597`) || strings.Contains(got, "Secretary") {
		t.Fatalf("an ordinary preview must keep the envelope and withhold only the preview: %s", got)
	}
	// A preview cut inside a placeholder is withheld (one rule with the
	// store, which has no cut-placeholder exemption; fails closed).
	cut := `{"data": [{"id": 1, "name": "Site 0001", "secret_key": "[withhe`
	if got := Final(envelope(cut)); strings.Contains(got, "Site 0001") {
		t.Fatalf("a preview cut inside a placeholder was kept: %s", got)
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

// TestHeaderHeuristicNeedsATable: like the store's textNamesSecret, only a
// multi-line text can carry a CSV/table header; a one-line tenant string or a
// prose list of field names is not withheld, a real CSV with a secret column
// still is.
func TestHeaderHeuristicNeedsATable(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	for _, keep := range []string{
		"Finance, password reset desk",
		"every tool result withholds credential-named fields (secret_key, token, client_secret, password, pin).",
	} {
		if got := Final(keep); got != keep {
			t.Fatalf("one-line text withheld: %q -> %q", keep, got)
		}
		b, _ := json.Marshal(map[string]string{"note": keep})
		if got := Final(string(b)); got != string(b) {
			t.Fatalf("one-line JSON string value rewritten: %s -> %s", b, got)
		}
	}
	for _, csv := range []string{"id,name,password\n1,Finance,hunter2-hunter2\n", "ID  NAME     TOKEN\n1   Finance  abcdef123456\n"} {
		if got := Final(csv); strings.Contains(got, "hunter2") || strings.Contains(got, "abcdef123456") {
			t.Fatalf("multi-line table with a secret header was not withheld: %q", got)
		}
	}
}

// TestFinalWithholdsBinaryEnvelopes: a base64 binary envelope is opaque to
// every rule, so read-only MCP withholds it.
func TestFinalWithholdsBinaryEnvelopes(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	env := `{"_pp_binary":true,"content_type":"application/zip","encoding":"base64","bytes":11,"data":"c2VjcmV0X2tleT1h"}`
	if got := Final(env); strings.Contains(got, "c2VjcmV0X2tleT1h") {
		t.Fatalf("a binary envelope passed read-only MCP: %s", got)
	}
	if got := Final(`{"data":` + env + `}`); strings.Contains(got, "c2VjcmV0X2tleT1h") {
		t.Fatalf("a nested binary envelope passed read-only MCP: %s", got)
	}
}

// TestFinalBracketLedTenantText: a tenant value that starts with [ or { but
// is not JSON, with a stray inch mark and a qualifier written as JSON, is
// withheld by read-only MCP as by the store; a human-text result of the same
// shape is withheld too; a cut binary envelope is withheld whole.
func TestFinalBracketLedTenantText(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	for _, v := range []string{
		"[Site notes] Rack: 19\" cabinet\nUninstall PIN [\"HQ\", \"Branch\"] laptops: SENT13",
		"[HQ] Rack 19\" cabinet. Admin password [\"Main Office\"] backup: SENT13",
		"[TODO] 27\" monitor. Uninstall PIN {\"site\": \"HQ\"} laptops: SENT13",
	} {
		b, _ := json.Marshal(map[string]any{"data": map[string]any{"id": 1, "attributes": map[string]any{"physical_address": v}}})
		if got := Final(string(b)); strings.Contains(got, "SENT13") {
			t.Fatalf("read-only MCP returned a bracket-led credential: %s", got)
		}
		if got := Final("Site 1  " + strings.ReplaceAll(v, "\n", " ")); strings.Contains(got, "SENT13") {
			t.Fatalf("a human-text result returned the credential: %s", got)
		}
	}
	cut := `{"_pp_truncated":true,"original_bytes":60001,"preview":"{\"data\":{\"_pp_binary\":true,\"content_type\":\"application/octet-stream\",\"encoding\":\"base64\",\"data\":\"H4sIAAAA`
	if got := Final(cut + `"}`); got != WithheldBlockMessage {
		t.Fatalf("a cut binary envelope passed read-only MCP: %s", got)
	}
}
