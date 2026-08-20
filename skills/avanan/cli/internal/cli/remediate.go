// Copyright 2026 geekbrownbear and contributors. Licensed under Apache-2.0. See LICENSE.

// `remediate` — quarantine or restore, then wait for the real outcome.
//
// pp:data-source live
//
// Two things make this more than a wrapper around POST /v1.0/action/entity.
//
// First, the action endpoints accept exactly one scope. A credential bound to
// several tenants gets HTTP 400 with a body that does not explain why — the
// vendor's own docs call this the most common integration mistake. This
// command resolves the scope, and when it cannot, it fails with the actual
// scope list instead of the bare 400.
//
// Second, actions are asynchronous: the API returns a task ID and nothing
// else. Every other tool hands that ID back and walks away. This command polls
// to a terminal state and reports per-item outcomes, and records the
// task-to-entity link the API never returns so `timeline` can use it later.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"avanan-pp-cli/internal/avananmirror"
	"avanan-pp-cli/internal/cliutil"
	"avanan-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

// avananActionResource is the local resource type holding the action log. It
// exists purely to supply the task-to-entity linkage the API omits.
const avananActionResource = "avanan_actions"

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		addNovelCommandIfAbsent(root, newNovelRemediateCmd(flags))
	})
}

type remediateOutcome struct {
	TargetID string `json:"target_id"`
	Status   string `json:"status"`
	Detail   string `json:"detail,omitempty"`
}

type remediateReport struct {
	Action     string             `json:"action"`
	TargetKind string             `json:"target_kind"`
	Scope      string             `json:"scope,omitempty"`
	TaskID     string             `json:"task_id,omitempty"`
	Waited     bool               `json:"waited"`
	Completed  bool               `json:"completed"`
	Outcomes   []remediateOutcome `json:"outcomes"`
	Note       string             `json:"note,omitempty"`
}

