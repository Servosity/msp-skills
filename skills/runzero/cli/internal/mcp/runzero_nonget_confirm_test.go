// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runzero handfix mcp-nonget-needs-confirm: runZero has no search-shaped POST,
// so every non-GET endpoint changes state or mints a credential and previews
// until confirm: true. POST /account/api/token (getAPIToken) was classified a
// read by its get- name and minted an access token through runzero_read.
func TestEveryNonGetEndpointNeedsConfirm(t *testing.T) {
	for i := range codeOrchEndpoints {
		ep := &codeOrchEndpoints[i]
		method := codeOrchNormalizeMethod(ep.Method)
		if method == "GET" {
			continue
		}
		if !codeOrchResolvedNeedsConfirm(ep, method, ep.Path, nil) {
			t.Errorf("%s (%s %s) runs without confirm", ep.ID, method, ep.Path)
		}
	}
	files, err := filepath.Glob(filepath.Join("..", "cli", "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no cli files: %v", err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		if strings.Contains(s, `"mcp:read-only": "true"`) && strings.Contains(s, `"pp:method": "`) && !strings.Contains(s, `"pp:method": "GET"`) {
			t.Errorf("%s: a non-GET command is annotated mcp:read-only", filepath.Base(f))
		}
	}
}
