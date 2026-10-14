// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature: posture - one row per client tenant rolling up 2SV coverage,
// admin exposure, licence waste, Drive, Calendar and app-sharing setting changes and (with the policy
// scope profile) the current Cloud Identity policy values.

// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"google-workspace-pp-cli/internal/client"
)

type tenantPosture struct {
	Tenant              string   `json:"tenant"`
	Domain              string   `json:"domain"`
	Status              string   `json:"status"` // critical | warn | ok
	ActiveUsers         int      `json:"active_users"`
	TwoSVEnrolledPct    float64  `json:"twosv_enrolled_pct"`
	UsersWithout2SV     int      `json:"users_without_2sv"`
	Admins              int      `json:"admins"`
	AdminsWithout2SV    int      `json:"admins_without_2sv"`
	StaleLicenses       int      `json:"stale_licenses"`
	SuspendedLicensed   int      `json:"suspended_licensed"`
	SharingSettingEdits int      `json:"drive_calendar_setting_changes"`
	TwoSVEnforcement    string   `json:"policy_2sv_enforcement"`
	DriveExternalShare  string   `json:"policy_drive_external_sharing"`
	GmailAutoForwarding string   `json:"policy_gmail_auto_forwarding"`
	Reasons             []string `json:"reasons,omitempty"`
}

// rollupPosture is pure: it turns one tenant's users, licences and sharing
// setting changes into a posture row. Rules: any admin without 2SV is
// critical; 2SV coverage under minPct, any stale licence, or any sharing
// setting change in the window is a warning.
func rollupPosture(t gwsTenant, users []dirUser, licenses []licenseAssignment, settingEdits int, now time.Time, inactiveDays, graceDays int, minPct float64) tenantPosture {
	p := tenantPosture{Tenant: t.Name, Domain: t.Domain, SharingSettingEdits: settingEdits}
	enrolled := 0
	for _, u := range users {
		if u.Suspended || u.Archived {
			continue
		}
		p.ActiveUsers++
		admin := adminRole(u) != ""
		if admin {
			p.Admins++
		}
		if u.IsEnrolledIn2Sv {
			enrolled++
			continue
		}
		p.UsersWithout2SV++
		if admin {
			p.AdminsWithout2SV++
		}
	}
	if p.ActiveUsers > 0 {
		p.TwoSVEnrolledPct = float64(int(float64(enrolled)*1000/float64(p.ActiveUsers)+0.5)) / 10
	}
	for _, s := range staleLicensesFor(t.Name, licenses, users, now, inactiveDays, graceDays) {
		p.StaleLicenses++
		if s.Reason == "suspended" {
			p.SuspendedLicensed++
		}
	}
	p.Status = "ok"
	if p.AdminsWithout2SV > 0 {
		p.Status = "critical"
		p.Reasons = append(p.Reasons, fmt.Sprintf("%d admin(s) without 2SV", p.AdminsWithout2SV))
	}
	warn := func(cond bool, reason string) {
		if cond {
			if p.Status == "ok" {
				p.Status = "warn"
			}
			p.Reasons = append(p.Reasons, reason)
		}
	}
	// Cross-multiplied, so neither display rounding nor float division moves
	// a tenant across the threshold: 1,899 of 2,000 is below 95% (it displays
	// as 95.0), and 57 of 100 is not below 57% (0.57*100 is 56.999...).
	warn(p.ActiveUsers > 0 && float64(enrolled)*100 < minPct*float64(p.ActiveUsers),
		fmt.Sprintf("2SV coverage %d of %d users, below %g%%", enrolled, p.ActiveUsers, minPct))
	warn(p.StaleLicenses > 0, fmt.Sprintf("%d stale licence(s)", p.StaleLicenses))
	warn(settingEdits > 0, fmt.Sprintf("%d setting change(s) (Drive, Calendar, app sharing)", settingEdits))
	return p
}

var postureRank = map[string]int{"critical": 0, "warn": 1, "ok": 2, "error": 3}

const policyScope = "https://www.googleapis.com/auth/cloud-identity.policies.readonly"

// #nosec G101 -- public Google API endpoint, not a credential
const policiesURL = "https://cloudidentity.googleapis.com/v1/policies"

type policyColumn struct {
	setting string   // Policy API setting type
	keys    []string // value keys to read (camelCase and snake_case forms)
	present bool     // report "enforced" when the key is present, not its value
	def     string   // the value when the winning policy omits the field (Google's documented default)
}

var postureColumns = []policyColumn{
	{setting: "security.two_step_verification_enforcement", keys: []string{"enforcedFrom", "enforced_from"}, present: true, def: "not_enforced"},
	{setting: "drive_and_docs.external_sharing", keys: []string{"externalSharingMode", "external_sharing_mode"}, def: "ALLOWED"},
	{setting: "gmail.auto_forwarding", keys: []string{"enableAutoForwarding", "enable_auto_forwarding"}, def: "true"},
}

