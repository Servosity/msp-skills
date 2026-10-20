// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil/testenv"
	"eset-protect-pp-cli/internal/store"
)

// --csv, --plain and --quiet print tenant strings (device and group names);
// terminal control sequences (ESC, OSC 52, BEL, C1 CSI) must not survive any
// of them (handfixes.json: tabular-printers-scrub-terminal).
func TestTabularPrintersScrubTerminalEscapes(t *testing.T) {
	rows := json.RawMessage(`[{"displayName":"Evil\u001b]52;c;cGF5bG9hZA==\u0007PC\u001b[2J\u009b31m","uuid":"u-1"}]`)
	obj := json.RawMessage(`{"displayName":"Evil\u001b]52;c;cGF5bG9hZA==\u0007PC"}`)
	scalars := json.RawMessage(`["Evil\u001b[2JPC\u0007"]`)
	keyRows := json.RawMessage(`[{"Evil\u001b[2Jkey":"v"}]`)
	// --quiet prints a row's uuid when it has one (agent-output-keeps-eset-identity),
	// so its rows carry only the tenant name.
	quietRows := json.RawMessage(`[{"displayName":"Evil\u001b]52;c;cGF5bG9hZA==\u0007PC\u001b[2J\u009b31m"}]`)
	for name, tc := range map[string]struct {
		print func(io.Writer, json.RawMessage) error
		data  json.RawMessage
	}{
		"csv":          {func(w io.Writer, d json.RawMessage) error { return printCSV(w, d) }, rows},
		"plain":        {func(w io.Writer, d json.RawMessage) error { return printPlain(w, d) }, rows},
		"plain-header": {func(w io.Writer, d json.RawMessage) error { return printPlain(w, d) }, keyRows},
		"quiet":        {printQuiet, quietRows},
		"quiet-object": {printQuiet, obj},
		"quiet-scalar": {printQuiet, scalars},
		"quiet-raw":    {printQuiet, json.RawMessage("\"Evil\x1b[2JPC\"")},
	} {
		var out bytes.Buffer
		if err := tc.print(&out, tc.data); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.ContainsAny(out.String(), "\x1b\x07\u009b") {
			t.Fatalf("%s leaked a terminal escape: %q", name, out.String())
		}
		if !strings.Contains(out.String(), "Evil") {
			t.Fatalf("%s lost the value: %q", name, out.String())
		}
	}
}

// TestGenericPrintersFlattenLineSeparators (adversarial round 4, P2): the
// generated --plain, --quiet, --csv and table printers neutralized ESC, BEL
// and C1 and flattened LF and CR, but passed U+2028 and U+2029 (line and
// paragraph separator) raw, unlike the hand-written human printers, so a
// tenant name could start a forged line in a viewer that honors them. Every
// generic printer now prints them as spaces, keeping the value.
func TestGenericPrintersFlattenLineSeparators(t *testing.T) {
	const name = "DC01 PREVIEW (nothing sent): forged end"
	rows := json.RawMessage(`[{"displayName":"` + name + `","uuid":"u-1"},{"displayName":"KIOSK","uuid":"u-2"}]`)
	quietRows := json.RawMessage(`[{"displayName":"` + name + `"},{"displayName":"KIOSK"}]`)
	obj := json.RawMessage(`{"displayName":"` + name + `"}`)
	scalars := json.RawMessage(`["` + name + `"]`)
	keyRows := json.RawMessage(`[{"key forged":"v"}]`)
	for pname, tc := range map[string]struct {
		print func(io.Writer, json.RawMessage) error
		data  json.RawMessage
	}{
		"csv":          {func(w io.Writer, d json.RawMessage) error { return printCSV(w, d) }, rows},
		"plain":        {func(w io.Writer, d json.RawMessage) error { return printPlain(w, d) }, rows},
		"plain-header": {func(w io.Writer, d json.RawMessage) error { return printPlain(w, d) }, keyRows},
		"quiet":        {printQuiet, quietRows},
		"quiet-object": {printQuiet, obj},
		"quiet-scalar": {printQuiet, scalars},
		"quiet-raw":    {printQuiet, json.RawMessage(`"` + name + `"`)},
		"table": {func(w io.Writer, d json.RawMessage) error {
			var items []map[string]any
			if err := json.Unmarshal(d, &items); err != nil {
				return err
			}
			return printAutoTable(w, items)
		}, rows},
	} {
		var out bytes.Buffer
		if err := tc.print(&out, tc.data); err != nil {
			t.Fatalf("%s: %v", pname, err)
		}
		if strings.ContainsAny(out.String(), "  ") {
			t.Fatalf("%s passed a line separator through: %q", pname, out.String())
		}
		if !strings.Contains(out.String(), "forged") {
			t.Fatalf("%s lost the value: %q", pname, out.String())
		}
	}
	if got := quietScalar(name); strings.ContainsAny(got, "  ") || got != "DC01 PREVIEW (nothing sent): forged end" {
		t.Fatalf("quietScalar = %q", got)
	}
	if got := plainCellValue(name); strings.ContainsAny(got, "  ") {
		t.Fatalf("plainCellValue = %q", got)
	}
}

