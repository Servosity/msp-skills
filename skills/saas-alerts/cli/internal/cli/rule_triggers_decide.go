// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"saas-alerts-pp-cli/internal/client"
	"saas-alerts-pp-cli/internal/cliutil"
	"saas-alerts-pp-cli/internal/store"
)

// rtDecisions maps --decision values to the Respond decision endpoints. Only
// approve takes no request body; the others take {"remarks": string|null}.
var rtDecisions = map[string]bool{
	"approve":            true,
	"reject":             true,
	"ignore":             true,
	"manually-remediate": true,
}

const (
	rtSkipNotFound       = "not_found"
	rtSkipLookupFailed   = "lookup_failed"
	rtSkipSafetyUnknown  = "action_safety_unknown"
	rtSkipStatusMismatch = "status_mismatch"
	rtSkipExpiredSuper   = "expired_superseded"
	rtSkipExpired12h     = "expired_12h"
	rtSkipDestructive    = "destructive_action_requires_allow_destructive"
)

type rtDecidePlanned struct {
	TriggerID    string      `json:"trigger_id"`
	Method       string      `json:"method"`
	Path         string      `json:"path"`
	Body         any         `json:"body"`
	StatusBefore string      `json:"status_before"`
	WouldFire    []string    `json:"would_fire"`
	Trigger      rqQueueItem `json:"trigger"`
}

type rtDecideSkipped struct {
	TriggerID string `json:"trigger_id"`
	Reason    string `json:"reason"`
	Detail    string `json:"detail"`
}

type rtDecideOutcome struct {
	TriggerID string `json:"trigger_id"`
	OK        bool   `json:"ok"`
	// Outcome is accepted (the POST answered with the documented RuleTrigger),
	// unknown_outcome (it answered 2xx with something else; never retried) or
	// error (the POST failed).
	Outcome      string `json:"outcome"`
	HTTPStatus   int    `json:"http_status"`
	StatusBefore string `json:"status_before"`
	StatusAfter  string `json:"status_after"`
	Verified     bool   `json:"verified"`
	Verification string `json:"verification"`
	ReceiptID    int64  `json:"receipt_id"`
	Error        string `json:"error"`
}

type rtDecideSummary struct {
	Planned    int `json:"planned"`
	Skipped    int `json:"skipped"`
	Succeeded  int `json:"succeeded"`
	Failed     int `json:"failed"`
	Verified   int `json:"verified"`
	Unverified int `json:"unverified"`
	// LookupErrors counts --ids whose direct read failed with anything but a 404.
	LookupErrors int `json:"lookup_errors"`
	// NotFound counts --ids that do not exist (HTTP 404 on the exact id).
	NotFound int `json:"not_found"`
	// Undecoded counts fetched rule-trigger rows that could not be decoded.
	Undecoded int `json:"undecoded"`
	// ActionSafetyUnknown counts approvals skipped because the actions the
	// rule would fire could not be established.
	ActionSafetyUnknown int `json:"action_safety_unknown"`
	// UnknownOutcomes counts decisions whose POST answer was not the
	// documented RuleTrigger and whose re-read did not show the decision.
	UnknownOutcomes int `json:"unknown_outcomes"`
}

type rtDecideResult struct {
	Mode         string            `json:"mode"`
	Decision     string            `json:"decision"`
	Source       string            `json:"source"`
	Fetched      int               `json:"fetched"`
	Truncated    bool              `json:"truncated"`
	TruncatedWhy string            `json:"truncated_reason"`
	Planned      []rtDecidePlanned `json:"planned"`
	Skipped      []rtDecideSkipped `json:"skipped"`
	Results      []rtDecideOutcome `json:"results"`
	Summary      rtDecideSummary   `json:"summary"`
	ReceiptError string            `json:"receipt_error"`
	Note         string            `json:"note"`

	// SupersessionUnchecked says why an --ids plan could not rule out a newer
	// trigger superseding a requested one (the list was capped or had rows
	// that did not decode); empty when every fetched page was read.
	SupersessionUnchecked string `json:"supersession_unchecked"`

	lookupErr error
}

