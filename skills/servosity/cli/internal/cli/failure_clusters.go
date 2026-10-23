// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: cluster backup failures by root cause across the book.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"servosity-msp-pp-cli/internal/client"
)

// Normalization turns one failure message into a signature that is the same
// for every host hitting the same root cause. Order matters: timestamps and
// UUIDs go before the generic hex and number rules would chew them up.
var failureNormalizers = []struct {
	re   *regexp.Regexp
	with string
}{
	{regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:?\d{2})?`), "<time>"},
	{regexp.MustCompile(`\d{4}-\d{2}-\d{2}`), "<date>"},
	{regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`), "<id>"},
	{regexp.MustCompile(`(?i)[a-z]:\\[^\s"']*`), "<path>"},
	{regexp.MustCompile(`\\\\[^\s"']+`), "<path>"},
	{regexp.MustCompile(`(^|[\s"'(=])/[^\s"')]+`), "$1<path>"},
	{regexp.MustCompile(`(?i)\b[0-9a-f]{8,}\b`), "<hex>"},
	{regexp.MustCompile(`\d+`), "<n>"},
	{regexp.MustCompile(`\s+`), " "},
}

// failureSignatureMax caps a signature so one verbose stack trace cannot
// become a cluster of its own per host.
const failureSignatureMax = 160

func normalizeFailureText(text string) string {
	s := strings.TrimSpace(text)
	for _, n := range failureNormalizers {
		s = n.re.ReplaceAllString(s, n.with)
	}
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > failureSignatureMax {
		s = string(r[:failureSignatureMax]) + "…"
	}
	if s == "" {
		return "(no error text)"
	}
	return s
}

type failureCluster struct {
	Engine    string   `json:"engine"`
	Signature string   `json:"signature"`
	Failures  int      `json:"failures"`
	Hosts     int      `json:"hosts"`
	Clients   int      `json:"clients"`
	ClientIDs []int64  `json:"client_ids"`
	HostNames []string `json:"host_names"`
	LastSeen  string   `json:"last_seen"`

	hostSet   map[string]bool
	clientSet map[int64]bool
}

// failureLookupGap is one client/engine whose failure list could not be read
// in full. Its failures up to the gap are still counted.
type failureLookupGap struct {
	CompanyID int64  `json:"company_id"`
	Engine    string `json:"engine"`
	Error     string `json:"error"`
}

// failureClustersResult is the --json shape: the clusters plus explicit
// coverage, so a lookup that failed can never read as "no failures".
type failureClustersResult struct {
	Month          string             `json:"month"`
	ClientsScanned int                `json:"clients_scanned"`
	Failures       int                `json:"failures"`
	Complete       bool               `json:"complete"`
	Incomplete     []failureLookupGap `json:"incomplete"`
	Clusters       []*failureCluster  `json:"clusters"`
}

// failureEvent is one failure as read from either engine's endpoint.
type failureEvent struct {
	engine, host, at, text string
	company                int64
}

// failurePageLimit bounds how many pages one company's failure list may span.
const failurePageLimit = 50

// fetchFailures reads every page of one company's failure list. A DRF page
// names the next one in "next"; its query (page or cursor) is re-sent
// against the same path so the client encodes it once.
func fetchFailures(ctx context.Context, c *client.Client, path string, params map[string]string) ([]map[string]any, error) {
	var items []map[string]any
	query := map[string]string{}
	for k, v := range params {
		query[k] = v
	}
	for page := 0; page < failurePageLimit; page++ {
		data, err := c.Get(ctx, path, query)
		if err != nil {
			return items, err
		}
		items = append(items, failureItems(data)...)
		var env struct {
			Next *string `json:"next"`
		}
		if json.Unmarshal(data, &env) != nil || env.Next == nil || *env.Next == "" {
			return items, nil
		}
		next, err := url.Parse(*env.Next)
		if err != nil {
			return items, fmt.Errorf("%s: unreadable next page link: %w", path, err)
		}
		for k, v := range next.Query() {
			if len(v) > 0 {
				query[k] = v[0]
			}
		}
	}
	return items, fmt.Errorf("%s: more than %d pages of failures", path, failurePageLimit)
}

