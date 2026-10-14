// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
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
)

type adminOrgAccess struct {
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
	OrgAccess        string `json:"org_access"`
	AccountStatus    string `json:"account_status"`
	TwoFactor        bool   `json:"two_factor_enabled"`
	HasAPIKey        bool   `json:"has_api_key"`
	LastActive       string `json:"last_active,omitempty"`
	NetworkGrants    int    `json:"network_grants"`
}

type adminSummary struct {
	Email          string           `json:"email"`
	Name           string           `json:"name"`
	OrgCount       int              `json:"org_count"`
	FullAccessOrgs int              `json:"full_access_orgs"`
	No2FAOrgs      int              `json:"no_2fa_orgs"`
	HasAPIKey      bool             `json:"has_api_key"`
	LastActive     string           `json:"last_active,omitempty"`
	Stale          bool             `json:"stale"`
	Flags          []string         `json:"flags"`
	Orgs           []adminOrgAccess `json:"orgs"`
}

type rawAdmin struct {
	Name                 string `json:"name"`
	Email                string `json:"email"`
	OrgAccess            string `json:"orgAccess"`
	AccountStatus        string `json:"accountStatus"`
	TwoFactorAuthEnabled bool   `json:"twoFactorAuthEnabled"`
	HasAPIKey            bool   `json:"hasApiKey"`
	LastActive           string `json:"lastActive"`
	Networks             []any  `json:"networks"`
}

// summarizeAdmins groups admin rows from many orgs by email and flags missing
// 2FA, API keys, pending/locked accounts and access unused for staleAfter.
func summarizeAdmins(byOrg map[fleetOrg][]rawAdmin, staleAfter time.Duration, now time.Time) []adminSummary {
	people := map[string]*adminSummary{}
	for org, admins := range byOrg {
		for _, a := range admins {
			key := strings.ToLower(strings.TrimSpace(a.Email))
			if key == "" {
				continue
			}
			p, ok := people[key]
			if !ok {
				p = &adminSummary{Email: a.Email, Name: a.Name, Flags: []string{}}
				people[key] = p
			}
			p.Orgs = append(p.Orgs, adminOrgAccess{OrganizationID: org.ID, OrganizationName: org.Name, OrgAccess: a.OrgAccess,
				AccountStatus: a.AccountStatus, TwoFactor: a.TwoFactorAuthEnabled, HasAPIKey: a.HasAPIKey, LastActive: a.LastActive,
				NetworkGrants: len(a.Networks)})
			if strings.EqualFold(a.OrgAccess, "full") {
				p.FullAccessOrgs++
			}
			if !a.TwoFactorAuthEnabled {
				p.No2FAOrgs++
			}
			if a.HasAPIKey {
				p.HasAPIKey = true
			}
			if a.LastActive > p.LastActive {
				p.LastActive = a.LastActive
			}
		}
	}
	out := make([]adminSummary, 0, len(people))
	for _, p := range people {
		p.OrgCount = len(p.Orgs)
		sort.Slice(p.Orgs, func(i, j int) bool { return p.Orgs[i].OrganizationName < p.Orgs[j].OrganizationName })
		if p.No2FAOrgs > 0 {
			p.Flags = append(p.Flags, "no_2fa")
		}
		if p.HasAPIKey {
			p.Flags = append(p.Flags, "api_key")
		}
		for _, o := range p.Orgs {
			if s := strings.ToLower(o.AccountStatus); s != "" && s != "ok" {
				p.Flags = append(p.Flags, "account_"+s)
				break
			}
		}
		t, ok := parseMerakiTime(p.LastActive)
		p.Stale = !ok || now.Sub(t) > staleAfter // never signed in counts as stale
		if p.Stale {
			p.Flags = append(p.Flags, "stale")
		}
		out = append(out, *p)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if len(out[i].Flags) != len(out[j].Flags) {
			return len(out[i].Flags) > len(out[j].Flags)
		}
		if out[i].OrgCount != out[j].OrgCount {
			return out[i].OrgCount > out[j].OrgCount
		}
		return strings.ToLower(out[i].Email) < strings.ToLower(out[j].Email)
	})
	return out
}

func newNovelFleetAdminsCmd(flags *rootFlags) *cobra.Command {
	var orgFilter []string
	var email, stale string
	var flaggedOnly bool
	cmd := &cobra.Command{
		Use:   "admins",
		Short: "List Dashboard admins across every org grouped by email, flagging no 2FA, API key holders and stale sign-ins (--stale, --email offboarding view)",
		Long: strings.TrimSpace(`
Lists Dashboard administrators from every organization the API key can see,
grouped into one row per email address. Each row counts the orgs the person
can reach and how many grant full access, and flags no_2fa (two-factor off in
at least one org), api_key (holds an API key), account_<status> (pending,
locked, ...) and stale (no sign-in within --stale). --email gives an
offboarding view: every org that still grants that person access.

Use this command for admin accounts and their access across orgs (offboarding, 2FA gaps, stale access).
Do NOT use this command to locate a device or client; use 'fleet find' instead.`),
		Example: strings.Trim(`
  cisco-meraki-cli fleet admins --stale 90d --agent
  cisco-meraki-cli fleet admins --email departed.tech@msp.example --json
  cisco-meraki-cli fleet admins --flagged --select email,org_count,flags`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet admins")
			}
			staleAfter, err := parseWindow("stale", stale)
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, orgs, err := fleetClientOrgs(ctx, cmd, flags, orgFilter)
			if err != nil {
				return err
			}
			outcomes := fanOrgs(ctx, orgs, func(ctx context.Context, o fleetOrg) ([]rawAdmin, error) {
				if !o.APIEnabled {
					return nil, errOrgAPIDisabled
				}
				data, err := c.GetNoCache(ctx, orgPath("/organizations/{organizationId}/admins", o.ID), nil)
				if err != nil {
					return nil, err
				}
				var admins []rawAdmin
				if err := json.Unmarshal(data, &admins); err != nil {
					return nil, fmt.Errorf("decoding admins: %w", err)
				}
				return admins, nil
			})
			reportFleetFailures(cmd.ErrOrStderr(), outcomes)
			byOrg := map[fleetOrg][]rawAdmin{}
			for _, oc := range outcomes {
				if oc.Err == nil {
					byOrg[oc.Org] = oc.Value
				}
			}
			people := summarizeAdmins(byOrg, staleAfter, time.Now().UTC())
			rows := make([]adminSummary, 0, len(people))
			want := strings.ToLower(strings.TrimSpace(email))
			for _, p := range people {
				if want != "" && strings.ToLower(p.Email) != want {
					continue
				}
				if flaggedOnly && len(p.Flags) == 0 {
					continue
				}
				rows = append(rows, p)
			}
			if want != "" && len(rows) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s is not an admin in any organization the key can see\n", email)
			}
			return printFleetRows(cmd, flags, rows, []string{"email", "name", "org_count", "full_access_orgs", "last_active", "flags"}, "No admins match.")
		},
	}
	addOrgFlag(cmd, &orgFilter)
	cmd.Flags().StringVar(&email, "email", "", "Offboarding view for one admin email: every org that still grants access")
	cmd.Flags().StringVar(&stale, "stale", "90d", "Flag admins with no sign-in for this long, e.g. 30d, 90d")
	cmd.Flags().BoolVar(&flaggedOnly, "flagged", false, "Only admins with at least one flag")
	return cmd
}
