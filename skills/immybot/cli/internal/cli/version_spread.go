// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Novel feature: cross-tenant software version spread.
// pp:data-source local

package cli

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// versionBucket is one version: ComputerCount counts the distinct computers
// carrying it and TenantCount the distinct tenants (immyTenantIndex: two
// tenants that share a name are two).
type versionBucket struct {
	Version       string   `json:"version"`
	ComputerCount int      `json:"computer_count"`
	TenantCount   int      `json:"tenant_count"`
	BelowFloor    bool     `json:"below_floor"`
	Tenants       []string `json:"tenants"`
}

// tenantLag is one tenant behind the floor. ComputerCount counts its distinct
// computers below the floor; ComputersWithTitle counts all of its computers
// carrying a matched title, at any version.
type tenantLag struct {
	Tenant             string `json:"tenant"`
	TenantID           string `json:"tenant_id,omitempty"`
	LowestVersion      string `json:"lowest_version"`
	ComputerCount      int    `json:"computer_count"`
	ComputersWithTitle int    `json:"computers_with_title"`
}

// versionSpreadView is the version-spread result. BelowFloor counts distinct
// computers below the floor, each once however many old builds or matched
// titles it carries; UnidentifiedBelowFloor counts the rows inside it that
// carry no computerId, each counted as its own computer (immyComputerCountKey).
// ScannedInstalls counts the DetectedComputerSoftwareResponse rows read.
type versionSpreadView struct {
	Title                  string          `json:"title"`
	MinVersion             string          `json:"min_version,omitempty"`
	MatchedInstalls        int             `json:"matched_installs"`
	MatchedTitles          []string        `json:"matched_titles"`
	UnknownVersions        int             `json:"unknown_version_installs"`
	ScannedInstalls        int             `json:"scanned_installs"`
	DistinctVersions       int             `json:"distinct_versions"`
	BelowFloor             int             `json:"computers_below_floor"`
	UnidentifiedBelowFloor int             `json:"computers_below_floor_unidentified,omitempty"`
	Versions               []versionBucket `json:"versions"`
	TenantsBehind          []tenantLag     `json:"tenants_behind"`
	Note                   string          `json:"note,omitempty"`
}

