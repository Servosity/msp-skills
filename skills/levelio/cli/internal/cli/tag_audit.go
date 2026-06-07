// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built transcendence feature for levelio-cli. Reads the local store.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"levelio-pp-cli/internal/store"
)

type tagDuplicate struct {
	Name   string   `json:"name"`
	TagIDs []string `json:"tag_ids"`
}

type tagAuditResult struct {
	TotalTags       int            `json:"total_tags"`
	TotalDevices    int            `json:"total_devices"`
	UntaggedDevices []string       `json:"untagged_devices,omitempty"`
	OrphanTags      []string       `json:"orphan_tags,omitempty"`
	DuplicateNames  []tagDuplicate `json:"duplicate_names,omitempty"`
}

// lvlComputeTagAudit anti-joins tags against device-tag membership: devices
// with zero tags, tags applied to nothing, and (case-insensitive) duplicate
// tag names that fragment fleet filters.
func lvlComputeTagAudit(tags []lvlTag, devices []lvlDevice, wantUntagged, wantOrphans, wantDuplicates bool) tagAuditResult {
	res := tagAuditResult{TotalTags: len(tags), TotalDevices: len(devices)}

	usedNames := map[string]bool{}
	for _, d := range devices {
		for _, t := range d.Tags {
			usedNames[strings.ToLower(strings.TrimSpace(t))] = true
		}
	}

	if wantUntagged {
		untagged := []string{}
		for _, d := range devices {
			if len(d.Tags) == 0 {
				untagged = append(untagged, lvlDeviceLabel(d))
			}
		}
		sort.Strings(untagged)
		res.UntaggedDevices = untagged
	}

	if wantOrphans {
		orphans := []string{}
		for _, t := range tags {
			if t.DeviceCount > 0 {
				continue
			}
			if usedNames[strings.ToLower(strings.TrimSpace(t.Name))] {
				continue
			}
			orphans = append(orphans, t.Name)
		}
		sort.Strings(orphans)
		res.OrphanTags = orphans
	}

	if wantDuplicates {
		byNorm := map[string][]string{}
		normOrder := []string{}
		for _, t := range tags {
			norm := strings.ToLower(strings.TrimSpace(t.Name))
			if norm == "" {
				continue
			}
			if _, ok := byNorm[norm]; !ok {
				normOrder = append(normOrder, norm)
			}
			byNorm[norm] = append(byNorm[norm], t.ID)
		}
		for _, norm := range normOrder {
			if ids := byNorm[norm]; len(ids) > 1 {
				sort.Strings(ids)
				res.DuplicateNames = append(res.DuplicateNames, tagDuplicate{Name: norm, TagIDs: ids})
			}
		}
		sort.SliceStable(res.DuplicateNames, func(i, j int) bool {
			return res.DuplicateNames[i].Name < res.DuplicateNames[j].Name
		})
	}
	return res
}

// pp:data-source local
func newNovelTagAuditCmd(flags *rootFlags) *cobra.Command {
	var untagged, orphans, duplicates bool
	var dbPath string

	cmd := &cobra.Command{
		Use:         "tag-audit",
		Short:       "Surface tag-data drift: untagged devices, orphan tags, duplicate tag names",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Audit tag hygiene across the fleet: devices carrying zero tags, tags
applied to nothing (orphans), and case-insensitive duplicate tag names that
fragment fleet filters and automations. Computed offline by anti-joining the
synced tags against device-tag membership — absences the Level UI cannot show.
By default all three sections are reported; pass --untagged, --orphans, or
--duplicates to narrow to specific sections.

Use this command for TAG-data hygiene (untagged devices, orphan/duplicate
tags). Do NOT use it for custom-field value gaps; use 'cf-coverage' instead.

Run 'levelio-cli sync' first to populate the local store.`,
		Example: strings.Trim(`
  # Full tag-hygiene audit
  levelio-cli tag-audit

  # Just the untagged devices, JSON for agents
  levelio-cli tag-audit --untagged --agent
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			out := cmd.OutOrStdout()
			if dbPath == "" {
				dbPath = defaultDBPath("levelio-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w\nRun 'levelio-cli sync' first.", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "tags") {
				hintIfStale(cmd, db, "tags", flags.maxAge)
			}

			// No section flag = all sections.
			wantAll := !untagged && !orphans && !duplicates

			tags, err := lvlTags(db)
			if err != nil {
				return fmt.Errorf("loading tags: %w", err)
			}
			devices, err := lvlDevices(db)
			if err != nil {
				return fmt.Errorf("loading devices: %w", err)
			}
			res := lvlComputeTagAudit(tags, devices,
				wantAll || untagged, wantAll || orphans, wantAll || duplicates)

			if flags.asJSON {
				return printJSONFiltered(out, res, flags)
			}
			fmt.Fprintf(out, "%d tag(s), %d device(s)\n", res.TotalTags, res.TotalDevices)
			if wantAll || untagged {
				fmt.Fprintf(out, "\n%d untagged device(s)\n", len(res.UntaggedDevices))
				for _, d := range res.UntaggedDevices {
					fmt.Fprintf(out, "  %s\n", d)
				}
			}
			if wantAll || orphans {
				fmt.Fprintf(out, "\n%d orphan tag(s) (applied to nothing)\n", len(res.OrphanTags))
				for _, t := range res.OrphanTags {
					fmt.Fprintf(out, "  %s\n", t)
				}
			}
			if wantAll || duplicates {
				fmt.Fprintf(out, "\n%d duplicate tag name(s)\n", len(res.DuplicateNames))
				for _, d := range res.DuplicateNames {
					fmt.Fprintf(out, "  %s (%s)\n", d.Name, strings.Join(d.TagIDs, ", "))
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&untagged, "untagged", false, "Report only devices with zero tags")
	cmd.Flags().BoolVar(&orphans, "orphans", false, "Report only tags applied to no device")
	cmd.Flags().BoolVar(&duplicates, "duplicates", false, "Report only duplicate (case-insensitive) tag names")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
