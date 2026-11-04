// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: guarded release with ticket.
// pp:data-source live
// pp:client-call

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"mimecast-pp-cli/internal/client"
	"mimecast-pp-cli/internal/cliutil"
)

type releaseOutcome struct {
	ID         string `json:"id"`
	Subject    string `json:"subject,omitempty"`
	Sender     string `json:"sender,omitempty"`
	HoldReason string `json:"hold_reason,omitempty"`
	Policy     string `json:"policy,omitempty"`
	RiskClass  string `json:"risk_class,omitempty"`
	RiskBasis  string `json:"risk_basis,omitempty"`
	Outcome    string `json:"outcome"` // released | refused | failed | planned
	Error      string `json:"error,omitempty"`
	Override   string `json:"override,omitempty"`
}

type releaseSafeView struct {
	Ticket   string           `json:"ticket,omitempty"`
	Plan     bool             `json:"plan"`
	Released int              `json:"released"`
	Refused  int              `json:"refused"`
	Failed   int              `json:"failed"`
	Results  []releaseOutcome `json:"results"`
}

func newNovelHeldReleaseSafeCmd(flags *rootFlags) *cobra.Command {
	var ticket, override, since, dbPath, accountCode string
	var plan bool
	var maxScanPages int

	cmd := &cobra.Command{
		Use:   "release-safe [id...]",
		Short: "Release held messages only after a hold-reason safety check, recording the ticket",
		Long: "Use this command to release specific held messages after a hold-reason safety check, recording a ticket reference. " +
			"Do NOT use it to review the queue; use 'held triage' first. For an unchecked raw release from automation that applies its own policy, use 'held release'.\n\n" +
			"Each ID is looked up in the hold queue first and classified by a keyword heuristic over the hold reason and policy name (not verified against " +
			"every Mimecast hold reason). never-release holds (impersonation, malware, sandbox, phishing, URL/attachment protect), policy-review holds " +
			"(content, DLP, attachment, size, recipient policies) and IDs not found in the scanned queue are refused unless --override gives a reason. " +
			"Allowed IDs are released in batches of 25; an all-failed batch is retried once. Every decision, including refusals, is written to the local " +
			"release ledger that 'held release-audit' reads. Exit 1 when any ID was refused, 5 when any release failed. MX-based (Cloud Gateway) deployments only.",
		Example:     "  mimecast-cli held release-safe eNo1jr0OgjAYRd-lExample --ticket T-1042 --plan\n  mimecast-cli held release-safe eNo1jr0OgjAYRd-lExample eNo2kq1PhkBZSe-mExample --ticket T-1042\n  mimecast-cli held release-safe eNo1jr0OgjAYRd-lExample --ticket T-1043 --override \"CFO confirmed by phone\"",
		Annotations: map[string]string{"pp:data-source": "live", "pp:typed-exit-codes": "0,1,2,4,5,7", "pp:happy-args": "id=example-value;--ticket=T-1042;--plan", "mimecast:preview-flag": "plan"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "held release-safe")
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("at least one held message ID is required (find IDs with 'held triage' or 'held find')"))
			}
			now := time.Now()
			start, err := parseSince(since, now)
			if err != nil {
				return usageErr(err)
			}
			if cliutil.IsDogfoodEnv() && maxScanPages > 2 {
				maxScanPages = 2
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			acct := mcAccountCode(accountCode)
			c, err := flags.newClient()
			if err != nil {
				return classifyAPIError(cmd.ErrOrStderr(), err, flags)
			}
			want := dedupe(args)
			held, page, err := fetchHeldUntil(ctx, c, start, now, acct, maxScanPages, func(found []heldMessage) bool {
				have := make(map[string]bool, len(found))
				for _, h := range found {
					have[h.ID] = true
				}
				for _, id := range want {
					if !have[id] {
						return false
					}
				}
				return true // every requested ID is in hand; stop paging
			})
			if err != nil {
				return classifyAPIError(cmd.ErrOrStderr(), err, flags)
			}
			byID := make(map[string]heldMessage, len(held))
			for _, h := range held {
				byID[h.ID] = h
			}

			view := releaseSafeView{Ticket: ticket, Plan: plan, Results: make([]releaseOutcome, 0, len(args))}
			var allowed, refused []string
			index := map[string]int{}
			for _, id := range dedupe(args) {
				index[id] = len(view.Results)
				out := releaseOutcome{ID: id}
				h, found := byID[id]
				if found {
					out.Subject, out.Sender, out.HoldReason, out.Policy = h.Subject, h.sender(), h.HeldReason, h.PolicyInfo
					out.RiskClass, out.RiskBasis = h.risk()
				} else {
					out.RiskClass = "unknown"
					out.RiskBasis = fmt.Sprintf("not found in the hold queue since %s (%d pages scanned)", mcTime(start), page.Pages)
				}
				// never-release and policy-review holds (DLP, content, attachment
				// policies) need a stated reason; so do IDs we could not verify.
				blocked := !found || out.RiskClass == riskNeverRelease || out.RiskClass == riskPolicyReview
				switch {
				case blocked && strings.TrimSpace(override) == "":
					out.Outcome = "refused"
					refused = append(refused, id)
					if found {
						out.Error = "hold reason is " + out.RiskClass + " (" + out.RiskBasis + "); review it, then pass --override \"<reason>\" to release"
					} else {
						out.Error = out.RiskBasis + "; widen --since/--max-scan-pages or pass --override \"<reason>\""
					}
				default:
					if blocked {
						out.Override = override
					}
					out.Outcome = "planned"
					allowed = append(allowed, id)
				}
				view.Results = append(view.Results, out)
			}

			var ledgerErr, firstCause error
			if !plan {
				// Open the ledger before releasing anything so an unwritable
				// store stops the run instead of leaving releases unrecorded.
				db, err := openAccountStore(ctx, flags, dbPath, acct)
				if err != nil {
					return err
				}
				defer db.Close()
				// Ledger writes use their own short deadline so an API --timeout
				// that expires mid-run cannot also drop the record of what ran.
				record := func(ids []string) error {
					lctx, lcancel := context.WithTimeout(context.WithoutCancel(cmd.Context()), 15*time.Second)
					defer lcancel()
					rows := make([]ledgerRow, 0, len(ids))
					at := time.Now().UTC().Format(time.RFC3339)
					for _, id := range ids {
						r := view.Results[index[id]]
						rows = append(rows, ledgerRow{At: at, HeldID: r.ID, Ticket: ticket, Action: "release", Sender: r.Sender,
							Subject: r.Subject, HoldReason: r.HoldReason, RiskClass: r.RiskClass, Override: r.Override, Outcome: r.Outcome, Error: r.Error})
					}
					return writeLedger(lctx, db, rows)
				}
				// Refusals are decisions too: record them before releasing anything.
				ledgerErr = record(refused)
				if ledgerErr == nil && len(allowed) > 0 {
					for batch := range slices.Chunk(allowed, 25) {
						for id, res := range releaseBatch(ctx, c, batch, acct, true) {
							i, ok := index[id]
							if !ok {
								continue // the API echoed an ID we did not ask to release
							}
							view.Results[i].Outcome, view.Results[i].Error = res.outcome, res.err
							if res.cause != nil && firstCause == nil {
								firstCause = res.cause
							}
						}
						// Record each batch as it finishes; stop releasing if the
						// ledger can no longer record what happened.
						if ledgerErr = record(batch); ledgerErr != nil {
							break
						}
					}
				}
			}
			for _, r := range view.Results {
				switch r.Outcome {
				case "released":
					view.Released++
				case "refused":
					view.Refused++
				case "failed":
					view.Failed++
				}
			}
			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				for _, r := range view.Results {
					fmt.Fprintf(cmd.OutOrStdout(), "%-9s %s  %s  [%s] %s\n", r.Outcome, r.ID, r.Subject, r.RiskClass, r.Error)
				}
			} else if err := printNovelJSON(cmd.OutOrStdout(), view, flags); err != nil {
				return err
			}
			if ledgerErr != nil {
				return fmt.Errorf("writing the release ledger failed, so no further batches were released (results above show what ran): %w", ledgerErr)
			}
			summary := fmt.Errorf("%d released, %d refused, %d failed", view.Released, view.Refused, view.Failed)
			switch {
			case plan:
				return nil
			case view.Failed > 0:
				return mcWriteOutcomeErr(summary, firstCause)
			case view.Refused > 0:
				// A safety refusal is a policy outcome, not an API error.
				return &cliError{code: 1, err: summary}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&ticket, "ticket", "", "Ticket or case reference recorded in the local release ledger")
	cmd.Flags().StringVar(&override, "override", "", "Reason for releasing a never-release or unverified hold anyway (recorded in the ledger)")
	cmd.Flags().BoolVar(&plan, "plan", false, "Look up and classify the IDs but do not release anything")
	cmd.Flags().StringVar(&since, "since", "30d", "How far back to scan the hold queue for the IDs")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 20, "Maximum hold-queue pages (100 messages each) to scan for the IDs")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite path for the release ledger (default: the CLI data dir)")
	cmd.Flags().StringVar(&accountCode, "account-code", "", "Delegated customer account code (MSP); defaults to MIMECAST_ACCOUNT_CODE")
	return cmd
}

