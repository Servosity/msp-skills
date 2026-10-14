// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature: oauth-apps - third-party OAuth apps with access to client
// data, aggregated by client ID across every user and tenant, ranked by a
// curated scope-risk tier.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"google-workspace-pp-cli/internal/client"
)

// pp:novel-static-reference
// scopeRiskTier is a curated mapping from OAuth scope to risk. high = can read
// or send all mail, read/write all Drive, or act as an admin; moderate =
// read-only access to mail/Drive, any access to every calendar's events, or
// full access to contacts/docs;
// everything else (sign-in, per-file Drive) is low.
func scopeRiskTier(scope string) int {
	if riskHigh[scope] || strings.HasPrefix(scope, "https://www.googleapis.com/auth/admin.") || strings.HasPrefix(scope, "https://www.googleapis.com/auth/apps.") ||
		strings.HasPrefix(scope, "https://www.googleapis.com/auth/gmail.settings.") || strings.HasPrefix(scope, "https://www.googleapis.com/auth/ediscovery") {
		return 3
	}
	if riskModerate[scope] || strings.HasPrefix(scope, "https://www.googleapis.com/auth/script.") {
		return 2
	}
	return 1
}

var riskHigh = map[string]bool{
	"https://mail.google.com/":                       true,
	"https://www.googleapis.com/auth/drive":          true,
	"https://www.googleapis.com/auth/cloud-platform": true,
	"https://www.googleapis.com/auth/gmail.modify":   true,
	"https://www.googleapis.com/auth/gmail.compose":  true,
	"https://www.googleapis.com/auth/gmail.send":     true,
	"https://www.googleapis.com/auth/gmail.insert":   true,
}

var riskModerate = map[string]bool{
	"https://www.googleapis.com/auth/gmail.readonly":           true,
	"https://www.googleapis.com/auth/gmail.metadata":           true,
	"https://www.googleapis.com/auth/drive.readonly":           true,
	"https://www.googleapis.com/auth/drive.metadata":           true,
	"https://www.googleapis.com/auth/drive.metadata.readonly":  true,
	"https://www.googleapis.com/auth/calendar":                 true,
	"https://www.googleapis.com/auth/calendar.events":          true,
	"https://www.googleapis.com/auth/calendar.readonly":        true,
	"https://www.googleapis.com/auth/calendar.events.readonly": true,
	"https://www.googleapis.com/auth/contacts":                 true,
	"https://www.googleapis.com/auth/spreadsheets":             true,
	"https://www.googleapis.com/auth/documents":                true,
	"https://www.googleapis.com/auth/presentations":            true,
}

var (
	tierName = map[int]string{3: "high", 2: "moderate", 1: "low"}
	riskRank = map[string]int{"low": 1, "moderate": 2, "high": 3}
)

// appGrant is one tenant's view of one app (pre-aggregation).
type appGrant struct {
	Tenant   string
	ClientID string
	App      string
	Scopes   []string
	Users    map[string]bool
	Last     string
}

type oauthApp struct {
	ClientID  string         `json:"client_id"`
	App       string         `json:"app"`
	Risk      string         `json:"risk"`
	Users     int            `json:"users"`
	Tenants   []string       `json:"tenants"`
	PerTenant map[string]int `json:"users_per_tenant"`
	Scopes    []string       `json:"scopes"`
	LastSeen  string         `json:"last_seen,omitempty"`
}

