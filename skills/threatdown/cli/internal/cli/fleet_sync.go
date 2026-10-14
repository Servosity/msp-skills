// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"threatdown-pp-cli/internal/cliutil"
	"threatdown-pp-cli/internal/fleet"
	"threatdown-pp-cli/internal/store"
)

type fleetSyncResult struct {
	Resource  string `json:"resource"`
	Stored    int    `json:"stored"`
	Removed   int    `json:"removed,omitempty"`
	Pages     int    `json:"pages"`
	Truncated bool   `json:"truncated"`
	Note      string `json:"note,omitempty"`
	Error     string `json:"error,omitempty"`
}

type fleetSyncView struct {
	Sites     int               `json:"sites"`
	Accounts  int               `json:"accounts"`
	Since     string            `json:"since"`
	DBPath    string            `json:"db"`
	Resources []fleetSyncResult `json:"resources"`
}

func newNovelFleetSyncCmd(flags *rootFlags) *cobra.Command {
	var since, siteFilter, dbPath string
	var resources []string
	var maxPages int
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync sites, endpoints, detections and jobs from every site into the local store",
		Long: `Pull every managed site, then page endpoint, detection and job searches across
all of their Nebula account_ids into the local SQLite store. The generic 'sync'
command cannot do this because OneView searches are POST requests that need an
explicit account_ids list. threats spread, threats queue and customer report
read what this command stores.`,
		Example: strings.Trim(`
  threatdown-cli fleet sync
  threatdown-cli fleet sync --resources endpoints,detections --since 7d
  threatdown-cli fleet sync --site "Acme" --json`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "mcp:local-write": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet sync")
			}
			window, err := cliutil.ParseDurationLoose(since)
			if err != nil || window <= 0 {
				return usageErr(fmt.Errorf("--since must be a positive duration like 7d or 24h, got %q", since))
			}
			want := map[string]bool{}
			for _, r := range resources {
				r = strings.ToLower(strings.TrimSpace(r))
				switch r {
				case "endpoints", "detections", "jobs", "sites":
					want[r] = true
				case "":
				default:
					return usageErr(fmt.Errorf("unknown --resources entry %q (use sites, endpoints, detections, jobs)", r))
				}
			}
			if len(want) == 0 {
				want = map[string]bool{"sites": true, "endpoints": true, "detections": true, "jobs": true}
			}
			if cliutil.IsDogfoodEnv() {
				maxPages = 1
			}
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			sites, err := fetchSites(ctx, c)
			if err != nil {
				return classifyAPIErrorOnly(fmt.Errorf("listing sites: %w", err))
			}
			if sites, err = scopeSites(sites, siteFilter); err != nil {
				return err
			}
			accounts := fleet.AccountIDs(sites, "")
			path := tdDBPath(dbPath)
			db, err := store.OpenWithContext(ctx, path)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()

			now := time.Now().UTC()
			view := fleetSyncView{Sites: len(sites), Accounts: len(accounts), Since: since, DBPath: path, Resources: make([]fleetSyncResult, 0, 4)}
			siteIdx := fleet.SiteIndex(sites)
			// Freshness is only recorded for a complete, unscoped pull; a
			// --site sync refreshes part of the fleet and must not read as all.
			fullScope := strings.TrimSpace(siteFilter) == ""

			if want["sites"] {
				items := make([]json.RawMessage, 0, len(sites))
				for _, s := range sites {
					if b, err := json.Marshal(s); err == nil {
						items = append(items, b)
					}
				}
				r, skipped := storeFleet(db, tdResSites, items, 1, false, nil)
				if skipped > 0 {
					r.Note = fmt.Sprintf("%d site(s) could not be stored; nothing removed", skipped)
				}
				if fullScope && r.Error == "" && skipped == 0 && !cliutil.IsAnyHarness() {
					ids := make([]string, 0, len(sites))
					for _, s := range sites {
						ids = append(ids, s.ID)
					}
					if n, err := db.ReconcileAll(tdResSites, ids, tdResSites, nil); err != nil {
						r.Error = "removing deleted sites: " + err.Error()
					} else {
						r.Removed = n
					}
				}
				recordFreshness(db, &r, false, fullScope, skipped == 0, "")
				view.Resources = append(view.Resources, r)
			}
			run := func(name, res, apiPath, key string, body map[string]any, coverage string, transform func([]json.RawMessage) ([]json.RawMessage, int)) []json.RawMessage {
				if !want[name] {
					return nil
				}
				if len(accounts) == 0 {
					view.Resources = append(view.Resources, fleetSyncResult{Resource: res, Note: "no sites with an account_id (assign a subscription first)"})
					return nil
				}
				body["account_ids"] = accounts
				raws, info, err := searchPages(ctx, c, apiPath, body, key, maxPages)
				bad := 0
				if transform != nil {
					raws, bad = transform(raws)
				}
				r, skipped := storeFleet(db, res, raws, info.Pages, info.Truncated, err)
				bad += skipped
				// Deleting rows or certifying freshness needs proof the list is
				// whole: the server ended the cursor chain and every record parsed.
				certain := err == nil && info.Complete && bad == 0 && r.Error == ""
				switch {
				case bad > 0:
					r.Note = fmt.Sprintf("%d record(s) could not be read; nothing removed, freshness not recorded", bad)
				case err == nil && !info.Complete && !info.Truncated:
					r.Note = "the API did not confirm the end of the list; nothing removed, freshness not recorded"
				}
				if res == tdResEndpoints && certain && !cliutil.IsAnyHarness() {
					if n, rerr := reconcileEndpoints(db, accounts, raws); rerr != nil {
						r.Error = "removing vanished endpoints: " + rerr.Error()
					} else {
						r.Removed = n
					}
				}
				recordFreshness(db, &r, info.Truncated, fullScope, certain, coverage)
				view.Resources = append(view.Resources, r)
				return raws
			}
			run("endpoints", tdResEndpoints, "/v1/endpoints", "endpoints", map[string]any{}, "", func(raws []json.RawMessage) ([]json.RawMessage, int) {
				eps, bad := parseEndpoints(raws)
				out := make([]json.RawMessage, 0, len(eps))
				for _, e := range eps {
					e.Site = siteIdx[e.AccountID].Name
					if b, err := json.Marshal(e); err == nil {
						out = append(out, b)
					}
				}
				return out, bad
			})
			windowStart := isoAgo(now, window)
			run("detections", tdResDetections, "/v1/detections", "detections", map[string]any{"reported_at_after": windowStart}, windowStart, validDetections)
			run("jobs", tdResJobs, "/v1/jobs/search", "jobs", map[string]any{"issued_at_after": windowStart, "populate": map[string]any{"online": true}}, windowStart, validJobs)

			failed := 0
			for _, r := range view.Resources {
				if r.Error != "" {
					failed++
				}
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, "removed", "note", "error"); err != nil {
					return err
				}
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Synced %d sites (%d with an account) into %s\n", view.Sites, view.Accounts, path)
				if err := printColumns(cmd.OutOrStdout(), view.Resources, "resource", "stored", "removed", "pages", "truncated", "note", "error"); err != nil {
					return err
				}
			}
			if failed > 0 {
				return apiErr(fmt.Errorf("fleet sync: %d of %d resources failed; see resources[].error", failed, len(view.Resources)))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "30d", "Window for detections and jobs (e.g. 7d, 24h); endpoints always sync in full")
	cmd.Flags().StringSliceVar(&resources, "resources", nil, "Subset to sync: sites, endpoints, detections, jobs (default all)")
	cmd.Flags().StringVar(&siteFilter, "site", "", "Only sync sites whose name contains this text")
	cmd.Flags().IntVar(&maxPages, "max-pages", tdDefaultMaxPages, "Maximum 2000-row pages per search")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the same store 'sync' writes)")
	return cmd
}

