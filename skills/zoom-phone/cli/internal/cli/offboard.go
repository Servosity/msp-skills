// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: ordered offboarding plan, optionally applied.
// pp:data-source auto
// Discovery is the footprint join (local base rows + live membership); the
// plan is printed by default and --apply runs each DELETE with a receipt.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"zoom-phone-pp-cli/internal/client"
	"zoom-phone-pp-cli/internal/cliutil"
)

type localOffboardStep struct {
	Step       int               `json:"step"`
	Family     string            `json:"family"`
	Action     string            `json:"action"`
	TargetID   string            `json:"target_id"`
	TargetName string            `json:"target_name,omitempty"`
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Query      map[string]string `json:"query,omitempty"`
}

// localOffboardReceipt echoes the resolved user on every step so a receipt
// read on its own still says whose membership was removed.
type localOffboardReceipt struct {
	Step          int    `json:"step"`
	Method        string `json:"method"`
	Path          string `json:"path"`
	UserID        string `json:"user_id"`
	UserEmail     string `json:"user_email"`
	UserExtension string `json:"user_extension"`
	Status        string `json:"status"`
	HTTPStatus    int    `json:"http_status,omitempty"`
	Error         string `json:"error,omitempty"`
}

type localNotCovered struct {
	Family string `json:"family"`
	Reason string `json:"reason"`
}

type localOffboardSummary struct {
	Planned        int `json:"planned"`
	Removed        int `json:"removed"`
	AlreadyRemoved int `json:"already_removed"`
	Failed         int `json:"failed"`
}

type localOffboardResult struct {
	Query           string                 `json:"query"`
	Mode            string                 `json:"mode"`
	OnError         string                 `json:"on_error"`
	Assessed        bool                   `json:"assessed"`
	MatchedBy       string                 `json:"matched_by,omitempty"`
	User            *localFootprintUser    `json:"user"`
	Steps           []localOffboardStep    `json:"steps"`
	Receipts        []localOffboardReceipt `json:"receipts"`
	Summary         localOffboardSummary   `json:"summary"`
	Refused         bool                   `json:"refused"`
	RefusalReasons  []string               `json:"refusal_reasons"`
	Unplannable     []string               `json:"unplannable"`
	NotCovered      []localNotCovered      `json:"not_covered"`
	SkippedLive     []string               `json:"skipped_live"`
	FetchFailures   []localFetchFailure    `json:"fetch_failures"`
	Truncated       []localTruncation      `json:"truncated"`
	DeviceConflicts []localDeviceConflict  `json:"device_conflicts"`
	MissingLocal    []string               `json:"missing_local"`
	MissingReasons  map[string]string      `json:"missing_reasons"`
	Note            string                 `json:"note,omitempty"`
}

// localOffboardNotCovered lists the membership families offboard does not
// discover or remove, so a plan is never mistaken for a complete cleanup.
var localOffboardNotCovered = []localNotCovered{
	{Family: "push_to_talk", Reason: "push-to-talk channel members (DELETE /phone/push_to_talk/channels/{channelId}/members/{extensionId}) are not discovered"},
	{Family: "private_directory", Reason: "private directory members (DELETE /phone/private_directory/members/{extensionId}) are not discovered"},
	{Family: "roles", Reason: "Zoom Phone admin role membership (/phone/roles/{roleId}/members) is not discovered"},
	{Family: "call_queue_managers", Reason: "the call queue member DELETE cannot unassign a call queue manager"},
	{Family: "shared_line_appearances", Reason: "shared line appearances granted to or by the user are not discovered"},
	{Family: "routing_targets", Reason: "auto receptionist, IVR and call-handling forwarding targets that point at this user are not changed"},
	{Family: "emergency_addresses", Reason: "the user's personal emergency addresses are not deleted"},
	{Family: "outbound_caller_id", Reason: "customized outbound caller ID numbers are not removed"},
}

