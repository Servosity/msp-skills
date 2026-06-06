// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel-feature command (Printing Press transcendence): offline
// full-text search across the CID-keyed fleet store (every tenant at once).

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// fleetSearchHit is one row of `fleet search`.
type fleetSearchHit struct {
	CID      string `json:"cid"`
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	Severity string `json:"severity,omitempty"`
	Status   string `json:"status,omitempty"`
}

// searchFleetEntities returns entities whose cid/kind/id/name/severity/status
// contains term (case-insensitive substring), sorted by kind, then cid, then id.
func searchFleetEntities(entities []fleetEntity, term string) []fleetSearchHit {
	t := strings.ToLower(strings.TrimSpace(term))
	out := []fleetSearchHit{}
	if t == "" {
		return out
	}
	for _, e := range entities {
		hay := strings.ToLower(strings.Join([]string{e.CID, e.Kind, e.ID, e.Name, e.Severity, e.Status}, " "))
		if strings.Contains(hay, t) {
			out = append(out, fleetSearchHit{CID: e.CID, Kind: e.Kind, ID: e.ID, Name: e.Name, Severity: e.Severity, Status: e.Status})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].CID != out[j].CID {
			return out[i].CID < out[j].CID
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// pp:data-source local
func newNovelFleetSearchCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "search <term>",
		Short: "Full-text search across all synced fleet entities (hosts, alerts, vulns, policies) across every tenant",
		Long: "Search the CID-keyed fleet store offline: matches hostnames, CVEs, alert names, " +
			"policy names, IDs, severity, and status across every synced tenant. Run 'fleet sync' " +
			"first. (The top-level 'search' command covers data synced via the top-level 'sync'; " +
			"'fleet search' covers the cross-tenant 'fleet sync' store.)",
		Example:     "  crowdstrike-cli fleet search ransomware --json --select cid,kind,name",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if len(args) == 0 {
				return cmd.Help()
			}
			term := strings.Join(args, " ")
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
			ents, err := loadFleetEntitiesFrom(cmd.Context(), st, "", false)
			if err != nil {
				return configErr(err)
			}
			hits := searchFleetEntities(ents, term)
			if len(hits) == 0 && !flags.asJSON {
				fmt.Fprintf(cmd.ErrOrStderr(), "no fleet entities match %q (run 'fleet sync' first?)\n", term)
			}
			return flags.printJSON(cmd, hits)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite store path (default: standard data dir)")
	return cmd
}
