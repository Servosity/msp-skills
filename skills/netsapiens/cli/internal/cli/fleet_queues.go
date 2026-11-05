// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"netsapiens-pp-cli/internal/client"
	"netsapiens-pp-cli/internal/cliutil"
	"netsapiens-pp-cli/internal/store"
)

// queueRow numbers are nil (JSON null) when the statistics or roster omit them:
// a queue with no statistics row has unknown abandonment, not 0%.
type queueRow struct {
	Domain          string   `json:"domain"`
	Callqueue       string   `json:"callqueue"`
	Name            string   `json:"name,omitempty"`
	HasStats        bool     `json:"has_statistics"`
	CallsTotal      *float64 `json:"calls_total"`
	Abandoned       *float64 `json:"calls_abandoned"`
	AbandonedRate   *float64 `json:"abandoned_rate"`
	ServiceLevelPct *float64 `json:"service_level_percent"`
	AvgQueueWaitSec *float64 `json:"avg_queue_wait_seconds"`
	AgentsAvailable *float64 `json:"agents_available"`
	AgentsTotal     *float64 `json:"agents_total"`
	AgentsNoPhone   *int     `json:"agents_without_registered_phone"` // nil unless every agent record states it
	Staffing        string   `json:"staffing"`                        // ok | none-available | agents-unreachable | no-agents | unknown
}

type queueFailure struct {
	Domain string `json:"domain"`
	Error  string `json:"error"`
}

type queueReport struct {
	WindowStart      string         `json:"window_start"`
	WindowEnd        string         `json:"window_end"`
	DomainsChecked   int            `json:"domains_checked"`
	DomainsAvailable int            `json:"domains_available"`
	DomainsCapped    bool           `json:"domains_capped"`
	Queues           []queueRow     `json:"queues"`
	FetchFailures    []queueFailure `json:"fetch_failures,omitempty"`
	Note             string         `json:"note,omitempty"`
	// Complete is false when a domain was capped out or failed to fetch.
	Complete bool `json:"complete"`
}

