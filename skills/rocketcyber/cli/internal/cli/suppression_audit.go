// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature command.
//
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"rocketcyber-pp-cli/internal/cliutil"
)

type auditRule struct {
	RuleID    int64  `json:"rule_id,omitempty"`
	RuleName  string `json:"rule_name"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at,omitempty"`
	AgeDays   int    `json:"age_days"`
}

type auditView struct {
	TotalRules       int         `json:"total_rules"`
	ActiveCount      int         `json:"active_count"`
	InactiveCount    int         `json:"inactive_count"`
	StaleAfter       string      `json:"stale_after"`
	StaleActiveRules []auditRule `json:"stale_active_rules"`
	RecentlyChanged  []auditRule `json:"recently_changed"`
	Note             string      `json:"note,omitempty"`
}

// classifySuppressionRules classifies rules by status and last-touched age:
// active rules untouched beyond staleAfter are stale; rules touched within
// 7 days are recently changed.
func classifySuppressionRules(items []json.RawMessage, now time.Time, staleAfter time.Duration) auditView {
	view := auditView{
		StaleActiveRules: []auditRule{},
		RecentlyChanged:  []auditRule{},
	}
	staleCutoff := now.Add(-staleAfter)
	recentCutoff := now.Add(-7 * 24 * time.Hour)
	for _, raw := range items {
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(raw, &probe); err != nil {
			continue
		}
		view.TotalRules++
		row := auditRule{
			RuleName: extractString(probe, "ruleName"),
			Status:   extractString(probe, "status"),
		}
		if id, ok := cliutil.ExtractInt(probe, "ruleId"); ok {
			row.RuleID = id
		}
		touched := extractString(probe, "updatedAt")
		if touched == "" {
			touched = extractString(probe, "createdAt")
		}
		row.UpdatedAt = touched
		t := parseAPITime(touched)
		if !t.IsZero() {
			row.AgeDays = int(now.Sub(t).Hours() / 24)
		}
		active := strings.EqualFold(row.Status, "active")
		if active {
			view.ActiveCount++
		} else {
			view.InactiveCount++
		}
		if active && !t.IsZero() && t.Before(staleCutoff) {
			view.StaleActiveRules = append(view.StaleActiveRules, row)
		}
		if !t.IsZero() && t.After(recentCutoff) {
			view.RecentlyChanged = append(view.RecentlyChanged, row)
		}
	}
	sort.Slice(view.StaleActiveRules, func(i, j int) bool { return view.StaleActiveRules[i].AgeDays > view.StaleActiveRules[j].AgeDays })
	sort.Slice(view.RecentlyChanged, func(i, j int) bool { return view.RecentlyChanged[i].AgeDays < view.RecentlyChanged[j].AgeDays })
	return view
}

func newNovelSuppressionAuditCmd(flags *rootFlags) *cobra.Command {
	var flagAccountID int
	var flagStaleAfter string

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Alert-suppression rules classified by status and age, flagging stale rules that may hide real detections.",
		Long: strings.Trim(`
Audits alert-suppression rules: counts by status, flags active rules
untouched beyond --stale-after (candidates hiding real detections), and
lists rules changed in the last 7 days.

Use this to review suppression-rule posture (active/stale/recently-changed).
Do NOT use it to fetch one rule's raw config; use 'suppression rule <ruleId>'.
`, "\n"),
		Example: strings.Trim(`
  rocketcyber-cli suppression audit --stale-after 90d --json
  rocketcyber-cli suppression audit --account-id 2 --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fetch /suppression/rules and classify rules by status and age")
				return nil
			}
			if flagStaleAfter == "" {
				flagStaleAfter = "90d"
			}
			staleAfter, err := cliutil.ParseDurationLoose(flagStaleAfter)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid --stale-after value %q: %w", flagStaleAfter, err))
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			params := map[string]string{}
			if flagAccountID != 0 {
				params["accountId"] = strconv.Itoa(flagAccountID)
			}
			data, err := c.Get(cmd.Context(), "/suppression/rules", params)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			items, _ := parseEnvelope(data)
			view := classifySuppressionRules(items, time.Now().UTC(), staleAfter)
			view.StaleAfter = flagStaleAfter
			if view.TotalRules == 0 {
				view.Note = "no suppression rules returned for this scope"
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().IntVar(&flagAccountID, "account-id", 0, "Account ID to scope the rule audit")
	cmd.Flags().StringVar(&flagStaleAfter, "stale-after", "90d", "Age beyond which an untouched active rule is flagged stale (e.g. 90d, 26w)")
	return cmd
}