type rtDecideOptions struct {
	decision         string
	ids              []string
	rules            []string
	orgs             []string
	accounts         []string
	statuses         []string
	remarks          *string
	allowDestructive bool
	includeExpired   bool
	maxPages         int
}

func newNovelRuleTriggersDecideCmd(flags *rootFlags) *cobra.Command {
	var flagDecision string
	var flagIDs string
	var flagRule string
	var flagOrg string
	var flagAccount string
	var flagStatus string
	var flagRemarks string
	var flagConfirm bool
	var flagAllowDestructive bool
	var flagIncludeExpired bool
	var flagMaxPages int
	var flagDB string

	cmd := &cobra.Command{
		Use:   "decide",
		Short: "Approve, reject, ignore, or manually remediate a filtered set of Respond triggers, previewed first and verified after.",
		Long: `Use this command to apply a decision to one or more Respond triggers. Do NOT use this command to list or prioritize triggers; use 'rule-triggers queue' instead. Do NOT use it for an ad-hoc action on an account with no trigger; use 'contain' instead.

The default run, without --confirm, is a preview: it fetches the selected triggers live and lists each call it would make, skipping expired or superseded PENDING triggers and, for approve, triggers whose rule fires Reset User Password or Delete User. --confirm sends the calls, re-reads each trigger to verify the decision took effect, and records a receipt in the local store. --dry-run only prints a stub and does not fetch or plan anything.`,
		Example: strings.Trim(`
  saas-alerts-cli rule-triggers decide --decision reject --rule RULE_ID_12345 --remarks "false positive" --agent
  saas-alerts-cli rule-triggers decide --decision approve --ids RULE_TRIGGER_ID_12345 --json
  saas-alerts-cli rule-triggers decide --decision manually-remediate --status REMEDIATION_FAILED --account jane.doe@example.com --remarks "reset by hand" --confirm`, "\n"),
		Annotations: map[string]string{
			"pp:data-source":             "live",
			"pp:preview-happy-path":      "true",
			"pp:happy-args":              "--decision=reject;--rule=RULE_ID_12345;--remarks=false-positive",
			"mcp:read-only-unless-flags": "confirm",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "rule-triggers decide")
			}
			decision := strings.ToLower(strings.TrimSpace(flagDecision))
			if decision == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--decision is required: approve, reject, ignore or manually-remediate"))
			}
			if !rtDecisions[decision] {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--decision %q is not one of approve, reject, ignore, manually-remediate", flagDecision))
			}
			opts := rtDecideOptions{
				decision:         decision,
				ids:              rqSplitCSV(flagIDs),
				rules:            rqSplitCSV(flagRule),
				orgs:             rqSplitCSV(flagOrg),
				accounts:         rqSplitCSV(flagAccount),
				allowDestructive: flagAllowDestructive,
				includeExpired:   flagIncludeExpired,
				maxPages:         flagMaxPages,
			}
			if len(opts.ids)+len(opts.rules)+len(opts.orgs)+len(opts.accounts) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("select triggers with at least one of --ids, --rule, --org or --account; deciding the whole queue at once is not allowed"))
			}
			statuses, err := rqParseStatuses(flagStatus, "status")
			if err != nil {
				_ = cmd.Usage()
				return usageErr(err)
			}
			opts.statuses = statuses
			if flagMaxPages < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--max-pages must be at least 1"))
			}
			if cmd.Flags().Changed("remarks") {
				if decision == "approve" {
					fmt.Fprintln(cmd.ErrOrStderr(), "warning: --remarks is ignored for approve (the approve endpoint takes no body)")
				} else {
					r := flagRemarks
					opts.remarks = &r
				}
			}
			if flagConfirm && cliutil.IsAnyHarness() {
				return writeHarnessRefusal(cmd.OutOrStdout(), flags, "rule-triggers decide --confirm")
			}

			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			result, err := rtPlanDecisions(ctx, c, opts)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			if result.Summary.Undecoded > 0 && len(opts.ids) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d fetched rule trigger row(s) could not be decoded; triggers matching the selection may be missing from this plan\n", result.Summary.Undecoded)
			}
			if result.SupersessionUnchecked != "" { // saas-alerts hand-fix: decide-ids-never-truncated
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s, so supersession was checked only against the triggers read; a newer trigger of the same rule beyond them could not mark a requested one superseded\n", result.SupersessionUnchecked)
			}
			if result.Summary.ActionSafetyUnknown > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d trigger(s) skipped: the actions approving would fire are unknown (action_safety_unknown)\n", result.Summary.ActionSafetyUnknown)
			}
			if result.Truncated { // saas-alerts hand-fix: a page-capped plan never reads as the whole selection
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: the trigger list is incomplete (%s); matching triggers beyond it were not planned or decided; rerun with a higher --max-pages or a narrower selection\n", result.TruncatedWhy)
			}
			var firstErr error
			if flagConfirm {
				dbPath := flagDB
				if dbPath == "" {
					dbPath = defaultDBPath("saas-alerts-cli")
				}
				firstErr = rtExecuteDecisions(ctx, c, &result, opts, dbPath, cmd)
			} else {
				result.Note = "preview only: nothing was changed; rerun with --confirm to apply"
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), result, flags); err != nil {
					return err
				}
			} else if err := rtPrintDecideHuman(cmd, result); err != nil {
				return err
			}
			if result.Summary.Failed > 0 {
				if result.Summary.Succeeded == 0 && firstErr != nil && result.lookupErr == nil {
					return classifyAPIErrorOnly(firstErr)
				}
				return partialFailureErr(fmt.Errorf("%d of %d decisions failed, %d trigger lookup(s) failed", result.Summary.Failed, result.Summary.Planned, result.Summary.LookupErrors))
			}
			if result.lookupErr != nil {
				if result.Summary.Succeeded > 0 {
					return partialFailureErr(fmt.Errorf("%d trigger lookup(s) failed: %v", result.Summary.LookupErrors, result.lookupErr))
				}
				return classifyAPIErrorOnly(result.lookupErr)
			}
			if result.Summary.UnknownOutcomes > 0 {
				unknownErr := fmt.Errorf("%d decision(s) have an unknown outcome: the answer was not the documented rule trigger and the re-read does not show the decision (not retried)", result.Summary.UnknownOutcomes)
				if result.Summary.Succeeded > 0 {
					return partialFailureErr(unknownErr)
				}
				return apiErr(unknownErr)
			}
			if result.Summary.ActionSafetyUnknown > 0 {
				safetyErr := fmt.Errorf("%d trigger(s) were not approved because the actions approving would fire could not be established", result.Summary.ActionSafetyUnknown)
				if result.Summary.Succeeded > 0 {
					return partialFailureErr(safetyErr)
				}
				return apiErr(safetyErr)
			}
			if result.Summary.Undecoded > 0 && len(opts.ids) == 0 { // an --ids selection reads each id it did not decode directly
				undecodedErr := fmt.Errorf("%d fetched rule trigger row(s) could not be decoded, so the selection may be incomplete", result.Summary.Undecoded)
				if result.Summary.Succeeded > 0 {
					return partialFailureErr(undecodedErr)
				}
				return apiErr(undecodedErr)
			}
			if result.Summary.NotFound > 0 {
				notFound := fmt.Errorf("%d requested trigger id(s) were not found (ids are matched exactly)", result.Summary.NotFound)
				if result.Summary.Succeeded > 0 {
					return partialFailureErr(notFound)
				}
				return notFoundErr(notFound)
			}
			if flagConfirm && result.Truncated { // saas-alerts hand-fix: a confirmed bulk decision that stopped at the page cap is partial
				return partialFailureErr(fmt.Errorf("decided %d trigger(s), but the trigger list is incomplete (%s); matching triggers beyond it were not decided", result.Summary.Succeeded, result.TruncatedWhy))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagDecision, "decision", "", "Decision to apply: approve, reject, ignore or manually-remediate (required)")
	cmd.Flags().StringVar(&flagIDs, "ids", "", "Comma-separated rule trigger IDs to decide")
	cmd.Flags().StringVar(&flagRule, "rule", "", "Comma-separated rule IDs whose triggers to decide")
	cmd.Flags().StringVar(&flagOrg, "org", "", "Comma-separated Respond organization IDs whose triggers to decide")
	cmd.Flags().StringVar(&flagAccount, "account", "", "Comma-separated account emails; selects triggers naming any of them")
	cmd.Flags().StringVar(&flagStatus, "status", "PENDING", "Trigger status(es) to act on (use REMEDIATION_FAILED for manually-remediate or ignore of failed remediations)")
	cmd.Flags().StringVar(&flagRemarks, "remarks", "", "Remarks sent with reject, ignore and manually-remediate (approve takes no body)")
	cmd.Flags().BoolVar(&flagConfirm, "confirm", false, "Apply the decisions; without it the command only previews")
	cmd.Flags().BoolVar(&flagAllowDestructive, "allow-destructive", false, "Allow approving triggers whose rule fires Reset User Password or Delete User")
	cmd.Flags().BoolVar(&flagIncludeExpired, "include-expired", false, "Also act on PENDING triggers past their 12h window or superseded by a newer trigger of the same rule for the same organization and accounts")
	cmd.Flags().IntVar(&flagMaxPages, "max-pages", rqDefaultMaxPages, "Maximum pages of live rule triggers to fetch (100 per page)")
	cmd.Flags().StringVar(&flagDB, "db", "", "Local store that receives decision receipts (default: the CLI's local store)")
	return cmd
}

