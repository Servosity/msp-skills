// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: review vulnerability suppressions.
// pp:data-source local

package cli

import (
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type suppressionAuditRow struct {
	ID             string   `json:"id"`
	CompanyID      string   `json:"company_id"`
	CompanyName    string   `json:"company_name"`
	AssetName      string   `json:"asset_name"`
	ProblemName    string   `json:"problem_name"`
	Flags          []string `json:"flags"`
	SuppressedTill string   `json:"suppressed_till"`
	RequestedBy    string   `json:"requested_by"`
	ApprovedBy     string   `json:"approved_by"`
	Reason         string   `json:"reason"`
	Status         string   `json:"suppression_status"`
	EPSSScore      float64  `json:"current_epss_score"`
	Severity       string   `json:"current_severity"`
}

// buildSuppressionAudit flags each active suppression and attaches the
// suppressed problem's current EPSS/severity. all=true keeps unflagged rows.
func buildSuppressionAudit(data map[string][]csRow, within time.Duration, all bool, now time.Time) []suppressionAuditRow {
	risk := csIndex(data[csResProblemsGlobal], "problem_name")
	out := make([]suppressionAuditRow, 0)
	for _, s := range data[csResSuppressions] {
		if csString(s["unsuppressed_on"]) != "" {
			continue // already lifted
		}
		row := suppressionAuditRow{
			ID: csString(s["id"]), CompanyID: csString(s["company_id"]), CompanyName: csString(s["company_name"]),
			AssetName: csString(s["asset_name"]), ProblemName: csString(s["problem_name"]),
			SuppressedTill: csString(s["suppressed_till"]), RequestedBy: csString(s["requested_by"]),
			ApprovedBy: csString(s["approved_by"]), Reason: csString(s["reason"]),
			Status: csString(s["suppression_status"]), Flags: make([]string, 0, 2),
		}
		if till, ok := csTime(s["suppressed_till"]); !ok {
			row.Flags = append(row.Flags, "open_ended")
		} else if till.Before(now) {
			row.Flags = append(row.Flags, "expired_but_active")
		} else if within > 0 && !till.After(now.Add(within)) {
			row.Flags = append(row.Flags, "expiring_soon")
		}
		switch {
		case row.ApprovedBy == "":
			row.Flags = append(row.Flags, "unapproved")
		case row.RequestedBy != "" && strings.EqualFold(row.ApprovedBy, row.RequestedBy):
			row.Flags = append(row.Flags, "self_approved")
		}
		if p, ok := risk[row.ProblemName]; ok {
			row.EPSSScore = csFloat0(p["epss_score"])
			row.Severity = csString(p["severity"])
		}
		if all || len(row.Flags) > 0 {
			out = append(out, row)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].EPSSScore != out[j].EPSSScore {
			return out[i].EPSSScore > out[j].EPSSScore
		}
		if ri, rj := csSeverityRank(out[i].Severity), csSeverityRank(out[j].Severity); ri != rj {
			return ri > rj
		}
		return out[i].CompanyName < out[j].CompanyName
	})
	return out
}

func newNovelSuppressionAuditCmd(flags *rootFlags) *cobra.Command {
	var withinFlag, dbPath string
	var all bool
	cmd := &cobra.Command{
		Use:   "suppression-audit",
		Short: "Flag suppressions that expired but stay active, expire soon, never expire, or were self-approved",
		Long: `Review every active vulnerability suppression and flag:
  expired_but_active  suppressed_till is in the past but it was never lifted
  expiring_soon       suppressed_till falls within --within
  open_ended          no suppressed_till at all
  unapproved          approved_by is empty
  self_approved       approved_by equals requested_by
Each row carries the suppressed problem's current EPSS and severity, riskiest
first. Reads the local store (run 'connectsecure-cli sync' first).`,
		Example: strings.Trim(`
  connectsecure-cli suppression-audit --within 14d
  connectsecure-cli suppression-audit --all --agent --select problem_name,flags,current_epss_score`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "suppression-audit")
			}
			within, err := csDuration("within", withinFlag)
			if err != nil {
				return err
			}
			data, unsynced, err := csLocalData(cmd, flags, dbPath, csResSuppressions, csResProblemsGlobal)
			if err != nil {
				return err
			}
			rows := buildSuppressionAudit(data, within, all, time.Now().UTC())
			return csEmit(cmd, flags, rows, unsynced, "No flagged suppressions in the local store.")
		},
	}
	cmd.Flags().StringVar(&withinFlag, "within", "14d", "Flag suppressions expiring within this window (0 disables)")
	cmd.Flags().BoolVar(&all, "all", false, "Include active suppressions that raised no flag")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: the synced store)")
	return cmd
}