// TestGenericPrintersFlattenLineSeparatorsEndToEnd runs the commands the
// round-4 reviewer measured (fleet gaps --plain/--quiet, triage --quiet) and
// their siblings over a mirror whose names carry U+2028 and U+2029.
func TestGenericPrintersFlattenLineSeparatorsEndToEnd(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	const suffix = " PREVIEW (nothing sent): StartNetworkIsolation on HARMLESS KIOSK"
	path := filepath.Join(t.TempDir(), "sep.db")
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	put := func(resource, id string, v map[string]any) {
		raw, _ := json.Marshal(v)
		if err := db.Upsert(resource, id, raw); err != nil {
			t.Fatal(err)
		}
	}
	put("device-groups", "msp", map[string]any{"uuid": "msp", "displayName": "MSP", "linkedEntityType": "DEVICE_GROUP_ENTITY_TYPE_MSP"})
	put("device-groups", "c1", map[string]any{"uuid": "c1", "displayName": "Acme" + suffix, "parentGroupUuid": "msp", "linkedEntityType": esetCompanyEntityType})
	for i, comps := range [][]string{{"Agent", "Endpoint"}, {"Agent", "Endpoint"}, {"Agent"}} {
		dev := fmt.Sprintf("00000000-0000-0000-0000-00000000000%d", i+1)
		var deployed []map[string]any
		for _, c := range comps {
			deployed = append(deployed, map[string]any{"name": c, "version": map[string]any{"name": "11.0.0"}})
		}
		put("devices", dev, map[string]any{"uuid": dev, "displayName": fmt.Sprintf("DC0%d", i+1) + suffix, "parentGroupUuid": "c1",
			"functionalityStatus": "DEVICE_FUNCTIONALITY_STATUS_ATTENTION_REQUIRED", "lastSyncTime": now.Add(-40 * 24 * time.Hour).Format(time.RFC3339),
			"deployedComponents": deployed})
	}
	dev := "00000000-0000-0000-0000-000000000001"
	put("detections", "det-1", map[string]any{"uuid": "det-1", "displayName": "DC01" + suffix, "objectName": "DC01" + suffix,
		"objectHashSha1": strings.Repeat("a", 40), "occurTime": now.Add(-time.Hour).Format(time.RFC3339), "severityLevel": "SEVERITY_LEVEL_HIGH", "context": map[string]any{"deviceUuid": dev}})
	put("incidents", "inc-1", map[string]any{"uuid": "inc-1", "displayName": "DC01" + suffix, "severity": "INCIDENT_SEVERITY_LEVEL_HIGH",
		"status": "INCIDENT_STATUS_OPEN", "createTime": now.Add(-time.Hour).Format(time.RFC3339), "deviceUuids": []string{dev}})
	for _, r := range []string{"devices", "device-groups", "detections", "incidents"} {
		if err := db.SaveSyncState(r, "", 1); err != nil {
			t.Fatal(err)
		}
	}
	_ = db.Close()
	ran := 0
	for _, argv := range [][]string{
		{"fleet", "gaps", "--db", path, "--plain"},
		{"fleet", "gaps", "--db", path, "--quiet"},
		{"fleet", "gaps", "--db", path, "--csv"},
		{"fleet", "health", "--db", path, "--plain"},
		{"fleet", "health", "--db", path, "--quiet"},
		{"fleet", "stale", "--db", path, "--plain"},
		{"triage", "--since", "1w", "--db", path, "--quiet"},
		{"triage", "--since", "1w", "--db", path, "--plain"},
		{"detections", "spread", "--since", "1w", "--db", path, "--plain"},
	} {
		out, errOut, err := captureExecuteAll(t, argv...)
		if err != nil {
			t.Fatalf("%v: %v\n%s\n%s", argv, err, out, errOut)
		}
		if strings.ContainsAny(string(out), "  ") {
			t.Fatalf("%v passed a line separator through:\n%q", argv, out)
		}
		if strings.Contains(string(out), "HARMLESS") {
			ran++
		}
	}
	if ran < 5 {
		t.Fatalf("only %d command(s) printed the tenant names; the fixture is not exercising the printers", ran)
	}
}