// aggregateApps merges per-tenant grants into one row per client ID, ranked
// by risk tier, then tenant count, then users.
func aggregateApps(grants []appGrant) []oauthApp {
	type acc struct {
		app      oauthApp
		scopes   map[string]bool
		tier     int
		tenantOK map[string]bool
	}
	by := map[string]*acc{}
	for _, g := range grants {
		a := by[g.ClientID]
		if a == nil {
			a = &acc{app: oauthApp{ClientID: g.ClientID, App: g.App, PerTenant: map[string]int{}}, scopes: map[string]bool{}, tenantOK: map[string]bool{}}
			by[g.ClientID] = a
		}
		if a.app.App == "" {
			a.app.App = g.App
		}
		for _, s := range g.Scopes {
			a.scopes[s] = true
			if t := scopeRiskTier(s); t > a.tier {
				a.tier = t
			}
		}
		if !a.tenantOK[g.Tenant] {
			a.tenantOK[g.Tenant] = true
			a.app.Tenants = append(a.app.Tenants, g.Tenant)
		}
		a.app.PerTenant[g.Tenant] += len(g.Users)
		a.app.Users += len(g.Users)
		if g.Last != "" && (a.app.LastSeen == "" || laterTS(g.Last, a.app.LastSeen)) {
			a.app.LastSeen = g.Last
		}
	}
	out := make([]oauthApp, 0, len(by))
	tiers := map[string]int{}
	for id, a := range by {
		if a.tier == 0 {
			a.tier = 1
		}
		for s := range a.scopes {
			a.app.Scopes = append(a.app.Scopes, s)
		}
		sort.Strings(a.app.Scopes)
		sort.Strings(a.app.Tenants)
		a.app.Risk = tierName[a.tier]
		tiers[id] = a.tier
		out = append(out, a.app)
	}
	sort.Slice(out, func(i, j int) bool {
		if tiers[out[i].ClientID] != tiers[out[j].ClientID] {
			return tiers[out[i].ClientID] > tiers[out[j].ClientID]
		}
		if len(out[i].Tenants) != len(out[j].Tenants) {
			return len(out[i].Tenants) > len(out[j].Tenants)
		}
		if out[i].Users != out[j].Users {
			return out[i].Users > out[j].Users
		}
		return out[i].ClientID < out[j].ClientID
	})
	return out
}

