// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"

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
	Reason     string `json:"reason"`
}

type complianceReport struct {
	Distribution []versionCount      `json:"distribution"`
	ModalVersion string              `json:"modal_version"`
	Target       string              `json:"target,omitempty"`
	NonCompliant []nonCompliantAgent `json:"non_compliant"`
	TotalAgents  int                 `json:"total_agents"`
	CompareNote  string              `json:"compare_note,omitempty"`
}

// parseAgentVersion parses a dotted numeric version ("24.10.38844", optional
// leading "v") into its components. Anything else is not comparable.
func parseAgentVersion(v string) ([]int, bool) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V")
	if v == "" {
		return nil, false
	}
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		if p == "" || len(p) > 9 {
			return nil, false
		}
		n := 0
		for _, c := range p {
			if c < '0' || c > '9' {
				return nil, false
			}
			n = n*10 + int(c-'0')
		}
		out = append(out, n)
	}
	return out, true
}

// compareAgentVersions compares two parsed versions numerically, component by
// component; a missing component is 0 (21.10 == 21.10.0).
func compareAgentVersions(a, b []int) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// computeCompliance computes the version distribution and (when target != "")
// the agents behind target, compared numerically per dotted component. An
// agent whose version cannot be compared is reported non-compliant with reason
// "unparseable_version" - never silently compliant. An unparseable target is a
// usage error.
func computeCompliance(db *store.Store, target string) (complianceReport, error) {
	var targetParts []int
	if target != "" {
		var ok bool
		if targetParts, ok = parseAgentVersion(target); !ok {
			return complianceReport{}, usageErr(fmt.Errorf("--target %q is not a dotted numeric version such as 24.10.38844", target))
		}
	}
	names, err := tenantNames(db)
	if err != nil {
		return complianceReport{}, err
	}
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
		rep.CompareNote = "versions compare numerically per dotted component; an agent whose version cannot be parsed is listed with reason unparseable_version"
	}

	for rows.Next() {
		var tenantID, hostname, version *string
		if err := rows.Scan(&tenantID, &hostname, &version); err != nil {
			return complianceReport{}, fmt.Errorf("reading agent_manager: %w", err)
		}
		v := deref(version)
		if v == "" {
			v = "(unknown)"
		}
		counts[v]++
		rep.TotalAgents++
		if target == "" {
			continue
		}
		reason := ""
		if parts, ok := parseAgentVersion(deref(version)); !ok {
			reason = "unparseable_version"
		} else if compareAgentVersions(parts, targetParts) < 0 {
			reason = "behind_target"
		}
		if reason != "" {
			tid := deref(tenantID)
			rep.NonCompliant = append(rep.NonCompliant, nonCompliantAgent{
				TenantID:   tid,
				TenantName: names[tid],
				Hostname:   deref(hostname),
				Version:    v,
				Reason:     reason,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return complianceReport{}, fmt.Errorf("reading agent_manager: %w", err)
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
					fmt.Fprintf(cmd.OutOrStdout(), "  %-24s %-20s %-16s %s\n", truncate(label, 24), truncate(a.Hostname, 20), a.Version, a.Reason)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagTarget, "target", "", "Target version (dotted numeric); agents numerically below it, or with an unparseable version, are flagged non-compliant")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/acronis-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum non-compliant agents to show (0 = all)")
	return cmd
}
