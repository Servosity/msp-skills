// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: release accountability audit.
// pp:data-source live
// pp:client-call

package cli

import (
	"cmp"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"mimecast-pp-cli/internal/cliutil"
)

type riskyRelease struct {
	Released   string   `json:"released"`
	Operator   string   `json:"operator"`
	Sender     string   `json:"sender"`
	Subject    string   `json:"subject"`
	HoldReason string   `json:"hold_reason"`
	Policy     string   `json:"policy,omitempty"`
	Flags      []string `json:"flags"`
	Ticket     string   `json:"ticket,omitempty"`
	NoTicket   bool     `json:"no_ticket"`
}

type operatorStats struct {
	Operator string         `json:"operator"`
	Released int            `json:"released"`
	Rejected int            `json:"rejected"`
	Risky    int            `json:"risky"`
	Reasons  map[string]int `json:"by_hold_reason"`
}

type releaseAuditView struct {
	Since        string          `json:"since"`
	ScannedLogs  int             `json:"scanned_logs"`
	MaxScanPages int             `json:"max_scan_pages"`
	Truncated    bool            `json:"truncated"`
	Operators    []operatorStats `json:"operators"`
	Risky        []riskyRelease  `json:"risky_releases"`
	Note         string          `json:"note,omitempty"`
}

func newNovelHeldReleaseAuditCmd(flags *rootFlags) *cobra.Command {
	var since, operator, dbPath, accountCode string
	var maxScanPages int

	cmd := &cobra.Command{
		Use:   "release-audit",
		Short: "Review who released or rejected held messages over a period, flagging risky releases and missing tickets",
		Long: "Use this command to review who released or rejected held messages over a period and which releases were risky. " +
			"Do NOT use it to explain one message's history; use 'why' instead. For the raw log rows use 'held release-logs'.\n\n" +
			"A release is flagged risky when its hold reason or policy falls in the never-release class, when Mimecast's spam verdict risk is high, " +
			"or when SPF, DKIM or DMARC did not pass. Risky releases are matched (sender + subject, within 15 minutes) against the local ledger that " +
			"'held release-safe' writes; risky releases with no ledger ticket are flagged no_ticket. Run from the CLI, fetched rows are also cached locally for 'senders suggest'; under the MCP server or with MIMECAST_NO_LOCAL_DATA=1 nothing is written.",
		Example:     "  mimecast-cli held release-audit --since 7d --agent\n  mimecast-cli held release-audit --since 30d --operator admin@example.com",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "held release-audit")
			}
			now := time.Now()
			start, err := parseSince(since, now)
			if err != nil {
				return usageErr(err)
			}
			if cliutil.IsDogfoodEnv() && maxScanPages > 1 {
				maxScanPages = 1
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			var search map[string]any
			if operator != "" {
				search = map[string]any{"fieldName": "operator", "value": operator}
			}
			c, err := flags.newClient()
			if err != nil {
				return classifyAPIError(cmd.ErrOrStderr(), err, flags)
			}
			logs, page, err := fetchReleaseLogs(ctx, c, start, now, mcAccountCode(accountCode), maxScanPages, search)
			if err != nil {
				return classifyAPIError(cmd.ErrOrStderr(), err, flags)
			}
			// Hand-fix (handfixes.json mimecast-read-only-workflows-write-nothing): a read-only tool writes nothing under MCP.
			db, writable, err := openReadOnlyWorkflowStore(ctx, flags, dbPath, mcAccountCode(accountCode))
			if err != nil {
				return err
			}
			if db != nil {
				defer db.Close()
			}
			if writable {
				if err := cacheReleaseLogs(ctx, db, logs); err != nil {
					return fmt.Errorf("caching release logs: %w", err)
				}
			}
			ledger, err := readLedger(ctx, db, start.Add(-24*time.Hour))
			if err != nil {
				return fmt.Errorf("reading release ledger: %w", err)
			}
			view := buildReleaseAudit(logs, ledger, operator)
			view.Since = mcTime(start)
			view.ScannedLogs = len(logs)
			view.MaxScanPages = maxScanPages
			view.Truncated = page.Truncated
			if len(logs) == 0 {
				view.Note = "no release or reject log entries in the window"
			}
			if n := page.note("raise --max-scan-pages for the full period"); n != "" {
				view.Note = strings.TrimSpace(view.Note + " " + n)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printNovelJSON(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Release log since %s: %d entries\n\n", view.Since, view.ScannedLogs)
			for _, o := range view.Operators {
				fmt.Fprintf(w, "%-40s released %3d  rejected %3d  risky %3d\n", o.Operator, o.Released, o.Rejected, o.Risky)
			}
			if len(view.Risky) > 0 {
				fmt.Fprintln(w, "\nRisky releases:")
				for _, r := range view.Risky {
					t := r.Ticket
					if r.NoTicket {
						t = "NO TICKET"
					}
					fmt.Fprintf(w, "  %s  %s  %s  %q  [%s] %s\n", r.Released, r.Operator, r.Sender, r.Subject, strings.Join(r.Flags, ","), t)
				}
			}
			if view.Note != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "\nnote: %s\n", view.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "7d", "Lookback window (7d, 30d) or an RFC3339 start time")
	cmd.Flags().StringVar(&operator, "operator", "", "Only include actions by this operator email address")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 10, "Maximum release-log pages (100 entries each) to scan")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite path for the ledger and release-log cache (default: the CLI data dir)")
	cmd.Flags().StringVar(&accountCode, "account-code", "", "Delegated customer account code (MSP); defaults to MIMECAST_ACCOUNT_CODE")
	return cmd
}