// rtPlanDecisions fetches candidate triggers live and splits them into the
// calls that would be made and the triggers that are skipped (with reasons).
func rtPlanDecisions(ctx context.Context, c *client.Client, opts rtDecideOptions) (rtDecideResult, error) {
	result := rtDecideResult{
		Mode:     "preview",
		Decision: opts.decision,
		Source:   "live",
		Planned:  make([]rtDecidePlanned, 0),
		Skipped:  make([]rtDecideSkipped, 0),
		Results:  make([]rtDecideOutcome, 0),
	}
	filter := rqFilter{Statuses: opts.statuses, OrgIDs: opts.orgs, RuleIDs: opts.rules}
	live, err := rqFetchLive(ctx, c, filter, opts.maxPages)
	if err != nil {
		return result, err
	}
	result.Truncated = live.Truncated
	result.TruncatedWhy = live.TruncatedReason
	// Rows that did not decode might be triggers the selection should have
	// covered; they are counted so the run can fail instead of under-acting.
	result.Summary.Undecoded = live.Undecoded
	set := filter.apply(live.Triggers)

	// --ids outside the fetched pages: read each one directly so a trigger
	// past --max-pages is still found (and its status still checked).
	if len(opts.ids) > 0 {
		have := map[string]bool{}
		for _, t := range set {
			have[t.ID] = true
		}
		for _, id := range opts.ids {
			if have[id] {
				continue
			}
			t, err := rqFetchOne(ctx, c, id)
			if err != nil {
				var apiErr *client.APIError
				if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
					// Ids are exact: a case variant of an existing trigger is a
					// different id, and a requested id that does not exist fails the run.
					result.Skipped = append(result.Skipped, rtDecideSkipped{TriggerID: id, Reason: rtSkipNotFound, Detail: "no rule trigger with this exact id (HTTP 404)"})
					result.Summary.NotFound++
					continue
				}
				// Anything else is not proof the trigger is absent: report it
				// for this id, keep deciding the others, and fail the run.
				result.Skipped = append(result.Skipped, rtDecideSkipped{TriggerID: id, Reason: rtSkipLookupFailed, Detail: err.Error()})
				result.Summary.LookupErrors++
				if result.lookupErr == nil {
					result.lookupErr = err
				}
				continue
			}
			if !filter.match(t) {
				result.Skipped = append(result.Skipped, rtDecideSkipped{TriggerID: id, Reason: rtSkipStatusMismatch,
					Detail: fmt.Sprintf("status %s, rule %s, organization %s does not match the selection (--status %s)", t.Status, t.Rule.ID, t.Organization.ID, strings.Join(opts.statuses, ","))})
				continue
			}
			have[id] = true
			set = append(set, t)
		}
		// Every requested id was found on a page or read directly, so neither
		// the page cap nor an undecodable row can hide one: an --ids selection
		// is complete. Supersession is worked out from the fetched pages only,
		// so a cut list still says so.
		if live.Truncated {
			result.SupersessionUnchecked = live.TruncatedReason
		} else if live.Undecoded > 0 {
			result.SupersessionUnchecked = fmt.Sprintf("%d fetched rule trigger row(s) could not be decoded", live.Undecoded)
		}
		result.Truncated, result.TruncatedWhy = false, "" // saas-alerts hand-fix: decide-ids-never-truncated
	}
	result.Fetched = len(set)

	// approve fires the rule's response actions, so the destructive gate needs
	// the complete action list. Where a trigger's list is missing or partial,
	// look it up; if it stays unknown, that trigger is never approved.
	safetyUnknown := map[string]string{}
	if opts.decision == "approve" {
		for i := range set {
			if set[i].actionsKnown || !rtSelected(set[i], opts) {
				continue
			}
			if why := rtResolveActions(ctx, c, &set[i]); why != "" {
				safetyUnknown[set[i].ID] = why
			}
		}
	}

	// Deadlines are computed over the whole fetched set so a newer trigger of
	// the same rule supersedes an older one even when only the older is selected.
	annotated := rqAnnotate(set, rqNow())
	rqSortQueue(annotated)
	byID := map[string]rqTrigger{}
	for _, t := range set {
		byID[t.ID] = t
	}
	for _, item := range annotated {
		t := byID[item.TriggerID]
		if !rtSelected(t, opts) {
			if len(opts.ids) > 0 && rqContainsExact(opts.ids, t.ID) { // saas-alerts hand-fix: a requested id is never dropped silently
				emails := make([]string, 0, len(t.Accounts))
				for _, a := range t.Accounts {
					emails = append(emails, a.Email)
				}
				result.Skipped = append(result.Skipped, rtDecideSkipped{TriggerID: t.ID, Reason: rtSkipStatusMismatch,
					Detail: fmt.Sprintf("accounts %s do not match the selection (--account %s)", strings.Join(emails, ","), strings.Join(opts.accounts, ","))})
			}
			continue
		}
		if strings.EqualFold(t.Status, "PENDING") && item.Expired && !opts.includeExpired {
			reason, detail := rtSkipExpired12h, "the 12h manual-approval window has passed"
			if item.ExpiryReason == "superseded" {
				reason, detail = rtSkipExpiredSuper, "superseded by newer trigger "+item.SupersededBy+" of the same rule for the same organization and accounts"
			}
			if item.ExpiresAt != nil {
				detail += " (expired " + *item.ExpiresAt + ")"
			}
			result.Skipped = append(result.Skipped, rtDecideSkipped{TriggerID: t.ID, Reason: reason, Detail: detail + "; pass --include-expired to act anyway"})
			continue
		}
		if why, unknown := safetyUnknown[t.ID]; unknown {
			result.Skipped = append(result.Skipped, rtDecideSkipped{TriggerID: t.ID, Reason: rtSkipSafetyUnknown,
				Detail: "the actions approving would fire could not be established (" + why + "); not approved"})
			result.Summary.ActionSafetyUnknown++
			continue
		}
		if opts.decision == "approve" && !opts.allowDestructive {
			if destructive := t.destructiveActions(); len(destructive) > 0 {
				result.Skipped = append(result.Skipped, rtDecideSkipped{TriggerID: t.ID, Reason: rtSkipDestructive,
					Detail: "approving fires " + strings.Join(destructive, ", ") + "; pass --allow-destructive to approve it"})
				continue
			}
		}
		var body any
		if opts.decision != "approve" {
			var remarks any // JSON null when --remarks is not given
			if opts.remarks != nil {
				remarks = *opts.remarks
			}
			body = map[string]any{"remarks": remarks}
		}
		result.Planned = append(result.Planned, rtDecidePlanned{
			TriggerID:    t.ID,
			Method:       "POST",
			Path:         rqRuleTriggersPath + "/" + url.PathEscape(t.ID) + "/" + opts.decision,
			Body:         body,
			StatusBefore: t.Status,
			WouldFire:    item.WouldFire,
			Trigger:      item,
		})
	}
	result.Summary.Planned = len(result.Planned)
	result.Summary.Skipped = len(result.Skipped)
	return result, nil
}

