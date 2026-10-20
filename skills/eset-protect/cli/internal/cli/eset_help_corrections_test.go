// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
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

// hand-fix guard (handfixes.json: help-text-matches-surface): the generator
// described five groups as "List, get, and create ..." although none has a
// create subcommand, told users `import --dry-run` sends no requests (it still
// mints a token when none is cached), and wrote `QUERY="$(...)" cli "$QUERY"`
// examples whose "$QUERY" expands before the prefix assignment takes effect.
func TestHelpTextMatchesSurface(t *testing.T) {
	root := RootCmd()
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if strings.Contains(strings.ToLower(c.Short), " create") && c.HasSubCommands() {
			found := false
			for _, sub := range c.Commands() {
				if strings.HasPrefix(sub.Name(), "create") || strings.HasPrefix(strings.ToLower(sub.Short), "create") {
					found = true
				}
			}
			if !found {
				t.Errorf("%q: Short %q names create, but it has no create subcommand", c.CommandPath(), c.Short)
			}
		}
		if strings.Contains(c.Example, `QUERY="$(`) {
			t.Errorf("%q: example uses QUERY=\"$(...)\" prefix assignment, which passes a stale query", c.CommandPath())
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
	imp, _, err := root.Find([]string{"import"})
	if err != nil {
		t.Fatal(err)
	}
	if f := imp.Flags().Lookup("dry-run"); f == nil || !strings.Contains(f.Usage, "access token may still be requested") {
		t.Errorf("import --dry-run usage must disclose the token request: %v", f)
	}
}
