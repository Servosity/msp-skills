// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"saas-alerts-pp-cli/internal/client"
	"saas-alerts-pp-cli/internal/cliutil"
	"saas-alerts-pp-cli/internal/store"
)

const (
	containExecutePath = "/respondApi/api/v1/response-actions/execute"
	containJobsPath    = "/respondApi/api/v1/response-actions/jobs"
	// A job's createdAt is stamped by the same server as the execute
	// response's executedAt, but the two stamps may be taken in either order:
	// a job is a candidate when createdAt falls in
	// [executedAt - containJobMatchSkew, executedAt + containJobMatchWindow]
	// (INFERRED bounds; the execute response carries no job id).
	containJobMatchSkew   = 5 * time.Second
	containJobMatchWindow = 2 * time.Minute
)

// containPollInterval is the pause between job polls; tests shorten it.
var containPollInterval = 2 * time.Second

// containActionDef maps an --actions token to the exact Respond enum.
type containActionDef struct {
	Flag        string
	Action      string
	Propagation string
}

// containActionOrder is the fixed execution order, regardless of the order
// --actions lists them: cut access first, then force or kill sessions, then
// rotate the credential, then enroll MFA. Delete User is deliberately absent.
var containActionOrder = []containActionDef{
	{Flag: "block", Action: "Block Sign In", Propagation: "Block Sign In can take up to 24 hours to propagate (vendor docs)"},
	{Flag: "force-change", Action: "Force User to Change Password on Next Sign In"},
	{Flag: "expire", Action: "Expire Account Logins", Propagation: "Expire Account Logins can take up to 15 minutes to propagate (vendor docs)"},
	{Flag: "reset", Action: "Reset User Password"},
	{Flag: "mfa", Action: "Setup User MFA"},
}

type containAccountRef struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type containAccountEntry struct {
	Application    string            `json:"application"`
	OrganizationID string            `json:"organizationId"`
	Account        containAccountRef `json:"account"`
}

// containExecuteBody is the POST /response-actions/execute request body.
type containExecuteBody struct {
	Accounts []containAccountEntry `json:"accounts"`
	Action   string                `json:"action"`
	Contacts []string              `json:"contacts"`
}

type containResolution struct {
	Source         string `json:"source"`
	OrganizationID string `json:"organization_id"`
	AccountID      string `json:"account_id"`
	TriggerID      string `json:"trigger_id"`
	Inferred       bool   `json:"inferred"`
	Note           string `json:"note"`
}

type containStep struct {
	Order           int                `json:"order"`
	Flag            string             `json:"flag"`
	Action          string             `json:"action"`
	Method          string             `json:"method"`
	Path            string             `json:"path"`
	Body            containExecuteBody `json:"body"`
	PropagationNote string             `json:"propagation_note"`
	// Status is planned (preview), submitted (execute accepted), error
	// (execute refused), unknown_outcome (the answer did not say whether it
	// was applied; never retried) or not_sent (skipped after an unknown
	// outcome). It never claims the action completed.
	Status    string            `json:"status"`
	Submitted *containSubmitted `json:"submitted"`
	JobMatch  *containJobMatch  `json:"job_match"`
	Error     string            `json:"error"`
}

// containSubmitted is what the 202 execute response says about the call.
type containSubmitted struct {
	ExecutedAt string          `json:"executed_at"`
	EventIDs   []string        `json:"event_ids"`
	Status     string          `json:"status"`
	ExecutedBy containOperator `json:"executed_by"`
}

// containJobMatch is the best-effort job lookup for one submitted action.
// Respond returns no job id from execute and jobs carry no account,
// organization or event link, so a match is never proof of this call.
type containJobMatch struct {
	Confidence string   `json:"confidence"` // heuristic | ambiguous | none
	JobIDs     []string `json:"job_ids"`
	Completed  bool     `json:"completed"`
	Total      *int     `json:"total"`
	Success    *int     `json:"success"`
	Failed     *int     `json:"failed"`
	Missed     *int     `json:"missed"`
	Note       string   `json:"note"`
	Warning    string   `json:"warning"`
}

const containJobMatchNote = "the API returns no job id; this job is matched by action, operator, and time only"

type containSummary struct {
	Planned       int `json:"planned"`
	Submitted     int `json:"submitted"`
	ExecuteErrors int `json:"execute_errors"`
	// UnknownOutcomes counts execute calls whose result could not be read;
	// NotSent counts the actions skipped after one.
	UnknownOutcomes int `json:"unknown_outcomes"`
	NotSent         int `json:"not_sent"`
	JobsHeuristic   int `json:"jobs_heuristic"`
	JobsAmbiguous   int `json:"jobs_ambiguous"`
	JobsUnmatched   int `json:"jobs_unmatched"`
	JobWarnings     int `json:"job_warnings"`
	JobsIncomplete  int `json:"jobs_incomplete"`
}

type containResult struct {
	Mode             string            `json:"mode"`
	Email            string            `json:"email"`
	Resolution       containResolution `json:"resolution"`
	Actions          []containStep     `json:"actions"`
	PropagationNotes []string          `json:"propagation_notes"`
	Wait             string            `json:"wait"`
	// Verified is always false: nothing in the Respond API ties a job to the
	// execute call that created it.
	Verified         bool           `json:"verified"`
	VerificationNote string         `json:"verification_note"`
	Summary          containSummary `json:"summary"`
	Note             string         `json:"note"`
}

const containVerificationNote = "POST /response-actions/execute returns no job id and response-action jobs carry no account or organization link, so each job_match is a best-effort guess by action, operator and time and never proves the action completed; check the account in the Microsoft 365 admin center"

