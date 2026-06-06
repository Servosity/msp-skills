// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// softwareTitleOut summarizes one software title across the fleet.
type softwareTitleOut struct {
	Name             string   `json:"name"`
	Installs         int      `json:"installs"`
	DistinctVersions int      `json:"distinctVersions"`
	Versions         []string `json:"versions"`
}

// pp:data-source local
func newNovelSoftwareAuditCmd(flags *rootFlags) *cobra.Command {
	var namePattern string
	var minVersions int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "software-audit",
		Short: "Aggregate fleet software inventory: distinct-version spread per title",
		Long: `Aggregates the synced software inventory across the whole fleet. For each
software title it counts installs and the number of distinct versions in use,
so version sprawl (the same app at many versions) and license counts surface
in one query. NinjaOne lists software per device but never aggregates it.

Reads the local store; run 'ninjaone-cli sync' first.`,
		Example: `  # Titles fragmented across 3+ versions
  ninjaone-cli software-audit --min-versions 3

  # Where is a specific product installed, as JSON
  ninjaone-cli software-audit --name "TeamViewer" --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openNovelStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, rtSoftware) {
				hintIfStale(cmd, db, rtSoftware, flags.maxAge)
			}

			rows, err := loadRows(db, rtSoftware)
			if err != nil {
				return fmt.Errorf("loading software inventory: %w", err)
			}
			out := aggregateSoftware(rows, namePattern, minVersions)

			if wantsStructured(flags) {
				return flags.printJSON(cmd, out)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no software inventory in local store — run 'ninjaone-cli sync' first")
			}
			if len(out) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No software titles matched.")
				return nil
			}
			tableRows := make([][]string, 0, len(out))
			for _, t := range out {
				tableRows = append(tableRows, []string{
					t.Name,
					fmt.Sprintf("%d", t.Installs),
					fmt.Sprintf("%d", t.DistinctVersions),
					strings.Join(t.Versions, ", "),
				})
			}
			return flags.printTable(cmd, []string{"SOFTWARE", "INSTALLS", "VERSIONS", "VERSION LIST"}, tableRows)
		},
	}
	cmd.Flags().StringVar(&namePattern, "name", "", "Case-insensitive substring match on software title")
	cmd.Flags().IntVar(&minVersions, "min-versions", 1, "Only show titles with at least this many distinct versions")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: local data store)")
	return cmd
}

// aggregateSoftware groups software rows by title, counting installs and
// distinct versions. Split out for table-driven testing.
func aggregateSoftware(rows []map[string]any, namePattern string, minVersions int) []softwareTitleOut {
	type acc struct {
		installs int
		versions map[string]bool
	}
	byTitle := map[string]*acc{}
	pat := strings.ToLower(strings.TrimSpace(namePattern))
	for _, r := range rows {
		name := nvStr(r, "name", "productName", "displayName", "title", "product")
		if name == "" {
			continue
		}
		if pat != "" && !strings.Contains(strings.ToLower(name), pat) {
			continue
		}
		a := byTitle[name]
		if a == nil {
			a = &acc{versions: map[string]bool{}}
			byTitle[name] = a
		}
		a.installs++
		ver := nvStr(r, "version", "productVersion", "displayVersion")
		if ver != "" {
			a.versions[ver] = true
		}
	}
	var out []softwareTitleOut
	for name, a := range byTitle {
		if len(a.versions) < minVersions {
			continue
		}
		vers := make([]string, 0, len(a.versions))
		for v := range a.versions {
			vers = append(vers, v)
		}
		sort.Strings(vers)
		out = append(out, softwareTitleOut{
			Name:             name,
			Installs:         a.installs,
			DistinctVersions: len(a.versions),
			Versions:         vers,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DistinctVersions != out[j].DistinctVersions {
			return out[i].DistinctVersions > out[j].DistinctVersions
		}
		return out[i].Name < out[j].Name
	})
	return out
}
