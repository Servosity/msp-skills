// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// --wait for the generated Avanan action commands.
//
// The generated `action post-entity` and `action post-event` offer --wait, but
// the press's generic job path cannot observe an Avanan task. It reads a task
// ID only at the top level of the body (or under data/job/result), while the
// documented action response is {responseEnvelope, responseData:[{taskId,
// entityId}]} (spec SaasEntityActionResponse / EventActionResponse). The ID was
// never found, nothing was polled, and --wait exited 0 the moment the action
// was accepted, so an operator read "finished" for a quarantine that had only
// been queued. Even with an ID in hand, the generic poller substitutes only
// {id} and {job_id} (the task path is /v1.0/task/{task_id}), sends no
// query-string scope (live-verified: a multi-scope credential gets HTTP 400 on
// every task poll without it) and reads status from the top level of an
// enveloped response.
//
// This helper replaces that path for these commands and FAILS CLOSED: --wait
// returns success only after every task the API returned reached a terminal
// success state. No task ID, a poll that cannot be made, a task that does not
// go terminal, or a terminal failure each return a non-zero exit.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"avanan-pp-cli/internal/avananmirror"
	"github.com/spf13/cobra"
)

// avananTaskSucceededStates is the subset of terminalTaskStates that means the
// action was applied. Every other terminal state is a failure.
var avananTaskSucceededStates = map[string]bool{
	"done": true, "completed": true, "success": true, "succeeded": true,
}

// avananTaskRef is one responseData record of an action response: the task
// and the target it belongs to (spec SaasEntityActionTask / EventActionTask).
type avananTaskRef struct {
	TaskID   string
	EntityID string
	EventID  string
}

// avananTaskRefs reads every task record of a decoded action response. It is
// the one record parser for action responses, shared by `remediate` and the
// action commands' --wait; it keeps remediate's original signals (taskId,
// task_id, id). EVERY record is returned: one whose task is null (Google Mail,
// live-verified) or that cannot be read comes back with an empty TaskID, a
// target nothing can be polled for. Callers must never count it as done.
func avananTaskRefs(env *avananmirror.Envelope) []avananTaskRef {
	records, err := env.Records()
	if err != nil {
		return nil
	}
	var out []avananTaskRef
	for _, r := range records {
		var obj map[string]any
		if json.Unmarshal(r, &obj) != nil {
			out = append(out, avananTaskRef{})
			continue
		}
		out = append(out, avananTaskRef{TaskID: str(obj, "taskId", "task_id", "id"), EntityID: str(obj, "entityId"), EventID: str(obj, "eventId")})
	}
	return out
}

// label names a ref's target for an operator.
func (r avananTaskRef) label(i int) string {
	switch {
	case r.EntityID != "":
		return r.EntityID
	case r.EventID != "":
		return r.EventID
	}
	return fmt.Sprintf("record %d", i+1)
}

// avananTaskIDs returns every distinct task ID in an action response, in
// response order.
//
// It keeps every signal the generic ExtractJobID reads (a top-level taskId,
// or one under data, job or result) and adds the documented Avanan shape
// through avananTaskRefs. A batch can return one task per target, so all of
// them are returned, not just the first. untasked names every record that
// carried no task. An envelope that reports failure is an error.
func avananTaskIDs(data []byte) (ids, untasked []string, err error) {
	seen := map[string]bool{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		ids = append(ids, id)
	}
	add(ExtractJobID(data, "taskId"))
	add(ExtractJobID(data, "task_id"))

	env, err := avananmirror.Decode(data)
	if err != nil {
		return ids, nil, err
	}
	if _, err := env.Records(); err != nil {
		return ids, nil, err
	}
	for i, ref := range avananTaskRefs(env) {
		if strings.TrimSpace(ref.TaskID) == "" {
			untasked = append(untasked, ref.label(i))
			continue
		}
		add(ref.TaskID)
	}
	return ids, untasked, nil
}

// avananTaskSucceeded reports whether a terminal task status means the action
// was applied.
func avananTaskSucceeded(status string) bool {
	return avananTaskSucceededStates[strings.ToLower(strings.TrimSpace(status))]
}

