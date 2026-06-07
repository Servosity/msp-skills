// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature (hand-built): status-page drift audit. Local pass reads status
// pages, monitors, and incidents from the mirror; the live pass fans out to
// /status-pages/{id}/resources (a nested, non-syncable resource) to map each
// page's published entries back to real monitors. Strategy is auto: pass
// --data-source local to skip the live fan-out.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"

	"github.com/spf13/cobra"
)

type statusPageFinding struct {
	PageID   string `json:"page_id"`
	Page     string `json:"page"`
	Severity string `json:"severity"` // drift | warning
	Resource string `json:"resource,omitempty"`
	Why      string `json:"why"`
}

type statusPageAuditReport struct {
	Findings      []statusPageFinding `json:"findings"`
	PagesAudited  int                 `json:"pages_audited"`
	LiveCheck     string              `json:"live_check"` // ok | skipped: <reason>
	ScannedLimit  bool                `json:"resource_scan_truncated,omitempty"`
	Note          string              `json:"note,omitempty"`
	FetchFailures []string            `json:"fetch_failures,omitempty"`
}

// pageResourceEntry is one element of GET /status-pages/{id}/resources.
type pageResourceEntry struct {
	ID         string
	PublicName string
	Type       string // e.g. Monitor, Heartbeat
	ResourceID string
}

// auditPagesLocal flags drift discoverable from the mirror alone: a page
// publishing a non-operational aggregate state while the account has zero
// open incidents (stale page or unsynced incidents — either way, look).
func auditPagesLocal(pages []statusPageRow, incidents []incidentRow) []statusPageFinding {
	openCount := 0
	for _, in := range incidents {
		if in.ResolvedAt == "" {
			openCount++
		}
	}
	findings := make([]statusPageFinding, 0)
	for _, p := range pages {
		name := p.CompanyName
		if name == "" {
			name = p.Subdomain
		}
		if p.AggregateState != "" && p.AggregateState != "operational" && openCount == 0 {
			findings = append(findings, statusPageFinding{
				PageID:   p.ID,
				Page:     name,
				Severity: "warning",
				Why:      fmt.Sprintf("page publishes aggregate_state=%q but the mirror has zero open incidents — stale page state or stale local incidents (run `sync`)", p.AggregateState),
			})
		}
	}
	return findings
}

// auditPageResources flags drift between one page's published resources and
// the mirror's monitor/incident state.
func auditPageResources(page statusPageRow, entries []pageResourceEntry, monitors map[string]monitorRow, openBySource map[string][]incidentRow) []statusPageFinding {
	name := page.CompanyName
	if name == "" {
		name = page.Subdomain
	}
	findings := make([]statusPageFinding, 0)
	for _, e := range entries {
		if e.Type != "" && e.Type != "Monitor" && e.Type != "monitor" {
			continue // only monitor-backed entries are auditable against the mirror
		}
		label := e.PublicName
		if label == "" {
			label = e.ResourceID
		}
		m, ok := monitors[e.ResourceID]
		if !ok {
			findings = append(findings, statusPageFinding{
				PageID: page.ID, Page: name, Severity: "warning", Resource: label,
				Why: "page lists a monitor that does not exist in the mirror (deleted monitor, or mirror needs `sync`)",
			})
			continue
		}
		if m.Paused {
			findings = append(findings, statusPageFinding{
				PageID: page.ID, Page: name, Severity: "warning", Resource: label,
				Why: "page lists a paused monitor — its public status will never change",
			})
		}
		if page.AggregateState == "operational" && len(openBySource[m.ID]) > 0 {
			findings = append(findings, statusPageFinding{
				PageID: page.ID, Page: name, Severity: "drift", Resource: label,
				Why: fmt.Sprintf("page shows operational while monitor %s has %d open incident(s)", m.ID, len(openBySource[m.ID])),
			})
		}
	}
	return findings
}

