// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: a --dry-run request body is printed only through the
// registered redactor (Codex review round 13, P1: an order body carrying a
// tenant admin password reached stderr verbatim).

package client

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func captureStderr(t *testing.T, run func()) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() // Windows cannot remove the TempDir while the file is open
	old := os.Stderr
	os.Stderr = f
	defer func() { os.Stderr = old }()
	run()
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(f.Name())
	return string(b)
}

func TestDryRunBodyGoesThroughTheRedactor(t *testing.T) {
	const sentinel = "SENTINEL-Hunter2-9f3a-4417"
	body := []byte(`{"orderItems":[{"attributes":[{"name":"adminPassword","value":"` + sentinel + `"}]}]}`)
	c := &Client{BaseURL: "https://ion.example.test"}
	prior := DryRunBodyFilter
	defer func() { DryRunBodyFilter = prior }()

	DryRunBodyFilter = nil
	out := captureStderr(t, func() { _, _, _ = c.dryRun("POST", c.BaseURL+"/orders", "/orders", nil, body, nil, "") })
	if strings.Contains(out, sentinel) || !strings.Contains(out, "withheld") {
		t.Fatalf("no redactor registered: body must be withheld, got %q", out)
	}

	DryRunBodyFilter = func(raw json.RawMessage) json.RawMessage {
		return json.RawMessage(strings.ReplaceAll(string(raw), sentinel, "[redacted]"))
	}
	out = captureStderr(t, func() { _, _, _ = c.dryRun("POST", c.BaseURL+"/orders", "/orders", nil, body, nil, "") })
	if strings.Contains(out, sentinel) || !strings.Contains(out, "[redacted]") || !strings.Contains(out, "adminPassword") {
		t.Fatalf("dry-run body did not go through the filter: %q", out)
	}
}
