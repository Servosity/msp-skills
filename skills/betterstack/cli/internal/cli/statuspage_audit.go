// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature (hand-built): status-page drift audit. Local pass reads status
// pages, monitors, and incidents from the mirror; the live pass fans out to
// /status-pages/{id}/resources (a nested, non-syncable resource) to map each
// page's published entries back to real monitors. Strategy is auto: pass
// --data-source local to skip the live fan-out.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"betterstack-pp-cli/internal/client"

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
	FetchFailures []string            `json:"fetch_failures,omitempty"`
	FirstError    string              `json:"first_error,omitempty"`
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
// open incidents (stale page or unsynced incidents: either way, look).
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
				Why:      fmt.Sprintf("page publishes aggregate_state=%q but the mirror has zero open incidents: stale page state or stale local incidents (run `sync`)", p.AggregateState),
			})
		}
	}
	return findings
}

// auditPageResources flags drift between one page's published resources and
// the mirror's monitor, heartbeat and incident state. Monitor and heartbeat
// entries are auditable against the mirror (Codex r1, betterstack 4.34.2
// reprint, class heartbeat-incidents-omitted: heartbeat entries were skipped,
// so an operational page listing a heartbeat with an open incident showed no
// drift); heartbeats is called only when a page lists one.
func auditPageResources(page statusPageRow, entries []pageResourceEntry, monitors map[string]monitorRow, heartbeats func() (map[string]heartbeatRow, error), open openIncidents) ([]statusPageFinding, error) {
	name := page.CompanyName
	if name == "" {
		name = page.Subdomain
	}
	findings := make([]statusPageFinding, 0)
	add := func(severity, label, why string) {
		findings = append(findings, statusPageFinding{PageID: page.ID, Page: name, Severity: severity, Resource: label, Why: why})
	}
	for _, e := range entries {
		label := e.PublicName
		if label == "" {
			label = e.ResourceID
		}
		switch {
		case e.Type == "" || strings.EqualFold(e.Type, "Monitor"):
			m, ok := monitors[e.ResourceID]
			if !ok {
				add("warning", label, "page lists a monitor that does not exist in the mirror (deleted monitor, or mirror needs `sync`)")
				continue
			}
			if m.Paused {
				add("warning", label, "page lists a paused monitor: its public status will never change")
			}
			if n := len(open.byMonitor[m.ID]); page.AggregateState == "operational" && n > 0 {
				add("drift", label, fmt.Sprintf("page shows operational while monitor %s has %d open incident(s)", m.ID, n))
			}
		case strings.EqualFold(e.Type, "Heartbeat"):
			hbs, err := heartbeats()
			if err != nil {
				return nil, err
			}
			h, ok := hbs[e.ResourceID]
			if !ok {
				add("warning", label, "page lists a heartbeat that does not exist in the mirror (deleted heartbeat, or mirror needs `sync`)")
				continue
			}
			if h.Paused {
				add("warning", label, "page lists a paused heartbeat: its public status will never change")
			}
			if n := len(open.byHeartbeat[h.ID]); page.AggregateState == "operational" && n > 0 {
				add("drift", label, fmt.Sprintf("page shows operational while heartbeat %s has %d open incident(s)", h.ID, n))
			}
		}
	}
	return findings, nil
}

// statusPageResourceMaxPages bounds one page's resource walk (250 per page);
// a walk past it is a failed fetch, never a truncated audit.
const statusPageResourceMaxPages = 1000

// fetchPageResources reads every page of GET /status-pages/{id}/resources
// (page / per_page 250) until pagination.next is null. Codex r1 (betterstack
// 4.34.2 reprint) class incomplete-paging-read-as-complete (a new instance of
// the pre-closed class export-first-page-only): one page was read, so a page
// whose 251st resource was a paused monitor reported no drift, "live check:
// ok". Any page that fails, does not parse, repeats the page before it, or
// names a next page while carrying no entries fails the whole walk.
func fetchPageResources(ctx context.Context, c *client.Client, pageID string) ([]pageResourceEntry, error) {
	var all []pageResourceEntry
	prevFirst := ""
	for page := 1; page <= statusPageResourceMaxPages; page++ {
		raw, err := c.Get(ctx, "/status-pages/"+url.PathEscape(pageID)+"/resources", map[string]string{"page": strconv.Itoa(page), "per_page": "250"})
		if err != nil {
			return nil, err
		}
		entries, more, err := parsePageResources(raw)
		if err != nil {
			return nil, fmt.Errorf("parsing resources (page %d): %w", page, err)
		}
		if len(entries) > 0 && page > 1 && entries[0].ID == prevFirst {
			return nil, fmt.Errorf("resources page %d repeats page %d (the page parameter was not honored)", page, page-1)
		}
		all = append(all, entries...)
		if !more {
			return all, nil
		}
		if len(entries) == 0 {
			return nil, fmt.Errorf("resources page %d is empty but names a next page", page)
		}
		prevFirst = entries[0].ID
	}
	return nil, fmt.Errorf("more than %d pages of resources; not audited", statusPageResourceMaxPages)
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
			open := indexOpenIncidents(incidents)
			var hbByID map[string]heartbeatRow
			heartbeats := func() (map[string]heartbeatRow, error) {
				if hbByID != nil {
					return hbByID, nil
				}
				hbs, err := loadHeartbeats(cmd.Context(), s)
				if err != nil {
					return nil, fmt.Errorf("reading heartbeats: %w", err)
				}
				hbByID = make(map[string]heartbeatRow, len(hbs))
				for _, h := range hbs {
					hbByID[h.ID] = h
				}
				return hbByID, nil
			}

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
						entries, err := fetchPageResources(cmd.Context(), c, p.ID)
						if err != nil {
							rep.FetchFailures = append(rep.FetchFailures, fmt.Sprintf("page %s: %v", p.ID, err))
							continue
						}
						found, err := auditPageResources(p, entries, monByID, heartbeats, open)
						if err != nil {
							return err
						}
						rep.Findings = append(rep.Findings, found...)
					}
					if len(rep.FetchFailures) == len(pages) && len(pages) > 0 {
						rep.LiveCheck = "skipped: every live resource fetch failed (check auth via `doctor`)"
					} else if len(rep.FetchFailures) > 0 {
						rep.LiveCheck = fmt.Sprintf("partial: %d of %d page fetches failed", len(rep.FetchFailures), len(pages))
					}
				}
			}
			if len(rep.FetchFailures) > 0 {
				rep.FirstError = rep.FetchFailures[0]
				// betterstack handfix (class all-fetches-failed-read-as-zero): every
				// live fetch failed, so nothing was audited live and "no drift" would
				// be a wrong answer.
				if len(rep.FetchFailures) == len(pages) {
					return fmt.Errorf("all %d live status-page resource fetches failed; no status-page resource was audited (first error: %s)", len(pages), rep.FirstError)
				}
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
