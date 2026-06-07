// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature: fire-and-confirm remediation. Hand-authored; preserved across regenerations.

// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"abnormal-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

var threatActions = map[string]bool{"remediate": true, "unremediate": true}
var caseActions = map[string]bool{
	"action_required":           true,
	"acknowledge_resolved":      true,
	"acknowledge_in_progress":   true,
	"acknowledge_not_an_attack": true,
}

type remediateWatchView struct {
	Target      string `json:"target"`
	ID          string `json:"id"`
	Action      string `json:"action"`
	ActionID    string `json:"actionId"`
	Status      string `json:"status"`
	Description string `json:"description,omitempty"`
	Polls       int    `json:"polls"`
	ElapsedMS   int64  `json:"elapsed_ms"`
}

// parseActionReceipt accepts both the threat (snake_case) and case (camelCase)
// manage-response shapes and returns the action ID.
func parseActionReceipt(data []byte) (string, error) {
	var receipt struct {
		ActionIDSnake string `json:"action_id"`
		ActionIDCamel string `json:"actionId"`
	}
	if err := json.Unmarshal(data, &receipt); err != nil {
		return "", fmt.Errorf("parsing manage response: %w", err)
	}
	if receipt.ActionIDSnake != "" {
		return receipt.ActionIDSnake, nil
	}
	if receipt.ActionIDCamel != "" {
		return receipt.ActionIDCamel, nil
	}
	return "", fmt.Errorf("manage response did not include an action id")
}

func newNovelRemediateWatchCmd(flags *rootFlags) *cobra.Command {
	var flagAction string
	var flagTimeout string
	var flagInterval string

	cmd := &cobra.Command{
		Use:   "remediate-watch <threat|case> <id>",
		Short: "Remediate a threat or case and block until the action reaches a terminal state",
		Long: strings.Trim(`
Use this command to remediate a threat or case and block until the action
reaches a terminal state ('done' or 'error'), returning a typed exit code and
the final receipt. Do NOT use it for a bare submit; use the generated
'threats create' / 'cases create' manage commands. Do NOT use it for triage
ranking; use 'triage'.

Threat actions: remediate (default), unremediate.
Case actions: action_required, acknowledge_resolved, acknowledge_in_progress,
acknowledge_not_an_attack (--action is required for cases).`, "\n"),
		Example: strings.Trim(`
  abnormal-cli remediate-watch threat 184712ab-6d8b-47b3-89d7-a314efef23ff --timeout 5m
  abnormal-cli remediate-watch threat 184712ab-6d8b-47b3-89d7-a314efef23ff --action unremediate
  abnormal-cli remediate-watch case 1234 --action acknowledge_resolved --interval 5s`, "\n"),
		Annotations: map[string]string{
			"pp:happy-args": "target=threat;id=184712ab-6d8b-47b3-89d7-a314efef23ff",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if len(args) < 2 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("remediate-watch requires a target type (threat|case) and an id"))
			}
			target := strings.ToLower(args[0])
			id := args[1]
			if target != "threat" && target != "case" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("target must be 'threat' or 'case', got %q", args[0]))
			}
			if flags.dataSource == "local" {
				return usageErr(fmt.Errorf("remediate-watch acts on the live API; no local data source"))
			}
			action := flagAction
			if action == "" {
				if target == "threat" {
					action = "remediate"
				} else {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("--action is required for cases (action_required, acknowledge_resolved, acknowledge_in_progress, acknowledge_not_an_attack)"))
				}
			}
			if target == "threat" && !threatActions[action] {
				return usageErr(fmt.Errorf("invalid threat action %q (valid: remediate, unremediate)", action))
			}
			if target == "case" && !caseActions[action] {
				return usageErr(fmt.Errorf("invalid case action %q (valid: action_required, acknowledge_resolved, acknowledge_in_progress, acknowledge_not_an_attack)", action))
			}
			basePath := "/threats/"
			if target == "case" {
				basePath = "/cases/"
			}
			managePath := basePath + url.PathEscape(id)
			if dryRunOK(flags) || cliutil.IsVerifyEnv() {
				fmt.Fprintf(cmd.OutOrStdout(), "would POST {\"action\": %q} to %s and poll %sactions/<action_id> until done or error\n", action, managePath, managePath+"/")
				return nil
			}
			timeout, err := cliutil.ParseDurationLoose(flagTimeout)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid --timeout %q: %w", flagTimeout, err))
			}
			interval, err := cliutil.ParseDurationLoose(flagInterval)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid --interval %q: %w", flagInterval, err))
			}
			if interval <= 0 {
				interval = 10 * time.Second
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			start := time.Now()
			data, _, err := c.Post(cmd.Context(), managePath, map[string]string{"action": action})
			if err != nil {
				return classifyAPIError(fmt.Errorf("submitting %s action on %s %s: %w", action, target, id, err), flags)
			}
			actionID, err := parseActionReceipt(data)
			if err != nil {
				return apiErr(err)
			}
			statusPath := managePath + "/actions/" + url.PathEscape(actionID)
			view := remediateWatchView{Target: target, ID: id, Action: action, ActionID: actionID, Status: "submitted"}
			// Round up so a timeout that interval doesn't divide still gets
			// its full budget instead of abandoning ~one interval early.
			maxPolls := int((timeout + interval - 1) / interval)
			if maxPolls < 1 {
				maxPolls = 1
			}
			if cliutil.IsDogfoodEnv() && maxPolls > 1 {
				maxPolls = 1
			}
			deadline := start.Add(timeout)
			for poll := 1; poll <= maxPolls; poll++ {
				sdata, err := c.GetNoCache(cmd.Context(), statusPath, nil)
				view.Polls = poll
				if err != nil {
					view.ElapsedMS = time.Since(start).Milliseconds()
					_ = printJSONFiltered(cmd.OutOrStdout(), view, flags)
					return classifyAPIError(fmt.Errorf("polling action status: %w", err), flags)
				}
				var st struct {
					Status      string `json:"status"`
					Description string `json:"description"`
				}
				if err := json.Unmarshal(sdata, &st); err == nil && st.Status != "" {
					view.Status = st.Status
					view.Description = st.Description
				}
				if view.Status == "done" || view.Status == "error" {
					break
				}
				if time.Now().Add(interval).After(deadline) || poll == maxPolls {
					break
				}
				select {
				case <-cmd.Context().Done():
					return cmd.Context().Err()
				case <-time.After(interval):
				}
			}
			view.ElapsedMS = time.Since(start).Milliseconds()
			if err := printJSONFiltered(cmd.OutOrStdout(), view, flags); err != nil {
				return err
			}
			switch view.Status {
			case "done":
				return nil
			case "error":
				return apiErr(fmt.Errorf("action %s on %s %s failed: %s", action, target, id, view.Description))
			default:
				return apiErr(fmt.Errorf("action %s on %s %s still %q after %s (%d polls) — check later with the generated '%ss actions' command", action, target, id, view.Status, flagTimeout, view.Polls, target))
			}
		},
	}
	cmd.Flags().StringVar(&flagAction, "action", "", "Action to perform (threat: remediate|unremediate, default remediate; case: required, see help)")
	cmd.Flags().StringVar(&flagTimeout, "timeout", "5m", "Maximum time to wait for the action to reach a terminal state (e.g. 30s, 5m)")
	cmd.Flags().StringVar(&flagInterval, "interval", "10s", "Polling interval for action status (e.g. 5s, 30s)")
	return cmd
}