// storeFleet upserts one resource's records and records sync state so the
// local readers can tell synced-empty from never-synced.
// storeFleet upserts items and returns how many the store could not extract
// (those block reconciliation and freshness like unreadable records do).
func storeFleet(db *store.Store, resource string, items []json.RawMessage, pages int, more bool, fetchErr error) (fleetSyncResult, int) {
	res := fleetSyncResult{Resource: resource, Pages: pages, Truncated: more}
	if fetchErr != nil {
		res.Error = fetchErr.Error()
	}
	skipped := 0
	if len(items) > 0 {
		stored, failed, err := db.UpsertBatch(resource, items)
		res.Stored, skipped = stored, failed
		if err != nil {
			res.Error = strings.TrimSpace(res.Error + "; store: " + err.Error())
		}
	}
	return res, skipped
}

// requireDecodable keeps records that decode into T (the type local reports
// read them with) and pass ok, counting the rest as unreadable.
func requireDecodable[T any](ok func(T) bool) func([]json.RawMessage) ([]json.RawMessage, int) {
	return func(raws []json.RawMessage) ([]json.RawMessage, int) {
		out := make([]json.RawMessage, 0, len(raws))
		bad := 0
		for _, r := range raws {
			var v T
			if json.Unmarshal(r, &v) != nil || !ok(v) {
				bad++
				continue
			}
			out = append(out, r)
		}
		return out, bad
	}
}