func newNovelVersionSpreadCmd(flags *rootFlags) *cobra.Command {
	var (
		flagMinVersion string
		flagTenant     string
		flagExact      bool
		flagContains   bool
		dbPath         string
	)

	cmd := &cobra.Command{
		Use:   "version-spread [title]",
		Short: "Semver-ordered spread of one software title across every tenant",
		Long: "Semver-ordered distribution of one software title across every tenant, flagging " +
			"everything below a floor. Each tenant in tenants_behind reports its distinct computers " +
			"below the floor (computer_count) and all of its computers carrying the title " +
			"(computers_with_title).\n\n" +
			"Use this command for the version distribution of one software title across every " +
			"tenant. Do NOT use this command for a whole-tenant compliance posture across all " +
			"titles; use 'drift' instead.",
		Example: strings.Trim(`
  immybot-cli version-spread "Google Chrome" --min-version 140
  immybot-cli version-spread "7-Zip" --contains --agent
  immybot-cli version-spread "Google Chrome" --min-version 140 --tenant Contoso --agent
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "title=Google Chrome",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "version-spread")
			}
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a software title is required"))
			}
			title := strings.TrimSpace(args[0])
			if flagExact && flagContains {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--exact and --contains are mutually exclusive"))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			dbPath = immyMirrorPath(dbPath)
			view := versionSpreadView{
				Title:         title,
				MinVersion:    strings.TrimSpace(flagMinVersion),
				Versions:      make([]versionBucket, 0),
				TenantsBehind: make([]tenantLag, 0),
				MatchedTitles: make([]string, 0),
			}
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

			// Installed software is DetectedComputerSoftwareResponse
			// (softwareName, version). computers-inventory rows are
			// DxComputerInventoryScriptResult, inventory script results that
			// carry neither field, so they are not installs and are not read.
			rows, err := db.DB().QueryContext(ctx, `
				SELECT
					json_extract(data,'$.softwareName'),
					json_extract(data,'$.version'),
					`+immyComputerSelect("tenants-software-from-inventory-dx")+`
				FROM resources
				WHERE resource_type = 'tenants-software-from-inventory-dx'`)
			if err != nil {
				return fmt.Errorf("querying software inventory: %w", err)
			}

			type installRow struct{ name, version, tenantID, tenantName, tenant, computer string }
			installs := make([]installRow, 0)
			tenantIndex := newImmyTenantIndex()
			for rows.Next() {
				var name, version sql.NullString
				var who immyComputerScan
				if err := rows.Scan(append([]any{&name, &version}, who.dest()...)...); err != nil {
					_ = rows.Close()
					return fmt.Errorf("scanning software inventory: %w", err)
				}
				c := who.ref()
				tenantIndex.add(c.TenantID, c.TenantName)
				installs = append(installs, installRow{
					name: nullStr(name), version: nullStr(version), tenantID: c.TenantID, tenantName: c.TenantName,
					computer: immyComputerCountKey(c.ID, len(installs)),
				})
			}
			if err := rows.Err(); err != nil {
				_ = rows.Close()
				return fmt.Errorf("iterating software inventory: %w", err)
			}
			if err := rows.Close(); err != nil {
				return fmt.Errorf("closing software inventory: %w", err)
			}

			needle := strings.ToLower(title)
			tenantFilter := strings.ToLower(strings.TrimSpace(flagTenant))
			matchedTitles := map[string]struct{}{}
			candidates := map[string]struct{}{}

			type vAgg struct {
				computers map[string]struct{}
				tenants   map[string]struct{}
			}
			byVersion := map[string]*vAgg{}
			// Keyed by tenant (immyTenantIndex), labeled at output.
			for i := range installs {
				installs[i].tenant = tenantIndex.key(installs[i].tenantID, installs[i].tenantName)
			}
			lowestByTenant := map[string]string{}
			tenantComputers := map[string]map[string]struct{}{}
			tenantBelow := map[string]map[string]struct{}{}
			floor := strings.TrimSpace(flagMinVersion)

			for _, in := range installs {
				view.ScannedInstalls++
				if in.name == "" {
					continue
				}
				// The default is the title itself: a substring also catches
				// sibling products ("Microsoft Edge Update", "... WebView2
				// Runtime") whose versions are a different line. --contains
				// opts in, and matched_titles always names what matched.
				lname := strings.ToLower(in.name)
				if flagContains {
					if !strings.Contains(lname, needle) {
						continue
					}
				} else if !strings.EqualFold(strings.TrimSpace(in.name), title) {
					if strings.Contains(lname, needle) {
						candidates[in.name] = struct{}{}
					}
					continue
				}
				if tenantFilter != "" && !strings.Contains(strings.ToLower(tenantIndex.label(in.tenant)), tenantFilter) {
					continue
				}
				view.MatchedInstalls++
				matchedTitles[in.name] = struct{}{}

				ver := strings.TrimSpace(in.version)
				if ver == "" {
					ver = "(unknown)"
					view.UnknownVersions++
				}
				a := byVersion[ver]
				if a == nil {
					a = &vAgg{computers: map[string]struct{}{}, tenants: map[string]struct{}{}}
					byVersion[ver] = a
				}
				a.computers[in.computer] = struct{}{}
				if in.tenant != "" {
					a.tenants[in.tenant] = struct{}{}
					if tc := tenantComputers[in.tenant]; tc == nil {
						tenantComputers[in.tenant] = map[string]struct{}{}
					}
					tenantComputers[in.tenant][in.computer] = struct{}{}
					if floor != "" && ver != "(unknown)" && immyCompareVersions(ver, floor) < 0 {
						if tenantBelow[in.tenant] == nil {
							tenantBelow[in.tenant] = map[string]struct{}{}
						}
						tenantBelow[in.tenant][in.computer] = struct{}{}
					}
					// The lowest KNOWN version: an undetected version is
					// counted in unknown_version_installs and never stands
					// in for a tenant's lowest, or it would hide a laggard.
					if ver != "(unknown)" {
						if cur, ok := lowestByTenant[in.tenant]; !ok || immyCompareVersions(ver, cur) < 0 {
							lowestByTenant[in.tenant] = ver
						}
					}
				}
			}

			// computers_below_floor is the union of the below-floor buckets'
			// computers: summing their counts would count a computer once per
			// old build or matched title it carries.
			belowFloor := map[string]struct{}{}
			for ver, a := range byVersion {
				labels := make([]string, 0, len(a.tenants))
				for key := range a.tenants {
					labels = append(labels, tenantIndex.label(key))
				}
				sort.Strings(labels)
				b := versionBucket{
					Version:       ver,
					ComputerCount: len(a.computers),
					TenantCount:   len(a.tenants),
					Tenants:       labels,
				}
				if floor != "" && ver != "(unknown)" && immyCompareVersions(ver, floor) < 0 {
					b.BelowFloor = true
					for key := range a.computers {
						belowFloor[key] = struct{}{}
					}
				}
				view.Versions = append(view.Versions, b)
			}
			view.BelowFloor = len(belowFloor)
			view.UnidentifiedBelowFloor = immyUnidentifiedComputers(belowFloor)
			// Highest version first, so the newest build heads the table and
			// the laggards sort to the bottom where they are read.
			sort.Slice(view.Versions, func(i, j int) bool {
				return immyCompareVersions(view.Versions[i].Version, view.Versions[j].Version) > 0
			})
			view.DistinctVersions = len(view.Versions)

			if floor != "" {
				for tenant, low := range lowestByTenant {
					if immyCompareVersions(low, floor) >= 0 {
						continue
					}
					// computer_count is the computers below the floor, the
					// count a tenant is listed for; computers_with_title is
					// every computer of the tenant carrying the title.
					view.TenantsBehind = append(view.TenantsBehind, tenantLag{
						Tenant:             tenantIndex.label(tenant),
						TenantID:           tenantIndex.id(tenant),
						LowestVersion:      low,
						ComputerCount:      len(tenantBelow[tenant]),
						ComputersWithTitle: len(tenantComputers[tenant]),
					})
				}
				sort.Slice(view.TenantsBehind, func(i, j int) bool {
					a, b := view.TenantsBehind[i], view.TenantsBehind[j]
					if c := immyCompareVersions(a.LowestVersion, b.LowestVersion); c != 0 {
						return c < 0
					}
					if a.Tenant != b.Tenant {
						return a.Tenant < b.Tenant
					}
					return a.TenantID < b.TenantID
				})
			}

			view.MatchedTitles = sortedSetSlice(matchedTitles)
			if view.ScannedInstalls == 0 {
				view.Note = "no software inventory in the local mirror; run 'immybot-cli sync --resources tenants-software-from-inventory-dx'"
			} else if view.MatchedInstalls == 0 && len(candidates) > 0 {
				names := sortedSetSlice(candidates)
				more := ""
				if len(names) > 10 {
					more = fmt.Sprintf(" (and %d more)", len(names)-10)
					names = names[:10]
				}
				view.Note = fmt.Sprintf("no software titled exactly %q; titles containing it: %s%s. "+
					"Pass one of them as the title, or add --contains to match every title containing %q",
					title, strings.Join(names, "; "), more, title)
			} else if view.MatchedInstalls == 0 {
				view.Note = fmt.Sprintf("scanned %d inventory rows without matching software title %q; "+
					"re-run 'immybot-cli sync --resources tenants-software-from-inventory-dx' if the mirror is stale",
					view.ScannedInstalls, title)
			} else if view.UnidentifiedBelowFloor > 0 {
				view.Note = immyUnidentifiedNote(view.UnidentifiedBelowFloor, "computers_below_floor")
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(view.Versions) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No installs of %q found (scanned %d inventory rows).\n", title, view.ScannedInstalls)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-8s %-8s %s\n", "VERSION", "MACHS", "TENANTS", "")
			for _, b := range view.Versions {
				marker := ""
				if b.BelowFloor {
					marker = "BELOW FLOOR"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-8d %-8d %s\n", immyTruncate(b.Version, 24), b.ComputerCount, b.TenantCount, marker)
			}
			if floor != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "\n%d computer(s) below %s across %d tenant(s).\n",
					view.BelowFloor, floor, len(view.TenantsBehind))
				for _, t := range view.TenantsBehind {
					fmt.Fprintf(cmd.OutOrStdout(), "  %-40s lowest %s (%d of %d machines below %s)\n", immyTruncate(t.Tenant, 40), t.LowestVersion,
						t.ComputerCount, t.ComputersWithTitle, floor)
				}
				if view.UnidentifiedBelowFloor > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "\n%s\n", view.Note)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagMinVersion, "min-version", "", "Flag installs below this version (semver-aware comparison)")
	cmd.Flags().StringVar(&flagTenant, "tenant", "", "Only consider inventory rows whose tenant name contains this substring")
	cmd.Flags().BoolVar(&flagExact, "exact", false, "Match the software title exactly, ignoring case (the default; kept for scripts)")
	cmd.Flags().BoolVar(&flagContains, "contains", false, "Match every software title that contains the given text (matched_titles lists them)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror database path")
	return cmd
}
