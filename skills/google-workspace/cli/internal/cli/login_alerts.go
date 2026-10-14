// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature: login-alerts - the login audit events Google raises for
// account compromise, across every client tenant, grouped by user.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"google-workspace-pp-cli/internal/client"
)

// pp:novel-static-reference
// loginAlertEvents are login-application event names from Google's Reports
// API appendix (appendix/activity/login), with a curated severity.
var loginAlertEvents = []struct{ name, severity string }{
	{"account_disabled_hijacked", "critical"},
	{"account_disabled_password_leak", "critical"},
	{"gov_attack_warning", "critical"},
	{"suspicious_login", "high"},
	{"suspicious_programmatic_login", "high"},
	{"suspicious_login_less_secure_app", "high"},
	{"user_signed_out_due_to_suspicious_session_cookie", "high"},
	{"email_forwarding_out_of_domain", "high"},
	{"2sv_disable", "high"},
	{"account_disabled_generic", "medium"},
	{"account_disabled_spamming", "medium"},
	{"account_disabled_spamming_through_relay", "medium"},
	{"risky_sensitive_action_blocked", "medium"},
}

type loginAlert struct {
	Tenant   string `json:"tenant"`
	User     string `json:"user"`
	Severity string `json:"severity"`
	Event    string `json:"event"`
	Count    int    `json:"count"`
	First    string `json:"first"`
	Last     string `json:"last"`
	Detail   string `json:"detail,omitempty"`
}

// groupLoginAlerts is pure: one row per (user, event) with counts and the
// time span. The affected user is affected_email_address when present.
func groupLoginAlerts(tenant, event, severity string, acts []auditActivity) []loginAlert {
	by := map[string]*loginAlert{}
	var order []string
	for _, a := range acts {
		for i, ev := range a.Events {
			if ev.Name != event {
				continue
			}
			user := a.param(i, "affected_email_address")
			if user == "" {
				user = a.Actor.Email
			}
			r := by[user]
			if r == nil {
				r = &loginAlert{Tenant: tenant, User: user, Severity: severity, Event: event, First: a.ID.Time, Last: a.ID.Time}
				by[user] = r
				order = append(order, user)
			}
			r.Count++
			if laterTS(r.First, a.ID.Time) {
				r.First = a.ID.Time
			}
			if laterTS(a.ID.Time, r.Last) {
				r.Last = a.ID.Time
			}
			if dest := a.param(i, "email_forwarding_destination_address"); dest != "" {
				r.Detail = "forwarding to " + dest
			}
		}
	}
	out := make([]loginAlert, 0, len(order))
	for _, u := range order {
		out = append(out, *by[u])
	}
	return out
}

func newNovelLoginAlertsCmd(flags *rootFlags) *cobra.Command {
	var ff fanoutFlags
	var since, events string
	var maxEvents int
	cmd := &cobra.Command{
		Use:   "login-alerts",
		Short: "Suspicious sign-ins, accounts Google disabled, 2SV turned off, and out-of-domain forwarding across clients",
		Long: "Polls each tenant's login audit log for the events Google raises for compromise,\n" +
			"one query per event: critical (account_disabled_hijacked, account_disabled_password_leak,\n" +
			"gov_attack_warning), high (suspicious_login*, suspicious session cookie,\n" +
			"email_forwarding_out_of_domain, 2sv_disable), medium (other account_disabled_*,\n" +
			"risky_sensitive_action_blocked). Rows are grouped per user and event.\n" +
			"Use this command for risky sign-in events in a recent window (suspicious logins, accounts\n" +
			"Google disabled, 2SV turned off). Do NOT use this command for who lacks 2SV today; use\n" +
			"'mfa-gaps' instead. Do NOT use it for Drive sharing changes; use 'sharing-drift' instead.",
		Example: "  google-workspace-cli login-alerts --all-tenants --since 7d\n" +
			"  google-workspace-cli login-alerts --all-tenants --events suspicious_login,2sv_disable --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if done, err := helpOrDryRun(cmd, args, flags, "poll risky login events per tenant"); done {
				return err
			}
			start, err := parseSince(since, 180)
			if err != nil {
				return err
			}
			selected := loginAlertEvents
			if events != "" {
				want := map[string]bool{}
				for _, e := range splitTenantCSV(events) {
					want[e] = true
				}
				selected = nil
				for _, e := range loginAlertEvents {
					if want[e.name] {
						selected = append(selected, e)
						delete(want, e.name)
					}
				}
				if len(selected) == 0 && len(want) == 0 {
					// --events ',,' would scan nothing and report no alerts.
					return usageErr(fmt.Errorf("--events names no event"))
				}
				if len(want) > 0 {
					var unknown []string
					for e := range want {
						unknown = append(unknown, e)
					}
					sort.Strings(unknown)
					return usageErr(fmt.Errorf("unknown --events %s (see --help for the supported list)", strings.Join(unknown, ",")))
				}
			}
			res, err := fanout(cmd, flags, &ff, func(ctx context.Context, t gwsTenant, c *client.Client) ([]loginAlert, error) {
				var out []loginAlert
				for _, e := range selected {
					acts, err := listActivities(ctx, c, "login", e.name, "", start, maxEvents)
					if err != nil {
						return nil, fmt.Errorf("login audit %s: %w", e.name, err)
					}
					out = append(out, groupLoginAlerts(t.Name, e.name, e.severity, acts)...)
				}
				return out, nil
			})
			if err != nil {
				return err
			}
			sortLoginAlerts(res.Results)
			return emitFanout(cmd, flags, res, func(rs []loginAlert) ([]string, [][]string) {
				body := [][]string{}
				for _, r := range rs {
					body = append(body, []string{r.Severity, r.Tenant, r.User, r.Event, fmt.Sprint(r.Count), r.Last})
				}
				return []string{"SEVERITY", "TENANT", "USER", "EVENT", "COUNT", "LAST"}, body
			})
		},
	}
	cmd.Flags().BoolVar(&ff.all, "all-tenants", false, allTenantsUsage)
	ff.bind(cmd)
	cmd.Flags().StringVar(&since, "since", "7d", "Look-back window, e.g. 24h, 7d, 30d (max 180d)")
	cmd.Flags().StringVar(&events, "events", "", "Only these event names, comma-separated (default: all listed in --help)")
	cmd.Flags().IntVar(&maxEvents, "max-events", 2000, "Cap audit events read per event per tenant")
	return cmd
}

// sortLoginAlerts orders by severity, then newest first by instant (a
// string compare puts ...:00Z ahead of the newer ...:00.001Z).
func sortLoginAlerts(rs []loginAlert) {
	sort.SliceStable(rs, func(i, j int) bool {
		a, b := rs[i], rs[j]
		if severityRank[a.Severity] != severityRank[b.Severity] {
			return severityRank[a.Severity] < severityRank[b.Severity]
		}
		return laterTS(a.Last, b.Last)
	})
}
