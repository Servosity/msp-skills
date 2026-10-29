// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"kaseya-vsa-pp-cli/internal/cliutil"
)

type scriptRunResult struct {
	ScriptID        string `json:"script_id"`
	DeviceID        string `json:"device_id"`
	ExecutionID     string `json:"execution_id"`
	State           string `json:"state"`
	ExitCode        *int   `json:"exit_code"`
	Output          string `json:"output"`
	VariableOutputs any    `json:"variable_outputs"`
	StartTime       any    `json:"start_time"`
	EndTime         any    `json:"end_time"`
	DurationSeconds any    `json:"duration_seconds"`
	Polls           int    `json:"polls"`
	TimedOut        bool   `json:"timed_out"`
}

func newNovelScriptsRunWaitCmd(flags *rootFlags) *cobra.Command {
	var flagDevice, flagAttach, flagWebhook, flagMaxWait, flagInterval string
	var flagVars []string

	cmd := &cobra.Command{
		Use:   "run-wait [script-id]",
		Short: "Run an automation script on a device and get its exit code and output when it finishes.",
		Long: `Runs an automation script on one device (POST /automation/scripts/{id}/run), then polls the
execution until it ends and returns its state, exit code, output and output variables. The device
must be online. --device takes a device name or Identifier; --var Id=Value overrides a script input
variable (repeatable). The run starts immediately; there is no confirmation prompt. --attach
EXECUTION_ID skips the run and only waits on an existing execution.

Exit codes: 0 when the script succeeded with exit code 0; 6 when it failed, was stopped, or exited
non-zero; 8 when it was still running after --max-wait (re-run with --attach to keep waiting).

Use this command to run one automation script on one device and get its output. Do NOT use this
command for a saved task that targets several devices or several scripts; use 'tasks run-wait'
instead.`,
		Example: strings.Trim(`
  kaseya-vsa-cli scripts run-wait 11111111-2222-3333-4444-555555555555 --device FRONTDESK-01 --max-wait 10m --agent
  kaseya-vsa-cli scripts run-wait 11111111-2222-3333-4444-555555555555 --device FRONTDESK-01 --var 251=30 --agent
  kaseya-vsa-cli scripts run-wait 11111111-2222-3333-4444-555555555555 --device FRONTDESK-01 --attach 64adc13b-bc8f-4ab3-a292-4154d0f1ab12`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "pp:typed-exit-codes": "0,6,8"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
					return writeDryRun(cmd.OutOrStdout(), flags, "scripts run-wait")
				}
				sid := strings.TrimSpace(args[0])
				if strings.TrimSpace(flagAttach) != "" {
					return writeRunPreview(cmd.OutOrStdout(), flags, "scripts run-wait", "GET",
						fmt.Sprintf("/automation/scripts/%s/device/%s/executions/%s", sid, flagDevice, flagAttach), nil,
						"poll until the execution ends; nothing is started")
				}
				body := map[string]any{"DeviceIdentifier": flagDevice}
				if vars, err := parseScriptVars(flagVars); err == nil && len(vars) > 0 {
					body["Variables"] = vars
				}
				if flagWebhook != "" {
					body["WebhookUrl"] = flagWebhook
				}
				return writeRunPreview(cmd.OutOrStdout(), flags, "scripts run-wait", "POST", "/automation/scripts/"+sid+"/run", body,
					"poll /automation/scripts/{id}/device/{device}/executions/{executionId} until it ends (device name resolves to its Identifier at run time)")
			}
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("script-id is required (see 'kaseya-vsa-cli scripts list')"))
			}
			if strings.TrimSpace(flagDevice) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--device is required (device name or Identifier)"))
			}
			maxWait, err := cliutil.ParseDurationLoose(flagMaxWait)
			if err != nil || maxWait <= 0 {
				return usageErr(fmt.Errorf("invalid --max-wait %q (e.g. 10m, 1h)", flagMaxWait))
			}
			interval, err := cliutil.ParseDurationLoose(flagInterval)
			if err != nil || interval <= 0 {
				return usageErr(fmt.Errorf("invalid --interval %q (e.g. 5s)", flagInterval))
			}
			vars, err := parseScriptVars(flagVars)
			if err != nil {
				return usageErr(err)
			}
			flagAttach = strings.TrimSpace(flagAttach)
			if cmd.Flags().Changed("attach") && flagAttach == "" {
				return usageErr(fmt.Errorf("--attach needs an execution ID"))
			}
			if flagAttach == "" && cliutil.IsAnyHarness() {
				return writeHarnessRefusal(cmd.OutOrStdout(), flags, "run automation script on a managed device")
			}
			scriptID := strings.TrimSpace(args[0])
			ctx, cancel := waitContext(cmd.Context(), maxWait)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			if err := requireTenantBaseURL(c); err != nil {
				return err
			}
			c.NoCache = true // polling must never be answered from the response cache
			deviceID, _, err := resolveDeviceID(ctx, flags, c, flagDevice)
			if err != nil {
				return err
			}
			res := scriptRunResult{ScriptID: scriptID, DeviceID: deviceID, ExecutionID: flagAttach}
			if res.ExecutionID == "" {
				body := map[string]any{"DeviceIdentifier": deviceID}
				if len(vars) > 0 {
					body["Variables"] = vars
				}
				if flagWebhook != "" {
					body["WebhookUrl"] = flagWebhook
				}
				raw, _, err := c.Post(ctx, "/automation/scripts/"+scriptID+"/run", body)
				if err != nil {
					return classifyAPIErrorOnly(err)
				}
				var started struct {
					ExecutionID string `json:"ExecutionId"`
				}
				if err := json.Unmarshal(vsaData(raw), &started); err != nil || started.ExecutionID == "" {
					return apiErr(fmt.Errorf("run script: response carried no ExecutionId: %s", bodyForError(c, raw)))
				}
				res.ExecutionID = started.ExecutionID
				fmt.Fprintf(cmd.ErrOrStderr(), "started execution %s; waiting up to %s\n", res.ExecutionID, maxWait)
			}
			path := fmt.Sprintf("/automation/scripts/%s/device/%s/executions/%s", scriptID, deviceID, res.ExecutionID)
			rec, polls, timedOut, err := pollExecution(ctx, c, path, interval)
			if err != nil {
				return err
			}
			res.Polls, res.TimedOut = polls, timedOut
			if rec != nil {
				res.State = fmt.Sprint(rec["State"])
				if code, ok := exitCodeValue(rec["ExitCode"]); ok {
					res.ExitCode = &code
				}
				res.Output, _ = rec["Output"].(string)
				res.VariableOutputs = rec["VariableOutputs"]
				res.StartTime, res.DurationSeconds = rec["StartTime"], rec["DurationInSeconds"]
				res.EndTime = rec["EndTime"]
				if res.EndTime == nil {
					res.EndTime = rec["Endtime"]
				}
			}
			if err := printScriptRun(cmd, flags, res); err != nil {
				return err
			}
			switch {
			case timedOut:
				return runTimeoutErr(fmt.Errorf("execution %s still %s after %s; re-run with --attach %s", res.ExecutionID, res.State, maxWait, res.ExecutionID))
			case !strings.EqualFold(res.State, "Successful") || (res.ExitCode != nil && *res.ExitCode != 0):
				return runFailedErr(fmt.Errorf("execution %s ended %s (exit code %s)", res.ExecutionID, res.State, intPtrString(res.ExitCode)))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagDevice, "device", "", "Device to run on: name or Identifier (required)")
	cmd.Flags().StringArrayVar(&flagVars, "var", nil, "Override a script input variable as Id=Value (repeatable)")
	cmd.Flags().StringVar(&flagAttach, "attach", "", "Wait on this existing execution ID instead of starting a new run")
	cmd.Flags().StringVar(&flagWebhook, "webhook-url", "", "Optional URL VSA 10 POSTs to when the execution finishes")
	cmd.Flags().StringVar(&flagMaxWait, "max-wait", "10m", "Stop waiting after this long (the run keeps going; exit code 8)")
	cmd.Flags().StringVar(&flagInterval, "interval", "5s", "Time between execution status polls")
	return cmd
}

func parseScriptVars(in []string) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(in))
	for _, kv := range in {
		k, v, ok := strings.Cut(kv, "=")
		id, err := strconv.Atoi(strings.TrimSpace(k))
		if !ok || err != nil {
			return nil, fmt.Errorf("invalid --var %q (want Id=Value with a numeric variable Id from 'scripts get')", kv)
		}
		out = append(out, map[string]any{"Id": id, "Value": v})
	}
	return out, nil
}

func printScriptRun(cmd *cobra.Command, flags *rootFlags, r scriptRunResult) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), r, flags)
	}
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "execution %s: %s (exit code %s, %d polls)\n", r.ExecutionID, r.State, intPtrString(r.ExitCode), r.Polls)
	if r.Output != "" {
		fmt.Fprintln(w, strings.TrimRight(r.Output, "\n"))
	}
	return nil
}

func intPtrString(p *int) string {
	if p == nil {
		return "unknown"
	}
	return strconv.Itoa(*p)
}
