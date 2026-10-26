// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// localReadExtraArgs are the inputs a local read needs to return rows from the
// two-tenant fixture. A local read missing here runs with no extra input.
var localReadExtraArgs = map[string][]string{
	"applications hunt": {"--hash", "aaaa"},
	"audit drift":       {"--since", "30d"},
}

// localReadCommands walks the real command tree and returns every command
// that declares itself a local-store read (pp:data-source local), keyed by
// its path below the root.
func localReadCommands(root *cobra.Command) map[string]*cobra.Command {
	found := map[string]*cobra.Command{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Annotations["pp:data-source"] == "local" && c.Runnable() {
			found[strings.TrimPrefix(c.CommandPath(), root.Name()+" ")] = c
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
	return found
}

// localReadSourceFiles lists the hand-written files that declare a local-store
// read in their header (`// pp:data-source local`), the press's data-source
// contract. Each must register a command carrying the same annotation, or the
// walk above would silently skip it.
func localReadSourceFiles(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.SplitN(string(b), "\n", 16)
		for _, l := range lines[:len(lines)-1] {
			if strings.TrimSpace(l) == "// pp:data-source local" {
				files = append(files, p)
				break
			}
		}
	}
	sort.Strings(files)
	return files
}

// Codex r2 P1: `applications hunt --org org-1 --hash H` still reported tenant
// org-2. Every local-store read must honour an explicit --org: with --org org-1
// it shows org-1's rows and none of org-2's. The command list comes from the
// real command tree, so a new local read is covered the day it is added.
func TestEveryLocalReadHonoursAnExplicitOrg(t *testing.T) {
	commands := localReadCommands(RootCmd())
	files := localReadSourceFiles(t)
	if len(commands) != len(files) {
		var names []string
		for n := range commands {
			names = append(names, n)
		}
		sort.Strings(names)
		t.Fatalf("%d file(s) declare `// pp:data-source local` (%v) but %d command(s) carry the pp:data-source=local annotation (%v): annotate every local read so this test runs it", len(files), files, len(commands), names)
	}
	dbPath := seedTwoTenantStore(t)
	var names []string
	for n := range commands {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			args := append(strings.Fields(name), localReadExtraArgs[name]...)
			args = append(args, "--json", "--db", dbPath, "--org", "org-1")
			out, err := runTLWithOrgEnv(t, "", "", args...)
			if err != nil {
				t.Fatalf("%s --org org-1: %v\n%s", name, err, out)
			}
			if !strings.Contains(out, "org-1") && !strings.Contains(out, "Tenant 1") {
				t.Fatalf("%s --org org-1 returned nothing from org-1, so the scope check below is vacuous:\n%s", name, out)
			}
			if strings.Contains(out, "org-2") || strings.Contains(out, "Tenant 2") {
				t.Fatalf("%s --org org-1 returned rows from org-2:\n%s", name, out)
			}
		})
	}
}