// localBuildOffboardPlan orders removals: queues, shared line groups, pickup
// groups, monitoring groups, device extension assignments, phone numbers and
// the calling plan LAST. Paths are the spec DELETE operations.
func localBuildOffboardPlan(fp *localFootprint) ([]localOffboardStep, []string) {
	steps := make([]localOffboardStep, 0)
	unplannable := make([]string, 0)
	if fp == nil || fp.User == nil {
		return steps, unplannable
	}
	u := fp.User
	add := func(s localOffboardStep) {
		s.Step = len(steps) + 1
		s.Method = http.MethodDelete
		steps = append(steps, s)
	}
	for _, m := range fp.CallQueues {
		add(localOffboardStep{Family: localFamilyQueues, Action: fmt.Sprintf("remove from call queue %s", m.GroupName), TargetID: m.GroupID, TargetName: m.GroupName,
			Path: "/phone/call_queues/" + url.PathEscape(m.GroupID) + "/members/" + url.PathEscape(m.MemberID)})
	}
	for _, m := range fp.SharedLineGroups {
		add(localOffboardStep{Family: localFamilySLG, Action: fmt.Sprintf("remove from shared line group %s", m.GroupName), TargetID: m.GroupID, TargetName: m.GroupName,
			Path: "/phone/shared_line_groups/" + url.PathEscape(m.GroupID) + "/members/" + url.PathEscape(m.MemberID)})
	}
	for _, m := range fp.GroupCallPickup {
		if m.MemberExtensionID == "" {
			unplannable = append(unplannable, fmt.Sprintf("group call pickup %s: no extension id for the member", m.GroupName))
			continue
		}
		add(localOffboardStep{Family: localFamilyPickup, Action: fmt.Sprintf("remove from group call pickup %s", m.GroupName), TargetID: m.GroupID, TargetName: m.GroupName,
			Path: "/phone/group_call_pickup/" + url.PathEscape(m.GroupID) + "/members/" + url.PathEscape(m.MemberExtensionID)})
	}
	for _, m := range fp.MonitoringGroups {
		if m.MemberExtensionID == "" {
			unplannable = append(unplannable, fmt.Sprintf("monitoring group %s: no extension id for the member", m.GroupName))
			continue
		}
		add(localOffboardStep{Family: localFamilyMonitoring, Action: fmt.Sprintf("remove %s member from monitoring group %s", m.MemberType, m.GroupName), TargetID: m.GroupID, TargetName: m.GroupName,
			Path:  "/phone/monitoring_groups/" + url.PathEscape(m.GroupID) + "/monitor_members/" + url.PathEscape(m.MemberExtensionID),
			Query: map[string]string{"member_type": m.MemberType}})
	}
	for _, d := range fp.DeskPhones {
		ext := d.AssigneeExtensionID
		if ext == "" {
			ext = u.ExtensionID
		}
		if ext == "" {
			unplannable = append(unplannable, fmt.Sprintf("desk phone %s: no extension id for the user", d.DisplayName))
			continue
		}
		add(localOffboardStep{Family: "devices", Action: fmt.Sprintf("unassign desk phone %s (%s)", d.DisplayName, d.MacAddress), TargetID: d.ID, TargetName: d.DisplayName,
			Path: "/phone/devices/" + url.PathEscape(d.ID) + "/extensions/" + url.PathEscape(ext)})
	}
	for _, n := range fp.PhoneNumbers {
		id := n.ID
		if id == "" {
			id = n.Number
		}
		if id == "" {
			continue
		}
		add(localOffboardStep{Family: "phone_numbers", Action: fmt.Sprintf("unassign phone number %s", n.Number), TargetID: id, TargetName: n.Number,
			Path: "/phone/users/" + url.PathEscape(u.ID) + "/phone_numbers/" + url.PathEscape(id)})
	}
	for _, p := range fp.CallingPlans {
		if p.Type == "" {
			unplannable = append(unplannable, fmt.Sprintf("calling plan %s: no plan type", p.Name))
			continue
		}
		step := localOffboardStep{Family: "calling_plans", Action: fmt.Sprintf("unassign calling plan %s", p.Name), TargetID: p.Type, TargetName: p.Name,
			Path: "/phone/users/" + url.PathEscape(u.ID) + "/calling_plans/" + url.PathEscape(p.Type)}
		if p.BillingAccountID != "" {
			step.Query = map[string]string{"billing_account_id": p.BillingAccountID}
		}
		add(step)
	}
	return steps, unplannable
}

// localOffboardPlanDependencies are the local resources the plan reads
// beyond the live membership scan: the user row, the user's desk phones (the
// "assigned" device partition) and numbers.
var localOffboardPlanDependencies = []string{"users", "assigned", "numbers"}

