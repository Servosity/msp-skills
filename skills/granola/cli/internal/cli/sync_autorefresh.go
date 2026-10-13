// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"granola-pp-cli/internal/client"
	"granola-pp-cli/internal/config"
	"granola-pp-cli/internal/granola"
	"granola-pp-cli/internal/store"
)

// PATCH(auto-refresh): new helper that lets the PersistentPreRunE hook call
// the public-API sync (sync-api) using sensible defaults, without going
// through Cobra's command dispatch or the worker-pool / output choreography
// in newSyncCmd's RunE.
//
// The user-facing `sync-api` command keeps all of its flags (--full, --since,
// --concurrency, --resources, --param, etc.) and its existing stdout/stderr
// shape. Auto-refresh deliberately bypasses that surface because:
//
//   1. The full ceremony (param parsing, worker pool, JSON event stream,
//      strict/critical exit-code policy) is not relevant to a per-command
//      refresh; the user already paid the cost of that choreography when
//      they ran `sync-api` themselves.
//   2. Auto-refresh must not write to stdout — pure JSON consumers should
//      see only the underlying command's output. The provenance line we do
//      emit goes to stderr from the autorefresh dispatcher (see U4).
//   3. Refactoring newSyncCmd's RunE in place would invite output-shape
//      drift the plan explicitly disallows.

// ApiSyncResult captures the headline numbers from a public-API sync.
// Per-resource success/warning/error counts mirror what newSyncCmd's
// exit-code policy uses, but auto-refresh treats anything that synced
// rows as "ok" — see runApiSync below.
type ApiSyncResult struct {
	TotalRecords int
	Resources    int
	Success      int
	Warned       int
	Errored      int
	Duration     time.Duration
}

// TotalRows is the headline count used by the auto-refresh provenance line.
func (r ApiSyncResult) TotalRows() int { return r.TotalRecords }

// runApiSync hits the public REST API for the default resource set and
// upserts results into the local store. It is intentionally tiny: defaults
// for concurrency, no --since filter, no --full, no per-resource param
// flags, no stdout output. Errors per resource are collected into the
// result struct; the function returns a non-nil error only when no
// resource synced any rows AND every resource failed (the "totally broken"
// case the user should see), mirroring the strictest branch of newSyncCmd's
// exit-code policy.
func runApiSync(ctx context.Context, flags *rootFlags) (ApiSyncResult, error) {
	started := time.Now()

	c, err := flags.newClient()
	if err != nil {
		return ApiSyncResult{Duration: time.Since(started)}, err
	}
	c.NoCache = true

	db, err := store.OpenWithContext(ctx, defaultDBPath("granola-cli"))
	if err != nil {
		return ApiSyncResult{Duration: time.Since(started)}, fmt.Errorf("opening local database: %w", err)
	}
	defer db.Close()

	resources := defaultSyncResources()
	var (
		totalSynced  int
		successCount int
		warnCount    int
		errCount     int
		firstErr     error
	)
	for _, resource := range resources {
		select {
		case <-ctx.Done():
			return ApiSyncResult{
				TotalRecords: totalSynced,
				Resources:    successCount + warnCount + errCount,
				Success:      successCount,
				Warned:       warnCount,
				Errored:      errCount,
				Duration:     time.Since(started),
			}, ctx.Err()
		default:
		}
		// Auto-refresh defaults: no --since cursor reset, no --full,
		// no max-pages override (newSyncCmd's default 100 wins), no
		// latest-only, no user params.
		res := syncResource(ctx, c, db, resource, "", false, 0, false, false, nil, nil)
		switch {
		case res.Err != nil:
			errCount++
			if firstErr == nil {
				firstErr = res.Err
			}
		case res.Warn != nil:
			// PATCH(autorefresh-warn-rows-count): include warned-resource
			// rows in TotalRecords. A warning ("partial success", e.g. a
			// rate-limit notice) does not mean zero rows were written;
			// excluding res.Count made provenance read "(0 rows)" for any
			// API sync where every resource warned even when real data
			// landed in the store.
			totalSynced += res.Count
			warnCount++
		default:
			totalSynced += res.Count
			successCount++
		}
	}

	// After the THIN sync (notes/folders) lands rows in the store, hydrate the
	// RICH tables (meetings/attendees/transcript_segments) for a small, bounded
	// set of recently-synced-but-not-yet-hydrated notes. Without this, KEEP
	// commands (talktime/collect/attendee/export/memo/notes-show/stats/
	// duplicates/preflight) read empty rich tables after a clean install because
	// only an explicit `sync`/`enrich` used to call enrichRecent. Best-effort:
	// any error is swallowed so the pre-command refresh never blocks the command,
	// and it is incremental (only un-hydrated notes) so steady-state runs issue
	// zero detail fetches and stay fast.
	enrichRecentForAutoRefresh(ctx, db.DB(), c)

	out := ApiSyncResult{
		TotalRecords: totalSynced,
		Resources:    successCount + warnCount + errCount,
		Success:      successCount,
		Warned:       warnCount,
		Errored:      errCount,
		Duration:     time.Since(started),
	}

	// "Totally broken" branch: nothing synced and no warnings — surface
	// the first concrete error so the provenance line can show a real
	// reason. Anything else (partial success, warnings only) is treated
	// as a non-fatal refresh outcome.
	if successCount == 0 && warnCount == 0 && errCount > 0 {
		if firstErr == nil {
			firstErr = errors.New("api sync failed with no per-resource error captured")
		}
		return out, firstErr
	}
	return out, nil
}

