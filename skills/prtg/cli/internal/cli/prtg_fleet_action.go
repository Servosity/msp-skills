// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: preview-then-act engine behind fleet ack, pause and resume.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"prtg-pp-cli/internal/cliutil"
	"prtg-pp-cli/internal/fleet"
)

// fleetAction is one filter-based bulk action sent to every core in scope.
type fleetAction struct {
	name string         // command name for dry-run and refusal text
	verb string         // "acknowledge", "pause", "resume": also the POST path segment
	kind string         // plural object kind: sensors, devices, groups, probes
	body map[string]any // request body; "filter" is added per run
	scan bool           // after the action, scan now each object it succeeded on
	// includeIndefinite widens resume from time-limited pauses to pauses
	// with no end time as well.
	includeIndefinite bool
	filter            string
}

// ActionRow is one core's preview or result.
type ActionRow struct {
	Core      string   `json:"core"`
	Action    string   `json:"action"`
	Kind      string   `json:"kind"`
	Filter    string   `json:"filter,omitempty"`
	Matched   *int     `json:"matched,omitempty"`
	Sample    []string `json:"sample,omitempty"`
	Sent      bool     `json:"sent"`
	Succeeded *int     `json:"succeeded,omitempty"`
	Failed    *int     `json:"failed,omitempty"`
	Failures  []string `json:"failures,omitempty"`
	Scanned   *bool    `json:"scanned,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// parseKind accepts sensor(s), device(s), group(s) or probe(s).
func parseKind(v string) (string, error) {
	k := strings.ToLower(strings.TrimSpace(v))
	k = strings.TrimSuffix(k, "s")
	switch k {
	case "sensor", "device", "group", "probe":
		return k + "s", nil
	}
	return "", fmt.Errorf("--kind %q: use sensor, device, group or probe", v)
}

// durationSeconds converts a --duration flag (0 or empty = indefinite) into
// the seconds PRTG expects; PRTG rounds up to whole minutes.
func durationSeconds(v string) (float64, error) {
	v = strings.TrimSpace(v)
	if v == "" || v == "0" {
		return 0, nil
	}
	d, err := cliutil.ParseDurationLoose(v)
	if err != nil || d < 0 {
		return 0, usageErr(fmt.Errorf("--duration %q: use a duration like 30m, 2h or 1d (0 = indefinite)", v))
	}
	// Fractional seconds as given: rounding a short duration down to 0 would
	// turn it into an indefinite acknowledgement or pause.
	return d.Seconds(), nil
}

// validateFilter checks that a filter is one complete expression before it
// is wrapped in parentheses and combined with a status restriction:
// balanced parentheses outside double-quoted text, and every string closed.
// Otherwise `a) or (b` would break out of the wrapper and escape the
// restriction. A string ends at the next double quote (PRTG documents no
// escapes), which is never later than where PRTG ends it, so a parenthesis
// counted here as outside a string is outside for PRTG too; a backslash is an
// ordinary character (Windows paths like "C:\").
func validateFilter(f string) error {
	depth, inString := 0, false
	for _, r := range f {
		switch {
		case inString && r == '"':
			inString = false
		case inString:
		case r == '"':
			inString = true
		case r == '(':
			depth++
		case r == ')':
			depth--
			if depth < 0 {
				return fmt.Errorf("--filter %q has a ')' without a matching '('", f)
			}
		}
	}
	if inString {
		return fmt.Errorf("--filter %q has an unterminated string", f)
	}
	if depth != 0 {
		return fmt.Errorf("--filter %q has an unclosed '('", f)
	}
	return nil
}

// statusTokens maps the PRTG filter tokens the actions use to the status
// values the API returns, so the filter scope and the preview cross-check
// are built from one table and cannot drift apart.
var statusTokens = map[string]string{
	"down": "DOWN", "partialdown": "DOWN_PARTIAL",
	"pausedbyuser": "PAUSED_BY_USER", "pausedbyuseruntil": "PAUSED_BY_USER_UNTIL",
}

// actionScope is the status restriction of one action: the objects whose
// status is one of tokens (or, with negate, is none of them).
type actionScope struct {
	tokens []string
	negate bool
}

// scope narrows each action to the objects it should change: acknowledge
// only down sensors (never re-acknowledging another technician's
// acknowledgement), pause only objects a person has not paused already (so a
// 2h pause cannot turn an indefinite pause into one that auto-resumes),
// resume only time-limited pauses unless --include-indefinite.
func (a fleetAction) scope() (actionScope, bool) {
	switch a.verb {
	case "acknowledge":
		return actionScope{tokens: []string{"down", "partialdown"}}, true
	case "pause":
		return actionScope{tokens: []string{"pausedbyuser", "pausedbyuseruntil"}, negate: true}, true
	case "resume":
		if a.includeIndefinite {
			return actionScope{tokens: []string{"pausedbyuser", "pausedbyuseruntil"}}, true
		}
		return actionScope{tokens: []string{"pausedbyuseruntil"}}, true
	}
	return actionScope{}, false
}

// scopedFilter combines the caller's filter with the action's scope using the
// documented status tokens and logical operators.
func (a fleetAction) scopedFilter() string {
	sc, ok := a.scope()
	if !ok {
		return a.filter
	}
	clauses := make([]string, len(sc.tokens))
	for i, t := range sc.tokens {
		clauses[i] = "status = " + t
	}
	restriction := "(" + strings.Join(clauses, " or ") + ")"
	if sc.negate {
		restriction = "not" + restriction
	}
	return "(" + a.filter + ") and " + restriction
}

// inScope reports whether a previewed object's status is one the action may
// change. The preview is cross-checked with it: a core that did not apply the
// status restriction (or returned an object with no status) is not acted on.
func (a fleetAction) inScope(status string) bool {
	if status == "" {
		return false
	}
	sc, ok := a.scope()
	if !ok {
		return true
	}
	listed := false
	for _, t := range sc.tokens {
		if statusTokens[t] == status {
			listed = true
		}
	}
	return listed != sc.negate
}

// relationshipPrefix matches PRTG's hierarchical filter form
// (<relationship> <filter>), which the API documents only as a prefix; it
// cannot be wrapped and combined with a status restriction.
var relationshipPrefix = regexp.MustCompile(`(?i)^\s*(ancestors|descendants|parents|children)\b`)

// run previews the action on every core and, with --yes, sends it to the
// cores where the filter matched something it can act on. Any failed send,
// failed object or failed scan makes the command exit non-zero after the
// rows are printed.
func (a fleetAction) run(cmd *cobra.Command, flags *rootFlags, scope *fleetScope) error {
	if strings.TrimSpace(a.filter) == "" {
		_ = cmd.Usage()
		return usageErr(fmt.Errorf("--filter is required (a PRTG filter such as 'name = \"fw01\"'); an empty filter would match every %s", strings.TrimSuffix(a.kind, "s")))
	}
	if err := validateFilter(a.filter); err != nil {
		return usageErr(err)
	}
	if relationshipPrefix.MatchString(a.filter) {
		return usageErr(fmt.Errorf("--filter %q uses a hierarchical relationship prefix, which cannot be combined with this command's status scope; select by parentid (e.g. 'parentid = \"40\"') or use the per-core '%s multi-%s' command", a.filter, a.kind, a.verb))
	}
	cores, done, err := scope.resolveOrEmpty(cmd, flags)
	if err != nil || done {
		return err
	}
	// The preview and the send use the same scoped filter, so what the
	// preview counts is what PRTG acts on.
	a.filter = a.scopedFilter()
	preview := fleet.Request{Method: fleet.MethodGetAll, Path: "/experimental/" + a.kind,
		Params: map[string]string{"filter": a.filter}}
	var previewMu sync.Mutex
	previewed := map[string]map[string]bool{} // core -> object IDs the preview showed
	rows, failed := collect(cmd, scope, flags, cores, []fleet.Request{preview},
		func(c fleet.Core, bodies []json.RawMessage) ([]ActionRow, error) {
			body, err := bodyOrErr(bodies[0])
			if err != nil {
				return nil, err
			}
			items := fleet.Items(body)
			ids := map[string]bool{}
			for _, it := range items {
				ids[fleet.Str(it, "id")] = true
			}
			previewMu.Lock()
			previewed[c.Name] = ids
			previewMu.Unlock()
			n := len(items)
			row := ActionRow{Core: c.Name, Action: "would " + a.verb, Kind: a.kind, Matched: &n, Filter: a.filter}
			out, outN := map[string]int{}, 0
			for _, it := range items {
				if st := fleet.Str(it, "status"); !a.inScope(st) {
					out[st]++
					outN++
				}
			}
			if outN > 0 {
				return nil, fmt.Errorf("the core returned %d object(s) outside the %s scope (statuses %v), so its filter handling cannot be trusted; nothing will be sent to it", outN, a.verb, out)
			}
			for _, it := range items {
				if len(row.Sample) >= previewSamples {
					break
				}
				row.Sample = append(row.Sample, fleet.Str(it, "name"))
			}
			if row.actionable() == 0 {
				row.Action = "nothing to " + a.verb
			}
			return []ActionRow{row}, nil
		},
		func(c fleet.Core, err error) ActionRow {
			return ActionRow{Core: c.Name, Action: "preview failed", Kind: a.kind, Error: err.Error()}
		})
	if !flags.yes {
		fmt.Fprintf(cmd.ErrOrStderr(), "preview only: nothing was sent; re-run with --yes to %s on the cores with matches\n", a.verb)
		if err := render(cmd, flags, rows, actionCols, "No cores in scope."); err != nil {
			return err
		}
		return allFailed(failed, len(cores))
	}
	if cliutil.IsAnyHarness() {
		return writeHarnessRefusal(cmd.OutOrStdout(), flags, a.name)
	}
	targets := make([]fleet.Core, 0, len(cores))
	byCore := map[string]int{}
	for i, r := range rows { // collect returns one row per core, in core order
		byCore[r.Core] = i
		if r.Error == "" && r.actionable() > 0 {
			targets = append(targets, cores[i])
		}
	}
	execFailed := 0
	if len(targets) > 0 {
		var mu sync.Mutex
		resumed := map[string][]string{} // core -> node IDs the action succeeded on; written from parallel builds
		results, _ := collect(cmd, scope, flags, targets, []fleet.Request{a.request("/" + a.kind + "/" + a.verb)},
			func(c fleet.Core, bodies []json.RawMessage) ([]ActionRow, error) {
				row := rows[byCore[c.Name]]
				row.Action, row.Sent = a.verb, true
				body, err := bodyOrErr(bodies[0])
				if err != nil {
					return nil, err
				}
				res, err := actionResults(body)
				if err != nil {
					return nil, err
				}
				// The preview and the action are different endpoints; report
				// any object the core acted on that the preview did not show.
				previewMu.Lock()
				seen := previewed[c.Name]
				previewMu.Unlock()
				if extra := unpreviewed(res.allIDs, seen); len(extra) > 0 {
					res.failed++ // counts toward a non-zero exit
					res.msgs = append(res.msgs, fmt.Sprintf("acted on %d object(s) the preview did not show: %s", len(extra), strings.Join(extra, ", ")))
				}
				ok, bad := len(res.okIDs), res.failed
				row.Succeeded, row.Failed, row.Failures = &ok, &bad, res.msgs
				mu.Lock()
				resumed[c.Name] = res.okIDs
				mu.Unlock()
				return []ActionRow{row}, nil
			},
			func(c fleet.Core, err error) ActionRow {
				// The POST may have reached the core before the failure, so the
				// row says it was sent and that the outcome is unknown.
				row := rows[byCore[c.Name]]
				row.Action, row.Sent, row.Error = a.verb+" outcome unknown", true, err.Error()
				return row
			})
		for _, r := range results {
			rows[byCore[r.Core]] = r
		}
		if a.scan {
			a.scanResumed(cmd, flags, scope, targets, resumed, rows, byCore)
		}
		for _, c := range targets {
			r := rows[byCore[c.Name]]
			if r.Error != "" || (r.Failed != nil && *r.Failed > 0) || (r.Scanned != nil && !*r.Scanned) {
				execFailed++
			}
		}
	}
	if err := render(cmd, flags, rows, actionCols, "No cores in scope."); err != nil {
		return err
	}
	if execFailed > 0 {
		return apiErr(fmt.Errorf("%s failed on %d of %d core(s) it was sent to; see the error, failed and failures fields", a.verb, execFailed, len(targets)))
	}
	if failed > 0 {
		// A core whose preview failed was never acted on: the requested
		// fleet-wide action is incomplete.
		return apiErr(fmt.Errorf("%s was not sent to %d of %d core(s) because their preview failed; see the error field", a.verb, failed, len(cores)))
	}
	return nil
}

// previewSamples is how many matched object names a preview row lists.
const previewSamples = 5

// actionCols is the human column order for action rows.
var actionCols = []string{"core", "action", "kind", "matched", "sent", "succeeded", "failed", "scanned", "sample", "error", "failures"}

// actionable is how many objects on this core the action would change.
func (r ActionRow) actionable() int {
	if r.Matched != nil {
		return *r.Matched
	}
	return 0
}

// request builds the bulk POST: the action body plus the filter.
func (a fleetAction) request(path string) fleet.Request {
	body := map[string]any{"filter": a.filter}
	for k, v := range a.body {
		body[k] = v
	}
	data, _ := json.Marshal(body)
	return fleet.Request{Method: fleet.MethodPost, Path: path, Body: data}
}

// maxScanIDs bounds the per-object scans after a resume; beyond it the scan
// is reported as skipped (and the command fails) rather than flooding a core.
const maxScanIDs = 200

// scanResumed triggers scan now for exactly the objects the resume succeeded
// on (POST /{kind}/{id}/scan). Re-evaluating the filter would miss them when
// the filter selected on the paused state the resume just cleared.
func (a fleetAction) scanResumed(cmd *cobra.Command, flags *rootFlags, scope *fleetScope, targets []fleet.Core,
	resumed map[string][]string, rows []ActionRow, byCore map[string]int) {
	r := scope.runner(flags)
	fleet.Each(cmd.Context(), scope.concurrency, targets, func(ctx context.Context, _ int, c fleet.Core) {
		row := &rows[byCore[c.Name]]
		ids := resumed[c.Name]
		if row.Error != "" || len(ids) == 0 {
			return
		}
		if len(ids) > maxScanIDs {
			no := false
			row.Scanned = &no
			row.Failures = append(row.Failures, fmt.Sprintf("scan: skipped, %d objects resumed (limit %d); run '%s multi-scan-now' with a stable filter", len(ids), maxScanIDs, a.kind))
			return
		}
		reqs := make([]fleet.Request, 0, len(ids))
		for _, id := range ids {
			reqs = append(reqs, fleet.Request{Method: fleet.MethodPost, Path: "/" + a.kind + "/" + id + "/scan"})
		}
		bodies, err := r.Fetch(ctx, c, reqs...)
		ok := err == nil
		if err != nil {
			row.Failures = append(row.Failures, "scan: "+err.Error())
		}
		for i, b := range bodies {
			if e, bad := fleet.RequestError(b); bad {
				ok = false
				if len(row.Failures) < 20 {
					row.Failures = append(row.Failures, fmt.Sprintf("scan %s: %s", ids[i], e.Message))
				}
			}
		}
		row.Scanned = &ok
	})
}

// actionSummary is a parsed ActionResult list.
type actionSummary struct {
	okIDs  []string
	allIDs []string // every node named in the result, succeeded or failed
	failed int
	msgs   []string
}

// unpreviewed lists IDs that are not in the previewed set (at most 20).
func unpreviewed(ids []string, previewed map[string]bool) []string {
	var out []string
	for _, id := range ids {
		if id != "" && !previewed[id] && len(out) < 20 {
			out = append(out, id)
		}
	}
	return out
}

// actionResults reads an ActionResult array with the shared reader
// (fleet.ActionOutcomes): any failure signal in an item fails it, and an item
// that names no node is not a success. "[]" means nothing matched; anything
// that is not an ActionResult array (including an empty or null body) is an
// error, never a silent success.
func actionResults(body json.RawMessage) (actionSummary, error) {
	var out actionSummary
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || string(trimmed) == "null" || trimmed[0] != '[' {
		return out, fmt.Errorf("unexpected action response: expected an ActionResult list")
	}
	items, isList := fleet.ActionOutcomes(trimmed)
	if !isList {
		return out, fmt.Errorf("unexpected action response: not an ActionResult list")
	}
	for _, it := range items {
		if !it.Failed && it.ID == "" {
			it.Failed, it.Reason = true, "entry names neither a node nor a failure"
		}
		if it.ID != "" {
			out.allIDs = append(out.allIDs, it.ID)
		}
		if !it.Failed {
			out.okIDs = append(out.okIDs, it.ID)
			continue
		}
		out.failed++
		if len(out.msgs) < 10 {
			name := ""
			if it.Name != "" {
				name = it.Name + ": "
			}
			out.msgs = append(out.msgs, name+it.Reason)
		}
	}
	return out, nil
}
