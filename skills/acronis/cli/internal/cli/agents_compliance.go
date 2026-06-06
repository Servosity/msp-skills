// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"

	"acronis-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

type versionCount struct {
	Version string `json:"version"`
	Count   int    `json:"count"`
}

type nonCompliantAgent struct {
	TenantID   string `json:"tenant_id"`
	TenantName string `json:"tenant_name"`
	Hostname   string `json:"hostname"`
	Version    string `json:"version"`
}

type complianceReport struct {
	Distribution []versionCount      `json:"distribution"`
	ModalVersion string              `json:"modal_version"`
	Target       string              `json:"target,omitempty"`
	NonCompliant []nonCompliantAgent `json:"non_compliant"`
	TotalAgents  int                 `json:"total_agents"`
	CompareNote  string              `json:"compare_note,omitempty"`
}

// computeCompliance computes the version distribution and (when target != "")
// the agents whose version is lexically below target.
func computeCompliance(db *store.Store, target string) (complianceReport, error) {
	names := tenantNames(db)
	rows, err := db.Query(`SELECT tenant_id, hostname, version FROM agent_manager`)
	if err != nil {
		return complianceReport{}, fmt.Errorf("querying agent_manager: %w", err)
	}
	defer rows.Close()

	counts := map[string]int{}
	var rep complianceReport
	rep.Target = target
	rep.NonCompliant = []nonCompliantAgent{}
	if target != "" {
		rep.CompareNote = "version comparison is lexical (string <) — works for zero-padded semver-like versions"
	}

	for rows.Next() {
		var tenantID, hostname, version *string
		if rows.Scan(&tenantID, &hostname, &version) != nil {
			continue
		}
		v := deref(version)
		if v == "" {
			v = "(unknown)"
		}
		counts[v]++
		rep.TotalAgents++
		if target != "" && v < target {
			tid := deref(tenantID)
			rep.NonCompliant = append(rep.NonCompliant, nonCompliantAgent{
				TenantID:   tid,
				TenantName: names[tid],
				Hostname:   deref(hostname),
				Version:    v,
			})
		}
	}

	rep.Distribution = make([]versionCount, 0, len(counts))
	modalCount := -1
	for v, c := range counts {
		rep.Distribution = append(rep.Distribution, versionCount{Version: v, Count: c})
		if c > modalCount || (c == modalCount && v > rep.ModalVersion) {
			modalCount = c
			rep.ModalVersion = v
		}
	}
	sort.Slice(rep.Distribution, func(i, j int) bool {
		if rep.Distribution[i].Count != rep.Distribution[j].Count {
			return rep.Distribution[i].Count > rep.Distribution[j].Count
		}
		return rep.Distribution[i].Version < rep.Distribution[j].Version
	})
	sort.Slice(rep.NonCompliant, func(i, j int) bool {
		if rep.NonCompliant[i].TenantID != rep.NonCompliant[j].TenantID {
			return rep.NonCompliant[i].TenantID < rep.NonCompliant[j].TenantID
		}
		return rep.NonCompliant[i].Hostname < rep.NonCompliant[j].Hostname
	})
	return rep, nil
}

// pp:data-source local
func newNovelAgentsComplianceCmd(flags *rootFlags) *cobra.Command {
	var flagTarget string
	var dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:         "compliance",
		Short:       "Show the distribution of agent versions across the estate and flag tenants behind the target version.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openNovelDB(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			maybeEmitSyncHints(cmd, db, "", flags.maxAge)

			rep, err := computeCompliance(db, flagTarget)
			if err != nil {
				return err
			}
			if limit > 0 && len(rep.NonCompliant) > limit {
				rep.NonCompliant = rep.NonCompliant[:limit]
			}

			if wantJSON(flags, cmd) {
				return encodeJSON(cmd, flags, rep)
			}
			if rep.TotalAgents == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No agents — run 'acronis-cli sync' first.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Version distribution (%d agents, modal=%s):\n", rep.TotalAgents, rep.ModalVersion)
			for _, vc := range rep.Distribution {
				fmt.Fprintf(cmd.OutOrStdout(), "  %-16s %d\n", vc.Version, vc.Count)
			}
			if flagTarget != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "\nBelow target %q (%d):\n", flagTarget, len(rep.NonCompliant))
				for _, a := range rep.NonCompliant {
					label := a.TenantName
					if label == "" {
						label = a.TenantID
					}
					fmt.Fprintf(cmd.OutOrStdout(), "  %-24s %-20s %s\n", truncate(label, 24), truncate(a.Hostname, 20), a.Version)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagTarget, "target", "", "Target version; agents lexically below this are flagged non-compliant")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/acronis-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum non-compliant agents to show (0 = all)")
	return cmd
}