func newNovelRemediateCmd(flags *rootFlags) *cobra.Command {
	var (
		entities []string
		events   []string
		scope    string
		wait     bool
		timeout  string
		reason   string
	)

	cmd := &cobra.Command{
		Use:   "remediate <quarantine|restore>",
		Short: "Quarantine or restore a batch, wait for the async task to finish, and report the real per-item outcome",
		Long: strings.Trim(`
Submit a quarantine or restore and report what actually happened.

Avanan's action endpoints are asynchronous: they return a task ID, and the
outcome only appears when that task reaches a terminal state. This command
polls for you and prints the per-item result.

The action endpoints also accept exactly one scope. If your credential reaches
more than one tenant and you do not pass --scope, this command fails with the
list of scopes you can choose from rather than the bare HTTP 400 the API
returns.

Use this command to quarantine or restore a batch and wait for the real
per-item outcome. Do NOT use this command when you want fire-and-forget
submission returning a task ID; use 'action post-entity' or 'action post-event',
then 'task <task_id>'.
`, "\n"),
		Example: strings.Trim(`
  avanan-cli remediate quarantine --entity f05b74da3ee859eea41aeac40aaad3c2 --dry-run
  avanan-cli remediate quarantine --entity f05b74da3ee859eea41aeac40aaad3c2 --wait
  avanan-cli remediate restore --event ebb3e4bc8a9b14d7a529bb54ea6991b6 --scope farm1:tenant-a --wait
`, "\n"),
		Annotations: map[string]string{
			"pp:happy-args":       "<action>=quarantine;--entity=f05b74da3ee859eea41aeac40aaad3c2",
			"pp:typed-exit-codes": "0,2",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				// --dry-run advertises "show request without sending", so show
				// the request: which action, which targets, which endpoint, and
				// which scope. A bare "would run remediate" tells the caller
				// nothing they did not already type, and the targets and scope
				// are exactly what they need to check before quarantining mail.
				return writeRemediateDryRun(cmd, flags, args, entities, events, scope)
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("an action is required: quarantine or restore"))
			}

			action := strings.ToLower(strings.TrimSpace(args[0]))
			if action != "quarantine" && action != "restore" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("unknown action %q; valid actions are quarantine and restore", action))
			}
			if len(entities) == 0 && len(events) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("at least one --entity or --event ID is required"))
			}
			if len(entities) > 0 && len(events) > 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("pass either --entity or --event IDs, not both: they route to different endpoints"))
			}

			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			waitFor := 2 * time.Minute
			if timeout != "" {
				d, err := cliutil.ParseDurationLoose(timeout)
				if err != nil {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("invalid --wait-timeout %q: %w", timeout, err))
				}
				waitFor = d
			}
			if cliutil.IsDogfoodEnv() && waitFor > 10*time.Second {
				// The live matrix runs every command under a flat per-command
				// timeout; a full remediation wait would exceed it.
				waitFor = 10 * time.Second
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			report := remediateReport{
				Action:   action,
				Waited:   wait,
				Outcomes: []remediateOutcome{},
			}
			targets := entities
			path := "/v1.0/action/entity"
			report.TargetKind = "entity"
			if len(events) > 0 {
				targets = events
				path = "/v1.0/action/event"
				report.TargetKind = "event"
			}

			resolvedScope, err := resolveActionScope(ctx, cmd, flags, c, scope)
			if err != nil {
				return err
			}
			report.Scope = resolvedScope

			requestData := map[string]any{
				"entityIds": targets,
			}
			if report.TargetKind == "event" {
				delete(requestData, "entityIds")
				requestData["eventIds"] = targets
			} else {
				requestData["entityType"] = "email"
			}
			if action == "quarantine" {
				requestData["entityActionName"] = "quarantine"
			} else {
				requestData["entityActionName"] = "restore"
			}
			if reason != "" {
				requestData["restoreDeclineReason"] = reason
			}
			if resolvedScope != "" {
				requestData["scope"] = resolvedScope
			}

			raw, _, err := c.Post(ctx, path, map[string]any{"requestData": requestData})
			if err != nil {
				return fmt.Errorf("submitting %s: %w", action, err)
			}
			env, err := avananmirror.Decode(raw)
			if err != nil {
				return fmt.Errorf("submitting %s: %w", action, err)
			}

			report.TaskID = extractTaskID(env)
			for _, t := range targets {
				report.Outcomes = append(report.Outcomes, remediateOutcome{TargetID: t, Status: "submitted"})
			}

			if report.TaskID == "" {
				report.Note = "the API accepted the action but returned no task ID, so the outcome cannot be polled"
			} else if wait {
				// Poll on a context derived from the ORIGINAL command context,
				// not the --timeout-bounded one. --timeout bounds a single
				// request; --wait-timeout bounds how long we watch an
				// already-submitted task. Reusing the bounded ctx would cut a
				// 2m wait short at the 60s default and then report a duration
				// that never elapsed.
				pollCtx, pollCancel := context.WithTimeout(cmd.Context(), waitFor)
				status, detail, completed := pollTask(pollCtx, c, report.TaskID, waitFor)
				pollCancel()
				report.Completed = completed
				for i := range report.Outcomes {
					report.Outcomes[i].Status = status
					report.Outcomes[i].Detail = detail
				}
				if !completed {
					report.Note = fmt.Sprintf(
						"task %s had not reached a terminal state after %s; re-check with 'avanan-cli task %s'",
						report.TaskID, waitFor, report.TaskID)
				}
			} else {
				report.Note = fmt.Sprintf("submitted as task %s; pass --wait to poll for the outcome", report.TaskID)
			}

			// Deliberately not the command context: it may already be expired
			// after a long --wait, and losing the task→entity link is what
			// makes `timeline` blind to the remediation.
			logCtx, logCancel := context.WithTimeout(context.WithoutCancel(cmd.Context()), 10*time.Second)
			recordActionLog(logCtx, cmd, flags, report, targets)
			logCancel()

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), report, flags)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%s: %d %s target(s)", action, len(targets), report.TargetKind)
			if report.Scope != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  scope=%s", report.Scope)
			}
			fmt.Fprintln(cmd.OutOrStdout())
			if report.TaskID != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "task: %s\n", report.TaskID)
			}
			fmt.Fprintln(cmd.OutOrStdout())
			for _, o := range report.Outcomes {
				fmt.Fprintf(cmd.OutOrStdout(), "  %-36s %s %s\n", o.TargetID, o.Status, o.Detail)
			}
			if report.Note != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "\n%s\n", report.Note)
			}
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&entities, "entity", nil, "Entity ID to act on (repeatable)")
	cmd.Flags().StringSliceVar(&events, "event", nil, "Event ID to act on (repeatable)")
	cmd.Flags().StringVar(&scope, "scope", "", "The single {farm}:{tenant} scope to act in (required when the credential reaches more than one)")
	cmd.Flags().BoolVar(&wait, "wait", false, "Poll the resulting task until it reaches a terminal state")
	cmd.Flags().StringVar(&timeout, "wait-timeout", "", "How long to poll when --wait is set (default 2m)")
	cmd.Flags().StringVar(&reason, "reason", "", "Decline reason, when restoring a denied restore request")
	return cmd
}