func parsePageResources(raw json.RawMessage) ([]pageResourceEntry, bool, error) {
	var envelope struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				PublicName   string `json:"public_name"`
				ResourceType string `json:"resource_type"`
				// json.Number preserves 7+ digit monitor IDs exactly; a
				// float64 decode would render them in scientific notation
				// and break the mirror join in auditPageResources.
				ResourceID json.Number `json:"resource_id"`
			} `json:"attributes"`
		} `json:"data"`
		Pagination struct {
			Next string `json:"next"`
		} `json:"pagination"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&envelope); err != nil {
		return nil, false, err
	}
	out := make([]pageResourceEntry, 0, len(envelope.Data))
	for _, d := range envelope.Data {
		out = append(out, pageResourceEntry{
			ID:         d.ID,
			PublicName: d.Attributes.PublicName,
			Type:       d.Attributes.ResourceType,
			ResourceID: d.Attributes.ResourceID.String(),
		})
	}
	return out, envelope.Pagination.Next != "", nil
}

// pp:data-source auto
func newNovelStatuspageAuditCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "statuspage-audit",
		Short: "Flags status pages showing operational while a backing monitor has an open incident, and page resources pointing at missing or paused monitors.",
		Long: "Cross-references each status page's published resources against the mirror's monitor and incident state. " +
			"Pages, monitors, and incidents come from the local mirror (run `sync` first); per-page resource membership is fetched live because it is a nested, non-syncable resource. " +
			"Pass --data-source local to skip the live fan-out and audit only what the mirror can prove.",
		Example:     "  betterstack-cli statuspage-audit\n  betterstack-cli statuspage-audit --agent\n  betterstack-cli statuspage-audit --data-source local",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "auto"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			s, err := openAnalyticsStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer s.Close()
			maybeEmitSyncHints(cmd, s, "status-pages", flags.maxAge)

			pages, err := loadStatusPages(cmd.Context(), s)
			if err != nil {
				return fmt.Errorf("reading status pages: %w", err)
			}
			incidents, err := loadIncidents(cmd.Context(), s)
			if err != nil {
				return fmt.Errorf("reading incidents: %w", err)
			}
			monitors, err := loadMonitors(cmd.Context(), s)
			if err != nil {
				return fmt.Errorf("reading monitors: %w", err)
			}
			monByID := make(map[string]monitorRow, len(monitors))
			for _, m := range monitors {
				monByID[m.ID] = m
			}
			openBySource := openIncidentsBySource(incidents)

			rep := statusPageAuditReport{PagesAudited: len(pages), LiveCheck: "ok", Findings: make([]statusPageFinding, 0)}
			rep.Findings = append(rep.Findings, auditPagesLocal(pages, incidents)...)

			if len(pages) == 0 {
				rep.LiveCheck = "skipped: no status pages in the local mirror (run `sync` first)"
			} else if flags.dataSource == "local" {
				rep.LiveCheck = "skipped: --data-source local"
			} else {
				c, err := flags.newClient()
				if err != nil {
					rep.LiveCheck = "skipped: " + err.Error()
				} else {
					for _, p := range pages {
						raw, err := c.Get(cmd.Context(), "/status-pages/"+url.PathEscape(p.ID)+"/resources", map[string]string{"per_page": "250"})
						if err != nil {
							rep.FetchFailures = append(rep.FetchFailures, fmt.Sprintf("page %s: %v", p.ID, err))
							continue
						}
						entries, truncated, err := parsePageResources(raw)
						if err != nil {
							rep.FetchFailures = append(rep.FetchFailures, fmt.Sprintf("page %s: parsing resources: %v", p.ID, err))
							continue
						}
						if truncated {
							rep.ScannedLimit = true
						}
						rep.Findings = append(rep.Findings, auditPageResources(p, entries, monByID, openBySource)...)
					}
					if len(rep.FetchFailures) == len(pages) && len(pages) > 0 {
						rep.LiveCheck = "skipped: every live resource fetch failed (check auth via `doctor`)"
					} else if len(rep.FetchFailures) > 0 {
						rep.LiveCheck = fmt.Sprintf("partial: %d of %d page fetches failed", len(rep.FetchFailures), len(pages))
					}
					if rep.ScannedLimit {
						rep.Note = "one or more pages have more than 250 resources; only the first page of resources was audited"
					}
				}
			}
			if len(rep.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d live page-resource fetches failed; findings computed from the remaining pages\n", len(rep.FetchFailures), len(pages))
			}

			// Drift first, then warnings, stable by page name.
			sort.SliceStable(rep.Findings, func(i, j int) bool {
				if rep.Findings[i].Severity != rep.Findings[j].Severity {
					return rep.Findings[i].Severity == "drift"
				}
				return rep.Findings[i].Page < rep.Findings[j].Page
			})

			if flags.asJSON {
				return flags.printJSON(cmd, rep)
			}
			if len(rep.Findings) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No status-page drift found across %d page(s) (live check: %s).\n", rep.PagesAudited, rep.LiveCheck)
				return nil
			}
			rows := make([][]string, 0, len(rep.Findings))
			for _, f := range rep.Findings {
				rows = append(rows, []string{f.PageID, truncateField(f.Page, 28), f.Severity, truncateField(f.Resource, 24), truncateField(f.Why, 60)})
			}
			if err := flags.printTable(cmd, []string{"PAGE-ID", "PAGE", "SEVERITY", "RESOURCE", "WHY"}, rows); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nlive check: %s\n", rep.LiveCheck)
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/betterstack-cli/data.db)")
	return cmd
}
