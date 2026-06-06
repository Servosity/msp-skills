// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel-feature command (Printing Press transcendence): the
// Flight Control fabric map. Joins the synced child_cid, cid_group,
// cid_group_member, user_group, user_group_member, and mssp_role entities into
// one offline tenant roster — the "who/what belongs where" RBAC tree no single
// Falcon API call returns.

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// fleetTenantUserGroup is one user group's role grants on a tenant.
type fleetTenantUserGroup struct {
	UserGroup string   `json:"user_group"`
	Roles     []string `json:"roles,omitempty"`
}

// fleetTenantRow is one child tenant with its fabric memberships.
type fleetTenantRow struct {
	CID        string                 `json:"cid"`
	Name       string                 `json:"name,omitempty"`
	Status     string                 `json:"status,omitempty"`
	CIDGroups  []string               `json:"cid_groups,omitempty"`
	UserGroups []fleetTenantUserGroup `json:"user_groups,omitempty"`
}

// fleetTenantsDiff reports drift between the synced roster and an expected list.
type fleetTenantsDiff struct {
	MissingFromFalcon []string `json:"missing_from_falcon"`
	NotInExpected     []string `json:"not_in_expected"`
}

// fleetTenantsView is the `fleet tenants` response envelope.
type fleetTenantsView struct {
	Tenants      []fleetTenantRow  `json:"tenants"`
	Counts       map[string]int    `json:"counts"`
	ExpectedDiff *fleetTenantsDiff `json:"expected_diff,omitempty"`
	Note         string            `json:"note,omitempty"`
}

// rawObj decodes a fleet entity's Raw payload, returning nil on any failure so
// callers can fall back to the typed columns.
func rawObj(e fleetEntity) map[string]any {
	if len(e.Raw) == 0 {
		return nil
	}
	var o map[string]any
	if err := json.Unmarshal(e.Raw, &o); err != nil {
		return nil
	}
	return o
}

// rawStrings extracts a []string field from a raw payload (tolerating []any).
func rawStrings(o map[string]any, key string) []string {
	if o == nil {
		return nil
	}
	switch v := o[key].(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	}
	return nil
}

// buildTenantFabric joins the fabric-kind entities into the tenant roster.
// Pure function over the loaded entities — the whole point of the feature is
// that this join happens offline against the CID-keyed store.
func buildTenantFabric(ents []fleetEntity) fleetTenantsView {
	view := fleetTenantsView{Counts: map[string]int{}}

	cidGroupName := map[string]string{}   // cid_group_id -> name
	userGroupName := map[string]string{}  // user_group_id -> name
	tenantGroups := map[string][]string{} // child cid -> cid_group names
	groupTenants := map[string][]string{} // cid_group_id -> child cids
	// cid_group_id -> user_group_id -> roles
	groupRoles := map[string]map[string][]string{}

	var tenants []fleetEntity
	for _, e := range ents {
		switch e.Kind {
		case kindChildCID:
			tenants = append(tenants, e)
		case kindCIDGroup:
			o := rawObj(e)
			id := e.ID
			name := e.Name
			if o != nil {
				if v := firstString(o, "cid_group_id", "id"); v != "" {
					id = v
				}
				if v := firstString(o, "name"); v != "" {
					name = v
				}
			}
			if id != "" {
				cidGroupName[id] = name
			}
		case kindUserGroup:
			o := rawObj(e)
			id := e.ID
			name := e.Name
			if o != nil {
				if v := firstString(o, "user_group_id", "id"); v != "" {
					id = v
				}
				if v := firstString(o, "name"); v != "" {
					name = v
				}
			}
			if id != "" {
				userGroupName[id] = name
			}
		case kindCIDGroupMember:
			o := rawObj(e)
			if o == nil {
				continue
			}
			groupID := firstString(o, "cid_group_id", "id")
			if groupID == "" {
				groupID = e.ID
			}
			for _, cid := range rawStrings(o, "cids") {
				groupTenants[groupID] = append(groupTenants[groupID], cid)
			}
		case kindMSSPRole:
			o := rawObj(e)
			if o == nil {
				continue
			}
			cg := firstString(o, "cid_group_id")
			ug := firstString(o, "user_group_id")
			role := firstString(o, "role_id", "role_name")
			if cg == "" || ug == "" || role == "" {
				continue
			}
			if groupRoles[cg] == nil {
				groupRoles[cg] = map[string][]string{}
			}
			groupRoles[cg][ug] = append(groupRoles[cg][ug], role)
		default:
			continue
		}
		view.Counts[e.Kind]++
	}

	// Resolve cid_group membership into per-tenant group-name lists.
	for groupID, cids := range groupTenants {
		name := cidGroupName[groupID]
		if name == "" {
			name = groupID
		}
		for _, cid := range cids {
			tenantGroups[cid] = append(tenantGroups[cid], name)
		}
	}

	rows := make([]fleetTenantRow, 0, len(tenants))
	for _, t := range tenants {
		row := fleetTenantRow{CID: t.ID, Name: t.Name, Status: t.Status}
		row.CIDGroups = append(row.CIDGroups, dedupeStrings(tenantGroups[t.ID])...)
		sort.Strings(row.CIDGroups)

		// Role grants reach a tenant through its cid groups: find each group
		// the tenant belongs to, then each user group granted roles on it.
		ugRoles := map[string][]string{}
		for groupID, cids := range groupTenants {
			member := false
			for _, cid := range cids {
				if cid == t.ID {
					member = true
					break
				}
			}
			if !member {
				continue
			}
			for ug, roles := range groupRoles[groupID] {
				ugRoles[ug] = append(ugRoles[ug], roles...)
			}
		}
		ugs := make([]string, 0, len(ugRoles))
		for ug := range ugRoles {
			ugs = append(ugs, ug)
		}
		sort.Strings(ugs)
		for _, ug := range ugs {
			name := userGroupName[ug]
			if name == "" {
				name = ug
			}
			roles := dedupeStrings(ugRoles[ug])
			sort.Strings(roles)
			row.UserGroups = append(row.UserGroups, fleetTenantUserGroup{UserGroup: name, Roles: roles})
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].CID < rows[j].CID })
	view.Tenants = rows
	if len(rows) == 0 {
		view.Note = "no Flight Control fabric in the local store; run 'fleet sync' (the default kinds include fabric) with a parent-CID API client"
	}
	return view
}