// avananActionScope returns the single scope the action was submitted in,
// which the task endpoint needs back as a query-string parameter.
// requestData.scope is the documented action field; a single-valued scopes
// header is the fallback.
func avananActionScope(body any, scopesHeader string) string {
	if m, ok := body.(map[string]any); ok {
		if rd, ok := m["requestData"].(map[string]any); ok {
			if s, ok := rd["scope"].(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	h := strings.TrimSpace(scopesHeader)
	if h != "" && !strings.ContainsAny(h, ", ;") {
		return h
	}
	return ""
}

type avananActionWaitTask struct {
	TaskID    string `json:"task_id"`
	Target    string `json:"target,omitempty"`
	Status    string `json:"status"`
	Detail    string `json:"detail,omitempty"`
	Completed bool   `json:"completed"`
	Succeeded bool   `json:"succeeded"`
}

type avananActionWaitReport struct {
	Endpoint  string                 `json:"endpoint"`
	Scope     string                 `json:"scope,omitempty"`
	Succeeded bool                   `json:"succeeded"`
	Tasks     []avananActionWaitTask `json:"tasks"`
}

// avananTaskGetter is the read the poll needs; the real client satisfies it.
type avananTaskGetter interface {
	Get(ctx context.Context, path string, params map[string]string) (json.RawMessage, error)
}

// avananActionWait polls every task an action response returned until each is
// terminal, prints the outcome, and returns a non-nil error unless every task
// reached a terminal success state.
func avananActionWait(cmd *cobra.Command, flags *rootFlags, c avananTaskGetter, endpoint string, data []byte, body any, scopesHeader string, timeout time.Duration) error {
	scope := avananActionScope(body, scopesHeader)
	recoveryCmd := func(id string) string {
		r := "avanan-cli task " + id
		if scope != "" {
			r += " --scope " + scope
		}
		return r
	}

	ids, untasked, err := avananTaskIDs(data)
	if err != nil {
		// An envelope that reports failure means the action itself failed.
		return apiErr(fmt.Errorf("action %s: %w", endpoint, err))
	}
	if len(ids) == 0 {
		// Google Mail returns taskId null. A 200 is not proof the change has
		// landed (measured 2026-08-22: still not quarantined right after), so
		// --wait cannot report success. The action WAS submitted: say so, so
		// nobody resubmits it.
		msg := "the action was accepted but the response carried no task ID, so --wait has nothing to poll and cannot confirm the outcome.\n" +
			"Do not resubmit. Check the target's own state instead, e.g. 'avanan-cli avanan-search get-saas-entity <entity_id>' (isQuarantined / isRestored)."
		if flags != nil && flags.asJSON {
			_ = json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
				"endpoint": endpoint,
				"status":   "submitted",
				"error":    msg,
				"code":     ExitJobPending,
			})
		}
		return &cliError{code: ExitJobPending, err: errors.New(msg)}
	}

	// The submission itself succeeded (the transport already turned an
	// envelope failure into an error), so keep the store in step with the
	// generic path before polling.
	writeMutationResponseToStore(cmd.Context(), "action", data, "responseData")

	if timeout <= 0 {
		// The flag documents 0 as "no timeout"; bound it at a day so a lost
		// task cannot hang an unattended run forever.
		timeout = 24 * time.Hour
	}
	submittedAt := time.Now().UTC()
	for _, id := range ids {
		_ = RecordJob(JobRow{
			JobID:          id,
			Resource:       "action",
			Endpoint:       endpoint,
			Status:         "submitted",
			SubmittedAt:    submittedAt,
			StatusResource: "task",
			StatusEndpoint: "get",
		})
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	report := avananActionWaitReport{Endpoint: endpoint, Scope: scope, Succeeded: true}
	var pending, failed []string
	for _, id := range ids {
		status, detail, completed := pollTask(pollCtx, c, id, scope, timeout)
		ok := completed && avananTaskSucceeded(status)
		report.Tasks = append(report.Tasks, avananActionWaitTask{
			TaskID: id, Status: status, Detail: detail, Completed: completed, Succeeded: ok,
		})
		_ = RecordJob(JobRow{Resource: "action", Endpoint: endpoint, JobID: id, Status: status})
		if !ok {
			report.Succeeded = false
		}
		switch {
		case !completed:
			pending = append(pending, id)
		case !ok:
			failed = append(failed, id+"="+status)
		}
	}
	// A target with no task was never observed: it can never count as done.
	for _, target := range untasked {
		report.Succeeded = false
		report.Tasks = append(report.Tasks, avananActionWaitTask{
			Target: target, Status: "no task", Detail: "the API returned no task for this target",
		})
	}

	if perr := avananPrintActionWait(cmd, flags, report); perr != nil {
		return perr
	}
	if len(pending) > 0 {
		werr := fmt.Errorf("task(s) %s did not reach a terminal state within %s", strings.Join(pending, ", "), timeout)
		return &cliError{code: ExitJobPending, err: &AsyncJobPendingError{
			JobID:           strings.Join(pending, ","),
			RecoveryCommand: recoveryCmd(pending[0]),
			Err:             werr,
		}}
	}
	if len(untasked) > 0 {
		return &cliError{code: ExitJobPending, err: fmt.Errorf(
			"the action was accepted, but the API returned no task for target(s) %s, so --wait cannot confirm them.\n"+
				"Do not resubmit. Check each target's own state, e.g. 'avanan-cli avanan-search get-saas-entity <entity_id>' (isQuarantined / isRestored).",
			strings.Join(untasked, ", "))}
	}
	if len(failed) > 0 {
		return apiErr(fmt.Errorf("action %s did not succeed: %s", endpoint, strings.Join(failed, ", ")))
	}
	return nil
}

func avananPrintActionWait(cmd *cobra.Command, flags *rootFlags, report avananActionWaitReport) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), report, flags)
	}
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s", report.Endpoint)
	if report.Scope != "" {
		fmt.Fprintf(w, "  scope=%s", report.Scope)
	}
	fmt.Fprintln(w)
	for _, t := range report.Tasks {
		fmt.Fprintf(w, "  task %-36s %s %s\n", orDefault(t.TaskID, "target "+t.Target), t.Status, t.Detail)
	}
	return nil
}
