// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"datto-rmm-pp-cli/internal/cliutil/testenv"
)

// TestNovelFleetUdfHelpWires smoke-tests that the fleet udf command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelFleetUdfHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"fleet", "udf", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("fleet udf --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "udf"} {
		if !strings.Contains(help, want) {
			t.Fatalf("fleet udf --help missing %q in output:\n%s", want, help)
		}
	}
}

func udfRaws(t *testing.T, devs ...map[string]any) [][]byte {
	t.Helper()
	var out [][]byte
	for _, d := range devs {
		b, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, b)
	}
	return out
}

func TestFleetUdfRollsUpOneSlot(t *testing.T) {
	raws := udfRaws(t,
		map[string]any{"uid": "d1", "hostname": "A1", "siteName": "Acme", "siteUid": "s1", "udf": map[string]any{"udf7": "Managed", "udf8": "x"}},
		map[string]any{"uid": "d2", "hostname": "A2", "siteName": "Acme", "siteUid": "s1", "udf": map[string]any{"udf7": " Managed "}},
		map[string]any{"uid": "d3", "hostname": "B1", "siteName": "Beta", "siteUid": "s2", "udf": map[string]any{"udf7": "Managed"}},
		map[string]any{"uid": "d4", "hostname": "B2", "siteName": "Beta", "siteUid": "s2", "udf": map[string]any{"udf7": "BYOD"}},
		map[string]any{"uid": "d5", "hostname": "B3", "siteName": "Beta", "siteUid": "s2", "udf": map[string]any{"udf7": nil}},
		map[string]any{"uid": "d6", "hostname": "C1", "siteName": "Gamma", "siteUid": "s3"},
	)
	v := computeUdf(raws, "udf7", "", "", 50)
	if v.DevicesTotal != 6 || v.WithValue != 4 || v.Empty != 2 || v.DistinctValues != 2 {
		t.Fatalf("totals %+v", v)
	}
	if v.Values[0] != (udfValueRow{Value: "Managed", Devices: 3, Sites: 2}) || v.Values[1] != (udfValueRow{Value: "BYOD", Devices: 1, Sites: 1}) {
		t.Fatalf("values %+v", v.Values)
	}
	if len(v.Devices) != 0 {
		t.Fatalf("no device list without --value/--empty: %+v", v.Devices)
	}
	e := computeUdf(raws, "udf7", "empty", "", 50)
	if len(e.Devices) != 2 || e.Devices[0].Hostname != "B3" || e.Devices[1].Hostname != "C1" {
		t.Fatalf("empty devices %+v", e.Devices)
	}
	m := computeUdf(raws, "udf7", "value", "Managed", 1)
	if len(m.Devices) != 1 || !m.Truncated || m.WithValue != 4 || len(m.Values) != 1 {
		t.Fatalf("limit keeps totals and marks truncation: %+v", m)
	}
	none := computeUdf(nil, "udf7", "", "", 50)
	if b, _ := json.Marshal(none); string(b) != `{"slot":"udf7","devices_total":0,"with_value":0,"empty":0,"distinct_values":0,"values":[],"devices":[],"truncated":false}` {
		t.Fatalf("empty store renders %s", b)
	}
}

func TestFleetUdfSlotNames(t *testing.T) {
	for in, want := range map[string]string{"udf7": "udf7", "UDF300": "udf300", " 3 ": "udf3"} {
		if got, err := normalizeUdfSlot(in); err != nil || got != want {
			t.Errorf("%q -> %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"0", "301", "udf07", "udfx", "", "-1"} {
		if _, err := normalizeUdfSlot(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
