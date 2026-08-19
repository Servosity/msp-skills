// Copyright 2026 geekbrownbear and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

func newNovelScoreCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "score",
		Short: "score subcommands: attribute, regressions",
		Example: strings.Trim(`
  cork-cli score regressions --since 7d --min-drop 10 --agent
  cork-cli score attribute 00000000-0000-0000-0000-000000000000 --since 30d --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelScoreAttributeCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelScoreRegressionsCmd(flags))
	return cmd
}
