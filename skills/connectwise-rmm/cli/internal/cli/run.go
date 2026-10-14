// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: resolve a script and endpoints by name, schedule, watch, collect results.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"connectwise-rmm-pp-cli/internal/client"
	"connectwise-rmm-pp-cli/internal/cliutil"
	"connectwise-rmm-pp-cli/internal/fleet"
)

var uuidLike = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type runTemplate struct {
	ID   string `json:"template_id"`
	Name string `json:"name"`
	Type string `json:"template_type"`
}

type runView struct {
	Preview   bool                 `json:"preview"`
	Template  runTemplate          `json:"template"`
	Targets   []fleet.Endpoint     `json:"targets"`
	Request   map[string]any       `json:"request"`
	TaskID    string               `json:"task_id,omitempty"`
	Instances []fleet.TaskInstance `json:"instances,omitempty"`
	Results   []fleet.TaskResult   `json:"results,omitempty"`
	Failed    []string             `json:"failed_endpoint_ids,omitempty"`
	TimedOut  bool                 `json:"timed_out,omitempty"`
	Note      string               `json:"note,omitempty"`
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		addNovelCommandIfAbsent(root, newRunCmd(flags))
	})
}

func newRunCmd(flags *rootFlags) *cobra.Command {
	var script, templateID, templateType, company, name, paramsJSON, waitTimeout, pollInterval, dbPath string
	var endpoints, params []string
	var confirm, wait bool
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a script on endpoints by name: preview, schedule, wait, and collect results",
		Long: strings.Trim(`
Resolve a script (fusion script or global shell-script template) and target
endpoints by name or ID, then schedule it to run immediately on those
endpoints. Without --confirm the command only prints the resolved plan and the
exact request; nothing is scheduled. With --confirm it schedules the task, and
with --wait it polls the task instance until every device finishes, then
returns per-endpoint results and failed_endpoint_ids.

Endpoint names resolve from the local store (run sync first; machine names are
cached by 'fleet whois'). Custom task templates have no listed ID, so pass
--template-id with --type custom for those.`, "\n"),
		Example: strings.Trim(`
  connectwise-rmm-cli run --script "Restart Print Spooler" --endpoint ACME-WS01,ACME-WS02
  connectwise-rmm-cli run --script "Restart Print Spooler" --endpoint ACME-WS01 --confirm --wait --agent
  connectwise-rmm-cli run --template-id 51a74346-e19b-11e7-9809-0800279505d9 --type script --endpoint 693f8dff-0c3b-4e44-9e10-fc2ec6bd28d6 --param body=hostname --param expectedExecutionTimeSec=120`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "pp:preview-happy-path": "true", "pp:happy-args": "--template-id=51a74346-e19b-11e7-9809-0800279505d9;--type=script;--endpoint=693f8dff-0c3b-4e44-9e10-fc2ec6bd28d6"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "run")
			}
			if len(args) > 0 {
				// `--confirm false` parses as --confirm plus a stray "false".
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("run takes no positional arguments (got %q); use --confirm=false or omit --confirm", strings.Join(args, " ")))
			}
			if script == "" && templateID == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--script (name or ID) or --template-id is required"))
			}
			refs := splitList(endpoints)
			if len(refs) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--endpoint is required (names or IDs, comma-separated)"))
			}
			parameters, err := buildRunParameters(params, paramsJSON)
			if err != nil {
				return usageErr(err)
			}
			// Validate wait settings before scheduling: a bad flag must never
			// leave a real task running without its task ID being reported.
			waitFor, err := parseDurationFlag("wait-timeout", waitTimeout)
			if err != nil {
				return err
			}
			every, err := parseDurationFlag("poll-interval", pollInterval)
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			targets, err := resolveRunTargets(ctx, cmd, dbPath, company, refs)
			if err != nil {
				return err
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			tpl := runTemplate{ID: templateID, Type: templateType, Name: script}
			if templateID == "" {
				if tpl, err = resolveRunTemplate(ctx, c, script, templateType); err != nil {
					return err
				}
			} else if tpl.Type == "" {
				tpl.Type = "script"
			}
			body := map[string]any{
				"name":          firstNonEmpty(name, tpl.Name+" via connectwise-rmm-cli"),
				"templateID":    tpl.ID,
				"templateType":  tpl.Type,
				"targets":       endpointIDs(targets),
				"targetType":    "MANAGED_ENDPOINT",
				"resourcesType": "Both",
				"schedule":      map[string]any{"regularity": "Immediate", "category": "STZ", "scheduleType": "TIME"},
			}
			if parameters != "" {
				body["parameters"] = parameters
			}
			view := runView{Preview: !confirm, Template: tpl, Targets: targets, Request: body}
			if !confirm {
				view.Note = "preview only; add --confirm to schedule this task"
				return printRun(cmd, flags, view)
			}
			if cliutil.IsAnyHarness() {
				return writeHarnessRefusal(cmd.OutOrStdout(), flags, "schedule a task on real endpoints")
			}
			raw, _, err := c.Post(ctx, "/api/platform/v1/automation/endpoints/schedule-tasks", body)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			view.TaskID = taskIDFrom(raw)
			if view.TaskID == "" {
				return apiErr(fmt.Errorf("schedule-tasks returned no taskId: %s", cliutil.SanitizeErrorBody(string(raw))))
			}
			rememberRunTargets(ctx, cmd, dbPath, view.TaskID, body["targets"])
			if wait {
				// Polling gets its own deadline: --wait-timeout, not --timeout,
				// bounds the wait; --timeout still bounds each request.
				if err := waitForTask(cmd, flags, c, &view, waitFor, every); err != nil {
					_ = printRun(cmd, flags, view)
					return err
				}
			} else {
				view.Note = fmt.Sprintf("scheduled; check with 'fleet task-failures %s' or 'tasks get-instance-summary %s'", view.TaskID, view.TaskID)
			}
			return printRun(cmd, flags, view)
		},
	}
	cmd.Flags().StringVar(&script, "script", "", "Script or template name (or ID) to run")
	cmd.Flags().StringVar(&templateID, "template-id", "", "Template ID to run directly (required for custom task templates)")
	cmd.Flags().StringVar(&templateType, "type", "", "Template type: fusionscript, script, or custom (default: detected from where --script is found)")
	cmd.Flags().StringSliceVar(&endpoints, "endpoint", nil, "Target endpoint names or IDs (comma-separated or repeated)")
	cmd.Flags().StringVar(&company, "company", "", "Resolve endpoint names within this company (ID or name)")
	cmd.Flags().StringArrayVar(&params, "param", nil, "Script parameter key=value (repeatable)")
	cmd.Flags().StringVar(&paramsJSON, "params-json", "", "Script parameters as a JSON object")
	cmd.Flags().StringVar(&name, "name", "", "Task name shown to technicians (default: '<script> via connectwise-rmm-cli')")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Actually schedule the task (default: preview only)")
	cmd.Flags().BoolVar(&wait, "wait", false, "With --confirm, poll until every device finishes and return results")
	cmd.Flags().StringVar(&waitTimeout, "wait-timeout", "15m", "Give up waiting after this long")
	cmd.Flags().StringVar(&pollInterval, "poll-interval", "15s", "Time between task status checks")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the CLI's local store)")
	return cmd
}

