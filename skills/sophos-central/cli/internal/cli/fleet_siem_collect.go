// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"sophos-central-pp-cli/internal/client"
	"sophos-central-pp-cli/internal/cliutil"
	"sophos-central-pp-cli/internal/store"
)

// siemWindow is how far back Sophos SIEM serves data. A cursor older than
// this silently restarts at "now minus 24h", so older gaps are lost.
const siemWindow = 24 * time.Hour

// siemCursor is a saved per-tenant, per-stream position.
type siemCursor struct {
	ID       string    `json:"id"` // "<tenant>:<stream>"
	Cursor   string    `json:"cursor"`
	SavedAt  time.Time `json:"saved_at"`
	TenantID string    `json:"tenant_id"`
	Stream   string    `json:"stream"`
	// EventAt is how far the stream has been read in event time: the last
	// emitted item's created_at, or the save time once fully drained. A
	// capped run leaves it behind, so the 24h check uses it, not SavedAt.
	EventAt time.Time `json:"event_at,omitzero"`
}

// siemRecord is one NDJSON line.
type siemRecord struct {
	RecordType string          `json:"record_type"` // event | alert | gap_warning | tenant_status | summary
	TenantID   string          `json:"tenant_id,omitempty"`
	TenantName string          `json:"tenant_name,omitempty"`
	Stream     string          `json:"stream,omitempty"`
	Item       json.RawMessage `json:"item,omitempty"`
	Status     string          `json:"status,omitempty"`
	Message    string          `json:"message,omitempty"`
	Count      int             `json:"count,omitempty"`
}

func newNovelFleetSiemCollectCmd(flags *rootFlags) *cobra.Command {
	var flagStream, dbPath string
	var flagLimit, flagMaxPages int
	var flagIncludeTrial bool

	cmd := &cobra.Command{
		Use:   "siem-collect",
		Short: "Pull SIEM events and alerts for every tenant from each tenant's saved cursor as NDJSON",
		Long: `Use this command to pull SIEM events and alerts for every tenant from each tenant's saved cursor and emit them as NDJSON, warning when a tenant's cursor is older than the 24-hour window. Do NOT use this command for an ad hoc SIEM read of the selected tenant; use 'siem events' or 'siem alerts' instead.

Each line is a JSON object with record_type event, alert, gap_warning, tenant_status or summary, stamped with tenant_id and tenant_name. A tenant's cursor is saved in the local store only after its page has been written, so an interrupted run resumes without loss. Sophos serves only the last 24 hours: a cursor older than that restarts at now-24h, and the command emits gap_warning so the gap is visible. Designed for cron: run it more often than every 24 hours.`,
		Example: strings.Trim(`
  sophos-central-cli fleet siem-collect --stream both
  sophos-central-cli fleet siem-collect --stream alerts --tenant 6f3c4a2e-1b9d-4e8f-a7c2-5d0e9b1f3a64 >> sophos-alerts.ndjson`, "\n"),
		// Hidden from MCP: the MCP shell-out truncates long output while
		// reporting success, which would advance cursors past events the
		// caller never received. Run it from cron or a shell instead.
		Annotations: map[string]string{"mcp:read-only": "false", "mcp:local-write": "true", "mcp:hidden": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet siem-collect")
			}
			// Cursors are saved as each page reaches stdout. A --deliver sink
			// (file, webhook) is written only after the command returns, so a
			// failed delivery would leave cursors past undelivered records.
			if d := strings.TrimSpace(flags.deliverSpec); d != "" && d != "stdout" {
				return usageErr(fmt.Errorf("fleet siem-collect writes NDJSON to stdout only; redirect it (>> file) instead of --deliver %s", d))
			}
			var streams []string
			switch flagStream {
			case "both":
				streams = []string{"events", "alerts"}
			case "events", "alerts":
				streams = []string{flagStream}
			default:
				return usageErr(fmt.Errorf("--stream %q: want events, alerts or both", flagStream))
			}
			if flagLimit < 200 || flagLimit > 1000 {
				return usageErr(fmt.Errorf("--limit must be 200-1000 (the Sophos SIEM page size range)"))
			}
			maxPages := flagMaxPages
			if cliutil.IsDogfoodEnv() {
				maxPages = 1
			}
			// No boundCtx: these runs span many tenants under the rate limit;
			// --timeout bounds each request inside the generated client.
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			filter, err := fleetTenantFilter()
			if err != nil {
				return err
			}
			tenants, _, err := fleetTenantsFiltered(ctx, c, filter, flagIncludeTrial)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			if cliutil.IsDogfoodEnv() && len(tenants) > 2 {
				tenants = tenants[:2]
			}
			path := fleetDBPath(dbPath)
			db, err := store.OpenWithContext(ctx, path)
			if err != nil {
				return fmt.Errorf("opening local store %s for SIEM cursors: %w", path, err)
			}
			defer db.Close()
			out := cmd.OutOrStdout()
			total, failures, unexpected := 0, 0, 0
			for _, t := range tenants {
				for _, stream := range streams {
					if ctx.Err() != nil {
						return fmt.Errorf("fleet siem-collect interrupted: %w", ctx.Err())
					}
					n, err := pullSiem(ctx, c, db, out, t, stream, flagLimit, maxPages, time.Now().UTC())
					total += n
					if ctx.Err() != nil {
						return fmt.Errorf("fleet siem-collect interrupted: %w", ctx.Err())
					}
					if err != nil {
						var outErr errSiemOutput
						if errors.As(err, &outErr) {
							return outErr.err
						}
						failures++
						status := classifyTenantErr(err)
						if status != "forbidden" && status != "no_api_host" {
							unexpected++
						}
						if werr := writeSiemRecord(out, siemRecord{RecordType: "tenant_status", TenantID: t.ID, TenantName: t.Name, Stream: stream, Status: status, Message: truncateErr(err)}); werr != nil {
							return werr
						}
					}
				}
			}
			if err := writeSiemRecord(out, siemRecord{RecordType: "summary", Count: total, Message: fmt.Sprintf("%d tenant(s), %d stream failure(s)", len(tenants), failures)}); err != nil {
				return err
			}
			if failures > 0 && failures == len(tenants)*len(streams) {
				return apiErr(fmt.Errorf("fleet siem-collect: every tenant stream failed"))
			}
			// Expected 403s are recorded; any other stream failure (5xx,
			// timeout, incomplete collection) fails the run after output.
			if unexpected > 0 {
				return apiErr(fmt.Errorf("fleet siem-collect: %d tenant stream(s) failed unexpectedly; see tenant_status records", unexpected))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagStream, "stream", "both", "Which SIEM stream to pull: events, alerts or both")
	cmd.Flags().IntVar(&flagLimit, "limit", 1000, "Items per SIEM page (200-1000)")
	cmd.Flags().IntVar(&flagMaxPages, "max-pages", 0, "Cap pages per tenant and stream (0 = until has_more is false)")
	cmd.Flags().BoolVar(&flagIncludeTrial, "include-trial", false, "Include trial tenants (they usually return 403 for partners)")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database holding the per-tenant cursors (default: resolved data directory data.db)")
	return cmd
}

// writeSiemRecord writes one NDJSON line. Its error must be checked: a cursor
// is only saved after its page reached the output.
func writeSiemRecord(w io.Writer, r siemRecord) error {
	b, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("encoding SIEM record: %w", err)
	}
	if _, err := fmt.Fprintln(w, string(b)); err != nil {
		return fmt.Errorf("writing SIEM output: %w", err)
	}
	return nil
}

