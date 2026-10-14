// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"cisco-meraki-pp-cli/internal/client"
	"cisco-meraki-pp-cli/internal/store"
)

// configurationChanges accepts at most a 365-day lookback.
const maxChangeLookback = 365 * 24 * time.Hour

type changeRow struct {
	TS               string `json:"ts"`
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
	NetworkID        string `json:"network_id"`
	NetworkName      string `json:"network_name"`
	AdminName        string `json:"admin_name"`
	AdminEmail       string `json:"admin_email"`
	Page             string `json:"page"`
	Label            string `json:"label"`
	SSIDName         string `json:"ssid_name,omitempty"`
	OldValue         string `json:"old_value"`
	NewValue         string `json:"new_value"`
	Source           string `json:"source"`
}

type changeGroup struct {
	Key           string   `json:"key"`
	ID            string   `json:"id,omitempty"`
	Changes       int      `json:"changes"`
	Organizations []string `json:"organizations"`
	Networks      int      `json:"networks"`
	FirstTS       string   `json:"first_ts"`
	LastTS        string   `json:"last_ts"`
}

type rawChange struct {
	TS          string `json:"ts"`
	AdminName   string `json:"adminName"`
	AdminEmail  string `json:"adminEmail"`
	NetworkID   string `json:"networkId"`
	NetworkName string `json:"networkName"`
	SSIDName    string `json:"ssidName"`
	Page        string `json:"page"`
	Label       string `json:"label"`
	OldValue    string `json:"oldValue"`
	NewValue    string `json:"newValue"`
}

func changeFromRaw(org fleetOrg, raw json.RawMessage, source string) (changeRow, bool) {
	var ch rawChange
	if json.Unmarshal(raw, &ch) != nil || ch.TS == "" {
		return changeRow{}, false
	}
	return changeRow{TS: ch.TS, OrganizationID: org.ID, OrganizationName: org.Name, NetworkID: ch.NetworkID, NetworkName: ch.NetworkName,
		AdminName: ch.AdminName, AdminEmail: ch.AdminEmail, Page: ch.Page, Label: ch.Label, SSIDName: ch.SSIDName,
		OldValue: ch.OldValue, NewValue: ch.NewValue, Source: source}, true
}

type changeFilter struct {
	since   time.Time
	admin   string
	network string
	page    string
}

func (f changeFilter) keep(r changeRow) bool {
	if t, ok := parseMerakiTime(r.TS); ok && t.Before(f.since) {
		return false
	}
	if f.admin != "" && !strings.Contains(strings.ToLower(r.AdminEmail), f.admin) && !strings.Contains(strings.ToLower(r.AdminName), f.admin) {
		return false
	}
	if f.network != "" && strings.ToLower(r.NetworkID) != f.network && !strings.Contains(strings.ToLower(r.NetworkName), f.network) {
		return false
	}
	if f.page != "" && !strings.Contains(strings.ToLower(r.Page), f.page) && !strings.Contains(strings.ToLower(r.Label), f.page) {
		return false
	}
	return true
}