// resolveActionScope enforces the single-scope rule before the request is sent.
//
// This guard FAILS CLOSED. Submitting a quarantine without a scope lets the
// server pick a default, which on a multi-tenant credential can remove mail
// from the wrong customer's mailboxes — silently, and with no way to tell from
// the response that it happened. So anything short of proof that exactly one
// scope applies is refused, and the user is told to pass --scope.
//
// Being unable to reach /v1.0/scopes is not evidence of single-tenancy, and
// neither is an unrecognized response shape. Both refuse.
func resolveActionScope(ctx context.Context, cmd *cobra.Command, flags *rootFlags, c interface {
	Get(ctx context.Context, path string, params map[string]string) (json.RawMessage, error)
}, explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}

	refuse := func(reason string) error {
		return usageErr(fmt.Errorf(
			"cannot confirm which tenant this action would apply to (%s)\n"+
				"action endpoints accept exactly one scope, and submitting without one lets the server\n"+
				"choose — which on a multi-tenant credential can act on the wrong customer's mail.\n"+
				"pass --scope <farm>:<tenant> explicitly, or run 'avanan-cli scopes' to list them",
			reason))
	}

	raw, err := c.Get(ctx, "/v1.0/scopes", nil)
	if err != nil {
		return "", refuse(fmt.Sprintf("listing scopes failed: %v", err))
	}
	env, err := avananmirror.Decode(raw)
	if err != nil {
		return "", refuse(fmt.Sprintf("scope list could not be read: %v", err))
	}

	scopes := extractScopes(env)
	switch len(scopes) {
	case 0:
		return "", refuse("the scope list came back empty or in an unrecognized shape")
	case 1:
		return scopes[0], nil
	default:
		return "", usageErr(fmt.Errorf(
			"this credential reaches %d scopes, and action endpoints accept exactly one\n"+
				"pass --scope with one of:\n  %s",
			len(scopes), strings.Join(scopes, "\n  ")))
	}
}

func extractScopes(env *avananmirror.Envelope) []string {
	records, err := env.Records()
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(records))
	for _, r := range records {
		var s string
		if err := json.Unmarshal(r, &s); err == nil && s != "" {
			out = append(out, s)
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(r, &obj); err == nil {
			if v := str(obj, "scope", "name", "id"); v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}

func extractTaskID(env *avananmirror.Envelope) string {
	records, err := env.Records()
	if err != nil {
		return ""
	}
	for _, r := range records {
		var obj map[string]any
		if err := json.Unmarshal(r, &obj); err != nil {
			continue
		}
		if id := str(obj, "taskId", "task_id", "id"); id != "" {
			return id
		}
	}
	return ""
}

// terminalTaskStates are the task states that mean "stop polling".
var terminalTaskStates = map[string]bool{
	"done": true, "completed": true, "success": true, "succeeded": true,
	"failed": true, "error": true, "partial": true, "canceled": true, "cancelled": true,
}

// pollTask waits for a task to reach a terminal state, backing off between
// checks so a slow remediation does not hammer a rate-limited API.
func pollTask(ctx context.Context, c interface {
	Get(ctx context.Context, path string, params map[string]string) (json.RawMessage, error)
}, taskID string, within time.Duration) (status, detail string, completed bool) {
	deadline := time.Now().Add(within)
	backoff := 2 * time.Second
	status = "pending"

	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return status, "polling canceled", false
		}
		raw, err := c.Get(ctx, "/v1.0/task/"+taskID, nil)
		if err == nil {
			if env, derr := avananmirror.Decode(raw); derr == nil {
				if records, rerr := env.Records(); rerr == nil {
					for _, r := range records {
						var obj map[string]any
						if json.Unmarshal(r, &obj) != nil {
							continue
						}
						if s := str(obj, "status", "state", "taskState"); s != "" {
							status = s
							detail = str(obj, "responseText", "message", "detail")
							if terminalTaskStates[strings.ToLower(s)] {
								return status, detail, true
							}
						}
					}
				}
			}
		}

		select {
		case <-ctx.Done():
			return status, "polling canceled", false
		case <-time.After(backoff):
		}
		if backoff < 15*time.Second {
			backoff *= 2
		}
	}
	return status, detail, false
}

