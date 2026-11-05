// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: tenant mail-flow health verdict.
// pp:data-source live
// pp:client-call

package cli

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"mimecast-pp-cli/internal/client"
	"mimecast-pp-cli/internal/cliutil"
)

type queueReading struct {
	Name  string `json:"name"`
	Size  int    `json:"size"`
	At    string `json:"at,omitempty"`
	State string `json:"state"` // ok | warn | fail
}

type stuckGroup struct {
	Destination string   `json:"destination"`
	Info        string   `json:"info"`
	Count       int      `json:"count"`
	MaxAttempts int      `json:"max_attempts"`
	SampleIDs   []string `json:"sample_ids"`
}

type mailflowView struct {
	Verdict       string            `json:"verdict"` // OK | WARN | FAIL
	Reasons       []string          `json:"reasons"`
	Queues        []queueReading    `json:"queues"`
	LastInbound   string            `json:"last_inbound,omitempty"`
	LastOutbound  string            `json:"last_outbound,omitempty"`
	StuckMessages int               `json:"stuck_messages"`
	Stuck         []stuckGroup      `json:"stuck_by_destination"`
	ProcessingCap bool              `json:"processing_scan_truncated,omitempty"`
	NotMeasured   []string          `json:"not_measured,omitempty"`
	SectionErrors map[string]string `json:"section_errors,omitempty"`
}

