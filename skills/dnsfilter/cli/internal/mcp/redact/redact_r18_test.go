// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "secrets-never-persisted" entry (Codex round 18, adversarial
// round 19): read-only results are withheld for a credential, never for
// their shape or size.

package redact

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
)

// r18CSVCell quotes a nested value the way the CLI's --csv writer does:
// compact JSON, inner quotes doubled, the cell quoted.
func r18CSVCell(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return `"` + strings.ReplaceAll(string(b), `"`, `""`) + `"`
}

func r18Agent(host string) map[string]any {
	return map[string]any{
		"age_bucket": "7d+", "agent_state": "protected", "agent_version": "2.4.1", "friendly_name": host,
		"hostname": host, "id": "a-" + host, "last_sync": "2026-10-01T10:00:00Z", "network_id": "50",
	}
}

// agentsStaleCSV is an agents stale --csv result: one row per org, its agents
// a nested array cell.
func agentsStaleCSV(t *testing.T, rows [][]map[string]any) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("agents,count,org_id,org_name\n")
	for i, agents := range rows {
		list := make([]any, len(agents))
		for j := range agents {
			list[j] = agents[j]
		}
		fmt.Fprintf(&b, "%s,%d,%d,Acme %d\n", r18CSVCell(t, list), len(agents), 7+i, i)
	}
	return b.String()
}

// The decode budget never withholds a whole ordinary CSV result: its nested
// JSON cells are decoded and judged by the key rule, so 300 clean records are
// returned as they are (one row or one row each), while a credential planted
// in one record never leaves the server.
func TestCSVResultIsNotWithheldForItsSize(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	agents := make([]map[string]any, 300)
	perRow := make([][]map[string]any, 300)
	for i := range agents {
		agents[i] = r18Agent(fmt.Sprintf("H%d", i))
		perRow[i] = []map[string]any{agents[i]}
	}
	for name, csv := range map[string]string{
		"one row":     agentsStaleCSV(t, [][]map[string]any{agents}),
		"row per org": agentsStaleCSV(t, perRow),
	} {
		if strings.Count(csv, "{") < maxFailedSegmentDecodes {
			t.Fatalf("%s: the fixture does not exceed the decode budget", name)
		}
		resetWithheldMemory()
		if got := FinalAlways(csv); got != csv {
			t.Errorf("%s: FinalAlways withheld or changed a clean CSV of 300 records: %.200s", name, got)
		}
		if got := Final(Raw(csv)); got != csv {
			t.Errorf("%s: Final(Raw) withheld or changed a clean CSV of 300 records: %.200s", name, got)
		}
	}
	for _, plant := range []struct{ key, value string }{
		{"secret_key", "PLANTEDS3CRET01"}, {"p a s s w o r d", "PLANTEDS3CRET02"}, {"Uninstall PIN (HQ)", "PLANTEDS3CRET03"},
	} {
		planted := make([]map[string]any, len(agents))
		copy(planted, agents)
		rec := r18Agent("H150")
		rec[plant.key] = plant.value
		planted[150] = rec
		csv := agentsStaleCSV(t, [][]map[string]any{planted})
		resetWithheldMemory()
		for _, got := range []string{FinalAlways(csv), Final(Raw(csv)), RawAlways(csv)} {
			if strings.Contains(got, plant.value) {
				t.Errorf("a %q credential planted in one record was returned: %.200s", plant.key, got)
			}
		}
	}
	// An exhausted budget still withholds: stray brackets, then a key only
	// the key rule reads.
	stray := strings.Repeat("[", maxFailedSegmentDecodes) + `{"p a s s w o r d":"STRAYS3CRET"}`
	if got := FinalAlways(stray); strings.Contains(got, "STRAYS3CRET") {
		t.Errorf("an exhausted decode budget returned the unexamined key: %.200s", got)
	}
}

