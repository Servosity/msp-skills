// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built novel feature: evidence keyword search.
// pp:data-source auto

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"blumira-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

// evidenceGetter is the minimal client surface evidence fetching needs (test seam).
type evidenceGetter interface {
	Get(ctx context.Context, path string, params map[string]string) (json.RawMessage, error)
}

// evidenceSearchView is the JSON envelope: matches plus scan/fetch accounting
// so agents can tell an empty result from an unexamined corpus.
type evidenceSearchView struct {
	Term          string          `json:"term"`
	Matches       []evidenceMatch `json:"matches"`
	EvidenceRows  int             `json:"evidence_rows_searched"`
	FindingsRaw   int             `json:"findings_searched"`
	Fetched       int             `json:"fetched_findings,omitempty"`
	MaxFetch      int             `json:"max_fetch,omitempty"`
	FetchFailures []fetchFailure  `json:"fetch_failures,omitempty"`
	Note          string          `json:"note,omitempty"`
}

func newNovelEvidenceSearchCmd(flags *rootFlags) *cobra.Command {
	var flagLimit int
	var flagFetch bool
	var flagMaxFetch int

	cmd := &cobra.Command{
		Use:   "evidence-search [term]",
		Short: "Find findings whose evidence mentions an IOC, hostname, or user",
		Long: "Use this command to find findings whose EVIDENCE rows contain a term (IOC,\n" +
			"hostname, user) when you do NOT already know the finding ID. Do NOT use this\n" +
			"command for searching finding names/categories; use 'search \"term\" --type\n" +
			"findings' instead.\n\n" +
			"Searches the local evidence cache plus the raw synced finding payloads. The API\n" +
			"only returns evidence per known finding ID, so 'which findings mention this IP'\n" +
			"is impossible upstream. Pass --fetch to populate the cache live: evidence is\n" +
			"pulled for open synced findings (bounded by --max-fetch) before searching.",
		Example: "  blumira-cli evidence-search \"rdp brute force\"\n" +
			"  blumira-cli evidence-search 10.0.4.21 --fetch --max-fetch 25 --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would search the local evidence corpus")
				return nil
			}
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a search term is required"))
			}
			if err := validateDataSourceStrategy(flags, "auto"); err != nil {
				return err
			}
			term := strings.TrimSpace(args[0])
			if flagLimit <= 0 {
				flagLimit = 50
			}
			if flagMaxFetch <= 0 {
				flagMaxFetch = 25
			}
			if cliutil.IsDogfoodEnv() && flagMaxFetch > 3 {
				flagMaxFetch = 3 // keep live-dogfood inside the per-command timeout
			}

			s, err := openAnalyticsStore(cmd.Context())
			if err != nil {
				return configErr(err)
			}
			defer s.Close()
			if !hintIfUnsynced(cmd, s, "") {
				hintIfStale(cmd, s, "", flags.maxAge)
			}
			findings, err := loadFindings(s)
			if err != nil {
				return apiErr(err)
			}

			view := evidenceSearchView{Term: term, Matches: []evidenceMatch{}}

			// Optional live fetch: populate the evidence cache for open findings
			// before searching. Respect --data-source local as "skip live".
			if flagFetch && flags.dataSource != "local" {
				c, err := flags.newClient()
				if err != nil {
					return err
				}
				now := time.Now().UTC()
				fetched := 0
				for _, f := range findings {
					if fetched >= flagMaxFetch {
						break
					}
					if f.IsResolved() || f.ID == "" {
						continue
					}
					raw, fetchErr := fetchFindingEvidence(cmd, c, f)
					if fetchErr != nil {
						view.FetchFailures = append(view.FetchFailures, fetchFailure{
							FindingID: f.ID, Error: fetchErr.Error(),
						})
						fetched++
						continue
					}
					items := extractEvidenceItems(raw)
					if err := storeEvidenceItems(s.DB(), f.ID, f.Account, items, now); err != nil {
						return apiErr(err)
					}
					fetched++
				}
				view.Fetched = fetched
				view.MaxFetch = flagMaxFetch
				if len(view.FetchFailures) > 0 && !flags.quiet {
					fmt.Fprintf(cmd.ErrOrStderr(),
						"warning: %d of %d evidence fetches failed; search covers the %d that succeeded plus the existing cache\n",
						len(view.FetchFailures), fetched, fetched-len(view.FetchFailures))
				}
			}

			// Search the evidence cache; keep one match per finding (first/best
			// snippet) so a multi-row evidence hit doesn't crowd out other
			// findings within --limit.
			cacheMatches, err := searchEvidenceCache(s.DB(), term, flagLimit)
			if err != nil {
				return apiErr(err)
			}
			nameByID := map[string]string{}
			for _, f := range findings {
				nameByID[f.ID] = f.Name
			}
			seen := map[string]bool{}
			for _, m := range cacheMatches {
				if seen[m.FindingID] {
					continue
				}
				seen[m.FindingID] = true
				if m.Name == "" {
					m.Name = nameByID[m.FindingID]
				}
				view.Matches = append(view.Matches, m)
			}

			// Also grep the raw synced finding payloads (indicators often appear
			// in the finding body itself, not only in evidence rows).
			if len(view.Matches) < flagLimit {
				raws, err := loadFindingRaws(s)
				if err != nil {
					return apiErr(err)
				}
				view.FindingsRaw = len(raws)
				for _, m := range matchFindingsRaw(findings, raws, term, flagLimit-len(view.Matches)) {
					if seen[m.FindingID] {
						continue
					}
					seen[m.FindingID] = true
					view.Matches = append(view.Matches, m)
				}
			}
			view.EvidenceRows = countEvidenceRows(s.DB())

			if len(view.Matches) == 0 {
				view.Note = fmt.Sprintf(
					"no matches for %q across %d cached evidence rows and %d synced findings; run with --fetch to pull evidence for open findings, or sync more data",
					term, view.EvidenceRows, view.FindingsRaw)
				if !flags.quiet {
					fmt.Fprintln(cmd.ErrOrStderr(), view.Note)
				}
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().IntVar(&flagLimit, "limit", 50, "Maximum matches to return")
	cmd.Flags().BoolVar(&flagFetch, "fetch", false, "Fetch evidence live for open synced findings before searching (bounded by --max-fetch)")
	cmd.Flags().IntVar(&flagMaxFetch, "max-fetch", 25, "Maximum findings to fetch evidence for when --fetch is set")
	return cmd
}

// fetchFindingEvidence pulls evidence for one finding, preferring the MSP
// account-scoped path when the finding carries an account id and falling back
// to the direct-org path.
func fetchFindingEvidence(cmd *cobra.Command, c evidenceGetter, f findingRec) (json.RawMessage, error) {
	ctx := cmd.Context()
	if f.Account != "" {
		path := "/msp/accounts/" + url.PathEscape(f.Account) + "/findings/" + url.PathEscape(f.ID) + "/evidence"
		if raw, err := c.Get(ctx, path, nil); err == nil {
			return raw, nil
		}
	}
	path := "/org/findings/" + url.PathEscape(f.ID) + "/evidence"
	raw, err := c.Get(ctx, path, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching evidence for %s: %w", f.ID, err)
	}
	return raw, nil
}

// countEvidenceRows reports the cache size for the search envelope.
func countEvidenceRows(db *sql.DB) int {
	if err := ensureEvidenceCache(db); err != nil {
		return 0
	}
	row := db.QueryRow(`SELECT COUNT(*) FROM finding_evidence_cache`)
	var n int
	if err := row.Scan(&n); err != nil {
		return 0
	}
	return n
}