func groupChanges(rows []changeRow, by string) []changeGroup {
	groups := map[string]*changeGroup{}
	orgSets := map[string]map[string]bool{}
	netSets := map[string]map[string]bool{}
	order := []string{}
	for _, r := range rows {
		// Group by identity, label by name: two orgs or networks can share a name.
		var k, label, id string
		switch by {
		case "admin":
			k = strings.ToLower(r.AdminEmail)
			label = r.AdminEmail
			if k == "" {
				k, label = r.AdminName, r.AdminName
			}
		case "org":
			k, label, id = r.OrganizationID, r.OrganizationName, r.OrganizationID
		case "network":
			k, label, id = r.OrganizationID+"|"+r.NetworkID, r.NetworkName, r.NetworkID
			if r.NetworkID == "" {
				label = "(organization-wide) " + r.OrganizationName
			}
		}
		g, ok := groups[k]
		if !ok {
			g = &changeGroup{Key: label, ID: id, FirstTS: r.TS, LastTS: r.TS}
			groups[k], orgSets[k], netSets[k] = g, map[string]bool{}, map[string]bool{}
			order = append(order, k)
		}
		g.Changes++
		if r.TS < g.FirstTS {
			g.FirstTS = r.TS
		}
		if r.TS > g.LastTS {
			g.LastTS = r.TS
		}
		orgSets[k][r.OrganizationName] = true
		if r.NetworkID != "" {
			netSets[k][r.NetworkID] = true
		}
	}
	out := make([]changeGroup, 0, len(order))
	for _, k := range order {
		g := groups[k]
		for o := range orgSets[k] {
			g.Organizations = append(g.Organizations, o)
		}
		sort.Strings(g.Organizations)
		g.Networks = len(netSets[k])
		out = append(out, *g)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Changes != out[j].Changes {
			return out[i].Changes > out[j].Changes
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// liveChanges reads every org's change log between fixed t0 and t1 so all
// pages and the local cutoff describe the same window.
func liveChanges(ctx context.Context, c *client.Client, orgs []fleetOrg, t0, t1 time.Time) ([]changeRow, []orgOutcome[[]changeRow]) {
	bounds := map[string]string{"t0": t0.Format(time.RFC3339), "t1": t1.Format(time.RFC3339)}
	outcomes := fanOrgs(ctx, orgs, func(ctx context.Context, o fleetOrg) ([]changeRow, error) {
		if !o.APIEnabled {
			return nil, errOrgAPIDisabled
		}
		res, err := merakiGetAll(ctx, c, orgPath("/organizations/{organizationId}/configurationChanges", o.ID), bounds, 0)
		if err != nil {
			return nil, err
		}
		rows := make([]changeRow, 0, len(res.Items))
		for _, raw := range res.Items {
			if r, ok := changeFromRaw(o, raw, "live"); ok {
				rows = append(rows, r)
			}
		}
		return rows, nil
	})
	rows := make([]changeRow, 0)
	for _, oc := range outcomes {
		rows = append(rows, oc.Value...)
	}
	return rows, outcomes
}

// localChanges reads the configuration_changes rows `sync` stored, joined to
// the synced organizations table for names.
func localChanges(ctx context.Context, cmd *cobra.Command, flags *rootFlags, dbPath string, orgFilter []string) ([]changeRow, error) {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, errNoMirror
	} else if err != nil {
		return nil, fmt.Errorf("local mirror at %s: %w", dbPath, err)
	}
	db, err := store.OpenReadOnlyContext(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local mirror: %w", err)
	}
	defer db.Close()
	// configuration_changes is an organizations dependent: a complete
	// dependent walk records its own watermark, which is the one --max-age
	// must judge (organizations can be fresher than the change log). Before
	// any complete walk there is no such row, so freshness falls back to
	// organizations rather than always saying "not synced yet".
	anchor := "configuration_changes"
	if st, err := readSyncHintState(db, anchor); err != nil || !st.hasState {
		anchor = "organizations"
	}
	if !hintIfUnsynced(cmd, db, anchor) {
		hintIfStale(cmd, db, anchor, flags.maxAge)
	}
	names := map[string]string{}
	orgRows, err := db.DB().QueryContext(ctx, `SELECT id, COALESCE(name, '') FROM organizations`)
	if err != nil {
		return nil, fmt.Errorf("reading organizations from the local mirror: %w", err)
	}
	for orgRows.Next() {
		var id, name string
		if err := orgRows.Scan(&id, &name); err != nil {
			_ = orgRows.Close()
			return nil, fmt.Errorf("scanning organizations: %w", err)
		}
		names[id] = name
	}
	if err := orgRows.Close(); err != nil {
		return nil, err
	}
	type stored struct {
		org  string
		data string
	}
	raws := make([]stored, 0)
	rows, err := db.DB().QueryContext(ctx, `SELECT organizations_id, data FROM configuration_changes`)
	if err != nil {
		return nil, fmt.Errorf("reading configuration_changes: %w", err)
	}
	for rows.Next() {
		var s stored
		if err := rows.Scan(&s.org, &s.data); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan configuration_changes: %w", err)
		}
		raws = append(raws, s)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()
	keep := fleetOrgFilter(orgFilter)
	out := make([]changeRow, 0, len(raws))
	for _, s := range raws {
		org := fleetOrg{ID: s.org, Name: names[s.org]}
		if !keep(org) {
			continue
		}
		if r, ok := changeFromRaw(org, json.RawMessage(s.data), "local"); ok {
			out = append(out, r)
		}
	}
	return out, nil
}

func newNovelFleetChangesCmd(flags *rootFlags) *cobra.Command {
	var orgFilter []string
	var since, admin, network, page, groupBy, dbPath string
	var limit int
	cmd := &cobra.Command{
		Use:   "changes",
		Short: "List configuration changes from every org in one feed, filtered by --admin, --org, --network or --page, or rolled up with --group-by",
		Long: strings.TrimSpace(`
Configuration changes (the Dashboard change log) from every organization the
API key can see, newest first, with old and new values. --admin, --network and
--page match substrings; --group-by admin|org|network rolls the list up into
counts. With --data-source auto (default) the change logs are read live and the
local mirror written by 'sync' is used only if the live read fails;
--data-source local reads only the mirror.

Use this command to list configuration changes across orgs by admin, org, network or time window.
Do NOT use this command to explain why a network or device went down; use 'fleet blame' instead.`),
		Example: strings.Trim(`
  cisco-meraki-cli fleet changes --since 7d --agent
  cisco-meraki-cli fleet changes --since 7d --group-by admin --agent
  cisco-meraki-cli fleet changes --admin tech@msp.example --network "Main Office" --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet changes")
			}
			window, err := parseWindow("since", since)
			if err != nil {
				return err
			}
			groupBy = strings.ToLower(strings.TrimSpace(groupBy))
			if groupBy != "" && groupBy != "admin" && groupBy != "org" && groupBy != "network" {
				return usageErr(fmt.Errorf("--group-by must be admin, org or network, got %q", groupBy))
			}
			if limit < 0 {
				return usageErr(fmt.Errorf("--limit must be 0 (all) or positive"))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			if dbPath == "" {
				dbPath = defaultDBPath("cisco-meraki-cli")
			}
			warn := func(s string) { fmt.Fprintln(cmd.ErrOrStderr(), s) }
			t0, t1, clamped := fixedWindow(time.Now().UTC(), window, maxChangeLookback)
			if clamped {
				warn("warning: configurationChanges looks back at most 365 days; --since was clamped")
			}
			mirrorHint := func() {
				warn(noMirrorHint(dbPath))
			}
			var rows []changeRow
			switch mode := strings.ToLower(flags.dataSource); mode {
			case "local":
				local, err := localChanges(ctx, cmd, flags, dbPath, orgFilter)
				if errors.Is(err, errNoMirror) {
					mirrorHint()
					local = make([]changeRow, 0)
				} else if err != nil {
					return err
				}
				rows = local
			default:
				c, err := flags.newClient()
				var orgs []fleetOrg
				if err == nil {
					c.NoCache = true
					orgs, err = loadFleetOrgs(ctx, c, orgFilter)
				}
				if err != nil {
					if mode == "live" {
						return classifyAPIError(cmd.OutOrStdout(), err, flags)
					}
					local, lerr := localChanges(ctx, cmd, flags, dbPath, orgFilter)
					if lerr != nil {
						if !errors.Is(lerr, errNoMirror) {
							warn("warning: local mirror unreadable for fallback: " + lerr.Error())
						}
						return classifyAPIError(cmd.OutOrStdout(), err, flags)
					}
					warn("warning: live read failed (" + err.Error() + "); showing the local mirror")
					rows = local
					break
				}
				var outcomes []orgOutcome[[]changeRow]
				rows, outcomes = liveChanges(ctx, c, orgs, t0, t1)
				failed := map[string]bool{}
				for _, oc := range outcomes {
					if oc.Err != nil {
						failed[oc.Org.ID] = true
					}
				}
				if mode != "live" && len(failed) > 0 {
					// auto: an org whose live read failed falls back to its mirrored rows.
					// Scope by the failed orgs' resolved IDs, not by --org names that
					// may have changed since the mirror was written.
					local, lerr := localChanges(ctx, cmd, flags, dbPath, nil)
					if lerr != nil && !errors.Is(lerr, errNoMirror) {
						warn("warning: local mirror unreadable for fallback: " + lerr.Error())
					}
					if lerr == nil {
						recovered := map[string]bool{}
						for _, r := range local {
							if failed[r.OrganizationID] {
								rows = append(rows, r)
								recovered[r.OrganizationID] = true
							}
						}
						for id := range recovered {
							warn("warning: organization " + id + ": live read failed; using its rows from the local mirror (source=local)")
						}
					}
				}
				reportFleetFailures(cmd.ErrOrStderr(), outcomes)
			}
			// The 365-day clamp bounds live requests only; mirrored rows keep
			// the window the caller asked for.
			f := changeFilter{since: t1.Add(-window), admin: strings.ToLower(strings.TrimSpace(admin)),
				network: strings.ToLower(strings.TrimSpace(network)), page: strings.ToLower(strings.TrimSpace(page))}
			kept := make([]changeRow, 0, len(rows))
			for _, r := range rows {
				if f.keep(r) {
					kept = append(kept, r)
				}
			}
			sort.SliceStable(kept, func(i, j int) bool { return kept[i].TS > kept[j].TS })
			if groupBy != "" {
				groups := groupChanges(kept, groupBy)
				if limit > 0 && len(groups) > limit {
					groups = groups[:limit]
				}
				return printFleetRows(cmd, flags, groups, []string{"key", "changes", "organizations", "networks", "last_ts"}, "No configuration changes in the window.")
			}
			if limit > 0 && len(kept) > limit {
				kept = kept[:limit]
			}
			return printFleetRows(cmd, flags, kept, []string{"ts", "organization_name", "network_name", "admin_email", "page", "label"}, "No configuration changes in the window.")
		},
	}
	addOrgFlag(cmd, &orgFilter)
	cmd.Flags().StringVar(&since, "since", "7d", "How far back to look, e.g. 24h, 7d, 30d (live reads cap at 365d)")
	cmd.Flags().StringVar(&admin, "admin", "", "Only changes by an admin whose email or name contains this text")
	cmd.Flags().StringVar(&network, "network", "", "Only changes on a network ID, or a network whose name contains this text")
	cmd.Flags().StringVar(&page, "page", "", "Only changes whose Dashboard page or label contains this text (e.g. SSIDs, Firewall, via API)")
	cmd.Flags().StringVar(&groupBy, "group-by", "", "Roll up into counts by admin, org or network")
	cmd.Flags().IntVar(&limit, "limit", 0, "Return at most N rows or groups (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror path (default: the sync database)")
	return cmd
}
