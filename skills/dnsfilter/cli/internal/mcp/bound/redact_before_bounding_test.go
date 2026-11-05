// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-read-only-redacts-before-bounding" entry.

package bound

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
)

func oversizedTwoArrayBody() []byte {
	rows := make([]any, 0, 200)
	inc := make([]any, 0, 200)
	for i := 0; i < 200; i++ {
		rows = append(rows, map[string]any{"id": i, "name": fmt.Sprintf("Site %04d", i), "secret_key": fmt.Sprintf("NETSECRET-%04d-a8f3c2d1e9b7", i), "physical_address": strings.Repeat("x", 200)})
		inc = append(inc, map[string]any{"id": i, "attributes": map[string]any{"secret_key": fmt.Sprintf("NETSECRET-INC-%04d", i)}})
	}
	b, _ := json.Marshal(map[string]any{"data": rows, "included": inc})
	return b
}

// TestBoundingRedactsBeforePreview: under the read-only switch every bounding
// entry point redacts the full body before it budgets it, so the raw-byte
// preview of an oversized result quotes only redacted bytes (and still shows
// the non-secret fields). Off the switch the preview is unchanged.
func TestBoundingRedactsBeforePreview(t *testing.T) {
	body := oversizedTwoArrayBody()
	if len(body) <= MaxBytes {
		t.Fatalf("fixture %d bytes must exceed %d", len(body), MaxBytes)
	}
	var decoded any
	_ = json.Unmarshal(body, &decoded)
	render := map[string]func() string{
		"EndpointResponse":     func() string { return EndpointResponse("GET", body) },
		"EndpointPageResponse": func() string { return EndpointPageResponse("GET", body, PageOptions{ReadOnly: true}) },
		"Text":                 func() string { return Text(string(body)) },
		"Text(capped stdout)":  func() string { return Text(string(body[:MaxBytes+1])) },
		"JSON": func() string {
			out, err := JSON(decoded)
			if err != nil {
				t.Fatal(err)
			}
			return out
		},
	}
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	if out := EndpointResponse("GET", body); !strings.Contains(out, "_pp_truncated") || !strings.Contains(out, "NETSECRET") {
		t.Fatalf("fixture does not reach the raw preview path: %.200s", out)
	}
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	for name, fn := range render {
		out := fn()
		if strings.Contains(out, "NETSECRET") {
			t.Fatalf("%s: preview quoted an unredacted secret_key: %.400s", name, out)
		}
		if !strings.Contains(out, "_pp_truncated") || !strings.Contains(out, "Site 0000") {
			t.Fatalf("%s: expected a bounded preview that keeps site names: %.400s", name, out)
		}
	}
}