// rtResolveActions completes a trigger's rule action list for approve: first
// from a direct GET /rule-triggers/{id}, then from GET /rules/{rule.id}. It
// returns why the actions are still unknown, or "" once they are known.
func rtResolveActions(ctx context.Context, c *client.Client, t *rqTrigger) string {
	ruleID := t.Rule.ID
	notes := make([]string, 0, 2)
	fresh, err := rqFetchOne(ctx, c, t.ID)
	switch {
	case err != nil:
		notes = append(notes, "re-reading the trigger failed: "+err.Error())
	case fresh.actionsKnown:
		t.Rule.ResponseActions = fresh.Rule.ResponseActions
		if t.Rule.ID == "" {
			t.Rule.ID = fresh.Rule.ID
		}
		t.actionsKnown = true
		return ""
	default:
		notes = append(notes, "the trigger's rule has no complete responseActions list")
		if ruleID == "" {
			ruleID = fresh.Rule.ID
		}
	}
	if ruleID == "" {
		return strings.Join(append(notes, "no rule id to look the actions up by"), "; ")
	}
	actions, err := rqFetchRule(ctx, c, ruleID)
	if err != nil {
		return strings.Join(append(notes, "rule "+ruleID+": "+err.Error()), "; ")
	}
	// INFERRED: the rule's current actions stand in for the ones it carried
	// when it triggered (the trigger's own copy was incomplete).
	t.Rule.ID = ruleID
	t.Rule.ResponseActions = actions
	t.actionsKnown = true
	return ""
}