func newNovelContainCmd(flags *rootFlags) *cobra.Command {
	var flagActions string
	var flagOrgID string
	var flagAccountID string
	var flagConfirm bool
	var flagWait string
	var flagDB string

	cmd := &cobra.Command{
		Use:   "contain [email]",
		Short: "Block sign-in, expire sessions, and reset the password for one account in order, previewed first, then follow the jobs.",
		Long: `Use this command for an ad-hoc containment sequence on one account. Do NOT use this command to act on a pending Respond trigger; use 'rule-triggers decide' instead.

Targets Microsoft 365 accounts only: Respond response actions run against application MS, so a Google Workspace or other non-Microsoft account is refused (exit 2).

Organization and account IDs come from --org-id and --account-id together, or are looked up in the local store, which needs synced rule-triggers or users (saas-alerts-cli sync --resources rule-triggers,users). Given alone, --org-id or --account-id narrows that lookup. If the email maps to more than one Microsoft 365 account, the command stops (exit 2) and lists the candidates. The users fallback assumes the Reports customer id equals the Respond organization id and is flagged inferred:true in the output.

The default run, without --confirm, is a preview of the exact execute calls and changes nothing; --confirm submits them in order. The API returns no job id and its jobs carry no account link, so the job_match reported for each action is best-effort (action, operator and time) and never proves completion: the output always says verified:false. The exit code is non-zero when an execute call is refused; when its answer does not say whether it was applied (an unknown_outcome, which is never retried and stops the remaining actions); or when a matched job reports failed or missed accounts, or a job response matching the call carries an explicit failure indicator. --dry-run only prints a stub and does not resolve or plan anything.`,
		Example: strings.Trim(`
  saas-alerts-cli contain jane.doe@example.com --actions block,expire,reset --agent
  saas-alerts-cli contain jane.doe@example.com --org-id ORGANIZATION_ID_12345 --account-id ACCOUNT_ID_12345 --json
  saas-alerts-cli contain jane.doe@example.com --actions block,expire --confirm --wait 5m`, "\n"),
		Annotations: map[string]string{
			"pp:data-source":             "live",
			"pp:preview-happy-path":      "true",
			"pp:happy-args":              "email=jane.doe@example.com;--org-id=ORGANIZATION_ID_12345;--account-id=ACCOUNT_ID_12345",
			"mcp:read-only-unless-flags": "confirm",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "contain")
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("name the account email to contain: contain <email>"))
			}
			if len(args) > 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("contain takes exactly one account email (got %d arguments)", len(args)))
			}
			email := strings.TrimSpace(args[0])
			if !strings.Contains(email, "@") {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("%q is not an email address", email))
			}
			defs, err := containParseActions(flagActions)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(err)
			}
			wait, err := cliutil.ParseDurationLoose(flagWait)
			if err != nil || wait <= 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--wait must be a positive duration like 90s, 2m or 10m (got %q)", flagWait))
			}
			dbPath := flagDB
			if dbPath == "" {
				dbPath = defaultDBPath("saas-alerts-cli")
			}
			res, err := containResolve(cmd.Context(), email, strings.TrimSpace(flagOrgID), strings.TrimSpace(flagAccountID), dbPath)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(err)
			}

			result := containResult{
				Mode:             "preview",
				Email:            email,
				Resolution:       res,
				Actions:          make([]containStep, 0, len(defs)),
				PropagationNotes: make([]string, 0),
				Wait:             wait.String(),
				VerificationNote: containVerificationNote,
			}
			for i, d := range defs {
				result.Actions = append(result.Actions, containStep{
					Order:  i + 1,
					Flag:   d.Flag,
					Action: d.Action,
					Method: "POST",
					Path:   containExecutePath,
					Body: containExecuteBody{
						Accounts: []containAccountEntry{{
							Application:    "MS",
							OrganizationID: res.OrganizationID,
							Account:        containAccountRef{ID: res.AccountID, Email: email},
						}},
						Action: d.Action,
						// contacts is nullable in the spec; null keeps the vendor default.
						Contacts: nil,
					},
					PropagationNote: d.Propagation,
					Status:          "planned",
				})
				if d.Propagation != "" {
					result.PropagationNotes = append(result.PropagationNotes, d.Propagation)
				}
			}
			result.Summary.Planned = len(result.Actions)

			if !flagConfirm {
				result.Note = "preview only: nothing was changed; rerun with --confirm to run these actions in order"
				return containPrint(cmd, flags, result)
			}
			if cliutil.IsAnyHarness() {
				return writeHarnessRefusal(cmd.OutOrStdout(), flags, "contain --confirm")
			}

			// Each action may poll for up to --wait, so the command budget is
			// --timeout plus that polling allowance rather than --timeout alone.
			ctx := cmd.Context()
			if flags.timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, flags.timeout+time.Duration(len(defs))*wait)
				defer cancel()
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			result.Mode = "executed"
			var firstErr error
			stopped := false
			for i := range result.Actions {
				step := &result.Actions[i]
				if stopped {
					step.Status = "not_sent"
					step.Error = "not sent: an earlier action's outcome is unknown, so no further account changes were made"
					result.Summary.NotSent++
					continue
				}
				if err := containRunStep(ctx, c, step, wait); err != nil && firstErr == nil {
					firstErr = err
				}
				if step.Status == "unknown_outcome" {
					// The API answered outside its contract: stop issuing writes
					// to it rather than stack more unreadable account changes.
					result.Summary.UnknownOutcomes++
					stopped = true
					continue
				}
				if step.Status != "submitted" {
					result.Summary.ExecuteErrors++
					continue
				}
				result.Summary.Submitted++
				// A warning counts whatever the confidence: a failure-flagged
				// job response warns even when no job could be attributed.
				if step.JobMatch.Warning != "" {
					result.Summary.JobWarnings++
				}
				switch step.JobMatch.Confidence {
				case "heuristic":
					result.Summary.JobsHeuristic++
					if !step.JobMatch.Completed {
						result.Summary.JobsIncomplete++
					}
				case "ambiguous":
					result.Summary.JobsAmbiguous++
				default:
					result.Summary.JobsUnmatched++
				}
			}
			result.Note = "actions submitted in order; " + containVerificationNote
			if stopped {
				result.Note = "stopped after an action with an unknown outcome; the remaining actions were not sent. " + containVerificationNote
			}
			if err := containPrint(cmd, flags, result); err != nil {
				return err
			}
			// Exit non-zero only for what is known: an execute call that failed,
			// or a heuristically matched job that reports failures. An
			// unmatched or ambiguous job leaves the accepted actions at exit 0.
			if result.Summary.UnknownOutcomes > 0 {
				return partialFailureErr(fmt.Errorf("%d containment action(s) have an unknown outcome and were not retried, %d not sent; check the account before rerunning", result.Summary.UnknownOutcomes, result.Summary.NotSent))
			}
			if result.Summary.ExecuteErrors > 0 {
				if result.Summary.Submitted == 0 && firstErr != nil {
					return classifyAPIErrorOnly(firstErr)
				}
				return partialFailureErr(fmt.Errorf("%d of %d containment actions were not accepted", result.Summary.ExecuteErrors, len(result.Actions)))
			}
			if result.Summary.JobWarnings > 0 {
				return partialFailureErr(fmt.Errorf("%d job match(es) report failed or missed accounts or carried a failure indicator; the match may be another job, so check the account", result.Summary.JobWarnings))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagActions, "actions", "block,expire,reset", "Comma-separated actions: block, force-change, expire, reset, mfa (always run in that order; Delete User is not supported)")
	cmd.Flags().StringVar(&flagOrgID, "org-id", "", "Respond organization ID (with --account-id, skips the store lookup; alone, narrows it)")
	cmd.Flags().StringVar(&flagAccountID, "account-id", "", "Respond account ID (with --org-id, skips the store lookup; alone, narrows it)")
	cmd.Flags().BoolVar(&flagConfirm, "confirm", false, "Run the actions; without it the command only previews")
	cmd.Flags().StringVar(&flagWait, "wait", "2m", "How long to look for and follow each action's best-effort job match")
	cmd.Flags().StringVar(&flagDB, "db", "", "Local store used to resolve organization and account IDs (default: the CLI's local store)")
	return cmd
}

// containParseActions validates --actions and returns them in fixed order.
func containParseActions(raw string) ([]containActionDef, error) {
	want := map[string]bool{}
	for _, tok := range rqSplitCSV(raw) {
		tok = strings.ToLower(tok)
		if tok == "delete" || tok == "delete-user" {
			return nil, fmt.Errorf("contain does not run Delete User; delete the account deliberately in the vendor console")
		}
		known := false
		for _, d := range containActionOrder {
			if d.Flag == tok {
				known = true
				break
			}
		}
		if !known {
			return nil, fmt.Errorf("unknown action %q in --actions (valid: block, force-change, expire, reset, mfa)", tok)
		}
		want[tok] = true
	}
	if len(want) == 0 {
		return nil, fmt.Errorf("--actions needs at least one of block, force-change, expire, reset, mfa")
	}
	out := make([]containActionDef, 0, len(want))
	for _, d := range containActionOrder {
		if want[d.Flag] {
			out = append(out, d)
		}
	}
	return out, nil
}

