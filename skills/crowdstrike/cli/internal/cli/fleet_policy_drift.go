// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel-feature command (Printing Press transcendence).

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelFleetPolicyDriftCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var baseline string
	cmd := &cobra.Command{
		Use:   "policy-drift",
		Short: "Diff each tenant's enabled prevention policies against a baseline to find under-protected tenants",
		Long: "Compare every tenant's enabled-prevention-policy signature against a baseline " +
			"CID (or the most common signature when --baseline is omitted) and report which " +
			"tenants diverge. Run 'fleet sync' first. Reads no live API.",
		Example:     "  crowdstrike-cli fleet policy-drift --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			st, err := openFleetStore(cmd.Context(), resolveFleetDB(dbPath))
			if err != nil {
				return configErr(err)
			}
			defer st.Close()
			if !hintIfFleetUnsynced(cmd, st) {
				hintIfFleetStale(cmd, st, flags.maxAge)
			}
			ents, err := loadFleetEntitiesFrom(cmd.Context(), st, kindPolicy, false)
			if err != nil {
				return configErr(err)
			}
			// An explicit baseline with no synced policies would diff the
			// whole fleet against an empty set and flag universal drift with
			// no signal that the baseline itself was bad. Refuse instead.
			if baseline != "" && len(policySignature(ents, baseline)) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("baseline CID %q has no synced prevention policies; check the CID or run 'fleet sync' for it first", baseline))
			}
			return flags.printJSON(cmd, policyDrift(ents, baseline))
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite store path (default: standard data dir)")
	cmd.Flags().StringVar(&baseline, "baseline", "", "Baseline CID to compare against (default: most common signature)")
	return cmd
}
