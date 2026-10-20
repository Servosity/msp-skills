// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestRegistryTextIsComplete guards registry-and-manifest-text: the MCP
// Registry description in server.json was cut mid-sentence at 99 characters
// ("...QBR reports built"), and manifest.json named the MSP console
// without the 2 of its product name. Skipped where the skill directory is absent.
func TestRegistryTextIsComplete(t *testing.T) {
	skill := filepath.Join("..", "..", "..")
	server, err := os.ReadFile(filepath.Join(skill, "server.json"))
	if err != nil {
		t.Skipf("server.json not beside the module: %v", err)
	}
	var doc struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(server, &doc); err != nil {
		t.Fatal(err)
	}
	const want = "Every ESET Connect endpoint, plus per-customer health, exposure and QBR reports from a local mirror"
	if doc.Description != want || len(doc.Description) > 100 {
		t.Fatalf("server.json description = %q (%d chars), want the complete sentence %q", doc.Description, len(doc.Description), want)
	}
	// Split so the module-wide Go-string scan does not read the pattern itself.
	bare := regexp.MustCompile("ESET MSP Admin" + "istrator($|[^ 2]| [^2])")
	for _, name := range []string{"manifest.json", "server.json"} {
		b, err := os.ReadFile(filepath.Join(skill, name))
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if bare.MatchString(line) {
				t.Errorf("%s:%d names the console without its 2: %s", name, i+1, strings.TrimSpace(line))
			}
		}
	}
}
