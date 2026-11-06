// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

// jamf-pro hand-fix (mcp-recipe-argv): a recipe intent runs its argv only
// when that argv resolves to the recipe's own command. The press's recipe
// lifter can splice a subcommand in as a tool argument (it read
// `fleet policy-hygiene` as `fleet <slug>`), and a tool argument must never
// choose which command runs. Values stay data: flag-like positionals already
// land after `--` (appendRecipePositional) and string flags are one
// --name=value element.

import (
	"context"
	"fmt"
	"strings"

	"jamf-pro-pp-cli/internal/cli"
	"jamf-pro-pp-cli/internal/mcp/cobratree"
)

// runRecipeCommand runs args through the companion CLI after checking that
// the command it resolves to is the one recipePath names (its non-flag words).
func runRecipeCommand(ctx context.Context, recipePath, args []string) (cobratree.CLICommandResult, error) {
	root := cli.RootCmd()
	want := []string{root.Name()}
	for _, w := range recipePath {
		if !strings.HasPrefix(w, "-") {
			want = append(want, w)
		}
	}
	cmd, _, err := root.Find(args)
	if err != nil || cmd == nil || cmd.CommandPath() != strings.Join(want, " ") {
		return cobratree.CLICommandResult{}, fmt.Errorf("refused: this tool runs only '%s'", strings.Join(want[1:], " "))
	}
	return cobratree.RunCLICommand(ctx, recipeCLIPath, args)
}
