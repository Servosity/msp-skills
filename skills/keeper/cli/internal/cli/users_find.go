// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (keeper handfix posture-report).
// pp:data-source local

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelUsersFindCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "find <email-or-fragment>",
		Short: "Find which managed company a user belongs to, with status, 2FA and last login, across every client",
		Long: `Use this command to find which managed company a user belongs to, across all companies, by email or fragment.
Do NOT use it to list every user of one known company; use 'users list --mc <id>' instead.
Do NOT use it to find problem users; use 'users risk' or 'users hygiene' instead.

Searches each company's newest stored posture run (no network call), so no Service Mode session
switch per client. Matching is case-insensitive on email and name. Each row names the run it came
from; run 'posture report' to refresh.`,
		Example: strings.Trim(`
  keeper-cli users find jane@example.com --agent
  keeper-cli users find @contoso.com --agent --select company_name,email,status,twofa`, "\n"),
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			q := strings.ToLower(strings.TrimSpace(args[0]))
			if len(q) < 2 {
				return usageErr(fmt.Errorf("give at least 2 characters of an email or name"))
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "users find")
			}
			snaps, err := latestSnapshots(cmd.Context(), "")
			if err != nil {
				return err
			}
			hintStalePosture(cmd.ErrOrStderr(), flags, snaps)
			out := []map[string]any{}
			for _, s := range snaps {
				for _, email := range sortedEmails(s.Users) {
					f := s.Users[email]
					if !strings.Contains(email, q) && !strings.Contains(strings.ToLower(f.Name), q) {
						continue
					}
					out = append(out, map[string]any{"company_id": s.CompanyID, "company_name": s.CompanyName, "email": email,
						"name": f.Name, "status": f.Status, "twofa": f.TwoFA, "admin": f.Admin, "last_login": f.LastLogin,
						"inactive": f.Inactive, "security_score": intOrNil(f.Score), "at_risk": intOrNil(f.AtRisk),
						"run_id": s.RunID, "taken_at": s.TakenAt})
				}
			}
			return flags.printJSON(cmd, out)
		},
	}
	return cmd
}