// inventoryGrants reads each active user's current OAuth tokens.
func inventoryGrants(ctx context.Context, c *client.Client, t gwsTenant, maxUsers int) ([]appGrant, error) {
	users, err := listUsers(ctx, c, t, "isSuspended=false")
	if err != nil {
		return nil, err
	}
	if len(users) > maxUsers {
		return nil, fmt.Errorf("%w: %d active users exceeds --max-users %d (raise it, or use --since for the token audit log)", errScanCapped, len(users), maxUsers)
	}
	by := map[string]*appGrant{}
	for _, u := range users {
		raw, err := getWithQuotaRetry(ctx, c, "/admin/directory/v1/users/"+url.PathEscape(u.ID)+"/tokens", nil)
		if err != nil {
			return nil, fmt.Errorf("%s tokens: %w", u.PrimaryEmail, err)
		}
		var env struct {
			Items []struct {
				ClientID    string   `json:"clientId"`
				DisplayText string   `json:"displayText"`
				Scopes      []string `json:"scopes"`
			} `json:"items"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			return nil, fmt.Errorf("decoding tokens: %w", err)
		}
		for _, tok := range env.Items {
			g := by[tok.ClientID]
			if g == nil {
				g = &appGrant{Tenant: t.Name, ClientID: tok.ClientID, App: tok.DisplayText, Users: map[string]bool{}}
				by[tok.ClientID] = g
			}
			g.Scopes = append(g.Scopes, tok.Scopes...)
			g.Users[u.PrimaryEmail] = true
		}
	}
	out := make([]appGrant, 0, len(by))
	for _, g := range by {
		out = append(out, *g)
	}
	return out, nil
}

// authorizeGrants reads token-log authorize events (apps newly granted).
func authorizeGrants(tenant string, acts []auditActivity) []appGrant {
	by := map[string]*appGrant{}
	for _, a := range acts {
		for i, ev := range a.Events {
			if ev.Name != "authorize" {
				continue
			}
			id := a.param(i, "client_id")
			if id == "" {
				continue
			}
			g := by[id]
			if g == nil {
				g = &appGrant{Tenant: tenant, ClientID: id, App: a.param(i, "app_name"), Users: map[string]bool{}}
				by[id] = g
			}
			if scopes := a.param(i, "scope"); scopes != "" {
				g.Scopes = append(g.Scopes, strings.Split(scopes, ",")...)
			}
			g.Users[a.Actor.Email] = true
			if g.Last == "" || laterTS(a.ID.Time, g.Last) {
				g.Last = a.ID.Time
			}
		}
	}
	out := make([]appGrant, 0, len(by))
	for _, g := range by {
		out = append(out, *g)
	}
	return out
}

func newNovelOauthAppsCmd(flags *rootFlags) *cobra.Command {
	var ff fanoutFlags
	var since, minRisk string
	var maxUsers, maxEvents int
	cmd := &cobra.Command{
		Use:   "oauth-apps",
		Short: "Third-party OAuth apps with access to client data, across every user and tenant, ranked by scope risk",
		Long: "Default mode lists every active user's current OAuth grants (one tokens call per user,\n" +
			"bounded by --max-users) and aggregates them by app client ID across tenants. --since 7d\n" +
			"instead reads the token audit log for apps newly authorized in that window, which is\n" +
			"cheap on large tenants. Risk tiers are a curated scope table: high = all mail, all\n" +
			"Drive, admin or cloud-platform access; moderate = read-only mail/Drive, any calendar\n" +
			"or event access, or full contacts/docs; low = sign-in and per-file access.\n" +
			"Use this command to inventory third-party OAuth apps across every user and client, ranked\n" +
			"by risk. Do NOT use this command to list or revoke one user's tokens; use 'tokens list'\n" +
			"or 'offboard' instead.",
		Example: "  google-workspace-cli oauth-apps --all-tenants --since 7d\n" +
			"  google-workspace-cli oauth-apps --all-tenants --min-risk high --agent --select results.app,results.risk,results.tenants,results.users",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if done, err := helpOrDryRun(cmd, args, flags, "inventory OAuth apps per tenant"); done {
				return err
			}
			floor := riskRank[minRisk]
			if floor == 0 {
				return usageErr(fmt.Errorf("--min-risk must be low, moderate or high"))
			}
			var start time.Time
			if since != "" {
				st, err := parseSince(since, 180)
				if err != nil {
					return err
				}
				start = st
			}
			res, err := fanout(cmd, flags, &ff, func(ctx context.Context, t gwsTenant, c *client.Client) ([]appGrant, error) {
				if start.IsZero() {
					return inventoryGrants(ctx, c, t, maxUsers)
				}
				acts, err := listActivities(ctx, c, "token", "authorize", "", start, maxEvents)
				if err != nil {
					return nil, fmt.Errorf("token audit: %w", err)
				}
				return authorizeGrants(t.Name, acts), nil
			})
			if err != nil {
				return err
			}
			apps := []oauthApp{}
			for _, a := range aggregateApps(res.Results) {
				if riskRank[a.Risk] >= floor {
					apps = append(apps, a)
				}
			}
			out := fanoutResult[oauthApp]{Meta: res.Meta, Results: apps}
			return emitFanout(cmd, flags, out, func(rs []oauthApp) ([]string, [][]string) {
				body := [][]string{}
				for _, r := range rs {
					body = append(body, []string{r.Risk, r.App, fmt.Sprint(len(r.Tenants)), fmt.Sprint(r.Users), r.ClientID})
				}
				return []string{"RISK", "APP", "TENANTS", "USERS", "CLIENT_ID"}, body
			})
		},
	}
	cmd.Flags().BoolVar(&ff.all, "all-tenants", false, allTenantsUsage)
	ff.bind(cmd)
	cmd.Flags().StringVar(&since, "since", "", "Read the token audit log for apps authorized in this window (e.g. 7d) instead of the full per-user inventory")
	cmd.Flags().StringVar(&minRisk, "min-risk", "low", "Only show apps at or above this risk: low, moderate, high")
	cmd.Flags().IntVar(&maxUsers, "max-users", 1000, "Inventory mode: refuse tenants with more active users than this (each user costs one API call)")
	cmd.Flags().IntVar(&maxEvents, "max-events", 5000, "--since mode: cap audit events read per tenant")
	return cmd
}