// A glued secret name inside an ordinary hostname (LAPTOP-SPIN1,
// TOKEN2-LAPTOP) never arms past the value or CSV cell it is in, so a result
// listing it is returned; a glued label that is followed by its separator
// (Uninstall PIN2: 4821) is still withheld.
func TestGluedHostnameNeverWithholdsAResult(t *testing.T) {
	for _, host := range []string{"LAPTOP-SPIN1", "TOKEN2-LAPTOP", "KIOSK-PIN", "FRONTDESK-01", "PIN2", "SPIN.2-LAB"} {
		// One quote inside an earlier value (an inch mark, CSV-doubled as
		// \"") shifts which quotes the JSON-text pass reads as string
		// boundaries, so the hostname is read outside a string there.
		quoted := r18Agent("FRONT-01")
		quoted["friendly_name"] = `Rack 19" kiosk`
		csv := agentsStaleCSV(t, [][]map[string]any{{r18Agent(host)}})
		shifted := agentsStaleCSV(t, [][]map[string]any{{quoted, r18Agent(host), r18Agent("H2")}})
		js, _ := json.Marshal(map[string]any{"agents": []any{r18Agent(host)}})
		for _, text := range []string{csv, shifted, string(js)} {
			resetWithheldMemory()
			if got := FinalAlways(text); got != text {
				t.Errorf("hostname %s: the result was withheld or changed: %.200s", host, got)
			}
		}
	}
	for _, text := range []string{
		"note: Uninstall PIN2: 4821",
		`{"note":"Uninstall PIN2: 4821"}`,
		agentsStaleCSV(t, [][]map[string]any{{{"hostname": "H1", "note": "Uninstall PIN2: 4821"}}}),
		agentsStaleCSV(t, [][]map[string]any{{{"hostname": "H1", "note": "PIN2, HQ: 4821"}}}),
		`a,"x ""Uninstall PIN2: 4821"" y"`,
	} {
		resetWithheldMemory()
		if got := FinalAlways(text); strings.Contains(got, "4821") {
			t.Errorf("a glued label with its value was returned: %q -> %.200s", text, got)
		}
	}
	for _, text := range []string{`""note"":""Uninstall PIN2: 4821""`, `{"note":"Uninstall PIN2: 4821"}`} {
		if !textNamesCredential(text, "") {
			t.Errorf("the JSON-text pass missed %q", text)
		}
	}
}

// An error body cut to its size limit before redaction no longer decodes,
// yet a key only the key rule reads ("pass word") is judged like the whole
// object's: the cut body is withheld, at any escape level; a cut body with no
// credential, or with an exempt value, is returned.
func TestTruncatedJSONBodyGetsTheKeyRule(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	const m = "Z9QX7"
	body := `{"pass word":"` + m + `","padding":"` + strings.Repeat("x", 5000) + `"}`
	if got := FinalAlways(body); strings.Contains(got, m) {
		t.Fatalf("the whole body leaked: %.120s", got)
	}
	cut := body[:4096]
	bs := string(rune(92))
	quoted, _ := json.Marshal(cut)
	if !strings.Contains(string(quoted), bs+`"pass word`+bs+`"`) {
		t.Fatal("the escaped fixture lost its backslashes")
	}
	for _, text := range []string{
		cut,
		"HTTP 422: " + cut,
		`networks.show: API error (422): ` + cut,
		`{"error":` + string(quoted)[:200],
		`"[{""pass word"":""` + m + `"",""padding"":""xx`,
	} {
		resetWithheldMemory()
		for _, got := range []string{FinalAlways(text), Final(Raw(text))} {
			if strings.Contains(got, m) {
				t.Errorf("a cut body kept its pass word: %.120q -> %.120s", text, got)
			}
		}
	}
	for _, text := range []string{
		(`{"name":"HQ","padding":"` + strings.Repeat("x", 5000) + `"}`)[:4096],
		(`{"pass word":null,"padding":"` + strings.Repeat("x", 5000) + `"}`)[:4096],
	} {
		resetWithheldMemory()
		if got := FinalAlways(text); got != text {
			t.Errorf("an ordinary cut body was withheld: %.80q -> %.80s", text, got)
		}
	}
}