func decodeRows(data json.RawMessage) ([]fleetRow, error) {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	var rows []fleetRow
	if err := dec.Decode(&rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// buildQueueRows joins the per-queue statistics object (keyed by queue id) with
// the queue roster and agent registration for one domain.
func buildQueueRows(domain string, stats map[string]fleetRow, queues, agents []fleetRow) []queueRow {
	roster := indexBy(queues, "callqueue")
	noPhone, unknownReach := map[string]int{}, map[string]int{}
	unattributed := 0 // agent rows without a callqueue: reachability unknown for every queue
	for _, a := range agents {
		q := a.fstr("callqueue")
		switch {
		case q == "":
			unattributed++
		case !has(a, "callqueue-agent-has-registered-devices"):
			unknownReach[q]++
		default:
			if v := strings.ToLower(a.fstr("callqueue-agent-has-registered-devices")); v == "no" || v == "false" || v == "0" {
				noPhone[q]++
			}
		}
	}
	ids := map[string]bool{}
	for id := range stats {
		ids[id] = true
	}
	for _, q := range queues {
		ids[q.fstr("callqueue")] = true
	}
	out := make([]queueRow, 0, len(ids))
	for _, id := range sortedKeys(ids) {
		if id == "" {
			continue
		}
		s, hasStats := stats[id]
		if s == nil {
			s = fleetRow{}
		}
		q := roster[key(domain, id)]
		if q == nil {
			q = fleetRow{}
		}
		r := queueRow{Domain: domain, Callqueue: id, Name: firstNonEmpty(s.fstr("name"), q.fstr("description")), HasStats: hasStats}
		reachKnown := unattributed == 0 && unknownReach[id] == 0
		if reachKnown {
			n := noPhone[id]
			r.AgentsNoPhone = &n
		}
		r.CallsTotal = s.fnumPtr("calls-total-count")
		r.Abandoned = s.fnumPtr("calls-abandoned-count")
		r.AbandonedRate = s.fnumPtr("calls-abandoned-rate")
		r.ServiceLevelPct = s.fnumPtr("calls-service-level-percent")
		r.AvgQueueWaitSec = s.fnumPtr("calls-queueing-avg-duration-seconds")
		r.AgentsAvailable = q.fnumPtr("callqueue-count-agents-available")
		r.AgentsTotal = q.fnumPtr("callqueue-count-agents-total")
		switch {
		case r.AgentsTotal == nil || r.AgentsAvailable == nil:
			r.Staffing = "unknown"
		case *r.AgentsTotal == 0:
			r.Staffing = "no-agents"
		case *r.AgentsAvailable == 0:
			r.Staffing = "none-available"
		case r.AgentsNoPhone != nil && *r.AgentsNoPhone > 0:
			r.Staffing = "agents-unreachable"
		case !reachKnown:
			r.Staffing = "unknown"
		default:
			r.Staffing = "ok"
		}
		out = append(out, r)
	}
	return out
}

func fetchDomainQueues(ctx context.Context, c *client.Client, domain, start, end string) ([]queueRow, error) {
	base := "/domains/" + url.PathEscape(domain)
	raw, err := c.Get(ctx, base+"/statistics/callqueues/per-queue", map[string]string{"datetime-start": start, "datetime-end": end})
	if err != nil {
		return nil, fmt.Errorf("queue statistics: %w", err)
	}
	stats := map[string]fleetRow{}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&stats); err != nil {
		return nil, fmt.Errorf("queue statistics are not an object keyed by queue: %w", err)
	}
	qraw, err := c.Get(ctx, base+"/callqueues", nil)
	if err != nil {
		return nil, fmt.Errorf("call queues: %w", err)
	}
	queues, err := decodeRows(qraw)
	if err != nil {
		return nil, fmt.Errorf("call queues: %w", err)
	}
	for _, q := range queues {
		if q.fstr("domain") == "" {
			q["domain"] = domain
		}
	}
	araw, err := c.Get(ctx, base+"/agents", nil)
	if err != nil {
		return nil, fmt.Errorf("agents: %w", err)
	}
	agents, err := decodeRows(araw)
	if err != nil {
		return nil, fmt.Errorf("agents: %w", err)
	}
	return buildQueueRows(domain, stats, queues, agents), nil
}

// queueTargetDomains enumerates the live domain list, so a domain that gained
// a queue after the last sync is still checked. Domains the mirror knows have
// call queues are ordered first (they survive --max-domains); the mirror never
// narrows the list. The caller applies --max-domains and reports the cap.
func queueTargetDomains(ctx context.Context, c *client.Client, dbPath string) ([]string, string, error) {
	first := map[string]bool{}
	if path := fleetDBPath(dbPath); fileExists(path) {
		if db, err := store.OpenReadOnlyContext(ctx, path); err == nil {
			cov := fleetCov{}
			rows, lerr := loadCovered(ctx, db, cov, rtCallqueues)
			_ = db.Close()
			if lerr == nil {
				for _, r := range rows {
					first[r.domainOf()] = true
				}
			}
		}
	}
	const page = 1000 // API maximum; /domains pages on a 0-based start
	var names []string
	prevFirst := ""
	for start := 0; ; start += page { // runs to a short page: a proven end, no silent ceiling
		// netsapiens hand-fix (netsapiens-fanout-enumerates-live-domains): fanout-scope;
		// a complete fan-out must see the live list, never the 5-minute HTTP cache.
		raw, err := c.GetNoCache(ctx, "/domains", map[string]string{"limit": strconv.Itoa(page), "start": strconv.Itoa(start)})
		if err != nil {
			return nil, "", err
		}
		rows, err := decodeRows(raw)
		if err != nil {
			return nil, "", fmt.Errorf("domain list is not a JSON array: %w", err)
		}
		if len(rows) > 0 {
			first := rows[0].fstr("domain")
			if start > 0 && first == prevFirst {
				return nil, "", fmt.Errorf("domain list did not advance at start=%d (server ignored start); cannot enumerate every domain", start)
			}
			prevFirst = first
		}
		for _, r := range rows {
			if n := r.fstr("domain"); n != "" {
				names = append(names, n)
			}
		}
		if len(rows) < page {
			break
		}
	}
	sort.Strings(names)
	if len(first) == 0 {
		return names, "live domain list", nil
	}
	sort.SliceStable(names, func(i, j int) bool { return first[names[i]] && !first[names[j]] })
	return names, "live domain list (domains with synced call queues first)", nil
}

func capDomains(d []string, n int) []string {
	if n > 0 && len(d) > n {
		return d[:n]
	}
	return d
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func newNovelFleetQueuesCmd(flags *rootFlags) *cobra.Command {
	var window, domain, dbPath string
	var maxDomains int
	cmd := &cobra.Command{
		Use:   "queues",
		Short: "Rank call queues across clients by abandonment and service level, with staffing and agent reachability",
		Long: `Use this command for call-queue abandonment, service level, and whether queues are staffed with reachable agents.
Do NOT use this command for direct-dial missed calls to individual users; use 'fleet missed' instead.

For each domain it reads the per-queue statistics for the window, the queue roster (agents
available/total) and the agent list (agents without a registered phone), then ranks every
queue together. Domains come from --domain, else the live domain list with domains that have
synced call queues first (capped by --max-domains). Failed domains are listed in
fetch_failures, and complete is false when any domain was capped out or failed.`,
		Example: strings.Trim(`
  netsapiens-cli fleet queues --window 7d --agent
  netsapiens-cli fleet queues --domain acme.example --window 24h`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet queues")
			}
			span, err := cliutil.ParseDurationLoose(window)
			if err != nil || span <= 0 {
				return usageErr(fmt.Errorf("--window must be a positive duration like 7d or 24h, got %q", window))
			}
			if cliutil.IsDogfoodEnv() {
				maxDomains = 1
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			end := time.Now().UTC().Truncate(time.Second)
			rep := queueReport{WindowStart: end.Add(-span).Format(time.RFC3339), WindowEnd: end.Format(time.RFC3339), Queues: []queueRow{}}
			targets := []string{domain}
			source := "--domain"
			if domain == "" {
				if targets, source, err = queueTargetDomains(ctx, c, dbPath); err != nil {
					return classifyAPIError(cmd.ErrOrStderr(), fmt.Errorf("listing domains: %w", err), flags)
				}
			}
			rep.DomainsAvailable = len(targets)
			if maxDomains > 0 && len(targets) > maxDomains {
				targets, rep.DomainsCapped = capDomains(targets, maxDomains), true
			}
			results, failures := cliutil.FanoutRun(ctx, targets, func(d string) string { return d },
				func(ctx context.Context, d string) ([]queueRow, error) {
					return fetchDomainQueues(ctx, c, d, rep.WindowStart, rep.WindowEnd)
				})
			rep.DomainsChecked = len(targets)
			for _, r := range results {
				rep.Queues = append(rep.Queues, r.Value...)
			}
			for _, f := range failures {
				rep.FetchFailures = append(rep.FetchFailures, queueFailure{Domain: f.Source, Error: f.Err.Error()})
			}
			val := func(p *float64) float64 { // unknown sorts last
				if p == nil {
					return -1
				}
				return *p
			}
			sort.SliceStable(rep.Queues, func(i, j int) bool {
				if a, b := val(rep.Queues[i].AbandonedRate), val(rep.Queues[j].AbandonedRate); a != b {
					return a > b
				}
				return val(rep.Queues[i].Abandoned) > val(rep.Queues[j].Abandoned)
			})
			if len(targets) > 0 && len(results) == 0 && len(failures) > 0 {
				return classifyAPIError(cmd.ErrOrStderr(), fmt.Errorf("all %d domain(s) failed; first %s: %w", len(targets), failures[0].Source, failures[0].Err), flags)
			}
			if len(failures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d domains failed; ranking covers the remaining %d\n", len(failures), len(targets), len(targets)-len(failures))
			}
			rep.Note = "domains from " + source
			if rep.DomainsCapped {
				rep.Note += fmt.Sprintf("; checked %d of %d domains (--max-domains), raise --max-domains or pass --domain for the rest", len(targets), rep.DomainsAvailable)
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+rep.Note)
			}
			rep.Complete = !rep.DomainsCapped && len(rep.FetchFailures) == 0
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if !rep.Complete && (fleetRowOutput(flags) || flags.selectFields != "") {
					return incompleteAuditErr(fmt.Sprintf("checked %d of %d domains, %d failed", len(targets)-len(failures), rep.DomainsAvailable, len(failures)), "raise --max-domains (0 checks every domain), pass --domain, or retry the failed domains")
				}
				if fleetRowOutput(flags) {
					return printJSONFiltered(cmd.OutOrStdout(), rep.Queues, flags)
				}
				return printJSONFiltered(cmd.OutOrStdout(), rep, flags)
			}
			if rep.DomainsCapped || len(rep.FetchFailures) > 0 {
				defer fmt.Fprintf(cmd.OutOrStdout(), "Note: %s; %d domain(s) failed to fetch\n", rep.Note, len(rep.FetchFailures))
			}
			if len(rep.Queues) == 0 && !rep.Complete {
				fmt.Fprintf(cmd.OutOrStdout(), "Incomplete: no call queues found in the %d domain(s) that answered, but not every domain was checked.\n", len(targets)-len(failures))
				return nil
			}
			if len(rep.Queues) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No call queues found across the %d domain(s) checked.\n", rep.DomainsChecked)
				return nil
			}
			table := make([]map[string]any, 0, len(rep.Queues))
			for _, q := range rep.Queues {
				table = append(table, map[string]any{"domain": q.Domain, "queue": strings.TrimSpace(q.Callqueue + " " + q.Name), "abandoned_rate": numText(q.AbandonedRate), "service_level": numText(q.ServiceLevelPct), "agents": numText(q.AgentsAvailable) + "/" + numText(q.AgentsTotal), "staffing": q.Staffing})
			}
			return printAutoTable(cmd.OutOrStdout(), table)
		},
	}
	cmd.Flags().StringVar(&window, "window", "7d", "Statistics window (e.g. 7d, 24h)")
	cmd.Flags().StringVar(&domain, "domain", "", "Only this client domain")
	cmd.Flags().IntVar(&maxDomains, "max-domains", 25, "Maximum domains to query when --domain is not set")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror used to pick domains with call queues (optional)")
	return cmd
}