// containResolve finds the Respond organization and account IDs for email:
// explicit flags first, then the newest synced rule trigger naming the
// account, then synced Reports users (customer id assumed to be the Respond
// organization id, flagged as inferred).
func containResolve(ctx context.Context, email, orgID, accountID, dbPath string) (containResolution, error) {
	if orgID != "" && accountID != "" {
		return containResolution{Source: "flags", OrganizationID: orgID, AccountID: accountID}, nil
	}
	missingHint := fmt.Sprintf("pass --org-id and --account-id, or sync first: saas-alerts-cli sync --resources rule-triggers,users --db %s", dbPath)
	if _, err := os.Stat(dbPath); err != nil {
		return containResolution{}, fmt.Errorf("cannot resolve Respond IDs for %s: no local mirror at %s; %s", email, dbPath, missingHint)
	}
	db, err := store.OpenReadOnlyContext(ctx, dbPath)
	if err != nil {
		return containResolution{}, fmt.Errorf("cannot resolve Respond IDs for %s: opening %s: %v; %s", email, dbPath, err, missingHint)
	}
	defer db.Close()

	// A single --org-id or --account-id narrows the synced candidates; it
	// never overrides half of a pair found for a different account.
	r, ok, nonMSTriggers, err := containResolveFromTriggers(ctx, db, email, orgID, accountID)
	if err != nil {
		return containResolution{}, err
	}
	if ok {
		return r, nil
	}
	r, ok, nonMSUsers, err := containResolveFromUsers(ctx, db, email, orgID, accountID)
	if err != nil {
		return containResolution{}, err
	}
	if ok {
		return r, nil
	}
	if nonMS := append(nonMSTriggers, nonMSUsers...); len(nonMS) > 0 {
		return containResolution{}, fmt.Errorf("Respond response actions support Microsoft 365 accounts only: %s was found only as %s; contain cannot act on it", email, strings.Join(nonMS, ", "))
	}
	selection := ""
	if orgID != "" {
		selection += " in organization " + orgID
	}
	if accountID != "" {
		selection += " with account id " + accountID
	}
	return containResolution{}, fmt.Errorf("cannot resolve Respond IDs for %s: no synced Microsoft 365 rule trigger or user has this email%s; %s", email, selection, missingHint)
}

// containResolveFromTriggers resolves from the newest synced rule trigger
// naming the account. Only accounts whose product.type is MS qualify; the
// non-Microsoft matches it skipped come back for the error message.
func containResolveFromTriggers(ctx context.Context, db *store.Store, email, orgID, accountID string) (containResolution, bool, []string, error) {
	// json_each's own columns are key/value/type/...; the unqualified data
	// argument is the outer resources row and value is the account element.
	rows, err := db.DB().QueryContext(ctx, `SELECT data FROM resources
		WHERE resource_type = 'rule-triggers'
		  AND EXISTS (SELECT 1 FROM json_each(data, '$.accounts')
		              WHERE lower(json_extract(value, '$.email')) = lower(?))`, email)
	if err != nil {
		return containResolution{}, false, nil, fmt.Errorf("reading synced rule-triggers: %w", err)
	}
	raws := make([]json.RawMessage, 0)
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			_ = rows.Close()
			return containResolution{}, false, nil, err
		}
		raws = append(raws, json.RawMessage(data))
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return containResolution{}, false, nil, err
	}
	_ = rows.Close()
	// Target identity is read from every matching row whatever its decision
	// status: a row the decision decoder would reject (null status, ...) still
	// names an organization and account, and dropping it could make another
	// candidate look unambiguous.
	targets := make([]containTargetRow, 0, len(raws))
	unreadable := 0
	for _, raw := range raws {
		row, ok := containReadTarget(raw, email, orgID)
		if !ok {
			unreadable++
			continue
		}
		targets = append(targets, row)
	}
	if unreadable > 0 {
		return containResolution{}, false, nil, fmt.Errorf("cannot resolve the target for %s automatically: %d synced rule trigger row(s) naming this email have an unreadable organization or account identity (organization id, account id or product type); pass --org-id and --account-id", email, unreadable)
	}
	sort.SliceStable(targets, func(i, j int) bool { return targets[i].at.After(targets[j].at) })
	// Collect every distinct Microsoft (organization, account) pair naming
	// this email across all synced triggers; rows are newest first, so the
	// first row seen for a pair is its newest.
	type pair struct{ org, account, trigger string }
	pairs := make([]pair, 0)
	seen := map[string]bool{}
	nonMS := make([]string, 0)
	for _, t := range targets {
		if orgID != "" && t.orgID != orgID {
			continue
		}
		for _, a := range t.accounts {
			if accountID != "" && a.id != accountID {
				continue
			}
			if a.product != containProductMicrosoft {
				nonMS = append(nonMS, fmt.Sprintf("a non-Microsoft account on rule trigger %s", t.triggerID))
				continue
			}
			key := t.orgID + "\x00" + a.id
			if seen[key] {
				continue
			}
			seen[key] = true
			pairs = append(pairs, pair{org: t.orgID, account: a.id, trigger: t.triggerID})
		}
	}
	switch len(pairs) {
	case 0:
		return containResolution{}, false, nonMS, nil
	case 1:
		return containResolution{
			Source:         "rule_trigger",
			OrganizationID: pairs[0].org,
			AccountID:      pairs[0].account,
			TriggerID:      pairs[0].trigger,
			Note:           "IDs taken from synced Respond triggers naming this Microsoft 365 account (newest shown)",
		}, true, nil, nil
	}
	list := make([]string, 0, len(pairs))
	for _, p := range pairs {
		list = append(list, fmt.Sprintf("--org-id %s --account-id %s (rule trigger %s)", p.org, p.account, p.trigger))
	}
	sort.Strings(list)
	return containResolution{}, false, nil, fmt.Errorf("%s matches %d Microsoft 365 accounts in synced rule triggers: %s; pick one with --org-id and --account-id",
		email, len(pairs), strings.Join(list, "; "))
}

// containTargetRow is the target identity read from one stored rule trigger:
// its organization id and the accounts whose email matches.
type containTargetRow struct {
	triggerID string
	at        time.Time
	orgID     string
	accounts  []containTargetAccount
}

type containTargetAccount struct {
	id      string
	product containProductClass
}

