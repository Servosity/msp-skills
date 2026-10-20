// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// createAliasAllowed lists the commands whose hidden `create` alias is
// accurate: each POSTs a new resource into a collection. Anything else that
// answers to `create` (close an incident, move a device, block an
// executable) is a destructive surprise for an agent that guesses the verb
// (handfixes.json: no-create-alias-on-non-create-commands).
var createAliasAllowed = map[string]string{
	"eset-protect-cli incidents comments incident-create-incident": "POST /v2/incidents/{uuid}/comments creates a comment",
	"eset-protect-cli policy-assignments assign-policy":            "POST /v2/policy-assignments creates an assignment",
}

func TestNoCreateAliasOnNonCreateCommands(t *testing.T) {
	root := RootCmd()
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		// Every sibling name and alias must resolve to exactly one command,
		// or an alias can shadow a real subcommand of the same name.
		seen := map[string]string{}
		for _, child := range c.Commands() {
			for _, name := range append([]string{child.Name()}, child.Aliases...) {
				if prev, dup := seen[name]; dup {
					t.Errorf("%s: %q is claimed by both %q and %q", c.CommandPath(), name, prev, child.Name())
				}
				seen[name] = child.Name()
			}
		}
		if c.HasAlias("create") {
			path := c.CommandPath()
			if _, ok := createAliasAllowed[path]; !ok && !strings.Contains(c.Name(), "create") {
				t.Errorf("%s (%s %s) answers to `create` but does not create anything", path, c.Annotations["pp:method"], c.Annotations["pp:path"])
			}
			if m := c.Annotations["pp:method"]; m != "POST" {
				t.Errorf("%s carries a create alias on a %s endpoint", path, m)
			}
		}
		for _, child := range c.Commands() {
			walk(child)
		}
	}
	walk(root)

	// The removed aliases must not resolve to the destructive commands.
	for _, parent := range []string{"incidents", "devices", "executables", "detection-groups", "detections-v2", "ip-sets", "incidents basic-attributes-update"} {
		args := append(strings.Fields(parent), "create")
		if found, _, err := root.Find(args); err == nil && found != nil && found.HasAlias("create") {
			t.Errorf("`%s create` still resolves to %q", parent, found.CommandPath())
		}
	}
}
