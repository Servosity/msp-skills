// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

// hand-fix guard (handfixes.json: generated-help-corrections): the generator
// title-cased the brand ("Eset Protect"), left novel parents as "Work with
// fleet", and described ESET_PROTECT_PASSWORD as "Set during initial auth setup".
func TestGeneratedHelpCorrections(t *testing.T) {
	root := RootCmd()
	if !strings.HasPrefix(root.Short, "ESET PROTECT CLI") || !strings.HasPrefix(root.Long, "ESET PROTECT CLI") {
		t.Errorf("root Short/Long = %q / %q", root.Short, root.Long[:40])
	}
	find := func(path ...string) (short string, parentGroup string) {
		c, _, err := root.Find(path)
		if err != nil || c == nil {
			t.Fatalf("%v not found: %v", path, err)
		}
		return c.Short, c.Annotations["pp:parent-group"]
	}
	if s, _ := find("auth"); s != "Manage authentication for ESET PROTECT (ESET Connect)" {
		t.Errorf("auth Short = %q", s)
	}
	for _, p := range []string{"fleet", "patches", "report"} {
		s, group := find(p)
		if strings.HasPrefix(s, "Work with") || group != "true" {
			t.Errorf("%s: Short = %q, pp:parent-group = %q", p, s, group)
		}
	}
	for _, v := range buildAgentContext(root).Auth.EnvVars {
		if v.Name == "ESET_PROTECT_PASSWORD" && !strings.Contains(v.Description, "exchanges it for a bearer token") {
			t.Errorf("ESET_PROTECT_PASSWORD description = %q", v.Description)
		}
	}
}
