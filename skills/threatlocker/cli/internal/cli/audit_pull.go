// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"threatlocker-pp-cli/internal/client"
	"threatlocker-pp-cli/internal/store"
)

// auditPullSource is one windowed audit search the generated sync cannot walk:
// both routes require a startDate/endDate window. audit export, audit drift,
// audit retention-check and organizations posture read the tables these fill.
type auditPullSource struct {
	Name     string // --source value and report label
	Path     string
	Resource string // store resource the rows upsert under
	// Body returns the request body for one page, minus paging.
	Body func(since, until string, actionID int, childOrgs bool) map[string]any
}

var auditPullSources = []auditPullSource{
	{Name: "audit", Path: "/ActionLog/ActionLogGetByParametersV2", Resource: "audit",
		Body: func(since, until string, actionID int, childOrgs bool) map[string]any {
			return map[string]any{"startDate": since, "endDate": until, "actionId": actionID, "showChildOrganizations": childOrgs}
		}},
	{Name: "system-audit", Path: "/SystemAudit/SystemAuditGetByParameters", Resource: "system-audit",
		Body: func(since, until string, _ int, childOrgs bool) map[string]any {
			return map[string]any{"startDate": since, "endDate": until, "viewChildOrganizations": childOrgs}
		}},
}

var auditPullActions = map[string]int{"permit": 1, "deny": 2, "any-deny": 99}

// audit pull copies live API data into the local store, so the root guard
// refuses it under THREATLOCKER_NO_LOCAL_DATA=1 exactly as it refuses sync and
// workflow archive (dry-run still previews). Hand-wired (class: local-data
// opt-out bypass); TestEveryLiveStoreWriterIsOptOutGuarded pins the class.
func init() {
	localDataWriterCommands["audit pull"] = true
}

// auditPullReport is what one source's pull did. Complete is true only when the
// walk reached a short or empty page with every fetched row stored: a failed
// page, a repeated page, the page cap or an unstored row leave it false, and
// the command then exits non-zero.
type auditPullReport struct {
	Source        string `json:"source"`
	Path          string `json:"path"`
	Pages         int    `json:"pages"`
	Fetched       int    `json:"fetched"`
	Stored        int    `json:"stored"`
	ExtractFailed int    `json:"extract_failures"`
	TypedFailed   int    `json:"typed_failures"`
	Complete      bool   `json:"complete"`
	Reason        string `json:"reason,omitempty"`
	FirstError    string `json:"first_error,omitempty"`
	CoverageNote  string `json:"coverage_note,omitempty"`
	// orgs are the organization ids the fetched rows carry; a complete pull
	// records coverage for each of them.
	orgs map[string]bool
}

// auditPageItems reads one page. Any failure signal wins: an envelope that
// declares failure, or a body that is neither a list nor a recognised list
// envelope, is an error rather than an empty page.
func auditPageItems(data json.RawMessage) ([]json.RawMessage, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err == nil && items != nil {
		return items, nil
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("response is not JSON")
	}
	if envelopeReportsFailure(envelope) {
		return nil, fmt.Errorf("the API reported failure")
	}
	for _, key := range []string{"data", "items", "results"} {
		raw, ok := envelope[key]
		if !ok {
			continue
		}
		if err := json.Unmarshal(raw, &items); err != nil || isJSONNull(raw) {
			return nil, fmt.Errorf("response field %q is not a list", key)
		}
		return items, nil
	}
	return nil, fmt.Errorf("response is not a list of audit rows")
}

// pullAuditSource walks one source page by page and upserts each page.
func pullAuditSource(ctx context.Context, c *client.Client, db *store.Store, src auditPullSource, body map[string]any, pageSize, maxPages int) auditPullReport {
	r := auditPullReport{Source: src.Name, Path: src.Path, orgs: map[string]bool{}}
	prevFirst := ""
	for page := 1; ; page++ {
		if page > maxPages {
			r.Reason = fmt.Sprintf("stopped at --max-pages %d", maxPages)
			return r
		}
		req := make(map[string]any, len(body)+2)
		for k, v := range body {
			req[k] = v
		}
		req["pageNumber"], req["pageSize"] = page, pageSize
		data, _, err := c.PostQueryWithParams(ctx, src.Path, nil, req)
		var items []json.RawMessage
		if err == nil {
			items, err = auditPageItems(data)
		}
		if err != nil {
			r.Reason = fmt.Sprintf("page %d failed", page)
			r.FirstError = err.Error()
			return r
		}
		r.Pages++
		r.Fetched += len(items)
		if len(items) > 0 {
			if page > 1 && string(items[0]) == prevFirst {
				r.Reason = fmt.Sprintf("page %d repeated the previous page", page)
				return r
			}
			prevFirst = string(items[0])
			for _, item := range items {
				var row struct {
					OrganizationID string `json:"organizationId"`
				}
				if json.Unmarshal(item, &row) == nil && row.OrganizationID != "" {
					r.orgs[row.OrganizationID] = true
				}
			}
			stored, extractFailed, typedFailed, upErr := db.UpsertBatchDetailed(src.Resource, items)
			r.Stored += stored
			r.ExtractFailed += extractFailed
			r.TypedFailed += typedFailed
			if upErr != nil {
				r.Reason = "store write failed"
				r.FirstError = upErr.Error()
				return r
			}
		}
		if len(items) < pageSize {
			break
		}
	}
	// Any failure signal wins: a row the store could not key or type, and a
	// row it skipped without counting (stored + extract failures short of
	// fetched), each leave the pull incomplete. Hand-wired (class: unaccounted
	// rows).
	if r.ExtractFailed > 0 || r.TypedFailed > 0 || r.Stored+r.ExtractFailed < r.Fetched {
		r.Reason = "some rows were not stored"
		return r
	}
	r.Complete = true
	return r
}