// errSiemOutput marks an output failure: the run must stop, not move on to
// the next tenant, because every later write would fail too.
type errSiemOutput struct{ err error }

func (e errSiemOutput) Error() string { return e.err.Error() }
func (e errSiemOutput) Unwrap() error { return e.err }

// siemPageParams builds the query for the next page: the saved cursor when it
// is still inside the window, otherwise a from_date at the window's edge.
// stale reports that a saved cursor fell outside the window.
func siemPageParams(cur *siemCursor, limit int, now time.Time) (params map[string]string, stale bool) {
	params = map[string]string{"limit": strconv.Itoa(limit)}
	if cur != nil && cur.Cursor != "" {
		mark := cur.SavedAt
		if !cur.EventAt.IsZero() && cur.EventAt.Before(mark) {
			mark = cur.EventAt
		}
		if now.Sub(mark) < siemWindow {
			params["cursor"] = cur.Cursor
			return params, false
		}
		stale = true
	}
	// A minute inside the window so clock skew never lands outside it.
	params["from_date"] = strconv.FormatInt(now.Add(-siemWindow+time.Minute).Unix(), 10)
	return params, stale
}

func siemMark(cur *siemCursor) time.Time {
	if !cur.EventAt.IsZero() && cur.EventAt.Before(cur.SavedAt) {
		return cur.EventAt
	}
	return cur.SavedAt
}

// lastSiemEventTime returns the created_at of the last item in a page (SIEM
// items are snake_case), falling back to the given time.
func lastSiemEventTime(items []json.RawMessage, fallback time.Time) time.Time {
	for i := len(items) - 1; i >= 0; i-- {
		var it struct {
			CreatedAt string `json:"created_at"`
		}
		if json.Unmarshal(items[i], &it) == nil && it.CreatedAt != "" {
			if t, err := time.Parse(time.RFC3339, it.CreatedAt); err == nil {
				return t.UTC()
			}
		}
	}
	return fallback
}

