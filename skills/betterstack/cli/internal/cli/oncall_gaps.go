// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature (hand-built): on-call coverage gaps from the local mirror.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

type onCallGap struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"default_calendar"`
	Why       string `json:"why"`
}

// pp:data-source local
func newNovelOncallGapsCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:         "oncall-gaps",
		Short:       "Detect on-call calendars with nobody currently on call.",
		Long:        "Use this command to find on-call rotations with nobody currently on call. Do NOT use it to find monitors with no escalation policy; use 'coverage' instead. Reads on-call calendars from the local mirror and flags any with nobody currently on call. Run `sync` first.",
		Example:     "  betterstack-cli oncall-gaps\n  betterstack-cli oncall-gaps --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			s, err := openAnalyticsStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer s.Close()
			maybeEmitSyncHints(cmd, s, "on-calls", flags.maxAge)

			onCalls, err := loadOnCalls(cmd.Context(), s)
			if err != nil {
				return fmt.Errorf("reading on-call calendars: %w", err)
			}

			gaps := make([]onCallGap, 0)
			for _, oc := range onCalls {
				if oc.OnCallUsers > 0 {
					continue
				}
				name := oc.Name
				if name == "" {
					name = oc.ID
				}
				gaps = append(gaps, onCallGap{ID: oc.ID, Name: name, IsDefault: oc.IsDefault, Why: "nobody currently on call"})
			}
			sort.SliceStable(gaps, func(i, j int) bool {
				if gaps[i].IsDefault != gaps[j].IsDefault {
					return gaps[i].IsDefault // default calendars first — most important to cover
				}
				return gaps[i].Name < gaps[j].Name
			})

			if flags.asJSON {
				return flags.printJSON(cmd, gaps)
			}
			if len(onCalls) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "local mirror has no on-call calendars — run `betterstack-cli sync` first")
			}
			if len(gaps) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No on-call gaps: every calendar has someone on call.")
				return nil
			}
			rows := make([][]string, 0, len(gaps))
			for _, g := range gaps {
				def := ""
				if g.IsDefault {
					def = "yes"
				}
				rows = append(rows, []string{g.ID, truncateField(g.Name, 40), def, g.Why})
			}
			return flags.printTable(cmd, []string{"ID", "CALENDAR", "DEFAULT", "GAP"}, rows)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/betterstack-cli/data.db)")
	return cmd
}