// tlAuditCoverageTable records, per audit source, action and tenant scope, the
// window end of the last COMPLETE pull. Rows a pull saves before it ends early
// still advance their synced_at, so freshness read from rows would report a
// partial pull as current; retention-check reads this table instead.
// Hand-wired (class: partial coverage read as fresh).
const tlAuditCoverageTable = "tl_audit_coverage"

// auditCoverageAllScope is the scope of a pull that covered every tenant the
// token can see: child organizations included and no tenant header.
const auditCoverageAllScope = "*"

// auditCoverageScopes names the tenants a complete pull covered: every tenant
// when it ran with child organizations and no tenant header, else the scoped
// tenant plus each tenant whose rows it returned. A tenant with no rows in a
// narrower pull is not marked covered.
func auditCoverageScopes(orgs map[string]bool, childOrgs bool) []string {
	scoped := ResolveTenantOrg(tenantOrgFlag)
	if childOrgs && scoped == "" {
		return []string{auditCoverageAllScope}
	}
	seen := map[string]bool{}
	var scopes []string
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			scopes = append(scopes, s)
		}
	}
	add(scoped)
	for org := range orgs {
		add(org)
	}
	return scopes
}

// recordAuditCoverage marks scopes covered up to until for source and action.
// Coverage is continuous archive: a window that starts after a scope's last
// covered end leaves a gap the pull never fetched, so that scope keeps its old
// end (an older all-tenant end counts for every tenant). The scopes left
// behind are returned as "scope until <end>". An --until in the future covers
// only up to now: events after the pull ran were never fetched. Coverage never
// moves backwards: a later backfill of an older window keeps the newer end.
func recordAuditCoverage(db *store.Store, source, action string, scopes []string, since, until time.Time) ([]string, error) {
	if now := time.Now(); until.After(now) {
		until = now
	}
	if _, err := db.DB().Exec(`CREATE TABLE IF NOT EXISTS ` + tlAuditCoverageTable + ` (
		source TEXT NOT NULL, action TEXT NOT NULL, scope TEXT NOT NULL,
		covered_until TEXT NOT NULL, recorded_at TEXT NOT NULL,
		PRIMARY KEY (source, action, scope))`); err != nil {
		return nil, err
	}
	prior := map[string]time.Time{}
	rows, err := db.DB().Query(`SELECT scope, covered_until FROM `+tlAuditCoverageTable+` WHERE source = ? AND action = ?`, source, action)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var scope, end string
		if err := rows.Scan(&scope, &end); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if t, ok := parseTLTime(end); ok {
			prior[scope] = t
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	end := until.UTC().Format(time.RFC3339)
	recorded := time.Now().UTC().Format(time.RFC3339)
	var gaps []string
	for _, scope := range scopes {
		last, covered := prior[auditCoverageAllScope]
		if own, ok := prior[scope]; ok && (!covered || own.After(last)) {
			last, covered = own, true
		}
		if covered && since.After(last) {
			gaps = append(gaps, scope+" until "+last.UTC().Format(time.RFC3339))
			continue
		}
		if _, err := db.DB().Exec(`INSERT INTO `+tlAuditCoverageTable+` (source, action, scope, covered_until, recorded_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (source, action, scope) DO UPDATE SET
			covered_until = MAX(covered_until, excluded.covered_until), recorded_at = excluded.recorded_at`,
			source, action, scope, end, recorded); err != nil {
			return nil, err
		}
	}
	return gaps, nil
}

func newNovelAuditPullCmd(flags *rootFlags) *cobra.Command {
	var flagSince, flagUntil, flagAction, flagSource string
	var flagChildOrgs bool
	var flagPageSize, flagMaxPages int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Page the Unified Audit and portal system audit for a time window into the local store",
		Long: `Pull pages ThreatLocker's windowed audit searches for one time window and
saves every row to the local store, keyed by its own id so re-pulling a window
never duplicates rows:

  audit         POST /ActionLog/ActionLogGetByParametersV2 (permit/deny events)
  system-audit  POST /SystemAudit/SystemAuditGetByParameters (portal admin actions)

Run it on a schedule shorter than ThreatLocker's 31-day retention and the
local archive keeps evidence the portal has deleted. audit export, audit
drift, audit retention-check and organizations posture read what it stores.

It writes only the local store; it never changes anything in ThreatLocker. A
pull that ends early (a failed page, a repeated page, --max-pages reached, or
a row the store could not key) reports complete=false for that source and
exits non-zero after saving the rows it did fetch. Only a complete pull counts
as coverage for audit retention-check. Refused while THREATLOCKER_NO_LOCAL_DATA=1.`,
		Example: strings.Trim(`
  threatlocker-cli audit pull --since 30d --agent
  threatlocker-cli audit pull --source audit --since 2026-10-01 --until 2026-10-08 --action permit
`, "\n"),
		Annotations: map[string]string{"mcp:local-write": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			now := time.Now().UTC()
			since, ok := resolveSince(flagSince, now)
			if !ok || strings.TrimSpace(flagSince) == "" {
				return usageErr(fmt.Errorf("--since must be a window (30d, 12h) or a date, got %q", flagSince))
			}
			until := now
			if strings.TrimSpace(flagUntil) != "" {
				u, ok := parseTLTime(flagUntil)
				if !ok {
					return usageErr(fmt.Errorf("--until must be a date, got %q", flagUntil))
				}
				until = u
			}
			if !since.Before(until) {
				return usageErr(fmt.Errorf("--since (%s) must be before --until (%s)", since.Format(time.RFC3339), until.Format(time.RFC3339)))
			}
			actionID, ok := auditPullActions[flagAction]
			if !ok {
				return usageErr(fmt.Errorf("--action must be permit, deny or any-deny, got %q", flagAction))
			}
			var sources []auditPullSource
			for _, src := range auditPullSources {
				if flagSource == "all" || flagSource == src.Name {
					sources = append(sources, src)
				}
			}
			if len(sources) == 0 {
				return usageErr(fmt.Errorf("--source must be all, audit or system-audit, got %q", flagSource))
			}
			if flagPageSize < 1 || flagPageSize > 1000 {
				return usageErr(fmt.Errorf("--page-size must be 1-1000, got %d", flagPageSize))
			}
			if flagMaxPages < 1 {
				return usageErr(fmt.Errorf("--max-pages must be at least 1, got %d", flagMaxPages))
			}
			sinceText, untilText := since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339)
			if dryRunOK(flags) {
				plan := make([]map[string]any, 0, len(sources))
				for _, src := range sources {
					plan = append(plan, map[string]any{"method": "POST", "path": src.Path, "body": src.Body(sinceText, untilText, actionID, flagChildOrgs)})
				}
				return flags.printJSON(cmd, map[string]any{"dry_run": true, "since": sinceText, "until": untilText, "requests": plan})
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			if dbPath == "" {
				dbPath = defaultDBPath("threatlocker-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()

			reports := make([]auditPullReport, 0, len(sources))
			incomplete := []string{}
			for _, src := range sources {
				r := pullAuditSource(cmd.Context(), c, db, src, src.Body(sinceText, untilText, actionID, flagChildOrgs), flagPageSize, flagMaxPages)
				if r.Complete {
					gaps, err := recordAuditCoverage(db, src.Name, flagAction, auditCoverageScopes(r.orgs, flagChildOrgs), since, until)
					switch {
					case err != nil:
						r.Complete = false
						r.Reason = "coverage record failed"
						r.FirstError = err.Error()
					case len(gaps) > 0:
						r.CoverageNote = "coverage not advanced: this window starts after the last complete pull's end (" + strings.Join(gaps, "; ") + "); pull again with --since at or before that end to close the gap"
					}
				}
				reports = append(reports, r)
				if !r.Complete {
					incomplete = append(incomplete, fmt.Sprintf("%s: %s", r.Source, r.Reason))
				}
			}
			if err := flags.printJSON(cmd, map[string]any{
				"since": sinceText, "until": untilText, "action": flagAction,
				"complete": len(incomplete) == 0, "sources": reports,
			}); err != nil {
				return err
			}
			if len(incomplete) > 0 {
				return fmt.Errorf("audit pull incomplete (%s); the rows that were fetched are saved", strings.Join(incomplete, "; "))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "7d", "Start of the window: a relative window (30d, 12h) or a date")
	cmd.Flags().StringVar(&flagUntil, "until", "", "End of the window as a date (default: now)")
	cmd.Flags().StringVar(&flagSource, "source", "all", "Which audit to pull: all, audit or system-audit")
	cmd.Flags().StringVar(&flagAction, "action", "deny", "Unified Audit events to pull: permit, deny or any-deny (system-audit ignores it)")
	cmd.Flags().BoolVar(&flagChildOrgs, "child-orgs", true, "Include every managed (child) organization; --child-orgs=false pulls the authenticating organization alone")
	// ponytail: a page shorter than --page-size ends the walk, the same rule sync
	// applies; 100 is the spec default, so a server-side cap below it would end
	// the walk early. Raise it only for a page size the API is known to honor.
	cmd.Flags().IntVar(&flagPageSize, "page-size", 100, "Rows per request (1-1000); a shorter page ends the pull")
	cmd.Flags().IntVar(&flagMaxPages, "max-pages", 200, "Stop after this many pages and report the pull incomplete")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
