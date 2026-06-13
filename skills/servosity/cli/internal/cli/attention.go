// Copyright 2026 servosity. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"servosity-msp-pp-cli/internal/client"
	"servosity-msp-pp-cli/internal/snapshot"
	"servosity-msp-pp-cli/internal/store"
)

// attentionCompany is one company row in the attention rollup.
type attentionCompany struct {
	CompanyID        int64  `json:"company_id"`
	CompanyName      string `json:"company_name"`
	Score            int    `json:"score"`
	OpenIssues       int    `json:"open_issues"`
	StaleBackups     int    `json:"stale_backups"`
	DRBackupInFlight int    `json:"drbackup_in_flight"`
}

// attentionResult is the full envelope written both to stdout and the snapshot.
type attentionResult struct {
	TakenAt   time.Time          `json:"taken_at"`
	Companies []attentionCompany `json:"companies"`
	Totals    map[string]int     `json:"totals"`
}

// newNovelAttentionCmd builds the morning fleet-sweep command. It merges open
// issues + stale backup sets into a
// per-company ranked view, persists the result as a snapshot for `drift`,
// and emits a JSON envelope on stdout.
//
// v1 ranking:  score = (open_issues * 2) + (stale_backups * 3)
//
// Restore-queue weighting is deferred to v0.2 (per-company iteration is too
// expensive without a synced table; see TODO below).
// pp:data-source auto
func newNovelAttentionCmd(flags *rootFlags) *cobra.Command {
	var refresh bool
	var since string
	var topN int

	cmd := &cobra.Command{
		Use:   "attention",
		Short: "Morning fleet sweep: rank companies by open issues + stale backups",
		Long: `Merge open issues and stale backup sets into a
per-company ranked view of where your attention is needed today.

By default reads from the local store (run 'sync' first). Pass --refresh to
pull live from the API. Results are snapshotted to pp_snapshots so future
runs of 'drift' can compute day-over-day changes.`,
		Example: `  # Use local store
  servosity-cli attention

  # Pull live, only items newer than 12h, top 5
  servosity-cli attention --refresh --since 12h --top 5`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate --since up front; bail with a usage error before
			// any IO or work that might mask the real issue.
			sinceDur, err := time.ParseDuration(since)
			if err != nil {
				return usageErr(fmt.Errorf("invalid --since value %q: %w", since, err))
			}
			if topN <= 0 {
				return usageErr(fmt.Errorf("--top must be > 0, got %d", topN))
			}

			// --dry-run short-circuits BEFORE any IO so verify probes don't
			// touch the store or hit the API.
			if dryRunOK(flags) {
				return nil
			}

			ctx := cmd.Context()
			// staleCutoff: a backup whose last success predates this is stale.
			// --since drives the morning-sweep window ("not backed up since…").
			staleCutoff := time.Now().Add(-sinceDur)

			// Per-company aggregate, keyed by company id.
			agg := map[int64]*attentionCompany{}

			// Open the local store once; both the issue and stale sources read
			// from it (and --refresh hydrates freshness into it first).
			db, err := store.Open(defaultDBPath("servosity-cli"))
			if err != nil {
				return fmt.Errorf("opening local store: %w\nRun 'servosity-cli sync' first.", err)
			}
			defer db.Close()
			if err := ensureLastSuccessTable(ctx, db); err != nil {
				return err
			}

			// ---- 1. Open issues ----
			// An OPEN issue needs attention regardless of when it was filed, so
			// open issues are never gated by age. (The old code filtered by
			// created_at >= now-since, which dropped every still-open issue
			// older than a day and made the whole sweep rank zero companies.)
			if refresh {
				c, err := flags.newClient()
				if err != nil {
					return err
				}
				// Partner tokens cannot hit /issues/ globally — that endpoint is
				// admin-only and 403s. Resolve the reseller and walk every page
				// of /resellers/{id}/issues/ so the ranking sees the whole book.
				resellerID, err := resolveResellerID(cmd.Context(), c)
				if err != nil {
					return fmt.Errorf("resolving reseller ID: %w", err)
				}
				if err := countIssuesPaged(cmd.Context(), c, fmt.Sprintf("/resellers/%d/issues/", resellerID), agg); err != nil {
					return classifyAPIError(err, flags)
				}
				// Hydrate per-backup freshness so the stale source is live.
				if _, _, herr := hydrateLastSuccess(ctx, c, db); herr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: freshness hydration failed (%v)\n", herr)
				}
			} else {
				if err := countIssuesFromStore(db, agg); err != nil {
					return err
				}
			}

			// ---- 2. Stale backups (partner-derived) ----
			// Derived from pp_last_success (latest-success per backup), not the
			// admin-only /reports/stale-backup-sets/ report. A backup counts as
			// stale when its last success predates the --since window, or it has
			// never reported a success.
			if err := countStaleFromFreshness(ctx, db, staleCutoff, agg); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: stale-backup section unavailable (%v)\n", err)
			}

			// ---- 3. DR backup in-flight ----
			// TODO(v0.2): /companies/{id}/restore-queues/ requires per-company
			// iteration; skip in v1 until a synced restore_queues table exists.
			// All drbackup_in_flight values stay 0 for now.

			// ---- Rank & truncate ----
			companies := make([]attentionCompany, 0, len(agg))
			for _, row := range agg {
				row.Score = row.OpenIssues*2 + row.StaleBackups*3
				companies = append(companies, *row)
			}
			sort.Slice(companies, func(i, j int) bool {
				if companies[i].Score != companies[j].Score {
					return companies[i].Score > companies[j].Score
				}
				// Stable secondary by name then id so output is deterministic
				// even when two companies score identically.
				if companies[i].CompanyName != companies[j].CompanyName {
					return companies[i].CompanyName < companies[j].CompanyName
				}
				return companies[i].CompanyID < companies[j].CompanyID
			})

			totalIssues := 0
			totalStale := 0
			for _, c := range companies {
				totalIssues += c.OpenIssues
				totalStale += c.StaleBackups
			}
			totalCompanies := len(companies)

			if len(companies) > topN {
				companies = companies[:topN]
			}

			result := attentionResult{
				TakenAt:   time.Now().UTC(),
				Companies: companies,
				Totals: map[string]int{
					"companies": totalCompanies,
					"issues":    totalIssues,
					"stale":     totalStale,
				},
			}

			// ---- Persist snapshot for `drift` ----
			payload, err := json.Marshal(result)
			if err != nil {
				return fmt.Errorf("marshal attention result: %w", err)
			}
			if db, err := store.Open(defaultDBPath("servosity-cli")); err == nil {
				if serr := snapshot.Save(ctx, db.DB(), "attention", result.TakenAt, json.RawMessage(payload)); serr != nil {
					// Snapshot save is best-effort; print to stderr but don't
					// fail the user's read. Drift will warn on a missing
					// anchor; that's a clearer signal than an attention exit.
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: snapshot save failed: %v\n", serr)
				}
				_ = db.Close()
			}

			// ---- Emit ----
			if flags.asJSON || !isTerminal(cmd.OutOrStdout()) {
				return printJSONFiltered(cmd.OutOrStdout(), result, flags)
			}
			return renderAttentionTable(cmd, result)
		},
	}

	cmd.Flags().BoolVar(&refresh, "refresh", false, "Pull live from the API instead of the local store")
	cmd.Flags().StringVar(&since, "since", "24h", "Staleness window: a backup not successful within this duration counts as stale (open issues are counted regardless of age)")
	cmd.Flags().IntVar(&topN, "top", 10, "Limit output to top N companies by score")

	return cmd
}