// recordActionLog persists the task-to-entity link the API never returns.
// Best-effort: a local bookkeeping failure must not mask a remediation that
// actually succeeded upstream.
func recordActionLog(ctx context.Context, cmd *cobra.Command, flags *rootFlags, report remediateReport, targets []string) {
	if report.TaskID == "" {
		return
	}
	dbPath := defaultDBPath("avanan-cli")
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not record the action locally (%v); 'timeline' will not show it\n", err)
		return
	}
	defer db.Close()

	entry := map[string]any{
		"task_id":      report.TaskID,
		"action":       report.Action,
		"target_kind":  report.TargetKind,
		"entity_ids":   targets,
		"scope":        report.Scope,
		"submitted_at": time.Now().UTC().Format(time.RFC3339),
	}
	if report.Completed && len(report.Outcomes) > 0 {
		entry["outcome"] = report.Outcomes[0].Status
		entry["completed_at"] = time.Now().UTC().Format(time.RFC3339)
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return
	}
	if err := db.Upsert(avananActionResource, report.TaskID, encoded); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not record the action locally (%v); 'timeline' will not show it\n", err)
	}
}

// remediateDryRun is the preview payload for --dry-run.
type remediateDryRun struct {
	DryRun     bool     `json:"dry_run"`
	Action     string   `json:"action"`
	TargetKind string   `json:"target_kind"`
	Targets    []string `json:"targets"`
	Endpoint   string   `json:"endpoint"`
	Scope      string   `json:"scope,omitempty"`
	ScopeNote  string   `json:"scope_note,omitempty"`
	Would      string   `json:"would"`
}

// writeRemediateDryRun previews the request without contacting the API.
//
// Scope is only reported when the user pinned it explicitly: resolving it
// requires calling /v1.0/scopes, and a dry run must not reach the network.
func writeRemediateDryRun(cmd *cobra.Command, flags *rootFlags, args, entities, events []string, scope string) error {
	action := "remediate"
	if len(args) > 0 {
		action = strings.ToLower(strings.TrimSpace(args[0]))
	}

	out := remediateDryRun{
		DryRun:     true,
		Action:     action,
		TargetKind: "entity",
		Targets:    entities,
		Endpoint:   "POST /v1.0/action/entity",
		Scope:      scope,
	}
	if len(events) > 0 {
		out.TargetKind = "event"
		out.Targets = events
		out.Endpoint = "POST /v1.0/action/event"
	}
	if out.Targets == nil {
		out.Targets = []string{}
	}
	if scope == "" {
		out.ScopeNote = "not resolved in dry-run: scope discovery requires a live call to /v1.0/scopes"
	}
	out.Would = fmt.Sprintf("%s %d %s target(s) via %s", action, len(out.Targets), out.TargetKind, out.Endpoint)

	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), out, flags)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "dry-run: %s\n", out.Would)
	for _, t := range out.Targets {
		fmt.Fprintf(cmd.OutOrStdout(), "  target: %s\n", t)
	}
	if out.Scope != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  scope:  %s\n", out.Scope)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "  scope:  %s\n", out.ScopeNote)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "no changes made")
	return nil
}