func splitList(vals []string) []string {
	out := make([]string, 0)
	for _, v := range vals {
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// buildRunParameters compiles --param / --params-json into the JSON string the
// schedule-tasks API expects in "parameters" (CW University tasking examples).
func buildRunParameters(kv []string, rawJSON string) (string, error) {
	m := map[string]any{}
	if strings.TrimSpace(rawJSON) != "" {
		var v any
		if err := json.Unmarshal([]byte(rawJSON), &v); err != nil {
			return "", fmt.Errorf("--params-json is not valid JSON: %w", err)
		}
		obj, ok := v.(map[string]any)
		if !ok {
			return "", fmt.Errorf("--params-json must be a JSON object, e.g. {\"body\":\"hostname\"}")
		}
		m = obj
	}
	for _, p := range kv {
		k, v, ok := strings.Cut(p, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return "", fmt.Errorf("--param %q must be key=value", p)
		}
		m[strings.TrimSpace(k)] = v
	}
	if len(m) == 0 {
		return "", nil
	}
	b, err := json.Marshal(m)
	return string(b), err
}

func resolveRunTargets(ctx context.Context, cmd *cobra.Command, dbPath, company string, refs []string) ([]fleet.Endpoint, error) {
	out := make([]fleet.Endpoint, 0, len(refs))
	var eps []fleet.Endpoint
	db, missing, err := openFleetStore(ctx, cmd, dbPath)
	if err != nil {
		return nil, err
	}
	if !missing {
		defer db.Close()
		if eps, err = loadFleet(db); err != nil {
			return nil, err
		}
		eps = filterFleet(eps, company, "")
	}
	for _, ref := range refs {
		e, err := resolveEndpoint(eps, ref)
		if err != nil {
			if !uuidLike.MatchString(ref) {
				return nil, err
			}
			e = fleet.Endpoint{ID: ref} // an ID not yet synced is still a valid target
		}
		out = append(out, e)
	}
	return out, nil
}

// resolveRunTemplate finds a script by ID or case-insensitive name across the
// fusion-script list and the global shell-script templates.
func resolveRunTemplate(ctx context.Context, c *client.Client, ref, wantType string) (runTemplate, error) {
	var candidates []runTemplate
	if wantType == "" || wantType == "fusionscript" {
		raw, err := c.Get(ctx, "/api/platform/v1/automation/scripts", nil)
		if err != nil {
			return runTemplate{}, classifyAPIErrorOnly(err)
		}
		for _, m := range fleet.Objects(raw, "scripts") {
			candidates = append(candidates, runTemplate{ID: fleet.Str(m, "id"), Name: fleet.Str(m, "name"), Type: "fusionscript"})
		}
	}
	if wantType == "" || wantType == "script" {
		raw, err := c.Get(ctx, "/api/platform/v1/automation/shellScripts", nil)
		if err != nil {
			return runTemplate{}, classifyAPIErrorOnly(err)
		}
		for _, m := range fleet.Objects(raw, "Templates", "templates") {
			candidates = append(candidates, runTemplate{ID: fleet.Str(m, "templateId", "templateID"), Name: fleet.Str(m, "name"), Type: "script"})
		}
	}
	var hits []runTemplate
	for _, t := range candidates {
		if strings.EqualFold(t.ID, ref) {
			return t, nil
		}
		if strings.EqualFold(t.Name, ref) {
			hits = append(hits, t)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return runTemplate{}, notFoundErr(fmt.Errorf("no script or template named %q; list them with 'scripts list' and 'scripts list-shell'", ref))
	}
	return runTemplate{}, usageErr(fmt.Errorf("%q matches %d scripts; pass --template-id or --type", ref, len(hits)))
}

func taskIDFrom(raw json.RawMessage) string {
	if m := fleet.Object(raw); m != nil {
		return fleet.Str(m, "taskId", "taskID", "id")
	}
	return ""
}

// rememberRunTargets lets 'fleet task-failures' list targets with no result.
func rememberRunTargets(ctx context.Context, cmd *cobra.Command, dbPath, taskID string, targets any) {
	db, missing, err := openFleetStore(ctx, cmd, dbPath)
	if err != nil || missing {
		return
	}
	defer db.Close()
	rec, _ := json.Marshal(map[string]any{"id": taskID, "targets": targets, "scheduledAt": time.Now().UTC().Format(time.RFC3339)})
	if err := db.Upsert(cacheTaskRun, taskID, rec); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not record run targets: %v\n", err)
	}
}

// waitForTask polls until the newest instance finishes or the wait times out.
// A timeout is reported in the view (exit 0, task still running); failing to
// read the final results is an error.
func waitForTask(cmd *cobra.Command, flags *rootFlags, c *client.Client, v *runView, timeout, every time.Duration) error {
	base := "/api/platform/v1/automation/tasks/" + url.PathEscape(v.TaskID) + "/instances/"
	waitCtx, cancelWait := context.WithTimeout(cmd.Context(), timeout+time.Minute)
	defer cancelWait()
	get := func(path string) (json.RawMessage, error) {
		ctx, cancel := boundCtx(waitCtx, flags)
		defer cancel()
		return c.GetNoCache(ctx, path, nil)
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	sawStatus := false
	for {
		raw, err := get(base + "summary")
		if err == nil {
			instances, ok := fleet.ParseTaskInstances(raw)
			if ok {
				sawStatus = true
				v.Instances = instances
				if len(v.Instances) > 0 && v.Instances[0].Done() {
					break
				}
			} else {
				lastErr = fmt.Errorf("unreadable instance summary")
			}
		} else {
			lastErr = err
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: polling task %s: %v\n", v.TaskID, err)
		}
		if time.Now().Add(every).After(deadline) {
			v.TimedOut = true
			if !sawStatus {
				v.Note = fmt.Sprintf("task %s was scheduled but its status could not be read; check with 'fleet task-failures %s'", v.TaskID, v.TaskID)
				return apiErr(fmt.Errorf("reading task status: %v", lastErr))
			}
			v.Note = fmt.Sprintf("still running after %s; check later with 'fleet task-failures %s'", timeout, v.TaskID)
			return nil
		}
		select {
		case <-waitCtx.Done():
			v.TimedOut = true
			v.Note = "stopped waiting: " + waitCtx.Err().Error()
			return nil
		case <-time.After(every):
		}
	}
	raw, err := get(base + url.PathEscape(v.Instances[0].ID) + "/results")
	if err != nil {
		v.Note = fmt.Sprintf("task %s finished, but reading results failed; retry with 'fleet task-failures %s'", v.TaskID, v.TaskID)
		return classifyAPIErrorOnly(err)
	}
	results, ok := fleet.ParseTaskResults(raw)
	if !ok {
		v.Note = fmt.Sprintf("task %s finished, but its results payload was unreadable; retry with 'fleet task-failures %s'", v.TaskID, v.TaskID)
		return apiErr(fmt.Errorf("task %s: unreadable results payload", v.TaskID))
	}
	v.Results = results
	for _, r := range v.Results {
		if r.Failed() {
			v.Failed = append(v.Failed, r.EndpointID)
		}
	}
	return nil
}

func printRun(cmd *cobra.Command, flags *rootFlags, v runView) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), v, flags)
	}
	w := cmd.OutOrStdout()
	names := make([]string, 0, len(v.Targets))
	for _, t := range v.Targets {
		names = append(names, firstNonEmpty(t.Name, t.ID))
	}
	fmt.Fprintf(w, "%s %q (%s, %s) on %d endpoint(s): %s\n", map[bool]string{true: "Would run", false: "Scheduled"}[v.Preview], v.Template.Name, v.Template.Type, v.Template.ID, len(v.Targets), strings.Join(names, ", "))
	if v.TaskID != "" {
		fmt.Fprintf(w, "task: %s\n", v.TaskID)
	}
	for _, r := range v.Results {
		fmt.Fprintf(w, "  %s  %s  %s\n", r.EndpointID, r.Status, firstNonEmpty(r.Error, r.Result))
	}
	if v.Note != "" {
		fmt.Fprintln(w, v.Note)
	}
	return nil
}