// localIncompleteDiscovery lists why a plan cannot be trusted as complete.
// --apply refuses to send any DELETE while this is non-empty. unplannable
// holds memberships discovered without the id their DELETE needs.
func localIncompleteDiscovery(fp *localFootprint, unplannable []string) []string {
	reasons := make([]string, 0)
	if fp == nil {
		return append(reasons, "no local store: the user and their desk phones, numbers and calling plans were not discovered")
	}
	if fp.User == nil {
		reasons = append(reasons, "the user was not resolved from the local store")
	}
	for _, f := range fp.FetchFailures {
		target := f.Family
		if f.GroupName != "" || f.GroupID != "" {
			target = fmt.Sprintf("%s %q (%s)", f.Family, f.GroupName, f.GroupID)
		}
		reasons = append(reasons, fmt.Sprintf("membership fetch failed for %s: %s", target, f.Error))
	}
	if len(fp.SkippedLive) > 0 {
		reasons = append(reasons, "live membership discovery skipped for "+strings.Join(fp.SkippedLive, ", "))
	}
	for _, tr := range fp.Truncated {
		target := tr.Family
		if tr.GroupName != "" || tr.GroupID != "" {
			target = fmt.Sprintf("%s %q (%s)", tr.Family, tr.GroupName, tr.GroupID)
		}
		reasons = append(reasons, fmt.Sprintf("truncated %s: %s", target, tr.Reason))
	}
	for _, u := range unplannable {
		reasons = append(reasons, "unplannable step: "+u)
	}
	for _, dc := range fp.DeviceConflicts {
		reasons = append(reasons, fmt.Sprintf("device %q (%s) has conflicting %s rows synced at the same time; re-sync assigned,unassigned",
			dc.DisplayName, dc.DeviceID, strings.Join(dc.Sources, "/")))
	}
	missing := map[string]bool{}
	for _, m := range fp.MissingLocal {
		missing[m] = true
	}
	deps := make([]string, 0)
	for _, d := range localOffboardPlanDependencies {
		if missing[d] {
			deps = append(deps, d)
		}
	}
	if len(deps) > 0 {
		reasons = append(reasons, "local resources the plan depends on are not synced: "+strings.Join(deps, ", ")+"; run: "+localSyncCommand(deps))
		for _, d := range deps {
			if r := fp.MissingReasons[d]; strings.HasPrefix(r, "the latest completed sync was filtered") {
				reasons = append(reasons, d+": "+r)
			}
		}
	}
	return reasons
}

// localDeleteAlreadyGone reports a DELETE failure that means the membership
// is already gone, so a re-run converges. From the spec responses: HTTP 404
// (except error code 2001, account does not exist), and HTTP 400 whose body
// carries error code 404 ("Member does not exist", "The group does not
// exist") or says the member, extension or group does not exist / is not
// assigned. A 400 about a call queue manager or an invalid id stays failed.
func localDeleteAlreadyGone(status int, body string) bool {
	var parsed struct {
		Code    zoomFlexString `json:"code"`
		Message string         `json:"message"`
	}
	_ = json.Unmarshal([]byte(body), &parsed)
	lower := strings.ToLower(body + " " + parsed.Message)
	if parsed.Code.String() == "2001" || strings.Contains(lower, "account does not exist") {
		return false
	}
	switch status {
	case http.StatusNotFound:
		return true
	case http.StatusBadRequest:
		if strings.Contains(lower, "manager") || strings.Contains(lower, "invalid") {
			return false
		}
		if parsed.Code.String() == "404" {
			return true
		}
		for _, phrase := range []string{"does not exist", "not exist", "not found", "not assigned", "is not a member", "not a member of"} {
			if strings.Contains(lower, phrase) {
				return true
			}
		}
	}
	return false
}