// containReadTarget reads a stored rule-trigger row's target identity without
// regard to its decision fields. ok is false when the identity is unreadable:
// the row or its organization is not an object, the organization id is not a
// non-empty string, accounts is not an array, or no account with this email
// has a readable non-empty id. A row in another organization than --org-id is
// still checked for a readable organization id (it might be the selected one).
func containReadTarget(raw json.RawMessage, email, orgID string) (containTargetRow, bool) {
	var row map[string]json.RawMessage
	if !rqIsJSONObject(raw) || json.Unmarshal(raw, &row) != nil {
		return containTargetRow{}, false
	}
	var out containTargetRow
	_ = json.Unmarshal(row["id"], &out.triggerID)
	var triggeredAt, createdAt rqMillis
	_ = json.Unmarshal(row["triggeredAt"], &triggeredAt)
	_ = json.Unmarshal(row["createdAt"], &createdAt)
	if triggeredAt > 0 {
		out.at = triggeredAt.time()
	} else if createdAt > 0 {
		out.at = createdAt.time()
	}
	var org map[string]json.RawMessage
	if !rqIsJSONObject(row["organization"]) || json.Unmarshal(row["organization"], &org) != nil ||
		json.Unmarshal(org["id"], &out.orgID) != nil || strings.TrimSpace(out.orgID) == "" {
		return containTargetRow{}, false
	}
	if orgID != "" && out.orgID != orgID {
		return out, true // readable, and not the selected organization
	}
	var accounts []json.RawMessage
	if !rqIsJSONArray(row["accounts"]) || json.Unmarshal(row["accounts"], &accounts) != nil {
		return containTargetRow{}, false
	}
	for _, a := range accounts {
		var acc map[string]json.RawMessage
		if !rqIsJSONObject(a) || json.Unmarshal(a, &acc) != nil {
			continue
		}
		var accEmail string
		if json.Unmarshal(acc["email"], &accEmail) != nil || !strings.EqualFold(accEmail, email) {
			continue
		}
		var target containTargetAccount
		if json.Unmarshal(acc["id"], &target.id) != nil || strings.TrimSpace(target.id) == "" {
			return containTargetRow{}, false
		}
		// The product decides Microsoft or not: an unknown product (missing,
		// malformed, or a type outside the Respond enum) is never treated as
		// non-Microsoft, it makes the row's identity unreadable.
		target.product = containClassifyRespondProduct(acc["product"])
		if target.product == containProductUnknown {
			return containTargetRow{}, false
		}
		out.accounts = append(out.accounts, target)
	}
	if len(out.accounts) == 0 {
		// The store matched this row on the email, yet no account entry
		// reads as that email with an id: its identity is not readable.
		return containTargetRow{}, false
	}
	return out, true
}

// containProductClass is a three-way product verdict: only a positively
// identified non-Microsoft product may be set aside; an unknown one blocks
// automatic target resolution.
type containProductClass int

const (
	containProductUnknown containProductClass = iota
	containProductMicrosoft
	containProductNonMicrosoft
)

// containClassifyRespondProduct reads a Respond accounts[].product: its
// documented type enum is MS | G_SUITE (Respond spec 2.0.0). A missing or
// non-string type, or any other value, is unknown.
func containClassifyRespondProduct(raw json.RawMessage) containProductClass {
	var p struct {
		Type *string `json:"type"`
	}
	if !rqIsJSONObject(raw) || json.Unmarshal(raw, &p) != nil || p.Type == nil {
		return containProductUnknown
	}
	switch strings.ToUpper(strings.TrimSpace(*p.Type)) {
	case "MS":
		return containProductMicrosoft
	case "G_SUITE":
		return containProductNonMicrosoft
	}
	return containProductUnknown
}

// containReportsMicrosoftFamily are Reports ApplicationType values for
// Microsoft-side monitoring products. A user listed only under one of them
// may well be a Microsoft 365 account in that customer, so they are not
// "positively non-Microsoft"; they classify as unknown (INFERRED).
var containReportsMicrosoftFamily = map[string]bool{
	"MS_DEFENDER": true, "DEFENDER_MANAGER": true, "DEFENDER_FOR_BUSINESS": true,
	"OFFICE_365_LOGIN_ANALYZER": true, "OFFICE_365_RISK_DETECTION": true, "OFFICE_365_LOG_MONITOR": true,
	"OFFICE_365_SECURE_SCORE": true, "MICROSOFT_EXCHANGE_THREAT_DETECTION": true,
	"ACTIVE_DIRECTORY_MONITOR_AND_SYNC": true,
}

// containReportsNonMicrosoft are the remaining Reports ApplicationType enum
// values (functions spec 3.362.0): products that are not Microsoft 365.
var containReportsNonMicrosoft = map[string]bool{
	"G_SUITE": true, "DROP_BOX": true, "SLACK": true, "IT_GLUE": true, "SALES_FORCE": true, "NINJA_ONE": true,
	"KASEYA_VSA": true, "DATTO_RMM": true, "SYNCRO_RMM": true, "N_ABLE_N_CENTRAL": true, "CONNECTWISE_AUTOMATE": true,
	"OKTA": true, "ZOOM": true, "KASEYA_VSA_X": true, "DUO": true, "CLOUDTRAIL": true, "ROCKETCYBER": true, "HUDU": true,
	"EGNYTE": true, "DATTO_EDR": true, "SENTINEL_ONE": true, "SAAS_ALERTS_MAIN": true, "SAAS_ALERTS_MANAGE": true,
	"SAAS_ALERTS_REPORTS": true, "SAAS_ALERTS_RESPOND": true, "SAAS_ALERTS_FORTIFY": true, "SAAS_ALERTS_UNIFY": true,
	"WEBROOT_MONITOR": true, "SENTINELONE_MONITOR": true, "BARRACUDA_ESS_MONITOR": true, "BITDEFENDER_MONITOR": true,
	"DATTO_RANSOMWARE_DETECTION": true, "ADVANCED_BREACH_DETECTION": true, "BARRACUDA_SENTINEL_MONITOR": true,
	"GRAPHUS": true, "IRONSCALES_MONITOR": true, "PWND_MONITOR": true, "SYSLOG_COLLECTOR": true,
	"MALICIOUS_FILE_DETECTION": true, "DARK_WEB_ID_MONITOR": true, "ENDPOINT_EVENT_LOG_MONITOR": true,
	"CRYPTO_MINING_DETECTION": true, "SUSPICIOUS_TOOLS": true, "FIREWALL_LOG_ANALYZER": true, "SOPHOS_MONITOR": true,
	"VULSCAN_COLLECTOR": true, "DNS_FILTER_MONITOR": true, "CYBER_TERRORIST_NETWORK_CONNECTIONS": true,
	"SUSPICIOUS_NETWORK_SERVICES": true, "PASSLY_MONITOR": true, "LOG4J_DETECTOR": true, "DEEP_INSTINCT_MONITOR": true,
	"PRINT_NIGHTMARE_HUNT": true, "CYLANCE_MONITOR": true, "VSA_THREAT_HUNT": true, "DATTO_EDR_AV_MONITOR": true,
	"IOC_DETECTION": true,
}

// containClassifyReportsProduct reads a Reports user's product, whose
// documented type is the ApplicationType enum. Only the type decides: MS is
// Microsoft, a non-Microsoft enum value is non-Microsoft, and everything else
// (no type, an id or name alone, a Microsoft-family monitor, or a value
// outside the enum) is unknown.
func containClassifyReportsProduct(raw json.RawMessage) containProductClass {
	var p struct {
		Type *string `json:"type"`
	}
	if !rqIsJSONObject(raw) || json.Unmarshal(raw, &p) != nil || p.Type == nil {
		return containProductUnknown
	}
	t := strings.ToUpper(strings.TrimSpace(*p.Type))
	switch {
	case t == "MS":
		return containProductMicrosoft
	case containReportsNonMicrosoft[t]:
		return containProductNonMicrosoft
	}
	return containProductUnknown
}

type containUserCandidate struct {
	customerID string
	userID     string
	product    string
}

