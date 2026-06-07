// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-implemented novel feature for skykick-cli (stub replaced; survives regen-merge).
package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"skykick-pp-cli/internal/fleet"
)

type coverageGapsView struct {
	Meta              fleetEnvelopeMeta `json:"meta"`
	Kind              string            `json:"kind"`
	Gaps              []fleet.GapRow    `json:"gaps"`
	UnknownEnablement int               `json:"unknown_enablement"`
	Note              string            `json:"note,omitempty"`
}

func newNovelCoverageGapsCmd(flags *rootFlags) *cobra.Command {
	var dbPath, kind string

	cmd := &cobra.Command{
		Use:   "coverage-gaps",
		Short: "Discovered-but-unprotected mailboxes and SharePoint sites per tenant",
		Long: strings.Trim(`
Use this to surface mailboxes and SharePoint sites that exist in SkyKick's
discovery but are explicitly NOT enabled for backup - the post-onboarding and
post-churn reconciliation gap. Entries whose enablement could not be parsed
from the API response are counted as unknown_enablement, never reported as
gaps. Reads the local fleet store - run 'fleet-sync' first. Do NOT use this
to trigger discovery; use 'backup discover-mailboxes' / 'backup discover-sites'
then 'watch-operation' first.
`, "\n"),
		Example: strings.Trim(`
  skykick-cli coverage-gaps --type mailboxes --agent
  skykick-cli coverage-gaps --type all --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would diff discovered vs backup-enabled mailboxes/sites from the fleet store")
				return nil
			}
			kind = strings.ToLower(strings.TrimSpace(kind))
			switch kind {
			case "", "all", "mailboxes", "sites":
			default:
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--type must be one of: mailboxes, sites, all"))
			}
			if kind == "" {
				kind = "all"
			}
			db, err := openFleetStore(cmdContext(cmd), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			state, run, err := latestFleetState(cmdContext(cmd), db)
			if err != nil {
				return err
			}
			companies := fleet.CompanyIndex(state.Subscriptions, state.Settings)
			gaps, unknown := fleet.CoverageGaps(state.Mailboxes, state.Sites, companies, kind)
			view := coverageGapsView{Meta: metaForRun(run), Kind: kind, Gaps: gaps, UnknownEnablement: unknown}
			if view.Gaps == nil {
				view.Gaps = []fleet.GapRow{}
			}
			if len(gaps) == 0 {
				view.Note = fmt.Sprintf("no explicit coverage gaps found (%d mailboxes, %d sites scanned; %d with unknown enablement)",
					len(state.Mailboxes), len(state.Sites), unknown)
			}

			humanRows := make([]map[string]any, 0, len(gaps))
			for _, g := range gaps {
				humanRows = append(humanRows, map[string]any{
					"company": g.Company, "kind": g.Kind, "name": g.Name, "subscription": g.SubscriptionID,
				})
			}
			summary := fmt.Sprintf("%d unprotected (%s) across the fleet, %d unknown enablement (run %d)", len(gaps), kind, unknown, run.ID)
			return fleetPrint(cmd, flags, view, humanRows, summary)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default ~/.local/share/skykick-cli/data.db)")
	cmd.Flags().StringVar(&kind, "type", "all", "What to check: mailboxes, sites, or all")
	return cmd
}