// rtSelected applies the selectors the API cannot filter on (--ids, --account);
// --rule/--org/--status were already applied by rqFilter.
func rtSelected(t rqTrigger, opts rtDecideOptions) bool {
	if len(opts.ids) > 0 && !rqContainsExact(opts.ids, t.ID) {
		return false
	}
	if len(opts.accounts) > 0 {
		hit := false
		for _, a := range t.Accounts {
			if rqContainsFold(opts.accounts, a.Email) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return true
}

const rtReceiptsDDL = `CREATE TABLE IF NOT EXISTS decision_receipts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	trigger_id TEXT NOT NULL,
	decision TEXT NOT NULL,
	remarks TEXT,
	decided_at TEXT NOT NULL,
	status_before TEXT,
	status_after TEXT,
	verified INTEGER NOT NULL DEFAULT 0,
	error TEXT
)`

// rtExecuteDecisions POSTs each planned decision, re-reads the trigger to
// verify the status moved, and appends a receipt row per attempt. It keeps
// going past failures and returns the first POST error.
func rtExecuteDecisions(ctx context.Context, c *client.Client, result *rtDecideResult, opts rtDecideOptions, dbPath string, cmd *cobra.Command) error {
	result.Mode = "executed"
	var db *store.Store
	if s, err := store.OpenWithContext(ctx, dbPath); err != nil {
		result.ReceiptError = fmt.Sprintf("opening %s: %v", dbPath, err)
	} else if _, err := s.DB().ExecContext(ctx, rtReceiptsDDL); err != nil {
		result.ReceiptError = fmt.Sprintf("creating decision_receipts: %v", err)
		_ = s.Close()
	} else {
		db = s
		defer db.Close()
	}
	if result.ReceiptError != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: decision receipts will not be recorded: %s\n", result.ReceiptError)
	}

	var firstErr error
	for _, p := range result.Planned {
		out := rtDecideOutcome{TriggerID: p.TriggerID, StatusBefore: p.StatusBefore}
		calledAt := rqNow()
		data, code, err := c.Post(ctx, p.Path, p.Body)
		out.HTTPStatus = code
		if err != nil {
			var apiErr *client.APIError
			if errors.As(err, &apiErr) {
				out.HTTPStatus = apiErr.StatusCode
			}
			out.Outcome = "error"
			out.Error = err.Error()
			if firstErr == nil {
				firstErr = err
			}
			result.Summary.Failed++
		} else {
			why := rtCheckDecisionResponse(data, p.TriggerID, opts.decision)
			// The verification read runs either way: it is the only evidence
			// left when the POST answer is not the documented RuleTrigger.
			after, verr := rqFetchOne(ctx, c, p.TriggerID)
			if verr != nil {
				out.Verification = "read_failed"
			} else {
				out.StatusAfter = after.Status
				out.Verified, out.Verification = rtVerifyDecision(opts.decision, after, calledAt)
			}
			switch {
			case why == "":
				out.Outcome, out.OK = "accepted", true
				result.Summary.Succeeded++
				if verr != nil {
					out.Error = "decision accepted but verification read failed: " + verr.Error()
				}
			case out.Verified:
				// The answer was off-contract but the re-read shows the decision.
				out.Outcome, out.OK = "unknown_outcome", true
				result.Summary.Succeeded++
				out.Verification += "; the POST answer was " + why + ", but the re-read shows the decision"
			default:
				out.Outcome = "unknown_outcome"
				result.Summary.UnknownOutcomes++
				out.Error = "the POST answered " + why + " and the re-read does not show the decision; it was not retried"
				if verr != nil {
					out.Error += " (verification read failed: " + verr.Error() + ")"
				}
				if firstErr == nil {
					firstErr = errors.New(out.Error)
				}
			}
			if out.Verified {
				result.Summary.Verified++
			} else {
				result.Summary.Unverified++
			}
		}
		if db != nil {
			id, rerr := rtWriteReceipt(ctx, db, out, opts)
			if rerr != nil && result.ReceiptError == "" {
				result.ReceiptError = rerr.Error()
			}
			out.ReceiptID = id
		}
		result.Results = append(result.Results, out)
	}
	return firstErr
}

// rtCheckDecisionResponse checks a decision POST answer against the spec:
// the updated RuleTrigger for the decided id that passes the shared failure
// rule (applied by rqParseTriggerBody to the body and any wrapped trigger)
// and whose status is an outcome of this decision (the same sets
// verification uses). It returns a value-free description of what was
// missing or wrong, or "" when the answer itself evidences the decision.
func rtCheckDecisionResponse(data json.RawMessage, id, decision string) string {
	t, why := rqParseTriggerBody(data, id)
	if why != "" {
		return "not the documented rule trigger (" + why + "; got " + rqDescribeJSON(data) + ")"
	}
	want := rtDecisionOutcomes[decision]
	if !rqContainsFold(want.statuses, t.Status) {
		return "a rule trigger whose status is not an outcome of " + decision + " (expected " + strings.Join(want.statuses, " or ") + ")"
	}
	return ""
}

// rtDecisionOutcomes lists, per decision, the statuses that show it took
// effect and the history entry type it writes. An approve moves on to a
// remediation state once the rule's actions run.
var rtDecisionOutcomes = map[string]struct {
	statuses []string
	history  string
}{
	"approve":            {statuses: []string{"APPROVED", "REMEDIATION_SUCCESSFUL", "REMEDIATION_FAILED", "REMEDIATION_PARTIAL"}, history: "APPROVED"},
	"reject":             {statuses: []string{"REJECTED"}, history: "REJECTED"},
	"ignore":             {statuses: []string{"IGNORED"}, history: "IGNORED"},
	"manually-remediate": {statuses: []string{"MANUALLY_REMEDIATED"}, history: "MANUALLY_REMEDIATED"},
}

// rtHistorySkew tolerates server/client clock skew when matching a history
// entry to the decision call (INFERRED tolerance).
const rtHistorySkew = 5 * time.Second

// rtVerifyDecision reports whether the re-read trigger shows this decision:
// a status the decision leads to, or (when the status lags) a history entry
// of the decision's type stamped at or after the call. Any other state, such
// as a reject that reads back AUTO_IGNORED, is not verified.
func rtVerifyDecision(decision string, after rqTrigger, calledAt time.Time) (bool, string) {
	want, ok := rtDecisionOutcomes[decision]
	if !ok {
		return false, "unknown decision"
	}
	if rqContainsFold(want.statuses, after.Status) {
		return true, "status " + strings.ToUpper(after.Status)
	}
	floor := calledAt.Add(-rtHistorySkew)
	for _, h := range after.History {
		if strings.EqualFold(h.Type, want.history) && h.Timestamp > 0 && !h.Timestamp.time().Before(floor) {
			return true, "history " + want.history + " (status still " + after.Status + ")"
		}
	}
	return false, fmt.Sprintf("status %s is not an outcome of %s (expected %s)", after.Status, decision, strings.Join(want.statuses, " or "))
}

func rtWriteReceipt(ctx context.Context, db *store.Store, out rtDecideOutcome, opts rtDecideOptions) (int64, error) {
	var remarks any
	if opts.remarks != nil {
		remarks = *opts.remarks
	}
	verified := 0
	if out.Verified {
		verified = 1
	}
	var errText any
	if out.Error != "" {
		errText = out.Error
	}
	res, err := db.DB().ExecContext(ctx,
		`INSERT INTO decision_receipts (trigger_id, decision, remarks, decided_at, status_before, status_after, verified, error) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		out.TriggerID, opts.decision, remarks, rqFormatTime(rqNow()), out.StatusBefore, out.StatusAfter, verified, errText)
	if err != nil {
		return 0, fmt.Errorf("writing decision receipt: %w", err)
	}
	return res.LastInsertId()
}

func rtPrintDecideHuman(cmd *cobra.Command, r rtDecideResult) error {
	w := saasHumanWriter(cmd.OutOrStdout()) // saas-alerts hand-fix: tenant strings never drive the terminal
	fmt.Fprintf(w, "%s: %s on %d trigger(s), %d skipped\n", r.Mode, r.Decision, len(r.Planned), len(r.Skipped))
	if r.Truncated { // saas-alerts hand-fix
		fmt.Fprintf(w, "incomplete: %s; matching triggers beyond it were not included\n", r.TruncatedWhy)
	}
	if r.SupersessionUnchecked != "" {
		fmt.Fprintf(w, "supersession unchecked: %s\n", r.SupersessionUnchecked)
	}
	if len(r.Planned) > 0 && r.Mode == "preview" {
		tw := newTabWriter(w)
		fmt.Fprintln(tw, "TRIGGER\tSTATUS\tRULE\tACCOUNTS\tWOULD FIRE\tCALL")
		for _, p := range r.Planned {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s %s\n", p.TriggerID, p.StatusBefore, truncate(p.Trigger.RuleName, 32),
				truncate(strings.Join(p.Trigger.Accounts, ","), 32), strings.Join(p.WouldFire, ", "), p.Method, p.Path)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	if len(r.Results) > 0 {
		tw := newTabWriter(w)
		fmt.Fprintln(tw, "TRIGGER\tOK\tBEFORE\tAFTER\tVERIFIED\tERROR")
		for _, o := range r.Results {
			fmt.Fprintf(tw, "%s\t%t\t%s\t%s\t%t\t%s\n", o.TriggerID, o.OK, o.StatusBefore, o.StatusAfter, o.Verified, truncate(o.Error, 60))
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	for _, s := range r.Skipped {
		fmt.Fprintf(w, "skipped %s: %s (%s)\n", s.TriggerID, s.Reason, s.Detail)
	}
	if r.Note != "" {
		fmt.Fprintln(w, r.Note)
	}
	return nil
}