// containResolveFromUsers resolves from synced Reports users, using only rows
// whose product.type is MS; positively non-Microsoft rows come back for the
// error message, and an unknown product blocks automatic resolution.
func containResolveFromUsers(ctx context.Context, db *store.Store, email, orgID, accountID string) (containResolution, bool, []string, error) {
	rows, err := db.DB().QueryContext(ctx, `SELECT id, data FROM resources
		WHERE resource_type = 'users' AND lower(json_extract(data, '$.email')) = lower(?)`, email)
	if err != nil {
		return containResolution{}, false, nil, fmt.Errorf("reading synced users: %w", err)
	}
	type row struct{ id, data string }
	found := make([]row, 0)
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.data); err != nil {
			_ = rows.Close()
			return containResolution{}, false, nil, err
		}
		found = append(found, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return containResolution{}, false, nil, err
	}
	_ = rows.Close()

	seen := map[string]bool{}
	cands := make([]containUserCandidate, 0)
	nonMS := make([]string, 0)
	unreadable := 0
	for _, r := range found {
		var u struct {
			ID          any             `json:"id"`
			ParentID    any             `json:"parent_id"`
			CustomersID any             `json:"customers_id"`
			Product     json.RawMessage `json:"product"`
		}
		// A matching row that cannot be decoded might be the right account
		// (or a second one), so it blocks automatic resolution.
		if err := json.Unmarshal([]byte(r.data), &u); err != nil {
			unreadable++
			continue
		}
		var product struct {
			Name string `json:"name"`
		}
		if rqIsJSONObject(u.Product) {
			_ = json.Unmarshal(u.Product, &product)
		}
		productClass := containClassifyReportsProduct(u.Product)
		// Dependent sync injects the parent customer id as parent_id (and
		// customers_id); the storage key also carries it after a NUL.
		customer := containScalar(u.ParentID)
		if customer == "" {
			customer = containScalar(u.CustomersID)
		}
		if customer == "" {
			if i := strings.IndexByte(r.id, 0); i >= 0 {
				customer = r.id[i+1:]
			}
		}
		userID := containScalar(u.ID)
		if userID == "" {
			userID = store.BareResourceID(r.id)
		}
		if customer == "" || userID == "" {
			unreadable++
			continue
		}
		if (orgID != "" && customer != orgID) || (accountID != "" && userID != accountID) {
			continue
		}
		if productClass == containProductUnknown {
			unreadable++
			continue
		}
		if productClass == containProductNonMicrosoft {
			nonMS = append(nonMS, fmt.Sprintf("a non-Microsoft user in customer %s", customer))
			continue
		}
		key := customer + "\x00" + userID
		if seen[key] {
			continue
		}
		seen[key] = true
		cands = append(cands, containUserCandidate{customerID: customer, userID: userID, product: product.Name})
	}
	if unreadable > 0 {
		return containResolution{}, false, nil, fmt.Errorf("cannot resolve the target for %s automatically: %d synced user row(s) for this email cannot be read (identity or product); pass --org-id and --account-id", email, unreadable)
	}
	if len(cands) == 0 {
		return containResolution{}, false, nonMS, nil
	}
	if len(cands) > 1 {
		list := make([]string, 0, len(cands))
		for _, c := range cands {
			list = append(list, fmt.Sprintf("customer %s / user %s (%s)", c.customerID, c.userID, c.product))
		}
		return containResolution{}, false, nil, fmt.Errorf("%s matches %d synced Microsoft 365 users: %s; pick one with --org-id and --account-id", email, len(cands), strings.Join(list, "; "))
	}
	c := cands[0]
	return containResolution{
		Source:         "users",
		OrganizationID: c.customerID,
		AccountID:      c.userID,
		Inferred:       true,
		Note:           "resolved from synced Reports users: the Reports customer id is assumed to equal the Respond organization id and the user id the Respond account id; confirm with --org-id/--account-id if unsure",
	}, true, nil, nil
}

func containScalar(v any) string {
	switch tv := v.(type) {
	case nil:
		return ""
	case string:
		return tv
	case float64:
		return formatCLIParamValue(tv)
	default:
		return fmt.Sprintf("%v", tv)
	}
}

type containOperator struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// containJob is the Respond ResponseActionJob subset this command follows.
type containJob struct {
	ID        string   `json:"id"`
	CreatedAt rqMillis `json:"createdAt"`
	Total     *int     `json:"total"`
	Success   *int     `json:"success"`
	Failed    *int     `json:"failed"`
	Missed    *int     `json:"missed"`
	// Completed is nil when the job object omits or nulls completed, so a
	// sparse detail read cannot turn a known completion into "not done".
	Completed *bool           `json:"completed"`
	Action    string          `json:"action"`
	CreatedBy containOperator `json:"createdBy"`
}

