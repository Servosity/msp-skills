// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type exclusionFinding struct {
	Value    string   `json:"value,omitempty"`
	Type     string   `json:"type,omitempty"`
	AgeDays  int      `json:"age_days"`
	Findings []string `json:"findings"`
}

// newNovelExclusionsAuditCmd flags risky exclusions the console lists but never
// judges — wildcard paths, stale entries, and exclusions no synced threat has
// ever matched (dead allow-list rules that only widen attack surface).
// pp:data-source local
func newNovelExclusionsAuditCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var staleDays int
	var limit int

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Flag risky exclusions: never-matched, wildcard paths, stale entries",
		Long: `Scan the locally-synced exclusions for risk the console will not flag:

  wildcard-path  the value contains '*' or '?' (broad, easy to abuse)
  stale          older than --stale-days (default 180)
  never-matched  a hash/path exclusion no synced threat ever matched

never-matched analysis is skipped (and noted) when no threats are synced,
since with an empty threat history every exclusion would look unmatched.`,
		Example: `  # Audit every exclusion
  sentinelone-cli exclusions audit

  # Tighten the stale window, as JSON
  sentinelone-cli exclusions audit --stale-days 90 --agent`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openS1Store(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "exclusions") {
				hintIfStale(cmd, db, "exclusions", flags.maxAge)
			}

			exclusions, err := loadResourceObjects(cmd.Context(), db, "exclusions")
			if err != nil {
				return fmt.Errorf("loading exclusions: %w", err)
			}
			if len(exclusions) == 0 {
				return honestEmptyJSON(cmd, flags,
					"No exclusions in the local store. Run 'sentinelone-cli sync --resources exclusions' first.", nil)
			}

			threats, err := loadThreats(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("loading threats: %w", err)
			}
			haveThreats := len(threats) > 0
			note := ""
			if !haveThreats {
				note = "Threat history is empty, so never-matched analysis was skipped. Run 'sentinelone-cli sync --resources threats' for match analysis."
			}

			now := time.Now()
			isHashType := func(typ string) bool {
				t := strings.ToLower(typ)
				return strings.Contains(t, "hash") || strings.Contains(t, "sha1") || strings.Contains(t, "certificate")
			}
			isPathType := func(typ string) bool {
				t := strings.ToLower(typ)
				return t == "" || strings.Contains(t, "path") || strings.Contains(t, "file") || strings.Contains(t, "folder")
			}
			// Non-wildcard prefix of a path value (everything before the first glob char).
			pathPrefix := func(v string) string {
				if i := strings.IndexAny(v, "*?"); i >= 0 {
					return v[:i]
				}
				return v
			}

			var rows []exclusionFinding
			flagged := 0
			for _, e := range exclusions {
				value := gstrFirst(e, "value", "data.value")
				typ := gstrFirst(e, "type", "data.type")
				created := gstrFirst(e, "createdAt", "data.createdAt")

				var findings []string
				if strings.ContainsAny(value, "*?") {
					findings = append(findings, "wildcard-path")
				}

				ageDays := 0
				if d, ok := daysSince(now, created); ok {
					ageDays = d
					if d > staleDays {
						findings = append(findings, "stale")
					}
				}

				if haveThreats && value != "" {
					matched := false
					switch {
					case isHashType(typ):
						for _, t := range threats {
							if strings.EqualFold(threatSHA1(t), value) {
								matched = true
								break
							}
						}
					case isPathType(typ):
						prefix := pathPrefix(value)
						if prefix == "" {
							// Glob at position 0 (e.g. "*.tmp"): no literal prefix to
							// match on, so never-matched is unprovable — don't flag.
							matched = true
						} else {
							for _, t := range threats {
								fp := gstrFirst(t, "threatInfo.filePath", "filePath")
								if fp != "" && strings.Contains(strings.ToLower(fp), strings.ToLower(prefix)) {
									matched = true
									break
								}
							}
						}
					default:
						// Unknown exclusion type: do not assert never-matched.
						matched = true
					}
					if !matched {
						findings = append(findings, "never-matched")
					}
				}

				if len(findings) == 0 {
					continue
				}
				flagged++
				rows = append(rows, exclusionFinding{
					Value:    value,
					Type:     typ,
					AgeDays:  ageDays,
					Findings: findings,
				})
			}

			// Most findings first.
			sort.SliceStable(rows, func(i, j int) bool { return len(rows[i].Findings) > len(rows[j].Findings) })
			total := len(exclusions)
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}

			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{
					"exclusions_scanned": total,
					"flagged":            flagged,
					"note":               note,
					"items":              rows,
				})
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Exclusion audit (%d scanned, %d flagged):\n", total, flagged)
			if note != "" {
				fmt.Fprintf(w, "%s\n", note)
			}
			fmt.Fprintln(w)
			fmt.Fprintf(w, "%-44s %-14s %6s  %s\n", "VALUE", "TYPE", "AGE-D", "FINDINGS")
			for _, r := range rows {
				fmt.Fprintf(w, "%-44s %-14s %6d  %s\n",
					clip(orUnknown(r.Value), 44), clip(orUnknown(r.Type), 14), r.AgeDays, strings.Join(r.Findings, ", "))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/sentinelone-cli/data.db)")
	cmd.Flags().IntVar(&staleDays, "stale-days", 180, "Exclusions older than this many days are flagged stale")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum flagged exclusions to show (0 = all)")
	return cmd
}