// autoRefreshEnrichCap bounds how many un-hydrated recent notes the
// pre-command auto-refresh enriches. Kept small so the refresh that runs before
// EVERY command stays fast; under the dogfood runner it shrinks further to fit
// the per-command timeout.
func autoRefreshEnrichCap() int {
	if isDogfoodEnv() {
		return 1
	}
	return 25
}

// unhydratedRecentNoteIDs returns up to limit REST note ids (newest first) that
// were synced into the thin `notes` table but never hydrated into the rich
// `meetings` table (no meetings row). This LEFT JOIN is the cheap incremental
// signal that keeps steady-state auto-refresh at ZERO detail fetches: once a
// note is enriched it has a meetings row and is skipped on subsequent commands.
func unhydratedRecentNoteIDs(ctx context.Context, db *sql.DB, limit int) ([]string, error) {
	// Re-enrich a note when it has no meetings row yet (never hydrated) OR when
	// the note was updated after its stored meeting (Granola advances a note's
	// updated_at as transcription/notes complete, so a first pass that saw only
	// partial detail is picked up again instead of being considered done forever).
	// Timestamps are UTC RFC3339, so lexical > is chronological.
	q := `SELECT n.id FROM notes n
		LEFT JOIN meetings m ON m.id = n.id
		WHERE n.id LIKE 'not_%'
		  AND (m.id IS NULL OR n.updated_at > COALESCE(m.updated_at, ''))
		ORDER BY n.updated_at DESC
		LIMIT ?`
	// LIMIT is a bound parameter, never spliced into the SQL text. SQLite
	// reads a negative LIMIT as "no limit", which is what limit <= 0 means here.
	lim := limit
	if lim <= 0 {
		lim = -1
	}
	rows, err := db.QueryContext(ctx, q, lim)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// enrichRecentForAutoRefresh hydrates the rich tables for a BOUNDED set of
// recently-synced-but-not-yet-hydrated notes so KEEP commands return data after
// a clean install without a manual `sync`/`enrich`. It reuses the caller's
// already-open db handle + client (the same public-REST auth). Best-effort: any
// error (network, MCP-unavailable) is swallowed — the pre-command refresh must
// never block the user's command. Returns meetings enriched (for tests). A nil
// db or client is a no-op.
func enrichRecentForAutoRefresh(ctx context.Context, db *sql.DB, c *client.Client) int {
	if db == nil || c == nil {
		return 0
	}
	// The generic store creates `notes`, but the rich tables
	// (meetings/attendees/transcript_segments) are the granola schema. Ensure
	// them BEFORE the incremental LEFT JOIN below queries `meetings`; otherwise a
	// clean install fails "no such table: meetings", the error is swallowed, and
	// first-run hydration never happens (regression caught by the Codex re-review).
	if err := granola.EnsureSchema(ctx, db); err != nil {
		return 0
	}
	ids, err := unhydratedRecentNoteIDs(ctx, db, autoRefreshEnrichCap())
	if err != nil || len(ids) == 0 {
		return 0
	}
	n, _, err := hydrateRichFromREST(ctx, db, c, ids)
	if err != nil {
		return n // partial progress still populated the rich tables; never propagate
	}
	return n
}

// apiKeyConfigured returns true when the CLI has a usable public-API
// credential set up — either GRANOLA_API_KEY in the environment or a
// stored API key / access token in the config file. Used by the
// auto-refresh dispatcher to decide whether to run runApiSync.
//
// Treating both env and config-file credentials as "configured" matches
// how config.Load surfaces them (cfg.GranolaApiKey is set in either
// case) and how doctor distinguishes auth_source.
func apiKeyConfigured(flags *rootFlags) bool {
	cfg, err := config.Load(flags.configPath)
	if err != nil {
		return false
	}
	if cfg.GranolaApiKey != "" {
		return true
	}
	if cfg.AuthHeaderVal != "" || cfg.AccessToken != "" {
		return true
	}
	return false
}
