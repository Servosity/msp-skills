// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"runzero-pp-cli/internal/inventory"
)

// pp:data-source local
func newNovelCertsExpiringCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var days int
	var weakOnly bool

	cmd := &cobra.Command{
		Use:   "certs-expiring",
		Short: "List TLS certificates expiring within a window or using weak crypto, joined to the asset and service presenting them.",
		Long: strings.Trim(`
Use this command for certs expiring soon or using weak crypto, with their
asset/service context. Do NOT use it for raw certificate rows or full-text
cert search; use 'inventory list certificates' instead.

Filters the locally-synced certificates by expiry window (already expired or
expiring within --days) and joins each to the presenting asset, ordered by
asset criticality and soonest expiry. --weak restricts to certificates that
are self-signed or carry an MD5/SHA-1 signature (detected from the stored
certificate data when present).

Reads the local store; run 'inventory sync' first.`, "\n"),
		Example: strings.Trim(`
  # Certs already expired or expiring in the next 30 days
  runzero-cli certs-expiring --agent

  # 90-day lookahead, weak-crypto certs only
  runzero-cli certs-expiring --days 90 --weak --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			st, err := openInvStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer st.Close()
			rows, err := inventory.CertsExpiring(cmd.Context(), st.DB(), days, weakOnly)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().IntVar(&days, "days", 30, "Expiry lookahead window in days (already-expired certs are always included)")
	cmd.Flags().BoolVar(&weakOnly, "weak", false, "Only certificates that are self-signed or use an MD5/SHA-1 signature")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/runzero-cli/data.db)")
	return cmd
}
