// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"mspbots-pp-cli/internal/store"
)

// newNovelRegistryCmd is the alias↔resourceId registry. The Public API has no
// enumeration endpoint, so this local table is the CLI's discovery surface.
func newNovelRegistryCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Name your bound datasets and widgets once, then use readable aliases everywhere instead of 19-digit resource IDs.",
		Long: strings.Trim(`
Use this command to register or list local alias→resourceId mappings.
Do NOT use it to fetch data; use 'pull' to fetch a registered resource.

The MSPbots Public API has no list endpoint: resource IDs come from the
MSPbots app UI (Settings → Public API). Register each one once and every
other command (pull, export, snapshot, diff, trend, describe) accepts the
alias.`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newRegistryAddCmd(flags))
	cmd.AddCommand(newRegistryListCmd(flags))
	cmd.AddCommand(newRegistryRmCmd(flags))
	return cmd
}

func newRegistryAddCmd(flags *rootFlags) *cobra.Command {
	var resourceType string
	var notes string
	var dbPath string
	cmd := &cobra.Command{
		Use:   "add [alias] [resourceId]",
		Short: "Register an alias for a dataset or widget resource ID",
		Example: strings.Trim(`
  mspbots-cli registry add open-tickets 1534956341424005122 --type dataset
  mspbots-cli registry add sla-widget 1534956341424005123 --type widget --notes "SLA board"`, "\n"),
		Annotations: map[string]string{
			// No mcp:read-only: registry add writes a row into the local SQLite
			// registry table (RegistryAdd) — a store update, which AGENTS.md
			// excludes from the read-only annotation.
			"pp:happy-args": "alias=example-alias;resourceId=1534956341424005122;--type=dataset",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if len(args) < 2 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("registry add needs <alias> and <resourceId>"))
			}
			if dryRunOK(flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "would register alias %q → %s (%s)\n", args[0], args[1], resourceType)
				return nil
			}
			alias, id := strings.TrimSpace(args[0]), strings.TrimSpace(args[1])
			if alias == "" || id == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("alias and resourceId must be non-empty"))
			}
			if rawResourceIDRe.MatchString(alias) {
				return usageErr(fmt.Errorf("alias %q looks like a resource ID; pass the human name first: registry add <alias> <resourceId>", alias))
			}
			if !rawResourceIDRe.MatchString(id) {
				return usageErr(fmt.Errorf("resourceId %q does not look like an MSPbots resource ID (long numeric snowflake from the app UI)", id))
			}
			if err := validateResourceType(resourceType, false); err != nil {
				return usageErr(err)
			}
			db, err := openMspbotsStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			entry := store.RegistryEntry{Alias: alias, ResourceID: id, ResourceType: resourceType, Notes: notes}
			if err := db.RegistryAdd(cmd.Context(), entry); err != nil {
				return fmt.Errorf("registering alias: %w", err)
			}
			saved, err := db.RegistryGet(cmd.Context(), alias)
			if err != nil {
				return fmt.Errorf("reading back alias: %w", err)
			}
			return flags.printJSON(cmd, saved)
		},
	}
	cmd.Flags().StringVar(&resourceType, "type", "dataset", "Resource type: dataset or widget")
	cmd.Flags().StringVar(&notes, "notes", "", "Free-text note about what this resource contains")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to the CLI's local store)")
	return cmd
}

func newRegistryListCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List every registered alias",
		Example: "  mspbots-cli registry list --json",
		Annotations: map[string]string{
			"mcp:read-only": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would list registered aliases")
				return nil
			}
			db, err := openMspbotsStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			entries, err := db.RegistryList(cmd.Context())
			if err != nil {
				return fmt.Errorf("listing registry: %w", err)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return flags.printJSON(cmd, entries)
			}
			if len(entries) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No aliases registered yet. Add one: mspbots-cli registry add <alias> <resourceId> --type dataset")
				return nil
			}
			rows := make([][]string, 0, len(entries))
			for _, e := range entries {
				rows = append(rows, []string{e.Alias, e.ResourceType, e.ResourceID, e.Notes})
			}
			return flags.printTable(cmd, []string{"ALIAS", "TYPE", "RESOURCE ID", "NOTES"}, rows)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to the CLI's local store)")
	return cmd
}

func newRegistryRmCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:     "rm [alias]",
		Short:   "Remove a registered alias (local only; never touches the API)",
		Example: "  mspbots-cli registry rm open-tickets",
		Annotations: map[string]string{
			// No mcp:read-only: registry rm DELETES a row from the local SQLite
			// registry table (RegistryRemove). A false readOnlyHint on a
			// destructive command is worse than a missing one — let it default
			// to "could write or delete" so MCP hosts prompt before removal.
			"pp:happy-args": "alias=example-alias",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if len(args) < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("registry rm needs <alias>"))
			}
			if dryRunOK(flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "would remove alias %q\n", args[0])
				return nil
			}
			db, err := openMspbotsStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			n, err := db.RegistryRemove(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("removing alias: %w", err)
			}
			if n == 0 {
				return notFoundErr(fmt.Errorf("alias %q is not registered", args[0]))
			}
			return flags.printJSON(cmd, map[string]any{"removed": args[0]})
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to the CLI's local store)")
	return cmd
}
