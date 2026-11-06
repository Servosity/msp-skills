// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package autotaskblackbox

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// agent-context prints the CLI's whole description, as JSON, inside
// class_harness's 60 s budget, run the way class_harness runs it (PATH and
// HOME only, no credential configured). The retired own-metadata-not-held
// declared the auth block as the CLI's own text for the old output layer;
// the kit holds no help text. With a credential configured the kit withholds
// the whole 841 KB line (.review kit finding agent-context-withheld-whole).
func TestAgentContextPrintsWholeAndFast(t *testing.T) {
	home := t.TempDir()
	cmd := exec.Command(bbCLI, "agent-context")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home}
	cmd.Dir = home
	start := time.Now()
	out, err := cmd.Output()
	if d := time.Since(start); d > 60*time.Second {
		t.Errorf("agent-context took %s (class_harness allows 60 s)", d)
	}
	if err != nil {
		t.Fatalf("agent-context: %v\n%.600s", err, out)
	}
	var doc struct {
		CLI  map[string]any `json:"cli"`
		Auth struct {
			EnvVars []struct{ Name string } `json:"env_vars"`
		} `json:"auth"`
		Commands []any `json:"commands"`
	}
	if err := json.Unmarshal(out, &doc); err != nil || len(doc.Commands) == 0 || doc.CLI["name"] != "autotask-cli" {
		t.Fatalf("agent-context is not the CLI's whole description (err %v):\n%.600s", err, out)
	}
	if len(doc.Auth.EnvVars) != 3 || strings.Contains(string(out), "credkit:") {
		t.Errorf("want the auth block's three variables and nothing withheld, got auth %+v", doc.Auth)
	}
}
