// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-implemented novel feature for skykick-cli (stub replaced; survives regen-merge).
package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"skykick-pp-cli/internal/fleet"
)

type partnerRollupView struct {
	Meta     fleetEnvelopeMeta      `json:"meta"`
	Partners []fleet.PartnerSummary `json:"partners"`
	Note     string                 `json:"note,omitempty"`
}

func newNovelPartnerRollupCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "partner-rollup",
		Short: "Protection posture aggregated by partner for distributor oversight",
		Long: strings.Trim(`
Use this for posture aggregated by partner (the distributor view): tenants per
partner, tenants with gaps, unprotected mailboxes/sites, stale tenants.
Subscriptions whose partner id is not exposed by the API group under
"(unknown)". Reads the local fleet store - run 'fleet-sync' first. For one
partner's raw subscription list, use 'backup by-partner'.
`, "\n"),
		Example: strings.Trim(`
  skykick-cli partner-rollup --agent
  skykick-cli partner-rollup --agent --select partners.partner_id,partners.tenants_with_gaps
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would aggregate fleet posture by partner from the local store")
				return nil
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
			postures := fleet.BuildPostures(state, time.Now().UTC())
			rollup := fleet.PartnerRollup(postures)
			view := partnerRollupView{Meta: metaForRun(run), Partners: rollup}
			if view.Partners == nil {
				view.Partners = []fleet.PartnerSummary{}
			}
			if len(rollup) == 1 && rollup[0].PartnerID == "(unknown)" {
				view.Note = "the /Backup list response did not expose partner ids for these subscriptions; all tenants grouped under (unknown). Partner-scoped reads are still available via 'backup by-partner <partnerId>'."
			}

			humanRows := make([]map[string]any, 0, len(rollup))
			for _, p := range rollup {
				humanRows = append(humanRows, map[string]any{
					"partner": p.PartnerID, "tenants": p.Tenants, "with_gaps": p.TenantsWithGaps,
					"unprot_mailboxes": p.UnprotectedBoxes, "unprot_sites": p.UnprotectedSites, "stale": p.StaleTenants,
				})
			}
			summary := fmt.Sprintf("%d partners, %d tenants (run %d)", len(rollup), len(postures), run.ID)
			return fleetPrint(cmd, flags, view, humanRows, summary)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default ~/.local/share/skykick-cli/data.db)")
	return cmd
}