// pullSiem drains one tenant stream and returns the number of items written.
func pullSiem(ctx context.Context, c *client.Client, db *store.Store, out io.Writer, t fleetTenant, stream string, limit, maxPages int, now time.Time) (int, error) {
	id := t.ID + ":" + stream
	var cur *siemCursor
	raw, err := db.Get(fleetSiemCursorType, id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("loading saved %s cursor: %w", stream, err)
	}
	if len(raw) > 0 {
		// Restarting from an unreadable cursor would skip the gap warning.
		var sc siemCursor
		if err := json.Unmarshal(raw, &sc); err != nil {
			return 0, fmt.Errorf("saved %s cursor is unreadable (%v); remove it from %s to restart deliberately", stream, err, fleetSiemCursorType)
		}
		// Only rows with a cursor and a save time are ever written.
		if sc.Cursor == "" || sc.SavedAt.IsZero() {
			return 0, fmt.Errorf("saved %s cursor has no cursor or save time; remove it from %s to restart deliberately", stream, fleetSiemCursorType)
		}
		cur = &sc
	}
	params, stale := siemPageParams(cur, limit, now)
	if stale {
		if err := writeSiemRecord(out, siemRecord{RecordType: "gap_warning", TenantID: t.ID, TenantName: t.Name, Stream: stream,
			Message: fmt.Sprintf("cursor position %s is older than 24h; Sophos only serves the last 24h, so data between then and %s is lost", siemMark(cur).Format(time.RFC3339), now.Add(-siemWindow).Format(time.RFC3339))}); err != nil {
			return 0, errSiemOutput{err}
		}
	}
	recordType := strings.TrimSuffix(stream, "s")
	written := 0
	seen := map[string]bool{}
	if params["cursor"] != "" {
		seen[params["cursor"]] = true
	}
	for page := 1; ; page++ {
		// No response cache: a cached page would replay the same cursor.
		raw, err := c.GetWithHeadersNoCache(ctx, "/siem/v1/"+stream, params, tenantHeaders(t.ID))
		if err != nil {
			return written, err
		}
		var env struct {
			HasMore    bool               `json:"has_more"`
			NextCursor string             `json:"next_cursor"`
			Items      *[]json.RawMessage `json:"items"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			return written, fmt.Errorf("decoding /siem/v1/%s: %w", stream, err)
		}
		// No items array means no page: never move the cursor past it.
		if env.Items == nil {
			return written, fmt.Errorf("/siem/v1/%s returned no items array; cursor not advanced", stream)
		}
		items := *env.Items
		for _, item := range items {
			if err := writeSiemRecord(out, siemRecord{RecordType: recordType, TenantID: t.ID, TenantName: t.Name, Stream: stream, Item: item}); err != nil {
				return written, errSiemOutput{err}
			}
		}
		written += len(items)
		if env.HasMore && env.NextCursor == "" {
			return written, fmt.Errorf("/siem/v1/%s reported has_more without a next_cursor; collection is incomplete", stream)
		}
		if env.HasMore && seen[env.NextCursor] {
			return written, fmt.Errorf("/siem/v1/%s repeated cursor %q; collection is incomplete", stream, env.NextCursor)
		}
		// Save the cursor only after the page is written: a crash re-emits
		// at most one page instead of skipping one.
		if env.NextCursor != "" {
			saved := time.Now().UTC()
			sc := siemCursor{ID: id, Cursor: env.NextCursor, SavedAt: saved, TenantID: t.ID, Stream: stream, EventAt: saved}
			if env.HasMore {
				sc.EventAt = lastSiemEventTime(items, saved)
			}
			if b, err := json.Marshal(sc); err == nil {
				if _, _, err := db.UpsertBatch(fleetSiemCursorType, []json.RawMessage{b}); err != nil {
					return written, fmt.Errorf("saving SIEM cursor for %s: %w", id, err)
				}
			}
		}
		if !env.HasMore {
			return written, nil
		}
		if maxPages > 0 && page >= maxPages {
			// An intentional cap, not a drained stream: say so.
			if err := writeSiemRecord(out, siemRecord{RecordType: "tenant_status", TenantID: t.ID, TenantName: t.Name, Stream: stream, Status: "capped",
				Message: fmt.Sprintf("stopped after %d page(s) with more data waiting; the cursor is saved and the next run continues from it", page)}); err != nil {
				return written, errSiemOutput{err}
			}
			return written, nil
		}
		seen[env.NextCursor] = true
		params = map[string]string{"limit": strconv.Itoa(limit), "cursor": env.NextCursor}
	}
}
