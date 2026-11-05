// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package liongardblackbox

// Liongard-only credential carrier: a launchpoint's SecureConfig object holds
// the inspector's secure fields ("usually passwords or Secret Tokens", spec
// CreateLaunchpoint.SecureConfig). The field names come from each inspector's
// template (GET /inspector/{id}/templates, Secure: true), so they need not
// read as credential names. Every value under SecureConfig is a credential:
// no preview, receipt, MCP result or file may repeat one.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecureConfigValuesNeverPrint(t *testing.T) {
	const (
		named   = "secureNamedPw-4c8e1a3f"    // under a credential-shaped name (Password)
		unnamed = "secureUnnamedVal-7b2d9f0e" // under an inspector field name that does not read as one
	)
	secure := `{"Password":"` + named + `","ServiceAccountPw":"` + unnamed + `"}`
	sv := echoServer(t)
	e := newEnv(t, sv, credEnvKey)
	var out strings.Builder
	for _, args := range [][]string{
		append(launchpointFlags("HQ lp"), "--secure-config", secure, "--dry-run"),
		append(launchpointFlags("HQ lp"), "--secure-config", secure, "--json", "--receipt"),
		{"launchpoints", "update", bbLaunchpoint, "--alias", "HQ lp", "--secure-config", secure, "--dry-run"},
	} {
		out.WriteString(e.run(t, args...))
	}
	out.WriteString(e.mcpCalls(t, []map[string]any{
		{"name": "liongard_execute", "arguments": map[string]any{"endpoint_id": "launchpoints.launchpoint",
			"params": map[string]any{"Alias": "HQ lp", "SecureConfig": map[string]any{"Password": named, "ServiceAccountPw": unnamed}}}},
	}))
	got := out.String()
	// The runs reached their previews and the send, so the scan is not vacuous.
	// Every preview repeats the SecureConfig it was given, so the shared
	// credential kit withholds each whole: its notice shows they ran.
	if !strings.Contains(got, kitNotice) {
		t.Fatalf("the SecureConfig previews did not run:\n%.3000s", got)
	}
	if n, _ := sv.sawHeader(bbAPIKey); n == 0 {
		t.Fatalf("the launchpoint create was never sent:\n%.3000s", got)
	}
	for _, v := range []string{named, unnamed} {
		if strings.Contains(got, v) {
			t.Errorf("output repeats the SecureConfig value %q", v)
		}
	}
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		rel, _ := filepath.Rel(e.home, path)
		for _, v := range []string{named, unnamed} {
			if strings.Contains(string(data), v) {
				t.Errorf("file %s repeats the SecureConfig value %q", rel, v)
			}
		}
		return nil
	})
}