// summarizePolicyValues collapses policies into one cell, applying Google's
// Max reduction (all three columns use it): within one applicability scope
// the policy with the greatest sortOrder wins, and a field it omits takes
// Google's documented default, so a system default overridden by an admin
// policy for the same users is not a difference. The scope is the policy's
// CEL query (org unit, group and licence conditions), or its orgUnit/group
// when the query is absent. The cell is the single winning value,
// "mixed:a,b" when scopes resolve differently, or "default" when no policy
// is set.
// ponytail: scopes are not resolved against the OU tree, so mixed means the
// scopes differ; read orgunits and walk inheritance if that ever matters.
func summarizePolicyValues(col policyColumn, policies []map[string]any, now time.Time) string {
	type winner struct {
		order float64
		value string
	}
	winners := map[string]winner{}
	for _, p := range policies {
		setting, _ := p["setting"].(map[string]any)
		value, _ := setting["value"].(map[string]any)
		v := col.def
		for _, k := range col.keys {
			if raw, ok := value[k]; ok {
				if col.present {
					v = enforcementState(fmt.Sprint(raw), now)
				} else {
					v = fmt.Sprint(raw)
				}
				break
			}
		}
		q, _ := p["policyQuery"].(map[string]any)
		scope, _ := q["query"].(string)
		if scope == "" {
			ou, _ := q["orgUnit"].(string)
			group, _ := q["group"].(string)
			scope = ou + "|" + group
		}
		order, _ := q["sortOrder"].(float64)
		if w, ok := winners[scope]; ok && w.order >= order {
			continue
		}
		winners[scope] = winner{order, v}
	}
	seen := map[string]bool{}
	var vals []string
	for _, w := range winners {
		if !seen[w.value] {
			seen[w.value] = true
			vals = append(vals, w.value)
		}
	}
	switch len(vals) {
	case 0:
		return "default"
	case 1:
		return vals[0]
	}
	sort.Strings(vals)
	return "mixed:" + strings.Join(vals, ",")
}

// enforcementState: enforcedFrom in the past means enforced today; in the
// future it is only scheduled.
func enforcementState(from string, now time.Time) string {
	t, err := time.Parse(time.RFC3339, from)
	switch {
	case err != nil:
		return "invalid:" + from
	case t.After(now):
		return "scheduled:" + t.UTC().Format("2006-01-02")
	}
	return "enforced"
}

func fetchPolicyColumn(ctx context.Context, c *client.Client, col policyColumn, now time.Time) (string, error) {
	params := map[string]string{"filter": "setting.type.matches('" + col.setting + "')", "pageSize": "100"}
	raw, err := listAll(ctx, c, policiesURL, params, "policies", 0)
	if err != nil {
		return "", fmt.Errorf("policy %s: %w", col.setting, err)
	}
	policies := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		var p map[string]any
		if err := json.Unmarshal(r, &p); err != nil {
			return "", err
		}
		policies = append(policies, p)
	}
	return summarizePolicyValues(col, policies, now), nil
}

// tenantPolicies fills the policy columns, or "not_granted" when the tenant
// did not register the policy scope profile.
func tenantPolicies(ctx context.Context, c *client.Client, t gwsTenant, p *tenantPosture, now time.Time) error {
	if len(hasScopes(t, []string{policyScope})) > 0 {
		p.TwoSVEnforcement, p.DriveExternalShare, p.GmailAutoForwarding = "not_granted", "not_granted", "not_granted"
		return nil
	}
	cells := []*string{&p.TwoSVEnforcement, &p.DriveExternalShare, &p.GmailAutoForwarding}
	for i, col := range postureColumns {
		v, err := fetchPolicyColumn(ctx, c, col, now)
		if err != nil {
			return err
		}
		*cells[i] = v
	}
	return nil
}

