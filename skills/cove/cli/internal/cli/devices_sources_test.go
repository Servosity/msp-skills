// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"cove-pp-cli/internal/cliutil/testenv"
)

// TestNovelDevicesSourcesHelpWires smoke-tests that the devices sources command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelDevicesSourcesHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"devices", "sources", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("devices sources --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "sources"} {
		if !strings.Contains(help, want) {
			t.Fatalf("devices sources --help missing %q in output:\n%s", want, help)
		}
	}
}

func TestDevicesSourcesDecodesAndFiltersMissingSource(t *testing.T) {
	srv := mockFleet(t, []map[string]any{
		{"AccountId": 1, "Settings": settingsList(map[string]string{"I1": "srv-full", "I8": "Acme", "I32": "2", "I78": "D1,D2"})},
		{"AccountId": 2, "Settings": settingsList(map[string]string{"I1": "srv-files", "I8": "Acme", "I32": "2", "I78": "D01, D99"})},
		{"AccountId": 3, "Settings": settingsList(map[string]string{"I1": "ws-files", "I8": "Globex", "I32": "1", "I78": "D1"})},
		{"AccountId": 4, "Settings": settingsList(map[string]string{"I1": "srv-unreported", "I8": "Globex", "I32": "2"})},
		{"AccountId": 5, "Settings": settingsList(map[string]string{"I1": "srv-empty", "I8": "Globex", "I32": "2", "I78": ""})},
	})
	fleetTestEnv(t, srv)

	out, err := runCoveCmd(t, "devices", "sources", "--without", "system state", "--os", "server", "--json")
	if err != nil {
		t.Fatalf("devices sources: %v\n%s", err, out)
	}
	var view struct {
		Without string `json:"without"`
		Items   []struct {
			DeviceName      string   `json:"device_name"`
			DataSources     []string `json:"data_sources"`
			DataSourceCodes []string `json:"data_source_codes"`
			UnknownCodes    []string `json:"unknown_codes"`
		} `json:"items"`
		Sources []struct {
			Code    string `json:"code"`
			Devices int    `json:"devices"`
		} `json:"sources"`
		UnreportedDevices int `json:"unreported_devices"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	var names []string
	for _, it := range view.Items {
		names = append(names, it.DeviceName)
	}
	// srv-full protects System State; ws-files is a workstation; srv-unreported
	// cannot be proven to lack it. srv-empty reported no sources at all.
	if strings.Join(names, ",") != "srv-files,srv-empty" { // sorted by customer, then device
		t.Fatalf("servers missing System State = %v, want [srv-files srv-empty]\n%s", names, out)
	}
	if view.Without != "System State (D2)" || view.UnreportedDevices != 1 {
		t.Fatalf("without=%q unreported=%d", view.Without, view.UnreportedDevices)
	}
	files := view.Items[0]
	if strings.Join(files.DataSourceCodes, ",") != "D1" || files.DataSources[0] != "Files and Folders" || strings.Join(files.UnknownCodes, ",") != "D99" {
		t.Fatalf("srv-files decode = %+v", files)
	}
	tally := map[string]int{}
	for _, s := range view.Sources {
		tally[s.Code] = s.Devices
	}
	if tally["D1"] != 2 || tally["D2"] != 1 {
		t.Fatalf("server tally = %v, want D1:2 D2:1", tally)
	}
}

func TestDevicesSourcesRefusesAnUnknownSourceOrOS(t *testing.T) {
	testenv.Isolate(t)
	for _, args := range [][]string{
		{"devices", "sources", "--without", "Sytem State"},
		{"devices", "sources", "--without", "D404"},
		{"devices", "sources", "--os", "servers"},
	} {
		if out, err := runCoveCmd(t, args...); err == nil {
			t.Errorf("%v was accepted:\n%s", args, out)
		}
	}
}
