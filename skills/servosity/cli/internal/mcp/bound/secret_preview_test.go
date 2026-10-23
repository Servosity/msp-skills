// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/servosity/handfixes.json under
// the "secret-family-never-persisted" entry. See docs/reprint-survival.md.

package bound

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestOversizedPreviewWithholdsSecretFields pins that a result over MaxBytes,
// whose first bytes become a raw "preview" string, carries no secret value:
// neither a complete JSON body nor one already cut mid-document.
func TestOversizedPreviewWithholdsSecretFields(t *testing.T) {
	var b strings.Builder
	b.WriteString(`[`)
	for i := 0; b.Len() < MaxBytes+1000; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"id":` + strings.Repeat("1", 3) + `,"name":"Acme Co","agent_provision_token_id":"prov-value","LanPassword":"lan-value"}`)
	}
	full := b.String() + `]`
	for name, text := range map[string]string{"whole document": full, "cut document": full[:MaxBytes+1], "ndjson": strings.ReplaceAll(strings.Trim(full, "[]"), "},{", "}\n{")} {
		out := Text(text)
		if !strings.Contains(out, `"_pp_truncated":true`) {
			t.Fatalf("%s: want a bounded preview, got %d bytes", name, len(out))
		}
		if strings.Contains(out, "prov-value") || strings.Contains(out, "lan-value") {
			t.Fatalf("%s: the preview carries a secret value", name)
		}
		if !strings.Contains(out, "Acme Co") {
			t.Fatalf("%s: the preview lost ordinary data", name)
		}
	}
}

// TestOversizedSingleItemPreviewWithholdsSecretFields: when the first item of
// a page alone is over the budget, its raw item_preview carries no secret.
func TestOversizedSingleItemPreviewWithholdsSecretFields(t *testing.T) {
	item := json.RawMessage(`{"id":1,"name":"Acme Co","agent_provision_token_id":"prov-value","notes":"` + strings.Repeat("n", MaxBytes+5000) + `"}`)
	out := fitJSONPageItems([]json.RawMessage{item}, 0, "", "", func(items []json.RawMessage, preview, next string) any {
		return map[string]any{"results": items, "item_preview": preview, "next": next}
	})
	if !strings.Contains(string(out), "Acme Co") {
		t.Fatalf("want an item preview, got %.200s", out)
	}
	if strings.Contains(string(out), "prov-value") {
		t.Fatal("the single-item preview carries the provisioning token")
	}
}