// countIssuesPaged walks every page of the /resellers/{id}/issues/ endpoint
// and increments per-company open-issue counts. Open issues are counted
// regardless of age — an issue that is still open needs attention whether it
// was filed today or a year ago.
//
// DRF returns an absolute `next` URL with an opaque cursor token. We can't
// feed that URL back through client.Get (which prepends BaseURL); instead we
// extract just the `cursor` query value and re-issue against the same relative
// path so the client encodes it exactly once.
func countIssuesPaged(ctx context.Context, c *client.Client, path string, agg map[int64]*attentionCompany) error {
	params := map[string]string{}
	guard := 0
	for guard < 100 {
		guard++
		data, err := c.Get(ctx, path, params)
		if err != nil {
			return err
		}
		for _, item := range unwrapList(data) {
			countIssueItem(item, agg)
		}
		var env struct {
			Next string `json:"next"`
		}
		if err := json.Unmarshal(data, &env); err != nil || env.Next == "" {
			break
		}
		u, perr := url.Parse(env.Next)
		if perr != nil {
			break
		}
		cursor := u.Query().Get("cursor")
		if cursor == "" {
			break
		}
		params["cursor"] = cursor
	}
	return nil
}

// countIssueItem increments the per-company open-issue count for one raw issue
// row. Closed/resolved/archived/ignored issues are skipped.
func countIssueItem(item json.RawMessage, agg map[int64]*attentionCompany) {
	var row struct {
		Company     any    `json:"company"`
		CompanyName string `json:"company_name"`
		State       string `json:"state"`
	}
	if err := json.Unmarshal(item, &row); err != nil {
		return
	}
	switch strings.ToLower(row.State) {
	case "closed", "resolved", "archived", "ignored":
		return
	}
	id := coerceID(row.Company)
	if id == 0 {
		return
	}
	bumpIssues(agg, id, row.CompanyName)
}