var (
	validDetections = requireDecodable(func(d fleet.Detection) bool { return d.ID != "" && d.AccountID != "" })
)

// validJobs keeps every row parseJobs (the local readers) can read, so a job
// whose online/data field has an odd type is stored, not counted unreadable.
func validJobs(raws []json.RawMessage) ([]json.RawMessage, int) {
	out := make([]json.RawMessage, 0, len(raws))
	bad := 0
	for _, r := range raws {
		if jobs, _ := parseJobs([]json.RawMessage{r}); len(jobs) == 1 && jobs[0].AccountID != "" {
			out = append(out, r)
			continue
		}
		bad++
	}
	return out, bad
}

// recordFreshness stamps sync state only for a complete, unscoped pull that
// stored and reconciled cleanly; anything partial says why it did not.
// coverage is the start of the detection/job window this pull covered, kept in
// the sync cursor so local reports can warn about older, unsynced history.
func recordFreshness(db *store.Store, res *fleetSyncResult, more, fullScope, certain bool, coverage string) {
	saved := false
	defer func() {
		// The older freshness stamp stays, but local readers must not take it
		// as proof the copy is whole: a row this pull saw may be missing.
		if fullScope && !saved {
			markSyncIncomplete(db, res.Resource)
		}
	}()
	switch {
	case res.Error != "":
	case more:
		res.Note = joinNotes(res.Note, "stopped at --max-pages; raise it for a complete copy (freshness not recorded)")
	case !certain:
		// the reason is already in res.Note
	case !fullScope:
		res.Note = "--site scoped sync; fleet-wide freshness not recorded"
	default:
		if err := db.SaveSyncState(res.Resource, coverage, res.Stored); err != nil {
			res.Error = "saving sync state: " + err.Error()
		} else {
			saved = true
		}
	}
}

// reconcileEndpoints deletes stored endpoints of the synced accounts that the
// complete endpoint pull no longer returned (removed or deleted machines).
func reconcileEndpoints(db *store.Store, accounts []string, items []json.RawMessage) (int, error) {
	seen := make(map[string][]string, len(accounts))
	for _, raw := range items {
		var e fleet.Endpoint
		if json.Unmarshal(raw, &e) == nil && e.ID != "" {
			seen[e.AccountID] = append(seen[e.AccountID], e.ID)
		}
	}
	removed := 0
	for _, acct := range accounts {
		n, err := db.ReconcilePartition(tdResEndpoints, "$.account_id", acct, seen[acct], tdResEndpoints, nil)
		if err != nil {
			return removed, err
		}
		removed += n
	}
	return removed, nil
}