func containIntValue(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

// containRunStep submits one action and then looks for its job. Submission
// is the only fact the API confirms; the job lookup is a best-effort match
// (no job id comes back) that is re-checked on every poll and downgraded to
// ambiguous as soon as a second candidate appears. The returned error is the
// execute failure, if any.
func containRunStep(ctx context.Context, c *client.Client, step *containStep, wait time.Duration) error {
	// Jobs that already exist cannot be this call's.
	before, beforeFlagged, snapErr := containListJobs(ctx, c)
	snapshot := map[string]bool{}
	for _, j := range before {
		snapshot[j.ID] = true
	}
	for _, fr := range beforeFlagged {
		snapshot[fr.job.ID] = true
	}

	data, code, err := c.Post(ctx, step.Path, step.Body)
	if err != nil {
		if containOutcomeUnknown(err) {
			step.Status = "unknown_outcome"
			step.Error = "the execute call failed without saying whether Respond applied it (" + err.Error() + "); it was not retried"
			return err
		}
		step.Status = "error"
		step.Error = err.Error()
		return err
	}
	exec, why := containReadAcceptance(code, data)
	if why != "" {
		step.Status = "unknown_outcome"
		step.Error = fmt.Sprintf("execute answered HTTP %d with %s, not the documented acceptance; the action may or may not have been applied and was not retried", code, why)
		return fmt.Errorf("execute %q: %s", step.Action, step.Error)
	}
	step.Status = "submitted"
	step.Submitted = &containSubmitted{EventIDs: exec.EventIDs, Status: exec.Status, ExecutedBy: exec.ExecutedBy}
	if step.Submitted.EventIDs == nil {
		step.Submitted.EventIDs = make([]string, 0)
	}
	match := &containJobMatch{Confidence: "none", JobIDs: make([]string, 0), Note: containJobMatchNote}
	step.JobMatch = match
	if exec.ExecutedAt <= 0 {
		match.Note += "; the execute response carried no executedAt, so no job was looked up"
		return nil
	}
	executedAt := exec.ExecutedAt.time()
	step.Submitted.ExecutedAt = rqFormatTime(executedAt)

	// From here on the action has been accepted. Every job-monitoring read
	// that fails to answer clearly is a problem: it goes into the warning and
	// makes the exit non-zero. Notes are only for informational facts.
	problems := make([]string, 0)
	addProblem := func(p string) {
		for _, q := range problems {
			if q == p {
				return
			}
		}
		problems = append(problems, p)
	}
	// flaggedEvidence is sticky: once a row or detail for this call's job
	// carried a failure indicator, failed to decode or contradicted the
	// vetted listing, completion is never taken from it and the step warns.
	flaggedEvidence := ""
	countWarning := ""
	compose := func() {
		parts := make([]string, 0, 2+len(problems))
		if countWarning != "" {
			parts = append(parts, countWarning)
		}
		if flaggedEvidence != "" {
			parts = append(parts, flaggedEvidence+"; it is not used as completion evidence")
		}
		parts = append(parts, problems...)
		match.Warning = ""
		if len(parts) > 0 {
			match.Warning = strings.Join(parts, "; ") + "; check the account"
		}
	}

	if snapErr != nil {
		addProblem("the job list could not be read before executing, so this call's job cannot be identified (" + snapErr.Error() + ")")
		compose()
		return nil
	}

	deadline := time.Now().Add(wait)
	// worst holds, per vetted job id, the highest failed and missed counts any
	// poll reported: a later, sparser read (counts omitted or null) must not
	// erase a failure that was already seen.
	type worstCounts struct{ failed, missed *int }
	worst := map[string]worstCounts{}
	// seen keeps every distinct candidate id across all polls of this step,
	// in the order first seen: a candidate that drops off the list and is
	// replaced by another is still a second candidate.
	seen := make([]string, 0, 2)
	seenSet := map[string]bool{}
	var lastVetted containJob
	evaluate := func(job containJob) {
		match.Confidence = "heuristic"
		match.JobIDs = []string{job.ID}
		w := worst[job.ID]
		w.failed = containLargerCount(job.Failed, w.failed)
		w.missed = containLargerCount(job.Missed, w.missed)
		worst[job.ID] = w
		job.Failed, job.Missed = w.failed, w.missed
		match.Completed = job.Completed != nil && *job.Completed
		match.Total, match.Success, match.Failed, match.Missed = job.Total, job.Success, job.Failed, job.Missed
		// A known failed or missed count warns whether or not the job has
		// finished: it can only grow, and waiting cannot clear it.
		countWarning = ""
		if containIntValue(job.Failed) > 0 || containIntValue(job.Missed) > 0 {
			countWarning = fmt.Sprintf("job %s reports failed=%d missed=%d; it may be another job",
				job.ID, containIntValue(job.Failed), containIntValue(job.Missed))
		}
		compose()
	}
	for {
		jobs, flaggedRows, err := containListJobs(ctx, c)
		if err != nil {
			addProblem("listing jobs after the call was accepted failed (" + err.Error() + ")")
			compose()
			return nil
		}
		cands := containJobCandidates(jobs, snapshot, step.Action, executedAt, exec.ExecutedBy)
		inCands := map[string]bool{}
		for _, j := range cands {
			inCands[j.ID] = true
		}
		// Rows for the already-vetted job are its evidence whatever their
		// action, operator or time now say: a flagged or undecodable row, or a
		// clean row that no longer matches, is warning evidence for that job.
		vettedAgain := false
		flaggedJobs := make([]containJob, 0, len(flaggedRows))
		flaggedWhy := map[string]string{}
		for _, fr := range flaggedRows {
			if lastVetted.ID != "" && fr.job.ID == lastVetted.ID {
				flaggedEvidence = "the job list row for " + fr.job.ID + " " + fr.why
				vettedAgain = true
				continue
			}
			if snapshot[fr.job.ID] {
				continue // existed before the call
			}
			if len(containJobCandidates([]containJob{fr.job}, nil, step.Action, executedAt, exec.ExecutedBy)) == 1 {
				flaggedJobs = append(flaggedJobs, fr.job)
				flaggedWhy[fr.job.ID] = fr.why
				continue
			}
			// A new unusable row that its readable metadata does not rule out
			// may be this call's job: that is a monitoring problem. It is still
			// never used for attribution or completion.
			if !containRowRuledOut(fr, step.Action, executedAt, exec.ExecutedBy) {
				addProblem("a new job list row " + fr.job.ID + " " + fr.why + ", and its action or time cannot be read to rule it out")
			}
		}
		if lastVetted.ID != "" {
			for _, j := range jobs {
				if j.ID == lastVetted.ID && !inCands[j.ID] {
					flaggedEvidence = "the job list row for " + j.ID + " no longer matches its action, operator or time"
					vettedAgain = true
				}
			}
		}
		flaggedCands := containJobCandidates(flaggedJobs, snapshot, step.Action, executedAt, exec.ExecutedBy)
		for _, j := range append(append([]containJob{}, cands...), flaggedCands...) {
			if !seenSet[j.ID] {
				seenSet[j.ID] = true
				seen = append(seen, j.ID)
			}
		}
		if len(flaggedCands) == 1 {
			flaggedEvidence = "the job list row for " + flaggedCands[0].ID + " " + flaggedWhy[flaggedCands[0].ID]
		}
		switch {
		case len(seen) > 1:
			match.Confidence = "ambiguous"
			match.JobIDs = append(make([]string, 0, len(seen)), seen...)
			// Attribution is lost, so the counts and any job evidence carried
			// from an earlier poll no longer belong to this call. Monitoring-read
			// problems already collected still stand and still warn.
			match.Completed, match.Total, match.Success, match.Failed, match.Missed = false, nil, nil, nil, nil
			countWarning, flaggedEvidence = "", ""
			match.Note = containJobMatchNote + fmt.Sprintf("; %d new %q jobs fit across the polls, so none is attributed to this call", len(seen), step.Action)
			compose()
			return nil
		case len(cands) == 1:
			// The list entry passed the candidate checks; the detail read only
			// replaces it if it is the same job and passes them too.
			job := cands[0]
			lastVetted = job
			detail, derr := containGetJob(ctx, c, job.ID)
			if derr == nil && len(containJobCandidates([]containJob{detail}, nil, step.Action, executedAt, exec.ExecutedBy)) != 1 {
				derr = &containFlaggedError{msg: "the job detail for " + job.ID + " contradicts the listed job's action, operator or time"}
			}
			var flagged *containFlaggedError
			switch {
			case derr == nil:
				job = containMergeJob(cands[0], detail)
			case errors.Is(derr, errContainDetailIDMismatch):
				// The detail answered for another job: that read did not answer
				// clearly, so it is a monitoring problem. The listing is kept.
				addProblem("the job detail read for " + job.ID + " answered for a different job id, so the listed entry's counts are shown")
			case errors.As(derr, &flagged):
				flaggedEvidence = flagged.Error()
			default:
				addProblem("reading the job detail for " + job.ID + " failed (" + derr.Error() + ")")
			}
			evaluate(job)
			if match.Completed {
				return nil
			}
		case vettedAgain:
			// This poll's row for the vetted job is unusable: the last vetted
			// listing stands in, and it is never completed by the new row.
			evaluate(lastVetted)
		case len(flaggedCands) == 1:
			// The only candidate this call has is unusable and was never
			// vetted: nothing can be attributed or completed from it.
			compose()
			return nil
		}
		if !time.Now().Before(deadline) {
			if match.Confidence == "none" {
				match.Note += fmt.Sprintf("; no candidate job appeared within %s", wait)
			} else {
				match.Note += fmt.Sprintf("; the job had not completed within %s", wait)
			}
			compose()
			return nil
		}
		select {
		case <-ctx.Done():
			match.Note += "; stopped waiting: " + ctx.Err().Error()
			compose()
			return nil
		case <-time.After(containPollInterval):
		}
	}
}

// containExecAcceptance is the documented 202 execute response:
// {eventIds, executedAt, executedBy{email,id}, executedByIp, status}.
type containExecAcceptance struct {
	EventIDs   []string        `json:"eventIds"`
	ExecutedAt rqMillis        `json:"executedAt"`
	ExecutedBy containOperator `json:"executedBy"`
	Status     string          `json:"status"`
}

// containAcceptedStatuses are the execute statuses that mean "accepted"
// (the spec example answers PROCESSING); compared case-insensitively.
var containAcceptedStatuses = map[string]bool{
	"PROCESSING": true, "PENDING": true, "QUEUED": true, "ACCEPTED": true, "SUCCESS": true, "COMPLETED": true,
}

// containReadAcceptance checks that an execute response is the documented
// acceptance: a 2xx JSON object that passes the shared failure rule
// (rqFailureIndicator, with status treated as data) and
// either an accepted status, or no status and a positive executedAt. It
// returns why not, describing the body by shape only (never its values).
func containReadAcceptance(code int, data json.RawMessage) (containExecAcceptance, string) {
	var exec containExecAcceptance
	if code < 200 || code > 299 {
		return exec, fmt.Sprintf("a non-2xx status %d", code)
	}
	var obj map[string]json.RawMessage
	if !rqIsJSONObject(data) || json.Unmarshal(data, &obj) != nil {
		return exec, "a body that is not a JSON object (" + rqDescribeJSON(data) + ")"
	}
	if err := json.Unmarshal(data, &exec); err != nil {
		return exec, "a body that does not decode as the execute acceptance (" + rqDescribeJSON(data) + ")"
	}
	// The shared failure rule, every clause except status: execute's status
	// is domain data, checked against the accepted set below.
	if why := rqFailureIndicator(obj, true); why != "" {
		return exec, "a body carrying " + why + " (" + rqDescribeJSON(data) + ")"
	}
	status := strings.ToUpper(strings.TrimSpace(exec.Status))
	switch {
	case status != "" && !containAcceptedStatuses[status]:
		// The status value itself is not echoed; only its class is.
		kind := "an unrecognized status"
		if containFailureStatuses[status] {
			kind = "a known failure status"
		}
		return exec, "a body with " + kind + " (accepted: PROCESSING, PENDING, QUEUED, ACCEPTED, SUCCESS, COMPLETED)"
	case status == "" && exec.ExecutedAt <= 0:
		return exec, "a body with neither an accepted status nor executedAt (" + rqDescribeJSON(data) + ")"
	}
	return exec, ""
}

// containFailureStatuses are execute statuses that explicitly report failure.
var containFailureStatuses = map[string]bool{
	"ERROR": true, "FAILED": true, "FAILURE": true, "REJECTED": true, "DENIED": true,
	"CANCELLED": true, "CANCELED": true, "ABORTED": true, "TIMEOUT": true, "TIMED_OUT": true,
}

// containJSONEmpty treats null, false, "", [] and {} as "no value".
func containJSONEmpty(v json.RawMessage) bool {
	switch string(bytes.TrimSpace(v)) {
	case "", "null", "false", `""`, "[]", "{}":
		return true
	}
	return false
}

// containOutcomeUnknown reports whether a failed execute call may still have
// been applied: a server error from the execute endpoint itself, or a
// transport failure after the request may have been sent. A 4xx (refused),
// a failure before sending (auth mint, no route to host) is a known "no".
func containOutcomeUnknown(err error) bool {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode >= 500 && strings.Contains(apiErr.Path, "/response-actions/execute")
	}
	msg := err.Error()
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host") {
		return false
	}
	return isNetworkError(err) || errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(msg, "timeout") || strings.Contains(msg, "EOF")
}