// countIssuesFromStore reads the local `issues` table directly. Open issues
// are counted regardless of created_at — see countIssuesPaged for why.
func countIssuesFromStore(db *store.Store, agg map[int64]*attentionCompany) error {
	rows, err := db.DB().Query(`
		SELECT COALESCE(company, 0), COALESCE(company_name, ''), COALESCE(state, '')
		  FROM issues
		 WHERE lower(COALESCE(state,'')) NOT IN ('closed','resolved','archived','ignored')`)
	if err != nil {
		return fmt.Errorf("query issues from store: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var companyID int64
		var companyName, state string
		if err := rows.Scan(&companyID, &companyName, &state); err != nil {
			continue
		}
		if companyID == 0 {
			continue
		}
		bumpIssues(agg, companyID, companyName)
	}
	return rows.Err()
}

// countStaleFromFreshness increments stale_backups per company from the
// hydrated pp_last_success table. A backup is stale when its last success
// predates staleCutoff, or it has never reported a success.
func countStaleFromFreshness(ctx context.Context, db *store.Store, staleCutoff time.Time, agg map[int64]*attentionCompany) error {
	rows, err := db.DB().QueryContext(ctx, `
		SELECT COALESCE(company_id, 0), COALESCE(last_success, '')
		  FROM pp_last_success`)
	if err != nil {
		return fmt.Errorf("reading freshness table: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var companyID int64
		var lastSuccess string
		if err := rows.Scan(&companyID, &lastSuccess); err != nil {
			continue
		}
		if companyID == 0 {
			continue
		}
		stale := false
		if strings.TrimSpace(lastSuccess) == "" {
			stale = true // never succeeded
		} else if t, perr := parseFlexTime(lastSuccess); perr == nil {
			stale = t.Before(staleCutoff)
		} else {
			stale = true // unparseable timestamp ⇒ treat as stale
		}
		if stale {
			bumpStale(agg, companyID, "")
		}
	}
	return rows.Err()
}

// unwrapList normalises a paginated envelope ({"results":[...]} or
// {"items":[...]}) and a bare top-level array into a slice of raw items.
// Returns nil for shapes it can't recognise so callers see an empty rollup
// section rather than a panic.
func unwrapList(data json.RawMessage) []json.RawMessage {
	if len(data) == 0 {
		return nil
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err == nil {
		return arr
	}
	var env struct {
		Results []json.RawMessage `json:"results"`
		Items   []json.RawMessage `json:"items"`
		Data    []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &env); err == nil {
		if len(env.Results) > 0 {
			return env.Results
		}
		if len(env.Items) > 0 {
			return env.Items
		}
		if len(env.Data) > 0 {
			return env.Data
		}
	}
	return nil
}

// coerceID accepts either a JSON number (decoded as float64) or a numeric
// string and returns it as int64; everything else becomes 0.
func coerceID(v any) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case int64:
		return t
	case int:
		return int64(t)
	case string:
		var n int64
		_, _ = fmt.Sscanf(t, "%d", &n)
		return n
	}
	return 0
}

func bumpIssues(agg map[int64]*attentionCompany, id int64, name string) {
	row, ok := agg[id]
	if !ok {
		row = &attentionCompany{CompanyID: id, CompanyName: name}
		agg[id] = row
	}
	if row.CompanyName == "" && name != "" {
		row.CompanyName = name
	}
	row.OpenIssues++
}

func bumpStale(agg map[int64]*attentionCompany, id int64, name string) {
	row, ok := agg[id]
	if !ok {
		row = &attentionCompany{CompanyID: id, CompanyName: name}
		agg[id] = row
	}
	if row.CompanyName == "" && name != "" {
		row.CompanyName = name
	}
	row.StaleBackups++
}

func renderAttentionTable(cmd *cobra.Command, r attentionResult) error {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Attention as of %s — %d companies, %d issues, %d stale backups\n\n",
		r.TakenAt.Format(time.RFC3339), r.Totals["companies"], r.Totals["issues"], r.Totals["stale"])
	if len(r.Companies) == 0 {
		fmt.Fprintln(w, "No companies need attention right now.")
		return nil
	}
	headers := []string{"SCORE", "COMPANY", "ISSUES", "STALE", "DR"}
	rows := make([][]string, 0, len(r.Companies))
	for _, c := range r.Companies {
		name := c.CompanyName
		if name == "" {
			name = fmt.Sprintf("company:%d", c.CompanyID)
		}
		rows = append(rows, []string{
			fmt.Sprintf("%d", c.Score),
			name,
			fmt.Sprintf("%d", c.OpenIssues),
			fmt.Sprintf("%d", c.StaleBackups),
			fmt.Sprintf("%d", c.DRBackupInFlight),
		})
	}
	// Reuse rootFlags.printTable — but it requires a *rootFlags, which we
	// don't have in scope here; emit via a simple tabwriter-less format to
	// keep this file dependency-light. The JSON path handles agent output.
	for i, h := range headers {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, h)
	}
	fmt.Fprintln(w)
	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				fmt.Fprint(w, "\t")
			}
			fmt.Fprint(w, cell)
		}
		fmt.Fprintln(w)
	}
	return nil
}
