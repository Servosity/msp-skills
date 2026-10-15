// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature: mfa-gaps — users without 2-Step Verification
// across every client tenant, admins first.

// pp:data-source live
// pp:client-call users.list through listUsers -> listAll -> getWithQuotaRetry -> client.GetNoCache (gws_fanout.go)

package cli

import (
	"context"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"google-workspace-pp-cli/internal/client"
)

type mfaGap struct {
	Tenant         string `json:"tenant"`
	Email          string `json:"email"`
	Name           string `json:"name"`
	Severity       string `json:"severity"`
	AdminRole      string `json:"admin_role,omitempty"`
	Enforced       bool   `json:"enforced_2sv"`
	LastLogin      string `json:"last_login"`
	DaysSinceLogin int    `json:"days_since_login"`
	OrgUnit        string `json:"org_unit"`
}

var severityRank = map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}

// classifyMFAGap: an admin without 2SV is critical; an active user is high;
// an account idle 30+ days is medium; one that never signed in is low.
func classifyMFAGap(u dirUser, now time.Time) string {
	if adminRole(u) != "" {
		return "critical"
	}
	d := daysSince(u.LastLoginTime, now)
	switch {
	case d < 0:
		return "low"
	case d <= 30:
		return "high"
	}
	return "medium"
}

func mfaGapsForUsers(tenant string, users []dirUser, now time.Time, includeSuspended, adminsOnly bool) []mfaGap {
	out := []mfaGap{}
	for _, u := range users {
		if u.IsEnrolledIn2Sv || (u.Suspended && !includeSuspended) || u.Archived {
			continue
		}
		if adminsOnly && adminRole(u) == "" {
			continue
		}
		out = append(out, mfaGap{
			Tenant: tenant, Email: u.PrimaryEmail, Name: u.Name.FullName,
			Severity: classifyMFAGap(u, now), AdminRole: adminRole(u), Enforced: u.IsEnforcedIn2Sv,
			LastLogin: u.LastLoginTime, DaysSinceLogin: daysSince(u.LastLoginTime, now), OrgUnit: u.OrgUnitPath,
		})
	}
	return out
}

func sortMFAGaps(rows []mfaGap) {
	sort.SliceStable(rows, func(i, j int) bool {
		if severityRank[rows[i].Severity] != severityRank[rows[j].Severity] {
			return severityRank[rows[i].Severity] < severityRank[rows[j].Severity]
		}
		if rows[i].Tenant != rows[j].Tenant {
			return rows[i].Tenant < rows[j].Tenant
		}
		return rows[i].Email < rows[j].Email
	})
}

func newNovelMfaGapsCmd(flags *rootFlags) *cobra.Command {
	var ff fanoutFlags
	var includeSuspended, adminsOnly bool
	cmd := &cobra.Command{
		Use:   "mfa-gaps",
		Short: "Users without 2-Step Verification across client tenants, admins first",
		Long: "Lists every account not enrolled in 2-Step Verification, ranked: admins (critical),\n" +
			"users active in the last 30 days (high), idle accounts (medium), never-signed-in (low).\n" +
			"Use this command for the 2SV enrollment sweep. Do NOT use it for licence waste;\n" +
			"use 'stale-licenses' instead, or 'posture' for a one-row-per-client summary.",
		Example: "  google-workspace-cli mfa-gaps --all-tenants\n" +
			"  google-workspace-cli mfa-gaps --all-tenants --admins-only --agent --select results.tenant,results.email,results.severity",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if done, err := helpOrDryRun(cmd, args, flags, "list users without 2SV per tenant"); done {
				return err
			}
			now := time.Now().UTC()
			res, err := fanout(cmd, flags, &ff, func(ctx context.Context, t gwsTenant, c *client.Client) ([]mfaGap, error) {
				q := "isEnrolledIn2Sv=false"
				if !includeSuspended {
					q += " isSuspended=false"
				}
				users, err := listUsers(ctx, c, t, q)
				if err != nil {
					return nil, err
				}
				return mfaGapsForUsers(t.Name, users, now, includeSuspended, adminsOnly), nil
			})
			if err != nil {
				return err
			}
			sortMFAGaps(res.Results)
			return emitFanout(cmd, flags, res, func(rs []mfaGap) ([]string, [][]string) {
				body := [][]string{}
				for _, r := range rs {
					body = append(body, []string{r.Severity, r.Tenant, r.Email, r.AdminRole, boolYes(r.Enforced), itoaOrDash(r.DaysSinceLogin)})
				}
				return []string{"SEVERITY", "TENANT", "EMAIL", "ADMIN", "ENFORCED", "DAYS_SINCE_LOGIN"}, body
			})
		},
	}
	cmd.Flags().BoolVar(&ff.all, "all-tenants", false, allTenantsUsage)
	ff.bind(cmd)
	cmd.Flags().BoolVar(&includeSuspended, "include-suspended", false, "Include suspended accounts")
	cmd.Flags().BoolVar(&adminsOnly, "admins-only", false, "Only super and delegated admins")
	return cmd
}

func boolYes(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