// failureItems accepts a bare array, a {results: [...]} page, or a single
// object (the spec declares the response as one object).
func failureItems(data json.RawMessage) []map[string]any {
	var arr []map[string]any
	if json.Unmarshal(data, &arr) == nil {
		return arr
	}
	var env struct {
		Results []map[string]any `json:"results"`
	}
	if json.Unmarshal(data, &env) == nil && env.Results != nil {
		return env.Results
	}
	var one map[string]any
	if json.Unmarshal(data, &one) == nil && len(one) > 0 {
		return []map[string]any{one}
	}
	return nil
}

func stringField(m map[string]any, key string) string {
	if v, ok := m[key]; ok && v != nil {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// failureHost names the backup a failure belongs to: restic failures nest
// resticbackupset.resticbackup, DR failures carry drbackup directly.
func failureHost(item map[string]any, engine string) string {
	var backup map[string]any
	if engine == "restic" {
		if set, ok := item["resticbackupset"].(map[string]any); ok {
			backup, _ = set["resticbackup"].(map[string]any)
		}
	} else {
		backup, _ = item["drbackup"].(map[string]any)
	}
	if backup == nil {
		return ""
	}
	if name := stringField(backup, "device_name"); name != "" {
		return name
	}
	return stringField(backup, "id")
}

func clusterFailures(events []failureEvent) []*failureCluster {
	byKey := map[string]*failureCluster{}
	for _, e := range events {
		sig := normalizeFailureText(e.text)
		key := e.engine + "\x00" + sig
		c := byKey[key]
		if c == nil {
			c = &failureCluster{Engine: e.engine, Signature: sig, hostSet: map[string]bool{}, clientSet: map[int64]bool{}}
			byKey[key] = c
		}
		c.Failures++
		host := e.host
		if host == "" {
			host = "(unnamed)"
		}
		// Two clients can each have a host called SERVER: a host is
		// identified within its company, never across companies.
		c.hostSet[fmt.Sprintf("%d\x00%s", e.company, host)] = true
		c.clientSet[e.company] = true
		if e.at > c.LastSeen {
			c.LastSeen = e.at
		}
	}
	out := make([]*failureCluster, 0, len(byKey))
	for _, c := range byKey {
		c.Hosts, c.Clients = len(c.hostSet), len(c.clientSet)
		names := map[string]bool{}
		for h := range c.hostSet {
			names[h[strings.IndexByte(h, 0)+1:]] = true
		}
		for name := range names {
			c.HostNames = append(c.HostNames, name)
		}
		sort.Strings(c.HostNames)
		for id := range c.clientSet {
			c.ClientIDs = append(c.ClientIDs, id)
		}
		sort.Slice(c.ClientIDs, func(i, j int) bool { return c.ClientIDs[i] < c.ClientIDs[j] })
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Hosts != b.Hosts {
			return a.Hosts > b.Hosts
		}
		if a.Clients != b.Clients {
			return a.Clients > b.Clients
		}
		if a.Failures != b.Failures {
			return a.Failures > b.Failures
		}
		return a.Signature < b.Signature
	})
	return out
}

// pp:data-source live
func newNovelFailureClustersCmd(flags *rootFlags) *cobra.Command {
	var month string
	var top int
	var companies string
	var engine string

	cmd := &cobra.Command{
		Use:   "failure-clusters",
		Short: "Group this month's backup failures across every client by root cause",
		Long: `Pull the restic and DR backup-failure text for every client for one month,
normalize away IDs, paths, numbers and timestamps, and group failures that share
a root cause. Clusters are ranked by how many hosts they hit, then how many
clients, then how many failures.

Clients come from --companies, or from the local store's companies table (run
'sync' first). Each client costs two read-only API calls.`,
		Example: `  # This month's top 10 root causes across the book
  servosity-cli failure-clusters --top 10

  # Restic only, one month, two clients, as JSON
  servosity-cli failure-clusters --month 2026-10 --engine restic --companies 4421,4766 --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "failure-clusters")
			}
			if month == "" {
				month = time.Now().UTC().Format("2006-01")
			}
			start, err := time.Parse("2006-01", month)
			if err != nil {
				return usageErr(fmt.Errorf("invalid --month %q (want YYYY-MM)", month))
			}
			if top < 0 {
				return usageErr(fmt.Errorf("--top must be 0 (all) or more"))
			}
			engine = strings.ToLower(strings.TrimSpace(engine))
			var engines []string
			switch engine {
			case "", "all":
				engines = []string{"restic", "dr"}
			case "restic", "dr":
				engines = []string{engine}
			default:
				return usageErr(fmt.Errorf("invalid --engine %q (want restic|dr|all)", engine))
			}

			targets, err := resolveRestoreQueueTargets(cmd.Context(), nil, flags, companies)
			if err != nil {
				return err
			}
			if len(targets) == 0 {
				return fmt.Errorf("no companies to scan; run 'servosity-cli sync' first, or pass --companies")
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}

			params := map[string]string{"month": start.Format("2006-01-02")}
			var events []failureEvent
			gaps := []failureLookupGap{}
			calls, failed := 0, 0
			for _, t := range targets {
				for _, eng := range engines {
					path := fmt.Sprintf("/companies/%d/restic-backup-failures/", t.ID)
					textKey, atKey := "errors", "started_at"
					if eng == "dr" {
						path = fmt.Sprintf("/companies/%d/spx-backup-failures/", t.ID)
						textKey, atKey = "text_body", "created_at"
					}
					calls++
					items, err := fetchFailures(cmd.Context(), c, path, params)
					if err != nil {
						if cmd.Context().Err() != nil {
							return cmd.Context().Err()
						}
						// Keep the pages already read; record the gap.
						failed++
						gaps = append(gaps, failureLookupGap{CompanyID: t.ID, Engine: eng, Error: err.Error()})
						fmt.Fprintf(os.Stderr, "warning: company %d %s failures incomplete: %v\n", t.ID, eng, err)
					}
					for _, item := range items {
						events = append(events, failureEvent{
							engine:  eng,
							host:    failureHost(item, eng),
							at:      stringField(item, atKey),
							text:    stringField(item, textKey),
							company: t.ID,
						})
					}
				}
			}
			if failed == calls {
				return fmt.Errorf("every failure lookup failed (%d calls); check 'servosity-cli doctor'", calls)
			}

			clusters := clusterFailures(events)
			fmt.Fprintf(os.Stderr, "%d failures in %d clusters across %d clients (%s)\n", len(events), len(clusters), len(targets), month)
			if len(gaps) > 0 {
				fmt.Fprintf(os.Stderr, "warning: coverage incomplete: %d of %d failure lookups did not finish; counts are a lower bound\n", len(gaps), calls)
			}
			if top > 0 && len(clusters) > top {
				clusters = clusters[:top]
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(failureClustersResult{
					Month:          month,
					ClientsScanned: len(targets),
					Failures:       len(events),
					Complete:       len(gaps) == 0,
					Incomplete:     gaps,
					Clusters:       clusters,
				})
			}
			if len(clusters) == 0 {
				return nil
			}
			headers := []string{"RANK", "HOSTS", "CLIENTS", "FAILURES", "ENGINE", "LAST_SEEN", "SIGNATURE"}
			out := make([][]string, 0, len(clusters))
			for i, cl := range clusters {
				out = append(out, []string{fmt.Sprint(i + 1), fmt.Sprint(cl.Hosts), fmt.Sprint(cl.Clients), fmt.Sprint(cl.Failures), cl.Engine, dashIfEmpty(cl.LastSeen), cl.Signature})
			}
			return flags.printTable(cmd, headers, out)
		},
	}
	cmd.Flags().StringVar(&month, "month", "", "Month to scan as YYYY-MM (default: the current UTC month)")
	cmd.Flags().IntVar(&top, "top", 10, "Show only the top N clusters (0 = all)")
	cmd.Flags().StringVar(&companies, "companies", "", "Comma-separated company IDs to scan (default: every company in the local store)")
	cmd.Flags().StringVar(&engine, "engine", "all", "Engine to scan: restic|dr|all")
	return cmd
}
