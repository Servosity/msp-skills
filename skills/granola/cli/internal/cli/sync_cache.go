// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"granola-pp-cli/internal/cliutil"
	"granola-pp-cli/internal/granola"
	"granola-pp-cli/internal/granola/safestorage"
)

// CacheSyncResult captures everything a cache sync produces, in a form that
// either the user-facing `sync` command or the auto-refresh hook can consume.
// HydrateErr is non-fatal: hydration of /v2/get-documents may fail while the
// cache decrypt itself succeeded, so callers surface it as a warning rather
// than aborting.
type CacheSyncResult struct {
	Version          int
	Meetings         int
	Attendees        int
	Segments         int
	Folders          int
	Memberships      int
	Panels           int
	Recipes          int
	Workspaces       int
	ChatThreads      int
	ChatMessages     int
	DocumentsFetched int
	HydrateErr       error
	StateWriteErr    error
	Duration         time.Duration
}

// TotalRows is the headline count used by the auto-refresh provenance line.
func (r CacheSyncResult) TotalRows() int {
	return r.Meetings + r.Attendees + r.Segments + r.Folders + r.Memberships +
		r.Panels + r.Recipes + r.Workspaces + r.ChatThreads + r.ChatMessages
}

// newSyncCacheCmd is registered as the top-level 'sync' replacement.
// Granola's public API only covers ~3 endpoints; the cache file is the
// real source of truth. We hydrate the SQLite store from the cache and
// emit one ndjson summary line so downstream agents and existing sync
// callers see a consistent shape.
func newSyncCacheCmd(flags *rootFlags) *cobra.Command {
	// Inherit the full generator-emitted sync surface (--resources, --since,
	// --db, --max-pages, --full, …) so `sync` accepts every flag the data
	// pipeline expects, then layer the v7.4x behavior on top: a best-effort
	// desktop-cache attempt (sealed on v7.4x+) followed by the framework REST
	// sync (notes/folders) + enrich of the rich tables.
	cmd := newSyncCmd(flags)
	cmd.Short = "Sync Granola into the local store (v7.4x: public REST API + enrich)"
	cmd.Long = `On Granola v7.4x+ the encrypted desktop store is sealed (app-private
behind Granola's macOS Keychain access group), so this command syncs notes and
folders from the public REST API (GRANOLA_API_KEY), then enriches meetings,
transcripts, and attendees from the /v1/notes/{id} detail endpoint — plus human
notes via Granola's official MCP when connected. On a pre-7.4x desktop whose
cache still decrypts, it hydrates from that cache instead.

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
		// 1. Best-effort desktop cache (pre-7.4x). Sealed on v7.4x+ → fall through.
		//    Skipped under verify: it touches real Keychain + internal-API state
		//    the mock HTTP server can't stand in for; the framework REST sync
		//    below is what the data-pipeline gate exercises.
		if !cliutil.IsVerifyEnv() {
			res, err := runCacheSync(cmd.Context())
			if err == nil {
				summary := map[string]any{
					"event":               "sync_summary",
					"source":              "granola_cache",
					"version":             res.Version,
					"meetings":            res.Meetings,
					"attendees":           res.Attendees,
					"transcript_segments": res.Segments,
					"folders":             res.Folders,
					"folder_memberships":  res.Memberships,
					"panel_templates":     res.Panels,
					"recipes":             res.Recipes,
					"workspaces":          res.Workspaces,
					"chat_threads":        res.ChatThreads,
					"chat_messages":       res.ChatMessages,
					"documents_fetched":   res.DocumentsFetched,
				}
				if res.HydrateErr != nil {
					summary["documents_fetch_error"] = res.HydrateErr.Error()
				}
				b, _ := json.Marshal(summary)
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
				if res.HydrateErr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: documents API hydrate failed: %v\n", res.HydrateErr)
				}
				if res.StateWriteErr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to write sync state: %v\n", res.StateWriteErr)
				}
				return nil
			} else if !errors.Is(err, safestorage.ErrKeyUnavailable) &&
				!errors.Is(err, safestorage.ErrDecryptFailed) &&
				!errors.Is(err, safestorage.ErrUnsupportedPlatform) &&
				!errors.Is(err, os.ErrNotExist) {
				// Fall through to REST whenever the desktop store is simply
				// unusable: sealed (v7.4x+), unsupported platform, or absent
				// (a REST-only machine with no Granola desktop app — the common
				// published-CLI case). Surface only a genuinely corrupt present
				// cache rather than masking it.
				return err
			}
			fmt.Fprintln(cmd.ErrOrStderr(), "note: desktop store unavailable; syncing via public REST API + enrich")
		}
		// 2. Framework REST sync (notes/folders) with the inherited flags.
		if err := frameworkRunE(cmd, args); err != nil {
			return err
		}
		// 3. Enrich the rich tables from the REST detail endpoint (+ MCP notes).
		//    Skipped under verify (the mock has no per-note detail flow).
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

// PATCH(auto-refresh): Factored out of newSyncCacheCmd.RunE so the auto-refresh
// hook in PersistentPreRunE can drive the same cache→SQLite hydration without
// going through Cobra's command dispatch. The user-visible sync command becomes
// a thin wrapper that adds JSON output formatting; auto-refresh consumes the
// returned struct and emits a one-line provenance summary instead.
//
// runCacheSync decrypts the encrypted desktop cache, hydrates documents from
// /v2/get-documents, upserts every row into the local SQLite store, and writes
// the SyncState record doctor reads. It is best-effort with respect to document
// hydration (returned in result.HydrateErr) but returns an error when the cache
// itself cannot be opened — the caller decides whether that is fatal.
func runCacheSync(ctx context.Context) (CacheSyncResult, error) {
	started := time.Now()
	// Load the encrypted desktop cache DIRECTLY (not via the now-soft-failing
	// openGranolaCache): a cache sync must detect a decrypt failure and abort,
	// never proceed on an empty cache — SyncFromCache would then wipe the
	// store's folder_memberships and write nothing. On v7.4x+ this always
	// errors; the `sync` command falls back to the REST path (see newSyncCmd).
	c, err := granola.LoadCache("")
	if err != nil {
		// PATCH(encrypted-cache): record the decrypt failure so doctor
		// can report it without itself prompting the Keychain.
		recordSyncDecryptStatus(err)
		return CacheSyncResult{Duration: time.Since(started)}, err
	}
	// PATCH(encrypted-cache): Granola desktop moved documents
	// out of cache-v6.json into the API around May 2026. Hydrate
	// from /v2/get-documents so SyncFromCache's meeting upsert
	// loop has something to iterate.
	docsFetched, hydrateErr := granola.HydrateDocumentsFromAPI(c, nil)
	s, err := openGranolaStore(ctx)
	if err != nil {
		return CacheSyncResult{Duration: time.Since(started)}, err
	}
	defer s.Close()
	sres, err := granola.SyncFromCache(ctx, s.DB(), c)
	if err != nil {
		return CacheSyncResult{Duration: time.Since(started)}, err
	}
	res := CacheSyncResult{
		Version:          c.Version,
		Meetings:         sres.Meetings,
		Attendees:        sres.Attendees,
		Segments:         sres.Segments,
		Folders:          sres.Folders,
		Memberships:      sres.Memberships,
		Panels:           sres.Panels,
		Recipes:          sres.Recipes,
		Workspaces:       sres.Workspaces,
		ChatThreads:      sres.ChatThreads,
		ChatMessages:     sres.ChatMessages,
		DocumentsFetched: docsFetched,
		HydrateErr:       hydrateErr,
		Duration:         time.Since(started),
	}
	// PATCH(encrypted-cache): record success so doctor can report
	// "ok (last decrypted: <time>)" without itself decrypting.
	state := granola.SyncState{
		LastSyncAt:           time.Now().UTC(),
		LastDecryptStatus:    granola.DecryptStatusOK,
		LastTokenSource:      tokenSourceLabel(granola.CurrentTokenSource()),
		LastDocumentsFetched: docsFetched,
	}
	if hydrateErr != nil {
		state.LastHydrateErrorMsg = hydrateErr.Error()
	}
	if writeErr := granola.WriteSyncState(state); writeErr != nil {
		// Surface state-write failure on the result so the wrapper can
		// route it to stderr the same way the original RunE did. Kept
		// separate from HydrateErr because the manual sync command
		// prints each with its own stderr label.
		res.StateWriteErr = writeErr
	}
	return res, nil
}

// PATCH(encrypted-cache): translate the load-error error chain into a
// sync-state record so doctor can surface "decrypt failed" specifically
// rather than the generic "load failed".
func recordSyncDecryptStatus(err error) {
	state := granola.SyncState{
		LastSyncAt:          time.Now().UTC(),
		LastDecryptStatus:   granola.DecryptStatusFailed,
		LastDecryptErrorMsg: err.Error(),
	}
	switch {
	case errors.Is(err, safestorage.ErrKeyUnavailable):
		state.LastDecryptErrorClass = "key_unavailable"
	case errors.Is(err, safestorage.ErrDecryptFailed):
		state.LastDecryptErrorClass = "decrypt_failed"
	case errors.Is(err, safestorage.ErrUnsupportedPlatform):
		state.LastDecryptErrorClass = "unsupported_platform"
	default:
		state.LastDecryptErrorClass = "other"
	}
	_ = granola.WriteSyncState(state)
}

// tokenSourceLabel returns a human-readable + JSON-stable label for the
// TokenSource enum. Used in the sync state record.
func tokenSourceLabel(s granola.TokenSource) string {
	switch s {
	case granola.TokenSourceEnvOverride:
		return "env_override"
	case granola.TokenSourcePlaintextSupabase:
		return "plaintext_supabase"
	case granola.TokenSourceEncryptedSupabase:
		return "encrypted_supabase"
	case granola.TokenSourceStoredAccounts:
		return "stored_accounts"
	}
	return "unknown"
}