// localVerifyUserLive re-reads the target user (GET /phone/users/{userId})
// immediately before any DELETE and confirms the email, extension id and
// extension number the plan was built from still belong to that user.
// Accounts are renamed and extension numbers reassigned, so a stale local row
// could otherwise aim the plan at someone else. The email is compared
// whenever the plan has one, and against the requested email when the query
// matched by email. Any mismatch or a failed read refuses the apply.
func localVerifyUserLive(ctx context.Context, c localGetter, u *localFootprintUser, matchedBy, query string) []string {
	reasons := make([]string, 0)
	if u == nil || u.ID == "" {
		return append(reasons, "no resolved user id to verify")
	}
	raw, err := c.Get(ctx, "/phone/users/"+url.PathEscape(u.ID), nil)
	if err != nil {
		return append(reasons, fmt.Sprintf("could not verify user %s live: %v", u.ID, err))
	}
	var live struct {
		ID              zoomFlexString `json:"id"`
		Email           zoomFlexString `json:"email"`
		ExtensionID     zoomFlexString `json:"extension_id"`
		ExtensionNumber zoomFlexString `json:"extension_number"`
	}
	if err := json.Unmarshal(raw, &live); err != nil {
		return append(reasons, fmt.Sprintf("could not decode live user %s: %v", u.ID, err))
	}
	if id := live.ID.String(); id != "" && id != u.ID {
		reasons = append(reasons, fmt.Sprintf("live user id is %s, plan was built for %s", id, u.ID))
	}
	liveEmail := live.Email.String()
	if u.Email != "" && !strings.EqualFold(liveEmail, u.Email) {
		reasons = append(reasons, fmt.Sprintf("live email of user %s is %q, plan was built for %q; re-sync users and re-plan", u.ID, liveEmail, u.Email))
	} else if matchedBy == localMatchEmail && !strings.EqualFold(liveEmail, strings.TrimSpace(query)) {
		reasons = append(reasons, fmt.Sprintf("live email of user %s is %q, not the requested %q", u.ID, liveEmail, strings.TrimSpace(query)))
	}
	if ext := live.ExtensionID.String(); ext != u.ExtensionID {
		reasons = append(reasons, fmt.Sprintf("live extension id is %q, plan was built for %q; re-sync users and re-plan", ext, u.ExtensionID))
	}
	if num := live.ExtensionNumber.String(); num != u.ExtensionNumber {
		reasons = append(reasons, fmt.Sprintf("live extension number is %q, plan was built for %q; re-sync users and re-plan", num, u.ExtensionNumber))
	}
	return reasons
}

// localApplyOffboard runs every step in order and continues past failures.
func localApplyOffboard(ctx context.Context, c *client.Client, u *localFootprintUser, steps []localOffboardStep) []localOffboardReceipt {
	receipts := make([]localOffboardReceipt, 0, len(steps))
	for _, s := range steps {
		r := localOffboardReceipt{Step: s.Step, Method: s.Method, Path: s.Path,
			UserID: u.ID, UserEmail: u.Email, UserExtension: u.ExtensionNumber}
		_, status, err := c.DeleteWithParams(ctx, s.Path, s.Query)
		r.HTTPStatus = status
		var apiE *client.APIError
		switch {
		case err == nil:
			r.Status = "removed"
		case errors.As(err, &apiE) && localDeleteAlreadyGone(apiE.StatusCode, apiE.Body):
			r.Status = "already_removed"
			r.HTTPStatus = apiE.StatusCode
		default:
			r.Status = "failed"
			r.Error = err.Error()
			if errors.As(err, &apiE) {
				r.HTTPStatus = apiE.StatusCode
			}
		}
		receipts = append(receipts, r)
	}
	return receipts
}

func newNovelOffboardCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var noLive bool
	var apply bool

	cmd := &cobra.Command{
		Use:   "offboard [user]",
		Short: "Plan the ordered removal of a departing user from queues, shared lines, groups, devices, numbers and calling plan; --apply executes it",
		Long: strings.TrimSpace(`
Use this command to remove a departing user from every call queue, shared line group, pickup and monitoring group, device, phone number and calling plan (dry-run plan by default, --apply to execute). Do NOT use this command to just look up what a user holds; use 'footprint' instead.

Discovery is the same join as 'footprint': the user, desk phones, numbers and
calling plans from the local store, group memberships read live. The plan
removes, in order: call queue members, shared line group members, group call
pickup members, monitoring group members, device extension assignments,
phone numbers, and the calling plan LAST.

Without --apply nothing is changed. --apply sends the DELETE requests
immediately, with no confirmation prompt (an MCP tool call sends them only
with confirm: true and otherwise returns the plan). Before using it, re-sync
(zoom-phone-cli sync --full --resources users,assigned,numbers) and
review the plan from a run without --apply. --apply accepts only an exact
user id, email or extension number (no name, substring or phone-number
match). Under --apply every group family is re-listed live (the synced
group lists and cached member counts are not trusted), and membership is
decided by user id / extension id, never by an extension number alone when
an id is present. It refuses (exit 5, nothing sent) when discovery is
incomplete: a failed membership fetch, skipped live discovery, a group or
member list cut off at its page cap (see truncated), a step that cannot be
planned (see unplannable), or an unsynced resource the plan depends on
(users, assigned devices, numbers). Immediately before the first DELETE it
re-reads the user live (GET /phone/users/{userId}) and refuses if the
extension id or number no longer matches the plan. refusal_reasons says
which check failed.

Every step runs in order and failures do not stop later steps (on_error:
continue). Each receipt echoes the user id, email and extension and has
status removed, already_removed (HTTP 404, or a 400 saying the member or
group no longer exists) or failed. Exit 6 when some steps failed, 5 when all
failed. not_covered lists the families this command does not touch
(push-to-talk, private directory, roles and more).`),
		Example: strings.Trim(`
  zoom-phone-cli offboard jane@example.com --agent
  zoom-phone-cli offboard jane@example.com --apply --json
  zoom-phone-cli offboard 1001 --no-live`, "\n"),
		Annotations: map[string]string{
			"pp:data-source":        "auto",
			"pp:happy-args":         "user=jane@example.com",
			"pp:preview-happy-path": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "offboard")
			}
			query := strings.TrimSpace(strings.Join(args, " "))
			if query == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a user is required: pass an email, extension number or name"))
			}
			skipLive := noLive || flags.dataSource == "local"
			if apply && skipLive {
				return usageErr(fmt.Errorf("--apply needs live membership discovery; drop --no-live / --data-source local"))
			}
			if apply && cliutil.IsAnyHarness() {
				return writeHarnessRefusal(cmd.OutOrStdout(), flags, "offboard --apply")
			}
			// zoom-phone hand-fix: zoom-phone-mcp-write-confirm. An MCP call
			// without confirm: true gets the plan, never the DELETEs.
			mcpPreview := apply && zoomMCPPreviewOnly(cmd)
			if mcpPreview {
				apply = false
			}

			res := &localOffboardResult{
				Query:           query,
				Mode:            "plan",
				OnError:         "continue",
				Steps:           make([]localOffboardStep, 0),
				Receipts:        make([]localOffboardReceipt, 0),
				RefusalReasons:  make([]string, 0),
				Unplannable:     make([]string, 0),
				NotCovered:      append([]localNotCovered(nil), localOffboardNotCovered...),
				SkippedLive:     make([]string, 0),
				FetchFailures:   make([]localFetchFailure, 0),
				Truncated:       make([]localTruncation, 0),
				DeviceConflicts: make([]localDeviceConflict, 0),
				MissingReasons:  map[string]string{},
				MissingLocal:    make([]string, 0),
			}
			if apply {
				res.Mode = "apply"
			}

			resolved, err := localResolveFootprintUser(cmd, dbPath, query, apply)
			if err != nil {
				return err
			}
			var fp *localFootprint
			notes := []string{}
			switch {
			case resolved.data == nil:
				fp = localUnassessedFootprint(query)
				res.MissingLocal = fp.MissingLocal
				res.MissingReasons = fp.MissingReasons
				notes = append(notes, fp.Note)
			case resolved.user == nil:
				res.MissingLocal = append(res.MissingLocal, resolved.data.missing...)
				res.MissingReasons = resolved.data.reasons
				res.Assessed = len(res.MissingLocal) == 0
				if resolved.note != "" {
					notes = append(notes, resolved.note)
				} else {
					notes = append(notes, "no phone users in the local store; run: "+localSyncCommand(localFootprintSyncResources))
				}
			default:
				res.MatchedBy = resolved.matchedBy
				// --apply re-lists every group family live and scans groups whose
				// cached member count is 0 (fresh discovery).
				fp = localBuildFootprint(cmd.Context(), cmd, flags, resolved.data, *resolved.user, query, skipLive, apply)
				res.User = fp.User
				res.SkippedLive = fp.SkippedLive
				res.FetchFailures = fp.FetchFailures
				res.Truncated = fp.Truncated
				res.DeviceConflicts = fp.DeviceConflicts
				res.MissingLocal = fp.MissingLocal
				res.MissingReasons = fp.MissingReasons
				res.Assessed = fp.Assessed
				res.Steps, res.Unplannable = localBuildOffboardPlan(fp)
				res.Summary.Planned = len(res.Steps)
				if fp.Note != "" {
					notes = append(notes, fp.Note)
				}
				if len(fp.SkippedLive) > 0 {
					notes = append(notes, "plan is incomplete: group memberships were not discovered")
				}
			}
			flags.agentSource = "local"
			if fp != nil && fp.liveRan {
				flags.agentSource = "live"
			}

			var exitErr error
			if apply {
				discoveryFP := fp
				if resolved.data != nil && resolved.user == nil {
					discoveryFP = &localFootprint{MissingLocal: res.MissingLocal, MissingReasons: res.MissingReasons}
				}
				if resolved.data == nil {
					discoveryFP = nil
				}
				if reasons := localIncompleteDiscovery(discoveryFP, res.Unplannable); len(reasons) > 0 {
					res.Refused = true
					res.RefusalReasons = reasons
					notes = append(notes, "--apply refused: discovery is incomplete; nothing was sent")
					exitErr = apiErr(fmt.Errorf("offboard --apply refused, discovery is incomplete (no DELETE sent): %s", strings.Join(reasons, "; ")))
				} else if len(res.Steps) > 0 {
					c, err := flags.newClient()
					if err != nil {
						return err
					}
					c.NoCache = true
					if reasons := localVerifyUserLive(cmd.Context(), c, res.User, res.MatchedBy, query); len(reasons) > 0 {
						res.Refused = true
						res.RefusalReasons = reasons
						notes = append(notes, "--apply refused: the live user no longer matches the plan; nothing was sent")
						exitErr = apiErr(fmt.Errorf("offboard --apply refused, live user check failed (no DELETE sent): %s", strings.Join(reasons, "; ")))
					} else {
						res.Receipts = localApplyOffboard(cmd.Context(), c, res.User, res.Steps)
						flags.agentSource = "live"
						for _, r := range res.Receipts {
							switch r.Status {
							case "removed":
								res.Summary.Removed++
							case "already_removed":
								res.Summary.AlreadyRemoved++
							default:
								res.Summary.Failed++
							}
						}
						switch {
						case res.Summary.Failed > 0 && res.Summary.Failed == len(res.Receipts):
							exitErr = apiErr(fmt.Errorf("offboard: all %d steps failed; see receipts", res.Summary.Failed))
						case res.Summary.Failed > 0:
							exitErr = partialFailureErr(fmt.Errorf("offboard: %d of %d steps failed; see receipts", res.Summary.Failed, len(res.Receipts)))
						}
					}
				}
			} else if mcpPreview {
				notes = append(notes, zoomMCPPreviewNote)
			} else if res.User != nil {
				notes = append(notes, "plan only: no changes made; re-sync, review this plan, then re-run with --apply and an exact id, email or extension to execute")
			}
			res.Note = strings.Join(notes, "; ")

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), res, flags); err != nil {
					return err
				}
			} else if err := localPrintOffboard(cmd, res); err != nil {
				return err
			}
			return exitErr
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite store path (default: the synced store for the active account)")
	cmd.Flags().BoolVar(&noLive, "no-live", false, "Skip the live membership fetch; the plan then omits queue, shared line, pickup and monitoring groups")
	cmd.Flags().BoolVar(&apply, "apply", false, "Send the plan's DELETE requests now (exact id, email or extension only); without it only the plan is printed")
	return cmd
}

