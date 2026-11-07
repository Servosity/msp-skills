// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"connectwise-automate-pp-cli/internal/fleet"
)

func newNovelOsInventoryCmd(flags *rootFlags) *cobra.Command {
	var eolOnly bool

	cmd := &cobra.Command{
		Use:   "os-inventory",
		Short: "Fleet-wide operating-system distribution with end-of-life OSes (Windows 7, Server 2008/2012) flagged",
		Long: strings.Trim(`
Group every computer by operating system with a count and an end-of-life flag
(Windows 7/8/10/XP/Vista/2000 except Windows 10 LTSC, Server 2003/2008/2012). Pass --eol-only to see just
the exposure. Fleet-wide OS posture for security and hardware-refresh planning.

Reads the local SQLite mirror — run 'sync' first.`, "\n"),
		Example: strings.Trim(`
  connectwise-automate-cli os-inventory --agent
  connectwise-automate-cli os-inventory --eol-only --agent`, "\n"),
		Annotations: map[string]string{"pp:data-source": "local", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			s, err := openTranscendStore(cmd)
			if err != nil {
				return err
			}
			defer s.Close()
			computers, err := loadResource(cmd, flags, s, "computers")
			if err != nil {
				return err
			}
			result := fleet.OSInventory(computers, eolOnly)
			return emitResult(cmd, flags, result)
		},
	}
	cmd.Flags().BoolVar(&eolOnly, "eol-only", false, "Show only end-of-life operating systems")
	return cmd
}