func newNovelPostureCmd(flags *rootFlags) *cobra.Command {
	var ff fanoutFlags
	var since string
	var inactiveDays, graceDays, maxEvents int
	var minPct float64
	cmd := &cobra.Command{
		Use:   "posture",
		Short: "One row per client tenant: 2SV coverage, admins without 2SV, stale licences, Drive, Calendar and app-sharing setting changes",
		Long: "The cross-client security and licence summary for QBRs and weekly review.\n" +
			"Each tenant is graded critical (an admin without 2SV), warn (2SV coverage below\n" +
			"--min-2sv-pct, any stale licence, or any Drive, Calendar or sharing setting change in --since), or ok;\n" +
			"a tenant that could not be read is an explicit error row, never a zeroed one.\n" +
			"Tenants registered with the 'policy' scope profile also report the current 2SV\n" +
			"enforcement, Drive external sharing mode and Gmail auto-forwarding policy\n" +
			"(Google's reduction: per scope - org unit, group and licence condition - the\n" +
			"highest-priority policy wins and a field it omits is Google's default;\n" +
			"scopes that differ show as mixed:...); others read not_granted.\n" +
			"Use this command for a one-row-per-client security and licence-waste summary. Do NOT\n" +
			"use it to list individual users; use 'mfa-gaps' (2SV), 'stale-licenses' (licence waste),\n" +
			"or 'leavers' (incomplete offboarding) instead. Do NOT use it to list sharing events; use\n" +
			"'sharing-drift' instead.",
		Example: "  google-workspace-cli posture --all-tenants\n" +
			"  google-workspace-cli posture --all-tenants --agent --select results.tenant,results.status,results.twosv_enrolled_pct,results.admins_without_2sv",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if done, err := helpOrDryRun(cmd, args, flags, "summarise posture per tenant"); done {
				return err
			}
			start, err := parseSince(since, 180)
			if err != nil {
				return err
			}
			// The same bounds stale-licenses enforces; NaN fails the range check.
			if inactiveDays < 1 || graceDays < 0 || !(minPct >= 0 && minPct <= 100) {
				return usageErr(fmt.Errorf("--inactive-days must be >= 1, --grace-days >= 0 and --min-2sv-pct between 0 and 100"))
			}
			now := time.Now().UTC()
			res, err := fanout(cmd, flags, &ff, func(ctx context.Context, t gwsTenant, c *client.Client) ([]tenantPosture, error) {
				users, err := listUsers(ctx, c, t, "")
				if err != nil {
					return nil, err
				}
				custID, err := tenantCustomerID(ctx, c, t)
				if err != nil {
					return nil, err
				}
				licenses, err := listProductLicenses(ctx, c, custID, defaultLicenceProducts)
				if err != nil {
					return nil, err
				}
				edits := 0
				// Hand-fix (handfixes.json sharing-drift-reads-created-deleted-settings):
				// the same events sharing-drift reads, so a sharing change it
				// reports is never missing here.
				for _, ev := range settingChangeEvents {
					acts, err := listActivities(ctx, c, "admin", ev, "", start, maxEvents)
					if err != nil {
						return nil, fmt.Errorf("admin audit %s: %w", ev, err)
					}
					edits += len(settingChanges(t.Name, acts))
				}
				p := rollupPosture(t, users, licenses, edits, now, inactiveDays, graceDays, minPct)
				if err := tenantPolicies(ctx, c, t, &p, now); err != nil {
					return nil, err
				}
				return []tenantPosture{p}, nil
			})
			if err != nil {
				return err
			}
			// A failed tenant is a visible error row, not a missing or zeroed one.
			for _, f := range res.Meta.FetchFailures {
				res.Results = append(res.Results, tenantPosture{Tenant: f.Tenant, Status: "error", Reasons: []string{f.Error}})
			}
			rows := res.Results
			sort.SliceStable(rows, func(i, j int) bool {
				if postureRank[rows[i].Status] != postureRank[rows[j].Status] {
					return postureRank[rows[i].Status] < postureRank[rows[j].Status]
				}
				return rows[i].Tenant < rows[j].Tenant
			})
			return emitFanout(cmd, flags, res, func(rs []tenantPosture) ([]string, [][]string) {
				body := [][]string{}
				for _, r := range rs {
					body = append(body, []string{r.Status, r.Tenant, fmt.Sprint(r.ActiveUsers), fmt.Sprintf("%.1f", r.TwoSVEnrolledPct),
						fmt.Sprint(r.AdminsWithout2SV), fmt.Sprint(r.StaleLicenses), fmt.Sprint(r.SharingSettingEdits), r.DriveExternalShare})
				}
				return []string{"STATUS", "TENANT", "USERS", "2SV_%", "ADMINS_NO_2SV", "STALE_LIC", "SETTING_CHANGES", "DRIVE_SHARING"}, body
			})
		},
	}
	cmd.Flags().BoolVar(&ff.all, "all-tenants", false, allTenantsUsage)
	ff.bind(cmd)
	cmd.Flags().StringVar(&since, "since", "30d", "Window for Drive, Calendar and app-sharing setting changes, e.g. 7d, 30d")
	cmd.Flags().IntVar(&maxEvents, "max-events", 1000, "Cap setting-change audit events read per query per tenant")
	cmd.Flags().IntVar(&inactiveDays, "inactive-days", 60, "Licence counts as stale after this many days without sign-in")
	cmd.Flags().IntVar(&graceDays, "grace-days", 14, "Skip accounts created within this many days")
	cmd.Flags().Float64Var(&minPct, "min-2sv-pct", 95, "Warn when 2SV enrollment is below this percentage")
	return cmd
}
