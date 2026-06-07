// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"runzero-pp-cli/internal/cliutil"
	"runzero-pp-cli/internal/inventory"
	"runzero-pp-cli/internal/store"
)

// openInvStore opens the shared local SQLite store (the same file the generated
// sync/search/sql commands use) and ensures the inventory tables exist. Every
// transcendence command (triage, diff, affected, software rollup, stale,
// exposure-map) and the inventory command group route through this helper so
// they all read and write one store.
func openInvStore(cmd *cobra.Command, dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("runzero-cli")
	}
	st, err := store.OpenWithContext(cmd.Context(), dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening store at %s: %w", dbPath, err)
	}
	if err := inventory.EnsureSchema(cmd.Context(), st.DB()); err != nil {
		_ = st.Close()
		return nil, fmt.Errorf("ensuring inventory schema: %w", err)
	}
	return st, nil
}

// newInventoryCmd is the parent for the local-store data layer that powers the
// transcendence commands. `inventory sync` pulls runZero's per-entity Export
// endpoints into local SQLite; `inventory status` reports what is stored.
func newInventoryCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inventory",
		Short: "Local attack-surface store: sync runZero's per-entity export data, then query it offline",
		Long: strings.Trim(`
Manage the local SQLite copy of your runZero attack surface that powers the
offline analysis commands (triage, diff, affected, software rollup,
stale, exposure-map, exposure-delta, certs-expiring).

runZero's HTTP API is organized by scope (/org, /account, /export), so a single
live query cannot join assets to their services, software, and vulnerabilities.
'inventory sync' pulls the per-entity Export endpoints
(/export/org/<entity>.json) into local tables once; the offline analysis commands
then answer cross-entity questions with zero additional API quota.`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newInventorySyncCmd(flags))
	cmd.AddCommand(newInventoryStatusCmd(flags))
	cmd.AddCommand(newInventoryListCmd(flags))
	return cmd
}

func newInventoryListCmd(flags *rootFlags) *cobra.Command {
	var dbPath, match string
	var limit int
	cmd := &cobra.Command{
		Use:   "list <entity>",
		Short: "Browse synced rows for an entity (assets, services, software, vulnerabilities, certificates, wireless, sites)",
		Long: strings.Trim(`
List the locally-synced rows for one entity, newest sync first. Returns the
original runZero JSON for each row, so --select with dotted paths narrows deeply
nested records. Reads the local store; run 'inventory sync' first.`, "\n"),
		Example: strings.Trim(`
  runzero-cli inventory list assets --limit 20 --agent
  runzero-cli inventory list vulnerabilities --match log4j --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			st, err := openInvStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer st.Close()
			rows, err := inventory.List(cmd.Context(), st.DB(), args[0], limit, match)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum rows to return")
	cmd.Flags().StringVar(&match, "match", "", "Case-insensitive substring filter against the row JSON")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/runzero-cli/data.db)")
	return cmd
}

func newInventorySyncCmd(flags *rootFlags) *cobra.Command {
	var dbPath, search, only string
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Pull assets, services, software, vulnerabilities, certificates, wireless, and sites into the local store",
		Long: strings.Trim(`
Pull runZero's per-entity Export data into the local SQLite store so the
offline analysis commands can run offline. Each run is recorded as a snapshot, so
running sync twice lets 'diff' show what changed on the attack surface.

Requires a runZero API token (RUNZERO_API_KEY). The token's scope must include
export access (Export ET, Organization OT, or Account CT key).`, "\n"),
		Example: strings.Trim(`
  # Sync the whole attack surface
  runzero-cli inventory sync

  # Sync only assets and services, filtered by a runZero query
  runzero-cli inventory sync --only assets,services --search 'alive:t'`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			// inventory sync dials the live Export API; keep verify runs offline.
			if cliutil.IsVerifyEnv() {
				fmt.Fprintln(cmd.OutOrStdout(), "would sync runZero export data into the local store")
				return nil
			}
			client, err := flags.newClient()
			if err != nil {
				return err
			}
			st, err := openInvStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer st.Close()
			var onlyList []string
			if strings.TrimSpace(only) != "" {
				onlyList = strings.Split(only, ",")
			}
			res, err := inventory.SyncAll(cmd.Context(), client, st.DB(), onlyList, search)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, res)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/runzero-cli/data.db)")
	cmd.Flags().StringVar(&search, "search", "", "runZero search query passed through to the export endpoints (e.g. 'alive:t os:\"Windows\"')")
	cmd.Flags().StringVar(&only, "only", "", "Comma-separated subset of entities to sync (assets,services,software,vulnerabilities,certificates,wireless,sites)")
	return cmd
}

func newInventoryStatusCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:         "status",
		Short:       "Show local store row counts per entity and the last sync time",
		Example:     "  runzero-cli inventory status --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			st, err := openInvStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer st.Close()
			res, err := inventory.Status(cmd.Context(), st.DB())
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, res)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/runzero-cli/data.db)")
	return cmd
}