func newNovelMailflowCmd(flags *rootFlags) *cobra.Command {
	var maxQueue, stuckAttempts int
	var staleAfterFlag string
	var failOnWarn bool
	var accountCode string
	var maxScanPages int

	cmd := &cobra.Command{
		Use:   "mailflow",
		Short: "One OK/WARN/FAIL verdict for tenant mail flow: queue backlog, inbound staleness and stuck deliveries",
		Long: "Use this command for tenant-wide mail-flow health: queue backlog, stalled inbound or outbound flow, and stuck deliveries grouped by destination. " +
			"Do NOT use it to locate one specific message; use 'why' instead.\n\n" +
			"Combines the email queue counts, gateway statistics (queue sizes, last inbound/outbound times) and the processing queue. " +
			"FAIL when any queue (inbound, outbound, processing, delivery) exceeds --max-queue; WARN when a queue is over half of it, when no inbound mail " +
			"arrived within --stale-after, when messages have been retried --stuck-attempts times, or when a section could not be measured " +
			"(listed in section_errors). Exits 1 on FAIL (and on WARN with --fail-on-warn) so RMM monitors can alert. MX-based (Cloud Gateway) deployments only.",
		Example:     "  mimecast-cli mailflow\n  mimecast-cli mailflow --max-queue 200 --stale-after 30m --agent\n  mimecast-cli mailflow --fail-on-warn --quiet",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:typed-exit-codes": "0,1,2,4,5,7"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "mailflow")
			}
			staleAfter, err := cliutil.ParseDurationLoose(staleAfterFlag)
			if err != nil {
				return usageErr(fmt.Errorf("invalid --stale-after %q: %w", staleAfterFlag, err))
			}
			if maxQueue < 1 || stuckAttempts < 1 {
				return usageErr(fmt.Errorf("--max-queue and --stuck-attempts must be at least 1"))
			}
			if cliutil.IsDogfoodEnv() && maxScanPages > 1 {
				maxScanPages = 1
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			acct := mcAccountCode(accountCode)
			now := time.Now()
			view := mailflowView{Reasons: []string{}, Queues: []queueReading{}, Stuck: []stuckGroup{}, SectionErrors: map[string]string{}}
			var firstErr error
			note := func(section string, err error) {
				view.SectionErrors[section] = err.Error()
				if firstErr == nil {
					firstErr = err
				}
			}

			if q, err := fetchQueueCounts(ctx, c, acct, now); err != nil {
				note("email_queues", err)
			} else {
				view.Queues = append(view.Queues, q...)
				for _, dir := range []string{"inbound_queue", "outbound_queue"} {
					if !slices.ContainsFunc(q, func(r queueReading) bool { return r.Name == dir }) {
						view.NotMeasured = append(view.NotMeasured, dir+" (no reading in the last hour)")
					}
				}
			}
			if acct != "" {
				// The statistics endpoint is REST with no documented account
				// selector; reading it would report the API app's own account.
				view.NotMeasured = append(view.NotMeasured, "statistics (no delegated-account selector on this endpoint)")
			} else if stats, err := c.GetNoCache(ctx, "/email/cloud-gateway/v1/statistics", nil); err != nil {
				note("statistics", err)
			} else {
				var s struct {
					ProcessingQueueSize *float64 `json:"processingQueueSize"`
					DeliveryQueueSize   *float64 `json:"deliveryQueueSize"`
					LastInboundDateTime string   `json:"lastInboundDateTime"`
					LastOutbound        string   `json:"lastOutboundDateTime"`
				}
				if err := json.Unmarshal(stats, &s); err != nil {
					note("statistics", fmt.Errorf("decoding statistics: %w", err))
				} else {
					if s.ProcessingQueueSize != nil {
						view.Queues = append(view.Queues, queueReading{Name: "processing", Size: int(*s.ProcessingQueueSize)})
					} else {
						view.NotMeasured = append(view.NotMeasured, "processing queue size (missing from statistics)")
					}
					if s.DeliveryQueueSize != nil {
						view.Queues = append(view.Queues, queueReading{Name: "delivery", Size: int(*s.DeliveryQueueSize)})
					} else {
						view.NotMeasured = append(view.NotMeasured, "delivery queue size (missing from statistics)")
					}
					if _, ok := parseMCTime(s.LastInboundDateTime); !ok {
						view.NotMeasured = append(view.NotMeasured, "inbound staleness (statistics lastInboundDateTime missing or unparsable)")
					}
					view.LastInbound, view.LastOutbound = s.LastInboundDateTime, s.LastOutbound
				}
			}
			if msgs, truncated, err := fetchProcessing(ctx, c, acct, maxScanPages); err != nil {
				note("processing", err)
			} else {
				view.Stuck, view.StuckMessages = groupStuck(msgs, stuckAttempts)
				view.ProcessingCap = truncated
			}
			// Three sections, minus statistics when it is skipped for a delegated account.
			attempted := 3
			if acct != "" {
				attempted = 2
			}
			if len(view.SectionErrors) == attempted {
				return classifyAPIError(cmd.ErrOrStderr(), firstErr, flags)
			}
			if len(view.SectionErrors) == 0 {
				view.SectionErrors = nil
			}
			judgeMailflow(&view, maxQueue, staleAfter, now)

			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				w := cmd.OutOrStdout()
				fmt.Fprintf(w, "Mail flow: %s\n", view.Verdict)
				for _, r := range view.Reasons {
					fmt.Fprintf(w, "  - %s\n", r)
				}
				for _, q := range view.Queues {
					fmt.Fprintf(w, "  %-18s %6d  %s\n", q.Name, q.Size, q.State)
				}
				for _, g := range view.Stuck {
					fmt.Fprintf(w, "  stuck: %d to %s (%s), up to %d attempts\n", g.Count, g.Destination, g.Info, g.MaxAttempts)
				}
				for _, n := range view.NotMeasured {
					fmt.Fprintf(cmd.ErrOrStderr(), "note: not measured: %s\n", n)
				}
				for _, s := range sortedKeys(view.SectionErrors) {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s unavailable: %s\n", s, view.SectionErrors[s])
				}
			} else if err := printNovelJSON(cmd.OutOrStdout(), view, flags); err != nil {
				return err
			}
			if view.Verdict == "FAIL" || (failOnWarn && view.Verdict == "WARN") {
				return &cliError{code: 1, err: fmt.Errorf("mail flow %s: %s", view.Verdict, strings.Join(view.Reasons, "; "))}
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&maxQueue, "max-queue", 200, "Queue size that makes the verdict FAIL (half of it is WARN)")
	cmd.Flags().StringVar(&staleAfterFlag, "stale-after", "30m", "WARN when no inbound mail arrived within this long (30m, 2h, 1d)")
	cmd.Flags().IntVar(&stuckAttempts, "stuck-attempts", 3, "Processing attempts after which a message counts as stuck")
	cmd.Flags().BoolVar(&failOnWarn, "fail-on-warn", false, "Exit 1 on WARN as well as FAIL")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 5, "Maximum processing-queue pages (100 messages each) to scan")
	cmd.Flags().StringVar(&accountCode, "account-code", "", "Delegated customer account code (MSP); defaults to MIMECAST_ACCOUNT_CODE")
	return cmd
}

// fetchQueueCounts reads the latest inbound/outbound queue counts for the last hour.
func fetchQueueCounts(ctx context.Context, c *client.Client, acct string, now time.Time) ([]queueReading, error) {
	item := map[string]any{"start": mcTime(now.Add(-time.Hour)), "end": mcTime(now)}
	resp, err := mcRead(ctx, c, "/api/email/get-email-queues", mcBody([]any{item}, 0, "", acct))
	if err != nil {
		return nil, err
	}
	out := make([]queueReading, 0, 2)
	for _, raw := range resp.Data {
		var q struct {
			Inbound  []queueCount `json:"inboundEmailQueue"`
			Outbound []queueCount `json:"outboundEmailQueue"`
		}
		if json.Unmarshal(raw, &q) != nil {
			continue
		}
		if r, ok := latestCount("inbound_queue", q.Inbound); ok {
			out = append(out, r)
		}
		if r, ok := latestCount("outbound_queue", q.Outbound); ok {
			out = append(out, r)
		}
	}
	return out, nil
}

