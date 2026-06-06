// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Execution tests: onboard plan (no writes) and --init template scaffolding.

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNovelOnboardPlan(t *testing.T) {
	tpl := `{"asset_layouts":[{"name":"Server"}],"folders":[{"name":"SOPs"}],"procedure_templates":[7]}`
	tplPath := filepath.Join(t.TempDir(), "tpl.json")
	if err := os.WriteFile(tplPath, []byte(tpl), 0o644); err != nil {
		t.Fatal(err)
	}
	flags := &rootFlags{asJSON: true}
	cmd := newNovelOnboardCmd(flags)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--company", "42", "--template", tplPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error: %v\n%s", err, buf.String())
	}
	var out struct {
		Apply bool          `json:"apply"`
		Plan  []onboardStep `json:"plan"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, buf.String())
	}
	if out.Apply {
		t.Error("plan run must not apply")
	}
	if len(out.Plan) != 3 {
		t.Errorf("expected 3 plan steps (1 layout, 1 folder, 1 procedure), got %d", len(out.Plan))
	}
}

func TestNovelOnboardInit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	flags := &rootFlags{asJSON: true}
	cmd := newNovelOnboardCmd(flags)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--init", "--template", "unit-test-tpl"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error: %v\n%s", err, buf.String())
	}
	want := filepath.Join(templatesDir(), "unit-test-tpl.json")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("--init did not write template at %s: %v", want, err)
	}
}

func TestCreatedObjectID(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"top-level id", `{"id": 7, "name": "Server"}`, 7},
		{"hudu single-key envelope", `{"asset_layout": {"id": 12, "name": "Server"}}`, 12},
		{"no id", `{"name": "Server"}`, 0},
		{"empty body", ``, 0},
		{"array body", `[{"id": 3}]`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := createdObjectID([]byte(tc.body)); got != tc.want {
				t.Errorf("createdObjectID(%s) = %d, want %d", tc.body, got, tc.want)
			}
		})
	}
}
