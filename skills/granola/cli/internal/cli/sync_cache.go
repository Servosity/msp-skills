// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"granola-pp-cli/internal/cliutil"
)

// newSyncCacheCmd is registered as the top-level 'sync' replacement.
//
// Granola v7.4x+ sealed the encrypted desktop store (cache-v6.json.enc, guarded
// by its own macOS Keychain access group) and its internal api.granola.ai
// endpoints, so a third-party binary can no longer read them. This command
// therefore sources ENTIRELY from the public REST API: it runs the framework
// REST sync (notes/folders) and then enriches the rich tables (meetings,
// transcripts, attendees) from the /v1/notes/{id} detail endpoint — plus human
// notes via Granola's official MCP when connected.
func newSyncCacheCmd(flags *rootFlags) *cobra.Command {
	// Inherit the full generator-emitted sync surface (--resources, --since,
	// --db, --max-pages, --full, …) so `sync` accepts every flag the data
	// pipeline expects, then layer the enrich step on top.
	cmd := newSyncCmd(flags)
	cmd.Short = "Sync Granola into the local store (public REST API + enrich)"
	cmd.Long = `On Granola v7.4x+ the encrypted desktop store and internal API are sealed
(app-private behind Granola's macOS Keychain access group), so this command
syncs notes and folders from the public REST API (GRANOLA_API_KEY), then
enriches meetings, transcripts, and attendees from the /v1/notes/{id} detail
endpoint — plus human notes via Granola's official MCP when connected.

The rich tables are keyed by REST not_ ids; a stale pre-7.4x store may still
hold UUID-keyed rows. For a clean REST-only store:
  rm ~/.local/share/granola-cli/data.db && granola-cli sync --full`
	// Override the generic framework example (which lists non-existent resources
	// like channels,messages) with Granola's real syncable surface.
	cmd.Example = "  granola-cli sync                      # notes + folders, enrich 50 most-recent\n" +
		"  granola-cli sync --full              # sync + enrich the whole library\n" +
		"  granola-cli sync --since 7d          # incremental since 7 days ago\n" +
		"  granola-cli sync --resources folders # folders only (skip note-detail enrich)"
	frameworkRunE := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if dryRunOK(flags) {
			return nil
		}
		if dbp, _ := cmd.Flags().GetString("db"); dbp != "" {
			granolaStoreOverride = dbp
		}
		// 1. Framework REST sync (notes/folders) with the inherited flags.
		if err := frameworkRunE(cmd, args); err != nil {
			return err
		}
		// 2. Enrich the rich tables from the REST detail endpoint (+ MCP notes).
		//    Skipped under verify (the mock has no per-note detail flow); the
		//    data-pipeline gate exercises the framework sync above.
		if cliutil.IsVerifyEnv() {
			return nil
		}
		// Respect --resources scoping: enrich fetches per-note detail, so it only
		// applies when notes were in scope. If the user synced only folders, skip
		// the note-detail fetches rather than pulling 50 unrelated notes.
		if res, _ := cmd.Flags().GetStringSlice("resources"); len(res) > 0 {
			notesScoped := false
			for _, r := range res {
				if r == "notes" {
					notesScoped = true
					break
				}
			}
			if !notesScoped {
				return nil
			}
		}
		full, _ := cmd.Flags().GetBool("full")
		lim := 50
		if full {
			lim = 0
		}
		if isDogfoodEnv() {
			lim = 2 // bound per-note detail fetches to fit the dogfood timeout
		}
		n, notes, req, ferr := enrichRecent(cmd.Context(), flags, lim)
		summary := map[string]any{
			"event": "enrich_summary", "source": "rest+enrich",
			"enriched": n, "requested": req, "human_notes": notes,
		}
		if ferr != nil {
			// The notes/folders base sync already landed; emit the enrich summary,
			// then exit non-zero so automation can tell a fully synced store from
			// one whose rich tables are stale. (Under verify this path is skipped.)
			summary["enrich_error"] = ferr.Error()
			_ = emitJSON(cmd, flags, summary)
			return apiErr(fmt.Errorf("enrich failed after base sync (notes/folders synced OK): %w", ferr))
		}
		return emitJSON(cmd, flags, summary)
	}
	return cmd
}