func localPrintOffboard(cmd *cobra.Command, res *localOffboardResult) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 4, 2, ' ', 0)
	if res.User == nil {
		fmt.Fprintf(w, "No phone user found for %q.\n", res.Query)
		for _, r := range res.RefusalReasons {
			fmt.Fprintf(w, "refused: %s\n", r)
		}
		if res.Note != "" {
			fmt.Fprintln(w, res.Note)
		}
		return w.Flush()
	}
	fmt.Fprintf(w, "Offboard %s <%s> ext %s, user id %s (%s, matched by %s)\n\n", res.User.Name, res.User.Email, res.User.ExtensionNumber, res.User.ID, res.Mode, res.MatchedBy)
	if res.Refused {
		fmt.Fprintln(w, "REFUSED: --apply sent nothing because discovery is incomplete:")
		for _, r := range res.RefusalReasons {
			fmt.Fprintf(w, "  - %s\n", r)
		}
		fmt.Fprintln(w)
	}
	receipts := map[int]localOffboardReceipt{}
	for _, r := range res.Receipts {
		receipts[r.Step] = r
	}
	fmt.Fprintln(w, "STEP\tACTION\tMETHOD\tPATH\tRESULT")
	for _, s := range res.Steps {
		result := "planned"
		if r, ok := receipts[s.Step]; ok {
			result = r.Status
			if r.Error != "" {
				result += ": " + r.Error
			}
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", s.Step, s.Action, s.Method, s.Path, result)
	}
	if len(res.Steps) == 0 {
		fmt.Fprintln(w, "-\tnothing to remove\t\t\t")
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Not covered:")
	for _, nc := range res.NotCovered {
		fmt.Fprintf(w, "  %s\t%s\n", nc.Family, nc.Reason)
	}
	if res.Note != "" {
		fmt.Fprintf(w, "\nnote: %s\n", res.Note)
	}
	return w.Flush()
}