// containMergeJob combines the vetted list entry with its matching detail
// read. The detail is the fresher view, but a completion or count it omits
// or nulls keeps the list's value, and for failed and missed the larger known
// count wins, so a known failure can never be erased by a sparser read.
func containMergeJob(listed, detail containJob) containJob {
	out := detail
	pick := func(d, l *int) *int {
		if d == nil {
			return l
		}
		return d
	}
	if detail.Completed == nil {
		out.Completed = listed.Completed
	}
	out.Total = pick(detail.Total, listed.Total)
	out.Success = pick(detail.Success, listed.Success)
	out.Failed = containLargerCount(detail.Failed, listed.Failed)
	out.Missed = containLargerCount(detail.Missed, listed.Missed)
	return out
}

// containLargerCount returns the larger of two optional counts; a missing
// count never replaces a known one.
func containLargerCount(a, b *int) *int {
	switch {
	case a == nil:
		return b
	case b == nil || *a >= *b:
		return a
	default:
		return b
	}
}

func containListJobs(ctx context.Context, c *client.Client) ([]containJob, []containFlaggedRow, error) {
	data, err := c.GetNoCache(ctx, containJobsPath, nil)
	if err != nil {
		return nil, nil, err
	}
	// The spec returns a bare array; a {"jobs": [...]} wrapper is tolerated,
	// but only if the wrapper itself passes the shared failure rule. A null
	// body is an error, not "no jobs".
	raw := data
	if !rqIsJSONArray(raw) {
		// Object first, then the failure rule, THEN the jobs shape: an explicit
		// failure is reported as one whatever the collection looks like.
		var wrapper map[string]json.RawMessage
		if !rqIsJSONObject(raw) || json.Unmarshal(raw, &wrapper) != nil {
			return nil, nil, fmt.Errorf("unexpected GET %s response: expected a job array, got %s", containJobsPath, rqDescribeJSON(data))
		}
		if why := rqFailureIndicator(wrapper, false); why != "" {
			return nil, nil, &containFlaggedError{msg: "the job list carried a failure indicator (" + why + ")"}
		}
		if !rqIsJSONArray(wrapper["jobs"]) {
			return nil, nil, fmt.Errorf("unexpected GET %s response: expected a job array, got %s", containJobsPath, rqDescribeJSON(data))
		}
		raw = wrapper["jobs"]
	}
	var rows []json.RawMessage
	if json.Unmarshal(raw, &rows) != nil {
		return nil, nil, fmt.Errorf("unexpected GET %s response: the job array did not decode (%s)", containJobsPath, rqDescribeJSON(raw))
	}
	jobs := make([]containJob, 0, len(rows))
	flagged := make([]containFlaggedRow, 0)
	for _, row := range rows {
		// The raw object is checked first: a typed decode would fail on a
		// boolean success:false (success is a numeric counter on jobs) before
		// the failure rule could see it.
		var obj map[string]json.RawMessage
		if !rqIsJSONObject(row) || json.Unmarshal(row, &obj) != nil {
			return nil, nil, &containFlaggedError{msg: "a job list row could not be read (" + rqDescribeJSON(row) + ")"}
		}
		why := ""
		if f := rqFailureIndicator(obj, false); f != "" {
			why = "carried a failure indicator (" + f + ")"
		}
		var job containJob
		if why == "" && json.Unmarshal(row, &job) != nil {
			why = "could not be decoded"
		}
		if why == "" {
			jobs = append(jobs, job)
			continue
		}
		// A flagged or undecodable row is never evidence, but its identity
		// is still read so the ambiguity and vetted-listing rules see it.
		fr, ok := containReadJobIdentity(obj)
		if !ok {
			return nil, nil, &containFlaggedError{msg: "a job list row " + why + " and has no readable id"}
		}
		fr.why = why
		flagged = append(flagged, fr)
	}
	return jobs, flagged, nil
}