type queueCount struct {
	Count int    `json:"count"`
	Date  string `json:"date"`
}

// latestCount picks the most recent bucket (order of the array is not documented).
func latestCount(name string, counts []queueCount) (queueReading, bool) {
	var best queueCount
	var bestT time.Time
	found := false
	for _, c := range counts {
		t, _ := parseMCTime(c.Date)
		if !found || t.After(bestT) {
			best, bestT, found = c, t, true
		}
	}
	return queueReading{Name: name, Size: best.Count, At: best.Date}, found
}

type processingMessage struct {
	ID       string `json:"id"`
	Attempts int    `json:"attempts"`
	Info     string `json:"info"`
	RemoteIP string `json:"remoteIp"`
	To       mcAddr `json:"to"`
}

func fetchProcessing(ctx context.Context, c *client.Client, acct string, maxPages int) ([]processingMessage, bool, error) {
	// Oldest first: during a backlog the long-retried messages are the stuck ones.
	item := map[string]any{"type": "processing", "sortOrder": "asc"}
	page, err := mcReadAll(ctx, c, "/api/gateway/find-processing-messages", []any{item}, acct, mcMaxPageSize, maxPages)
	if err != nil {
		return nil, false, err
	}
	out := make([]processingMessage, 0)
	for _, raw := range unwrapList(page.Data, "messages") {
		var m processingMessage
		if json.Unmarshal(raw, &m) == nil && m.ID != "" {
			out = append(out, m)
		}
	}
	return out, page.Truncated, nil
}

// groupStuck groups messages at or over the attempt threshold by
// destination (remote IP, else recipient domain) and processing info.
func groupStuck(msgs []processingMessage, minAttempts int) ([]stuckGroup, int) {
	byKey := map[[2]string]*stuckGroup{}
	total := 0
	for _, m := range msgs {
		if m.Attempts < minAttempts {
			continue
		}
		total++
		dest := cmp.Or(m.RemoteIP, addrDomain(m.To.EmailAddress))
		key := [2]string{dest, m.Info}
		g := byKey[key]
		if g == nil {
			g = &stuckGroup{Destination: dest, Info: m.Info, SampleIDs: []string{}}
			byKey[key] = g
		}
		g.Count++
		g.MaxAttempts = max(g.MaxAttempts, m.Attempts)
		if len(g.SampleIDs) < 3 {
			g.SampleIDs = append(g.SampleIDs, m.ID)
		}
	}
	out := make([]stuckGroup, 0, len(byKey))
	for _, g := range byKey {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out, total
}

// judgeMailflow sets queue states and the overall verdict.
func judgeMailflow(v *mailflowView, maxQueue int, staleAfter time.Duration, now time.Time) {
	worst := 0 // 0 ok, 1 warn, 2 fail
	raise := func(level int, reason string) {
		worst = max(worst, level)
		v.Reasons = append(v.Reasons, reason)
	}
	for i := range v.Queues {
		q := &v.Queues[i]
		switch {
		case q.Size > maxQueue:
			q.State = "fail"
			raise(2, fmt.Sprintf("%s has %d messages (over %d)", q.Name, q.Size, maxQueue))
		case q.Size*2 > maxQueue:
			q.State = "warn"
			raise(1, fmt.Sprintf("%s has %d messages (over half of %d)", q.Name, q.Size, maxQueue))
		default:
			q.State = "ok"
		}
	}
	if t, ok := parseMCTime(v.LastInbound); ok && staleAfter > 0 && now.Sub(t) > staleAfter {
		raise(1, fmt.Sprintf("no inbound mail since %s (over %s)", v.LastInbound, staleAfter))
	}
	if v.StuckMessages > 0 {
		raise(1, fmt.Sprintf("%d message(s) stuck in processing across %d destination(s)", v.StuckMessages, len(v.Stuck)))
	}
	if v.ProcessingCap {
		raise(1, "processing queue scan stopped at --max-scan-pages; stuck counts are a lower bound")
	}
	// An unmeasured section (for example a product or permission the API app
	// lacks) must not read as healthy.
	for _, section := range sortedKeys(v.SectionErrors) {
		raise(1, section+" not measured: "+v.SectionErrors[section])
	}
	for _, n := range v.NotMeasured {
		raise(1, "not measured: "+n)
	}
	v.Verdict = []string{"OK", "WARN", "FAIL"}[worst]
}

func sortedKeys(m map[string]string) []string { return slices.Sorted(maps.Keys(m)) }