type batchResult struct {
	outcome string
	err     string
	cause   error // the transport/API error, kept so the exit code can be classified
}

// releaseBatch calls hold-release for up to 25 IDs and maps per-ID results.
// Failures marked retryable are retried once when retry is true.
func releaseBatch(ctx context.Context, c *client.Client, ids []string, accountCode string, retry bool) map[string]batchResult {
	out := make(map[string]batchResult, len(ids))
	items := make([]any, 0, len(ids))
	for _, id := range ids {
		items = append(items, map[string]any{"id": id})
	}
	resp, err := mcWrite(ctx, c, "/api/gateway/hold-release", mcBody(items, 0, "", accountCode))
	if err != nil {
		// The client turns an all-failed batch (HTTP 200 + fail[] only) into
		// an error, so per-item retryable flags are not visible here; a
		// held-message release is safe to repeat, so retry the batch once.
		if retry && strings.Contains(err.Error(), "mimecast function failure") {
			return releaseBatch(ctx, c, ids, accountCode, false)
		}
		for _, id := range ids {
			out[id] = batchResult{"failed", err.Error(), err}
		}
		return out
	}
	for _, raw := range resp.Data {
		var r struct {
			ID      string `json:"id"`
			Release bool   `json:"release"`
		}
		if decodeErr := json.Unmarshal(raw, &r); decodeErr == nil && r.ID != "" {
			if r.Release {
				out[r.ID] = batchResult{"released", "", nil}
			} else {
				out[r.ID] = batchResult{"failed", "Mimecast returned release=false", nil}
			}
		}
	}
	var retryIDs []string
	for _, f := range resp.Fail {
		for _, id := range f.keyIDs() {
			if retry && f.retryable() {
				retryIDs = append(retryIDs, id)
				continue
			}
			out[id] = batchResult{"failed", f.text(), nil}
		}
	}
	if len(retryIDs) > 0 {
		for id, res := range releaseBatch(ctx, c, retryIDs, accountCode, false) {
			out[id] = res
		}
	}
	for _, id := range ids {
		if _, ok := out[id]; !ok {
			out[id] = batchResult{"failed", "no result returned for this ID", nil}
		}
	}
	return out
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