// containFlaggedRow is a job list row that cannot be used as evidence.
type containFlaggedRow struct {
	job containJob
	why string
	// actionOK and timeOK say whether the row's action and createdAt could
	// be read; an unreadable one cannot be used to rule the row out.
	actionOK, timeOK bool
}

// containReadJobIdentity reads only a job's identity fields (id, action,
// createdAt, createdBy) from its raw object, tolerating any other field.
func containReadJobIdentity(obj map[string]json.RawMessage) (containFlaggedRow, bool) {
	var job containJob
	if json.Unmarshal(obj["id"], &job.ID) != nil || strings.TrimSpace(job.ID) == "" {
		return containFlaggedRow{}, false
	}
	actionOK := json.Unmarshal(obj["action"], &job.Action) == nil && strings.TrimSpace(job.Action) != ""
	_ = json.Unmarshal(obj["createdAt"], &job.CreatedAt)
	timeOK := job.CreatedAt > 0
	if rqIsJSONObject(obj["createdBy"]) {
		var by map[string]json.RawMessage
		if json.Unmarshal(obj["createdBy"], &by) == nil {
			_ = json.Unmarshal(by["id"], &job.CreatedBy.ID)
			_ = json.Unmarshal(by["email"], &job.CreatedBy.Email)
		}
	}
	return containFlaggedRow{job: job, actionOK: actionOK, timeOK: timeOK}, true
}

// containRowRuledOut reports whether a flagged row's readable metadata
// clearly excludes it from this call: a readable action that differs, a
// readable createdAt outside the window, or a readable creator that
// contradicts executedBy. Unreadable fields never rule a row out.
func containRowRuledOut(fr containFlaggedRow, action string, executedAt time.Time, by containOperator) bool {
	if fr.actionOK && fr.job.Action != action {
		return true
	}
	if fr.timeOK {
		at := fr.job.CreatedAt.time()
		if at.Before(executedAt.Add(-containJobMatchSkew)) || at.After(executedAt.Add(containJobMatchWindow)) {
			return true
		}
	}
	match, _ := containSameOperator(fr.job.CreatedBy, by)
	return !match
}

// containSameOperator compares a job's createdBy with the execute response's
// executedBy on every identity field both sides carry. positive reports that
// at least one field was compared and matched.
func containSameOperator(job, by containOperator) (match, positive bool) {
	if job.ID != "" && by.ID != "" {
		if job.ID != by.ID {
			return false, false
		}
		positive = true
	}
	if job.Email != "" && by.Email != "" {
		if !strings.EqualFold(job.Email, by.Email) {
			return false, false
		}
		positive = true
	}
	return true, positive
}

// containJobCandidates lists the jobs that could belong to one execute
// call: absent from the pre-call snapshot, with an action equal to the
// requested one (a job without an action never fits), created within
// [executedAt - containJobMatchSkew, executedAt + containJobMatchWindow], and
// with a creator that does not contradict executedBy. These are heuristics:
// no field links a job to the call, the account, or the organization.
func containJobCandidates(jobs []containJob, snapshot map[string]bool, action string, executedAt time.Time, by containOperator) []containJob {
	lo, hi := executedAt.Add(-containJobMatchSkew), executedAt.Add(containJobMatchWindow)
	cands := make([]containJob, 0)
	for _, j := range jobs {
		if j.ID == "" || snapshot[j.ID] || j.Action == "" || j.Action != action || j.CreatedAt <= 0 {
			continue
		}
		at := j.CreatedAt.time()
		if at.Before(lo) || at.After(hi) {
			continue
		}
		if match, _ := containSameOperator(j.CreatedBy, by); !match {
			continue
		}
		cands = append(cands, j)
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].CreatedAt < cands[j].CreatedAt })
	return cands
}

func containGetJob(ctx context.Context, c *client.Client, id string) (containJob, error) {
	data, err := c.GetNoCache(ctx, containJobsPath+"/"+url.PathEscape(id), nil)
	if err != nil {
		return containJob{}, err
	}
	// The raw object is read first: identity, then the failure rule, then the
	// typed decode (which a boolean success:false would otherwise fail).
	var obj map[string]json.RawMessage
	if !rqIsJSONObject(data) || json.Unmarshal(data, &obj) != nil {
		return containJob{}, &containFlaggedError{msg: "the job detail for " + id + " could not be read (" + rqDescribeJSON(data) + ")"}
	}
	var gotID string
	if json.Unmarshal(obj["id"], &gotID) != nil || strings.TrimSpace(gotID) == "" {
		return containJob{}, &containFlaggedError{msg: "the job detail for " + id + " has no readable id"}
	}
	if gotID != id {
		return containJob{}, errContainDetailIDMismatch
	}
	if why := rqFailureIndicator(obj, false); why != "" {
		return containJob{}, &containFlaggedError{msg: "the job detail for " + id + " carried a failure indicator (" + why + ")"}
	}
	var job containJob
	if err := json.Unmarshal(data, &job); err != nil {
		return containJob{}, &containFlaggedError{msg: "the job detail for " + id + " could not be decoded (" + rqDescribeJSON(data) + ")"}
	}
	return job, nil
}

// errContainDetailIDMismatch marks a detail that answered for another job:
// a monitoring problem (warning), with the vetted listing kept.
var errContainDetailIDMismatch = errors.New("job detail id mismatch (the body is a different job)")

// containFlaggedError reports a job response that carried an explicit
// failure indicator or could not be read. Such a response is never evidence
// of completion, and it makes the run exit non-zero (a check-the-account
// warning) instead of dropping to a note.
type containFlaggedError struct{ msg string }

func (e *containFlaggedError) Error() string { return e.msg }

func containPrint(cmd *cobra.Command, flags *rootFlags, r containResult) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), r, flags)
	}
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s: contain %s (organization %s, account %s, resolved from %s)\n",
		r.Mode, r.Email, r.Resolution.OrganizationID, r.Resolution.AccountID, r.Resolution.Source)
	if r.Resolution.Inferred {
		fmt.Fprintf(w, "note: %s\n", r.Resolution.Note)
	}
	tw := newTabWriter(w)
	fmt.Fprintln(tw, "#\tACTION\tSTATUS\tJOB MATCH\tJOB\tDONE\tSUCCESS\tFAILED\tMISSED\tERROR")
	for _, s := range r.Actions {
		confidence, jobs, done := "-", "-", "-"
		var success, failed, missed *int
		if m := s.JobMatch; m != nil {
			confidence, jobs, done = m.Confidence, strings.Join(m.JobIDs, ","), fmt.Sprintf("%t", m.Completed)
			success, failed, missed = m.Success, m.Failed, m.Missed
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", s.Order, s.Action, s.Status, confidence, jobs, done,
			containIntCell(success), containIntCell(failed), containIntCell(missed), truncate(s.Error, 60))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	for _, s := range r.Actions {
		if s.JobMatch != nil && s.JobMatch.Warning != "" {
			fmt.Fprintf(w, "warning: %s: %s\n", s.Action, s.JobMatch.Warning)
		}
	}
	if r.Mode == "executed" {
		fmt.Fprintf(w, "verified: false (%s)\n", r.VerificationNote)
	}
	for _, n := range r.PropagationNotes {
		fmt.Fprintf(w, "note: %s\n", n)
	}
	if r.Note != "" && r.Mode != "executed" {
		fmt.Fprintln(w, r.Note)
	}
	return nil
}

func containIntCell(v *int) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%d", *v)
}
