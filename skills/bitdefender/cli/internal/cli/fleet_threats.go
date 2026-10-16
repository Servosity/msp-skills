// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"bitdefender-pp-cli/internal/client"

	"github.com/spf13/cobra"
)

type threatRow struct {
	SHA256       string   `json:"sha256"`
	ThreatName   string   `json:"threat_name,omitempty"`
	Companies    int      `json:"companies"`
	Endpoints    int      `json:"endpoints"`
	CompanyNames []string `json:"company_names"`
	FirstSeen    string   `json:"first_seen"`
	LastSeen     string   `json:"last_seen"`
	// Blocklisted is true only when every affected company blocks the hash.
	Blocklisted      bool     `json:"blocklisted"`
	BlockedIn        []string `json:"blocked_in"`
	NotBlockedIn     []string `json:"not_blocked_in"`
	BlocklistUnknown []string `json:"blocklist_unknown_in,omitempty"`
	FilePaths        []string `json:"file_paths"`
	companyIDs       []string
}

type threatsView struct {
	Window           string         `json:"window"`
	MinCompanies     int            `json:"min_companies"`
	Threats          []threatRow    `json:"threats"`
	UnblockedHashes  []string       `json:"unblocked_hashes"`
	ScannedItems     int            `json:"scanned_items"`
	ScannedCompanies int            `json:"scanned_companies"`
	FetchFailures    []fetchFailure `json:"fetch_failures"`
	Note             string         `json:"note,omitempty"`
}

type quarantineHit struct {
	id, sha, threat, path, endpoint, quarantined string
	company                                      gzCompany
}

func newNovelFleetThreatsCmd(flags *rootFlags) *cobra.Command {
	var window string
	var minCompanies, maxPages, maxCompanies int
	cmd := &cobra.Command{
		Use:   "threats",
		Short: "Find files (SHA256) quarantined at two or more companies and whether each affected company blocklists them",
		Long: `Pages getQuarantineItemsList (computers service, quarantined since --window ago) for every
company in the partner tree, groups the items by file SHA256, and keeps hashes seen at
--min-companies or more companies: a likely campaign rather than one user's mistake. Each
hash is checked against the companies' hash blocklists (getBlocklistItems v1.2).
Blocklist rules belong to a company, so coverage is checked per affected company: blocklisted
is true only when every affected company blocks the hash. Read-only: unblocked_hashes lists
the hashes to add with 'blocklist add --type hash --rules ...'. A company whose quarantine or
blocklist could not be read completely is reported in fetch_failures, never as clean.

Use this command to find the same quarantined file (SHA256) across multiple companies and
whether it is already blocklisted. Do NOT use this command for the incident triage queue;
use 'fleet incident-queue' instead.`,
		Example:     "  bitdefender-cli fleet threats --window 7d --agent\n  bitdefender-cli fleet threats --window 30d --min-companies 3 --select threats.sha256,threats.companies",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet threats")
			}
			span, err := parseSpan(window)
			if err != nil {
				return fleetUsageErr(cmd, flags, fmt.Errorf("--window: %w", err))
			}
			if minCompanies < 1 {
				return fleetUsageErr(cmd, flags, fmt.Errorf("--min-companies must be at least 1"))
			}
			// No whole-command deadline: a fan-out over many companies runs far
			// longer than --timeout, which the generated client applies per request.
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			s, err := openGZStore(ctx)
			if err != nil {
				return err
			}
			defer s.Close()
			tree, failures, err := companyTreeFor(ctx, c, s.DB(), flags.maxAge, maxCompanies)
			if err != nil && len(tree) == 0 {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			view := threatsView{Window: window, MinCompanies: minCompanies, Threats: make([]threatRow, 0), UnblockedHashes: make([]string, 0), FetchFailures: make([]fetchFailure, 0)}
			view.FetchFailures = append(view.FetchFailures, failures...)
			if err != nil {
				// e.g. --max-companies: companies beyond the cap were never read.
				view.FetchFailures = append(view.FetchFailures, fetchFailure{Scope: "company tree", Error: err.Error()})
			}
			limit := len(tree)
			if dogfoodLimit(&limit) {
				maxPages = 1
				view.Note = fmt.Sprintf("live dogfood: %d of %d companies.", limit, len(tree))
			}
			start := time.Now().UTC().Add(-span).Format(time.RFC3339)
			var hits []quarantineHit
			byID := map[string]gzCompany{}
			for _, co := range tree {
				byID[co.ID] = co
			}
			seenItem := map[string]bool{}
			for _, co := range tree[:limit] {
				h, n, err := companyQuarantine(ctx, c, co, start, maxPages)
				view.ScannedItems += n
				if err != nil {
					view.FetchFailures = append(view.FetchFailures, fetchFailure{Scope: "quarantine " + co.Name, Error: err.Error()})
					continue
				}
				view.ScannedCompanies++
				for _, hit := range h {
					// A partner-scope query may return children's items: trust the item's own company.
					if owner, ok := byID[hit.company.ID]; ok {
						hit.company = owner
					}
					if hit.id != "" && seenItem[hit.id] {
						continue
					}
					seenItem[hit.id] = true
					hits = append(hits, hit)
				}
			}
			rows := correlateThreats(hits, minCompanies)
			// Blocklist lookups only for companies that appear in a correlated hash;
			// a failed lookup leaves that company "unknown", never "blocked".
			names := map[string]string{}
			for _, h := range hits {
				names[h.company.ID] = h.company.Name
			}
			blocklists := map[string]map[string]bool{}
			for i := range rows {
				for _, id := range rows[i].companyIDs {
					if _, done := blocklists[id]; done {
						continue
					}
					hashes, err := companyBlockedHashes(ctx, c, id)
					if err != nil {
						view.FetchFailures = append(view.FetchFailures, fetchFailure{Scope: "blocklist " + names[id], Error: err.Error()})
						hashes = nil
					}
					blocklists[id] = hashes
				}
			}
			for i := range rows {
				r := &rows[i]
				r.BlockedIn, r.NotBlockedIn = make([]string, 0), make([]string, 0)
				for _, id := range r.companyIDs {
					switch bl := blocklists[id]; {
					case bl == nil:
						r.BlocklistUnknown = append(r.BlocklistUnknown, names[id])
					case bl[r.SHA256]:
						r.BlockedIn = append(r.BlockedIn, names[id])
					default:
						r.NotBlockedIn = append(r.NotBlockedIn, names[id])
					}
				}
				r.Blocklisted = len(r.NotBlockedIn) == 0 && len(r.BlocklistUnknown) == 0
				if len(r.NotBlockedIn) > 0 {
					view.UnblockedHashes = append(view.UnblockedHashes, r.SHA256)
				}
			}
			view.Threats = rows
			if len(rows) == 0 && view.Note == "" && len(view.FetchFailures) > 0 {
				view.Note = fmt.Sprintf("no correlated hash among the %d companies read; %d fetches failed, so this is not a clean result", view.ScannedCompanies, len(view.FetchFailures))
			} else if len(rows) == 0 && view.Note == "" {
				view.Note = fmt.Sprintf("no SHA256 was quarantined at %d+ companies since %s (%d items scanned); widen --window or lower --min-companies", minCompanies, start, view.ScannedItems)
			}
			if n := len(view.FetchFailures); n > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d fetches failed; correlation covers %d companies\n", n, view.ScannedCompanies)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			humanNote(cmd, view.Note)
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No SHA256 was quarantined at enough companies in the window.")
				return nil
			}
			table := make([]map[string]any, 0, len(rows))
			for _, r := range rows {
				table = append(table, map[string]any{"sha256": r.SHA256, "threat": r.ThreatName, "companies": r.Companies,
					"endpoints": r.Endpoints, "last_seen": r.LastSeen, "blocklisted": r.Blocklisted})
			}
			return printAutoTable(cmd.OutOrStdout(), table)
		},
	}
	cmd.Flags().StringVar(&window, "window", "7d", "Look back this far for quarantined items (e.g. 24h, 7d, 30d)")
	cmd.Flags().IntVar(&minCompanies, "min-companies", 2, "Keep hashes quarantined at this many companies or more")
	cmd.Flags().IntVar(&maxPages, "max-pages", 10, "Maximum quarantine pages (100 items each) per company")
	cmd.Flags().IntVar(&maxCompanies, "max-companies", 5000, "Stop walking the company tree after this many companies")
	return cmd
}

