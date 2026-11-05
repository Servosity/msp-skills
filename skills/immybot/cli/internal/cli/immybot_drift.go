// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Absorbed feature (manifest row 38): tenant compliance posture across all titles.
// pp:data-source local

package cli

import (
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type driftTitle struct {
	Title         string `json:"title"`
	TenantVersion string `json:"tenant_version"`
	FleetLatest   string `json:"fleet_latest"`
	ComputerCount int    `json:"computer_count"`
	// ComputersBehind counts the tenant's computers below the fleet latest.
	ComputersBehind int `json:"computers_behind"`
	// LagLevel names the first version segment where the tenant's lowest
	// install differs from the fleet latest: major, minor, patch or build.
	LagLevel string `json:"lag_level,omitempty"`
	// LagDistance is how far behind at that segment (141 vs 128 is 13).
	LagDistance int `json:"lag_distance,omitempty"`
}

// driftTenant is one tenant's posture. ComputersBehind counts the tenant's
// distinct computers behind on at least one title: the union across titles, so
// a computer behind on Chrome and on Edge is one computer, not two.
// UnidentifiedBehind counts the lagging inventory rows inside that total that
// carry no computerId; each stands as its own computer (immyComputerCountKey),
// so the total overstates when two of those rows are one machine. A tenant is
// its tenantId with its name (immyTenantIndex): two tenants that share a name
// stay two.
type driftTenant struct {
	Tenant             string       `json:"tenant"`
	TenantID           string       `json:"tenant_id,omitempty"`
	TitlesTracked      int          `json:"titles_tracked"`
	TitlesBehind       int          `json:"titles_behind"`
	ComputersBehind    int          `json:"computers_behind"`
	UnidentifiedBehind int          `json:"computers_behind_unidentified,omitempty"`
	WorstLag           string       `json:"worst_lag,omitempty"`
	Behind             []driftTitle `json:"behind"`
	lagCounts          [4]int
	worstLevel         int
	worstDistance      int
}

type driftView struct {
	ScannedInstalls int           `json:"scanned_installs"`
	TitlesTracked   int           `json:"titles_tracked"`
	Tenants         []driftTenant `json:"tenants"`
	Note            string        `json:"note,omitempty"`
}

func newNovelDriftCmd(flags *rootFlags) *cobra.Command {
	var (
		flagTenant string
		flagLimit  int
		dbPath     string
	)
	cmd := &cobra.Command{
		Use:   "drift",
		Short: "Per-tenant compliance posture across every software title",
		Long: "Rank tenants by how far their installed software has drifted behind the newest " +
			"version seen anywhere in the fleet. Every computer below the fleet latest counts, " +
			"and tenants rank by lag size (major, then minor, patch, build) before lag count. " +
			"A tenant's computers_behind counts each lagging computer once, however many titles it lags on.\n\n" +
			"Use this command for a whole-tenant compliance posture across all titles. Do NOT " +
			"use this command for the version distribution of one title; use 'version-spread' instead.",
		Example: strings.Trim(`
  immybot-cli drift
  immybot-cli drift --tenant Contoso --agent
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "--limit=25",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "drift")
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			dbPath = immyMirrorPath(dbPath)
			view := driftView{Tenants: make([]driftTenant, 0)}
			if immyMirrorMissing(cmd, dbPath, "tenants-software-from-inventory-dx") {
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				return nil
			}
			db, err := immyOpenMirror(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "tenants-software-from-inventory-dx") {
				hintIfStale(cmd, db, "tenants-software-from-inventory-dx", flags.maxAge)
			}

			rows, err := db.DB().QueryContext(ctx, `
				SELECT json_extract(data,'$.softwareName'), json_extract(data,'$.version'),
				       `+immyComputerSelect("tenants-software-from-inventory-dx")+`
				FROM resources WHERE resource_type = 'tenants-software-from-inventory-dx'`)
			if err != nil {
				return fmt.Errorf("querying software inventory: %w", err)
			}
			// tenant is the immyTenantIndex key, labeled at output.
			type row struct{ title, version, tenantID, tenantName, tenant, computer string }
			all := make([]row, 0)
			tenantIndex := newImmyTenantIndex()
			for rows.Next() {
				var title, version sql.NullString
				var who immyComputerScan
				if err := rows.Scan(append([]any{&title, &version}, who.dest()...)...); err != nil {
					_ = rows.Close()
					return fmt.Errorf("scanning inventory: %w", err)
				}
				c := who.ref()
				tenantIndex.add(c.TenantID, c.TenantName)
				all = append(all, row{title: nullStr(title), version: strings.TrimSpace(nullStr(version)),
					tenantID: c.TenantID, tenantName: c.TenantName, computer: immyComputerCountKey(c.ID, len(all))})
			}
			if err := rows.Err(); err != nil {
				_ = rows.Close()
				return fmt.Errorf("iterating inventory: %w", err)
			}
			if err := rows.Close(); err != nil {
				return fmt.Errorf("closing inventory: %w", err)
			}
			for i := range all {
				all[i].tenant = tenantIndex.key(all[i].tenantID, all[i].tenantName)
			}

			// A tenant lags on a title when ANY of its computers runs below the
			// fleet latest, not only when its newest install does: the newest
			// install on one machine says nothing about the others.
			fleetLatest := map[string]string{}
			type tk struct{ tenant, title string }
			tenantLowest := map[tk]string{}
			tenantComputers := map[tk]map[string]struct{}{}
			tenantTitles := map[string]map[string]struct{}{}

			for _, r := range all {
				view.ScannedInstalls++
				if r.title == "" || r.version == "" || r.tenant == "" {
					continue
				}
				if cur, ok := fleetLatest[r.title]; !ok || immyCompareVersions(r.version, cur) > 0 {
					fleetLatest[r.title] = r.version
				}
				k := tk{r.tenant, r.title}
				if cur, ok := tenantLowest[k]; !ok || immyCompareVersions(r.version, cur) < 0 {
					tenantLowest[k] = r.version
				}
				if tenantComputers[k] == nil {
					tenantComputers[k] = map[string]struct{}{}
				}
				tenantComputers[k][r.computer] = struct{}{}
				if tenantTitles[r.tenant] == nil {
					tenantTitles[r.tenant] = map[string]struct{}{}
				}
				tenantTitles[r.tenant][r.title] = struct{}{}
			}
			view.TitlesTracked = len(fleetLatest)
			// Second pass, now that every fleet latest is known: the computers
			// of each tenant below it, per title.
			behindComputers := map[tk]map[string]struct{}{}
			for _, r := range all {
				if r.title == "" || r.version == "" || r.tenant == "" {
					continue
				}
				if immyCompareVersions(r.version, fleetLatest[r.title]) >= 0 {
					continue
				}
				k := tk{r.tenant, r.title}
				if behindComputers[k] == nil {
					behindComputers[k] = map[string]struct{}{}
				}
				behindComputers[k][r.computer] = struct{}{}
			}

			filter := strings.ToLower(strings.TrimSpace(flagTenant))
			for tenant, titles := range tenantTitles {
				label := tenantIndex.label(tenant)
				if filter != "" && !strings.Contains(strings.ToLower(label), filter) {
					continue
				}
				dt := driftTenant{Tenant: label, TenantID: tenantIndex.id(tenant), TitlesTracked: len(titles), Behind: make([]driftTitle, 0)}
				// The tenant total is the union of its lagging computers across
				// titles; summing the per-title counts would count a computer
				// once per title it lags on.
				lagging := map[string]struct{}{}
				for title := range titles {
					k := tk{tenant, title}
					low := tenantLowest[k]
					want := fleetLatest[title]
					if want == "" || low == "" || len(behindComputers[k]) == 0 {
						continue
					}
					level, distance := immyVersionLag(low, want)
					dt.Behind = append(dt.Behind, driftTitle{
						Title: title, TenantVersion: low, FleetLatest: want,
						ComputerCount:   len(tenantComputers[k]),
						ComputersBehind: len(behindComputers[k]),
						LagLevel:        immyLagLevelNames[level],
						LagDistance:     distance,
					})
					dt.lagCounts[level]++
					if len(dt.Behind) == 1 || level < dt.worstLevel || (level == dt.worstLevel && distance > dt.worstDistance) {
						dt.worstLevel, dt.worstDistance = level, distance
					}
					for key := range behindComputers[k] {
						lagging[key] = struct{}{}
					}
				}
				dt.ComputersBehind = len(lagging)
				dt.UnidentifiedBehind = immyUnidentifiedComputers(lagging)
				dt.TitlesBehind = len(dt.Behind)
				for level, n := range dt.lagCounts {
					if n > 0 {
						dt.WorstLag = immyLagLevelNames[level]
						break
					}
				}
				// Largest lag first inside the tenant, then by title.
				sort.Slice(dt.Behind, func(i, j int) bool {
					li, lj := immyLagLevelRank(dt.Behind[i].LagLevel), immyLagLevelRank(dt.Behind[j].LagLevel)
					if li != lj {
						return li < lj
					}
					if dt.Behind[i].LagDistance != dt.Behind[j].LagDistance {
						return dt.Behind[i].LagDistance > dt.Behind[j].LagDistance
					}
					return dt.Behind[i].Title < dt.Behind[j].Title
				})
				view.Tenants = append(view.Tenants, dt)
			}
			// Rank by lag size: the largest single lag first (its segment,
			// then how far at that segment), then more major-version lags,
			// minor, patch and build, then more lagging computers, then name.
			sort.Slice(view.Tenants, func(i, j int) bool {
				a, b := view.Tenants[i], view.Tenants[j]
				if (a.TitlesBehind > 0) != (b.TitlesBehind > 0) {
					return a.TitlesBehind > 0
				}
				if a.TitlesBehind > 0 {
					if a.worstLevel != b.worstLevel {
						return a.worstLevel < b.worstLevel
					}
					if a.worstDistance != b.worstDistance {
						return a.worstDistance > b.worstDistance
					}
				}
				for level := range a.lagCounts {
					if a.lagCounts[level] != b.lagCounts[level] {
						return a.lagCounts[level] > b.lagCounts[level]
					}
				}
				if a.ComputersBehind != b.ComputersBehind {
					return a.ComputersBehind > b.ComputersBehind
				}
				if a.Tenant != b.Tenant {
					return a.Tenant < b.Tenant
				}
				return a.TenantID < b.TenantID
			})
			if flagLimit > 0 && len(view.Tenants) > flagLimit {
				view.Tenants = view.Tenants[:flagLimit]
			}
			unidentified := 0
			for _, t := range view.Tenants {
				unidentified += t.UnidentifiedBehind
			}
			if view.ScannedInstalls == 0 {
				view.Note = "no software inventory in the local mirror; run 'immybot-cli sync --resources tenants-software-from-inventory-dx'"
			} else if unidentified > 0 {
				view.Note = immyUnidentifiedNote(unidentified, "computers_behind")
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(view.Tenants) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), firstNonEmpty(view.Note, "No tenants matched."))
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-34s %-8s %-10s %-8s %s\n", "TENANT", "BEHIND", "COMPUTERS", "WORST", "TRACKED")
			for _, t := range view.Tenants {
				fmt.Fprintf(cmd.OutOrStdout(), "%-34s %-8d %-10d %-8s %d\n", immyTruncate(t.Tenant, 34), t.TitlesBehind,
					t.ComputersBehind, firstNonEmpty(t.WorstLag, "-"), t.TitlesTracked)
			}
			if view.Note != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "\n%s\n", view.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagTenant, "tenant", "", "Only report tenants whose name contains this substring")
	cmd.Flags().IntVar(&flagLimit, "limit", 50, "Maximum tenants to return (0 for all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror database path")
	return cmd
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		addNovelCommandIfAbsent(root, newNovelDriftCmd(flags))
	})
}

// immyComputerCountKey is the key a distinct-computer count joins an inventory
// row on: its computerId, which DetectedComputerSoftwareResponse carries on
// every row. A row with no computerId fails closed: it gets a key of its own
// (row is its index in the scan), so it counts as one computer. A name or a
// version can belong to two machines, so merging rows by either could hide a
// lagging machine, and dropping the row would hide it outright. The price is
// that one such machine lagging on two titles counts twice, so callers report
// immyUnidentifiedComputers next to the total.
func immyComputerCountKey(computerID string, row int) string {
	if id := strings.TrimSpace(computerID); id != "" {
		return "id:" + id
	}
	return "row:" + strconv.Itoa(row)
}

// immyUnidentifiedComputers counts the keys in a distinct-computer set that
// stand for a row with no computerId.
func immyUnidentifiedComputers(set map[string]struct{}) int {
	n := 0
	for key := range set {
		if strings.HasPrefix(key, "row:") {
			n++
		}
	}
	return n
}

// immyUnidentifiedNote says that a count includes rows with no computerId.
func immyUnidentifiedNote(n int, field string) string {
	return fmt.Sprintf("%d inventory row(s) counted in %s carry no computerId; each is counted as its own computer, "+
		"so %s can count one machine more than once", n, field, field)
}

// immyLagLevelNames names a version lag by the first segment that differs.
var immyLagLevelNames = [4]string{"major", "minor", "patch", "build"}

// immyVersionLag returns the index (0 major .. 3 build or later) of the first
// version segment where have and want differ, and the numeric gap there (1
// when the segment is not a number).
func immyVersionLag(have, want string) (int, int) {
	hs, ws := immySplitVersion(have), immySplitVersion(want)
	n := len(hs)
	if len(ws) > n {
		n = len(ws)
	}
	for i := range n {
		hv, wv := "0", "0"
		if i < len(hs) {
			hv = hs[i]
		}
		if i < len(ws) {
			wv = ws[i]
		}
		if immyCompareVersions(hv, wv) != 0 {
			distance := 1
			if h, err := strconv.Atoi(hv); err == nil {
				if w, err := strconv.Atoi(wv); err == nil && w > h {
					distance = w - h
				}
			}
			if i > 3 {
				i = 3
			}
			return i, distance
		}
	}
	return 3, 1
}

func immyLagLevelRank(name string) int {
	for i, n := range immyLagLevelNames {
		if n == name {
			return i
		}
	}
	return len(immyLagLevelNames)
}