// diffExpected compares the synced roster against an expected CID list.
func diffExpected(rows []fleetTenantRow, expected []string) *fleetTenantsDiff {
	have := map[string]bool{}
	for _, r := range rows {
		have[r.CID] = true
	}
	want := map[string]bool{}
	diff := &fleetTenantsDiff{MissingFromFalcon: []string{}, NotInExpected: []string{}}
	for _, e := range expected {
		if want[e] {
			continue
		}
		want[e] = true
		if !have[e] {
			diff.MissingFromFalcon = append(diff.MissingFromFalcon, e)
		}
	}
	for _, r := range rows {
		if !want[r.CID] {
			diff.NotInExpected = append(diff.NotInExpected, r.CID)
		}
	}
	sort.Strings(diff.MissingFromFalcon)
	sort.Strings(diff.NotInExpected)
	return diff
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	out := in[:0:0]
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// parseExpectedCIDs reads a newline-delimited CID list (# comments allowed).
func parseExpectedCIDs(data []byte) []string {
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// pp:data-source local
func newNovelFleetTenantsCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var expectedPath string
	cmd := &cobra.Command{
		Use:   "tenants",
		Short: "Map the whole Flight Control fabric offline: every child CID, its CID groups, user groups, and role grants",
		Long: "Use this command for the whole-fabric \"who/what belongs where\" RBAC roster " +
			"across all tenants, joined offline from the synced Flight Control objects. " +
			"With --expected, also diff the roster against a newline-delimited CID list to " +
			"catch orphaned or un-onboarded tenants. Run 'fleet sync' first (fabric is in " +
			"the default kinds).\n" +
			"Do NOT use this command for live single-tenant CID CRUD; use the 'mssp' " +
			"commands instead. Do NOT use it for security-posture rollups; use " +
			"'fleet scorecard' instead.",
		Example: "  crowdstrike-cli fleet tenants --json\n" +
			"  crowdstrike-cli fleet tenants --expected cids.txt --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			st, err := openFleetStore(cmd.Context(), resolveFleetDB(dbPath))
			if err != nil {
				return configErr(err)
			}
			defer st.Close()
			if !hintIfFleetUnsynced(cmd, st) {
				hintIfFleetStale(cmd, st, flags.maxAge)
			}
			ents, err := loadFleetEntitiesFrom(cmd.Context(), st, "", true)
			if err != nil {
				return configErr(err)
			}
			view := buildTenantFabric(ents)
			if expectedPath != "" {
				// #nosec G304 -- expectedPath is the user's own --expected CLI flag
				// naming a local CID-list file they chose to read; not untrusted input.
				data, rerr := os.ReadFile(expectedPath)
				if rerr != nil {
					return usageErr(fmt.Errorf("reading --expected file: %w", rerr))
				}
				view.ExpectedDiff = diffExpected(view.Tenants, parseExpectedCIDs(data))
			}
			return flags.printJSON(cmd, view)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite store path (default: standard data dir)")
	cmd.Flags().StringVar(&expectedPath, "expected", "", "Newline-delimited CID list to diff the synced roster against (# comments allowed)")
	return cmd
}