func companyQuarantine(ctx context.Context, c *client.Client, co gzCompany, start string, maxPages int) ([]quarantineHit, int, error) {
	var hits []quarantineHit
	scanned, pagesCount, total := 0, 0, -1 // pagesCount and total arrive on page 1 only
	seenIDs := map[string]bool{}
	for page := 1; page <= maxPages; page++ {
		raw, err := gzRead(ctx, c, gzPathQuarantineList, map[string]any{
			"companyId": co.ID, "page": page, "perPage": 100, "filters": map[string]any{"startDate": start},
		})
		if err != nil {
			return hits, scanned, err
		}
		var resp struct {
			Items []struct {
				ID            string `json:"id"`
				EndpointID    string `json:"endpointId"`
				QuarantinedOn string `json:"quarantinedOn"`
				ThreatName    string `json:"threatName"`
				CompanyID     string `json:"companyId"`
				Details       struct {
					FilePath   string `json:"filePath"`
					FileSha256 string `json:"fileSha256"`
				} `json:"details"`
			} `json:"items"`
			PagesCount int  `json:"pagesCount"`
			Total      *int `json:"total"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return hits, scanned, fmt.Errorf("unexpected getQuarantineItemsList response: %w", err)
		}
		if page == 1 {
			pagesCount = resp.PagesCount
			if resp.Total != nil {
				total = *resp.Total
			}
		}
		for _, it := range resp.Items {
			if it.ID != "" {
				if seenIDs[it.ID] {
					continue
				}
				seenIDs[it.ID] = true
			}
			scanned++
			sha := strings.ToLower(strings.TrimSpace(it.Details.FileSha256))
			if sha == "" {
				continue
			}
			owner := co
			if it.CompanyID != "" && it.CompanyID != co.ID {
				owner = gzCompany{ID: it.CompanyID, Name: it.CompanyID}
			}
			hits = append(hits, quarantineHit{id: it.ID, sha: sha, threat: it.ThreatName, path: it.Details.FilePath, endpoint: it.EndpointID, quarantined: it.QuarantinedOn, company: owner})
		}
		if len(resp.Items) < 100 || (pagesCount > 0 && page >= pagesCount) {
			// An item released while we paged shifts later pages and hides one
			// still quarantined: fewer unique items than page 1 counted.
			if total >= 0 && scanned < total {
				return hits, scanned, fmt.Errorf("incomplete: the quarantine changed while paging (%d of %d items); rerun", scanned, total)
			}
			return hits, scanned, nil
		}
	}
	return hits, scanned, fmt.Errorf("incomplete: more than %d quarantine pages (raise --max-pages or narrow --window)", maxPages)
}

func correlateThreats(hits []quarantineHit, minCompanies int) []threatRow {
	type agg struct {
		row       threatRow
		companies map[string]string
		endpoints map[string]bool
		paths     map[string]bool
		first     time.Time
		last      time.Time
	}
	by := map[string]*agg{}
	for _, h := range hits {
		a, ok := by[h.sha]
		if !ok {
			a = &agg{row: threatRow{SHA256: h.sha}, companies: map[string]string{}, endpoints: map[string]bool{}, paths: map[string]bool{}}
			by[h.sha] = a
		}
		if a.row.ThreatName == "" {
			a.row.ThreatName = h.threat
		}
		a.companies[h.company.ID] = h.company.Name
		a.endpoints[h.company.ID+"/"+h.endpoint] = true
		if h.path != "" && len(a.paths) < 5 {
			a.paths[h.path] = true
		}
		if t, ok := parseGZTime(h.quarantined); ok {
			if a.first.IsZero() || t.Before(a.first) {
				a.first = t
			}
			if t.After(a.last) {
				a.last = t
			}
		}
	}
	rows := make([]threatRow, 0)
	for _, a := range by {
		if len(a.companies) < minCompanies {
			continue
		}
		r := a.row
		r.Companies, r.Endpoints = len(a.companies), len(a.endpoints)
		r.CompanyNames = make([]string, 0, len(a.companies))
		for id, n := range a.companies {
			r.CompanyNames = append(r.CompanyNames, n)
			r.companyIDs = append(r.companyIDs, id)
		}
		sort.Strings(r.CompanyNames)
		sort.Strings(r.companyIDs)
		r.FilePaths = make([]string, 0, len(a.paths))
		for p := range a.paths {
			r.FilePaths = append(r.FilePaths, p)
		}
		sort.Strings(r.FilePaths)
		if !a.first.IsZero() {
			r.FirstSeen, r.LastSeen = a.first.Format(time.RFC3339), a.last.Format(time.RFC3339)
		}
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Companies != rows[j].Companies {
			return rows[i].Companies > rows[j].Companies
		}
		return rows[i].SHA256 < rows[j].SHA256
	})
	return rows
}

// companyBlockedHashes returns the SHA256 hash rules on one company's blocklist.
func companyBlockedHashes(ctx context.Context, c *client.Client, companyID string) (map[string]bool, error) {
	const maxBlocklistPages = 50
	out := map[string]bool{}
	pagesCount, total, seen := 0, -1, 0 // pagesCount and total arrive on page 1 only
	ids := map[string]bool{}
	for page := 1; page <= maxBlocklistPages; page++ {
		raw, err := gzRead(ctx, c, gzPathBlocklistItems, map[string]any{"companyId": companyID, "page": page, "perPage": 100})
		if err != nil {
			return out, err
		}
		var resp struct {
			Total *int `json:"total"`
			Items []struct {
				ID      string `json:"id"`
				Type    string `json:"type"`
				Details struct {
					Algorithm string `json:"algorithm"`
					Hash      string `json:"hash"`
				} `json:"details"`
			} `json:"items"`
			PagesCount int `json:"pagesCount"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return out, fmt.Errorf("unexpected getBlocklistItems response: %w", err)
		}
		if page == 1 {
			pagesCount = resp.PagesCount
			if resp.Total != nil {
				total = *resp.Total
			}
		}
		for _, it := range resp.Items {
			if it.ID == "" || !ids[it.ID] {
				ids[it.ID] = true
				seen++
			}
			if it.Type == "hash" && strings.EqualFold(it.Details.Algorithm, "sha256") && it.Details.Hash != "" {
				out[strings.ToLower(it.Details.Hash)] = true
			}
		}
		if len(resp.Items) < 100 || (pagesCount > 0 && page >= pagesCount) {
			if total >= 0 && seen < total {
				return nil, fmt.Errorf("incomplete: the blocklist changed while paging (%d of %d rules)", seen, total)
			}
			return out, nil
		}
	}
	return nil, fmt.Errorf("incomplete: blocklist has more than %d pages", maxBlocklistPages)
}
