// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (keeper handfix posture-report).
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var riskKinds = []string{"no-2fa", "at-risk", "weak", "reused"}

func newNovelUsersRiskCmd(flags *rootFlags) *cobra.Command {
	var kinds []string
	var adminsOnly bool
	var minCount int
	var mc string
	cmd := &cobra.Command{
		Use:   "risk",
		Short: "List users without 2FA (admins outside any 2FA-enforcing role first), BreachWatch at-risk and weak or reused password users across every client",
		Long: `Use this command to list individual users who weaken security across all companies: no 2FA (admins first), BreachWatch at-risk, weak or reused passwords.
Do NOT use it for per-company coverage percentages; use 'posture report' instead.
Do NOT use it for inactive, invited or locked accounts; use 'users hygiene' instead.

Reads each company's newest stored posture run (no network call). no-2fa lists active users with
2FA off; admins in no role that enforces 2FA sort first. weak and reused list users whose counts
are at least --min. at-risk lists BreachWatch at-risk users (companies without BreachWatch have
none to list).`,
		Example: strings.Trim(`
  keeper-cli users risk --kind no-2fa --agent
  keeper-cli users risk --kind no-2fa --admins-only --agent
  keeper-cli users risk --kind weak,reused --min 5 --mc 4421 --agent`, "\n"),
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			want, err := parseKinds(kinds, riskKinds)
			if err != nil {
				return err
			}
			if minCount < 1 {
				return usageErr(fmt.Errorf("--min must be at least 1"))
			}
			if mc, err = validateMC(mc); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "users risk")
			}
			snaps, err := latestSnapshots(cmd.Context(), mc)
			if err != nil {
				return err
			}
			hintStalePosture(cmd.ErrOrStderr(), flags, snaps)
			type hit struct {
				row      map[string]any
				priority int
			}
			var hits []hit
			for _, s := range snaps {
				for _, email := range sortedEmails(s.Users) {
					f := s.Users[email]
					if adminsOnly && !f.Admin {
						continue
					}
					add := func(kind string, priority int) {
						hits = append(hits, hit{priority: priority, row: map[string]any{"company_id": s.CompanyID, "company_name": s.CompanyName,
							"email": email, "name": f.Name, "kind": kind, "admin": f.Admin, "twofa_enforced": f.TwoFAEnforced,
							"weak": intOrNil(f.Weak), "reused": intOrNil(f.Reused), "at_risk": intOrNil(f.AtRisk),
							"security_score": intOrNil(f.Score), "run_id": s.RunID, "taken_at": s.TakenAt}})
					}
					if want["no-2fa"] && strings.EqualFold(f.Status, "active") && !f.TwoFA {
						p := 2
						if f.Admin && !f.TwoFAEnforced {
							p = 0
						} else if f.Admin {
							p = 1
						}
						add("no-2fa", p)
					}
					if want["at-risk"] && f.AtRisk != nil && *f.AtRisk > 0 {
						add("at-risk", 3)
					}
					if want["weak"] && f.Weak != nil && *f.Weak >= minCount {
						add("weak", 4)
					}
					if want["reused"] && f.Reused != nil && *f.Reused >= minCount {
						add("reused", 5)
					}
				}
			}
			sort.SliceStable(hits, func(i, j int) bool { return hits[i].priority < hits[j].priority })
			out := make([]map[string]any, 0, len(hits))
			for _, h := range hits {
				out = append(out, h.row)
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().StringSliceVar(&kinds, "kind", nil, "Kinds to list: no-2fa, at-risk, weak, reused (default all)")
	cmd.Flags().BoolVar(&adminsOnly, "admins-only", false, "Only users holding an admin role")
	cmd.Flags().IntVar(&minCount, "min", 1, "For weak and reused: the smallest count to list")
	cmd.Flags().StringVar(&mc, "mc", "", "Only this managed company id (company_id; \"self\" for a non-MSP account)")
	return cmd
}
