// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: no MCP tool takes a filesystem path.

package cli

import (
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// pathFlag matches flag names that take a filesystem path: --notes-file,
// --playbook-file, --playbook-notes-file, --db, any *-dir or *-path.
var pathFlag = regexp.MustCompile(`^(.+-)?(file|dir|path|db)$`)

// The generated teach and teach-playbook commands read the file a path flag
// names into the learning store, and recall, playbook list and the sql tool
// return it, so over MCP a prompt-injected agent could read credentials.toml
// (the ION refresh token) back into its context. mcp:write-flags drops a flag
// from the MCP schema and refuses it as an argument; this walk adds every
// path-taking flag in the tree so a regenerated command cannot reopen it.
func init() {
	registerNovelCommand(func(root *cobra.Command, _ *rootFlags) {
		annotatePathFlags(root)
	})
}

func annotatePathFlags(cmd *cobra.Command) {
	var names []string
	cmd.LocalNonPersistentFlags().VisitAll(func(f *pflag.Flag) {
		if f.Value.Type() != "bool" && pathFlag.MatchString(f.Name) {
			names = append(names, f.Name)
		}
	})
	if len(names) > 0 {
		if cmd.Annotations == nil {
			cmd.Annotations = map[string]string{}
		}
		existing := cmd.Annotations["mcp:write-flags"]
		cmd.Annotations["mcp:write-flags"] = strings.Trim(existing+","+strings.Join(names, ","), ",")
	}
	for _, child := range cmd.Commands() {
		annotatePathFlags(child)
	}
}