// buildReleaseAudit groups logs by operator and flags risky releases.
func buildReleaseAudit(logs []releaseLog, ledger []ledgerRow, operator string) releaseAuditView {
	byOp := map[string]*operatorStats{}
	tickets := attributeTickets(logs, ledger)
	view := releaseAuditView{Operators: make([]operatorStats, 0), Risky: make([]riskyRelease, 0)}
	for i, l := range logs {
		op := cmp.Or(strings.ToLower(l.Operator.EmailAddress), "(system)") // housekeeping expiries have no operator
		if operator != "" && !strings.EqualFold(op, operator) {
			continue
		}
		s := byOp[op]
		if s == nil {
			s = &operatorStats{Operator: op, Reasons: map[string]int{}}
			byOp[op] = s
		}
		s.Reasons[cmp.Or(l.HeldReason, "(unknown)")]++
		if !strings.EqualFold(l.Status, "released") {
			s.Rejected++
			continue
		}
		s.Released++
		flags := releaseRiskFlags(l)
		if len(flags) == 0 {
			continue
		}
		s.Risky++
		r := riskyRelease{Released: l.Released, Operator: op, Sender: l.sender(), Subject: l.Subject, HoldReason: l.HeldReason, Policy: l.Policy, Flags: flags}
		r.Ticket = tickets[i]
		r.NoTicket = r.Ticket == ""
		view.Risky = append(view.Risky, r)
	}
	for _, s := range byOp {
		view.Operators = append(view.Operators, *s)
	}
	sort.Slice(view.Operators, func(i, j int) bool {
		if view.Operators[i].Risky != view.Operators[j].Risky {
			return view.Operators[i].Risky > view.Operators[j].Risky
		}
		return view.Operators[i].Released > view.Operators[j].Released
	})
	return view
}

// releaseRiskFlags explains why a released message deserved scrutiny.
func releaseRiskFlags(l releaseLog) []string {
	var out []string
	if class, basis := classifyHold(l.HeldReason, l.Policy, "", l.spamScore()); class == riskNeverRelease {
		out = append(out, "never-release hold ("+basis+")")
	}
	if d := l.SpamProcessingDetail; d != nil {
		if d.Verdict != nil && strings.Contains(strings.ToLower(d.Verdict.Risk), "high") {
			out = append(out, "spam verdict risk "+d.Verdict.Risk)
		}
		for _, c := range d.failedChecks() {
			out = append(out, c+" failed")
		}
	}
	return out
}

// ledgerCandidates lists the release-safe ledger rows that could record a
// release-log row: same sender and subject, released within 15 minutes
// (release-log IDs differ from hold IDs, so there is no exact key).
func ledgerCandidates(l releaseLog, ledger []ledgerRow) []int {
	released, ok := parseMCTime(l.Released)
	if !ok {
		return nil
	}
	var out []int
	for i, r := range ledger {
		if r.Action != "release" || r.Outcome != "released" || r.Ticket == "" {
			continue
		}
		if !strings.EqualFold(r.Sender, l.sender()) || !strings.EqualFold(strings.TrimSpace(r.Subject), strings.TrimSpace(l.Subject)) {
			continue
		}
		if at, err := time.Parse(time.RFC3339, r.At); err == nil && released.Sub(at).Abs() <= 15*time.Minute {
			out = append(out, i)
		}
	}
	return out
}

// attributeTickets gives each released log row a ledger ticket only when the
// match is one-to-one across all releases (risky or not): a release with two
// candidate rows, or a row claimed by two releases, stays unattributed.
func attributeTickets(logs []releaseLog, ledger []ledgerRow) map[int]string {
	cands := map[int][]int{}
	claims := map[int]int{}
	for i, l := range logs {
		if !strings.EqualFold(l.Status, "released") {
			continue
		}
		c := ledgerCandidates(l, ledger)
		cands[i] = c
		for _, r := range c {
			claims[r]++
		}
	}
	out := map[int]string{}
	for i, c := range cands {
		if len(c) == 1 && claims[c[0]] == 1 {
			out[i] = ledger[c[0]].Ticket
		}
	}
	return out
}
