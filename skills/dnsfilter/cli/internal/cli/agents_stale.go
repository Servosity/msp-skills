// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"dnsfilter-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

type staleAgent struct {
	ID           string `json:"id"`
	Hostname     string `json:"hostname"`
	FriendlyName string `json:"friendly_name"`
	AgentType    string `json:"agent_type"`
	AgentVersion string `json:"agent_version"`
	AgentState   string `json:"agent_state"`
	LastSync     string `json:"last_sync"`
	Reason       string `json:"reason"` // silent, unprotected, bypassed, never_synced
	AgeBucket    string `json:"age_bucket"`
}

// staleCollections is every collection the stale scan joins: clients, their
// sites (network -> org) and the orgs (--org by name, grouping).
var staleCollections = []string{"organizations", "networks", "user-agents"}

type staleOrgRow struct {
	OrgID   string       `json:"org_id"`
	OrgName string       `json:"org_name"`
	Count   int          `json:"count"`
	Agents  []staleAgent `json:"agents"`
}

func newNovelAgentsStaleCmd(flags *rootFlags) *cobra.Command {
	var olderThan, orgFilter, dbPath string

	cmd := &cobra.Command{
		Use:   "stale",
		Short: "List roaming clients that stopped checking in or are unprotected, grouped by client org",
		Long: `Use this command for roaming clients that are silent (last_sync older than --older-than) or not protected (unprotected or bypassed) across every client org.
Do NOT use this command for version drift after an agent release; use 'agents drift' instead.

Reads the local mirror: run 'dnsfilter-cli sync --full --resources organizations,networks,user-agents' first. Uninstalled clients are excluded.`,
		Example: strings.Trim(`
  dnsfilter-cli agents stale --older-than 7d --agent
  dnsfilter-cli agents stale --org 'Acme Dental'`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "agents stale")
			}
			if err := rejectExtraArgs(args, 0); err != nil {
				return err
			}
			window, err := cliutil.ParseDurationLoose(olderThan)
			if err != nil {
				return usageErr(fmt.Errorf("--older-than: %w", err))
			}
			db, ok, err := openLocalMirror(cmd, flags, dbPath, "organizations,networks,user-agents", incompleteMirrorJSON(make([]staleOrgRow, 0), staleCollections))
			if !ok {
				return err
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "user-agents") {
				hintIfStale(cmd, db, "user-agents", flags.maxAge)
			}
			ctx := cmd.Context()
			// Rows and their completeness come from one snapshot.
			snap, err := mirrorSnapshot(ctx, db)
			if err != nil {
				return err
			}
			defer snap.Rollback()
			agents, err := loadLocal(ctx, snap, "user-agents")
			if err != nil {
				return err
			}
			networks, err := loadLocal(ctx, snap, "networks")
			if err != nil {
				return err
			}
			orgs, err := loadLocal(ctx, snap, "organizations")
			if err != nil {
				return err
			}
			afterMirrorRowsRead()
			incomplete, err := incompleteCollections(ctx, snap, staleCollections...)
			if err != nil {
				return err
			}
			rows, scanned := computeStale(agents, networks, orgs, orgFilter, window, time.Now())
			human := wantsHumanTable(cmd.OutOrStdout(), flags)
			if len(incomplete) > 0 {
				// An unwalked site or org list empties --org; do not blame the filter.
				if !human {
					fmt.Fprintln(cmd.ErrOrStderr(), "hint: "+incompleteSyncNote(incomplete))
				}
			} else if scanned == 0 && len(agents) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "hint: no installed roaming client in the local mirror matches --org %q; nothing was checked\n", orgFilter)
			}
			if !human {
				if len(incomplete) > 0 {
					return printJSONFiltered(cmd.OutOrStdout(), incompleteMirrorJSON(rows, incomplete), flags)
				}
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			w := cmd.OutOrStdout()
			printRows := func() {
				for _, r := range rows {
					fmt.Fprintf(w, "%s (%d)\n", humanText(r.OrgName), r.Count)
					for _, a := range r.Agents {
						fmt.Fprintf(w, "  %-12s %-10s %-24s %-10s last_sync=%s\n", humanText(a.Reason), humanText(a.AgeBucket), humanText(a.Hostname), humanText(a.AgentVersion), humanText(a.LastSync))
					}
				}
			}
			if len(agents) == 0 {
				// An unsynced mirror is not a healthy fleet; never print the all-clear.
				fmt.Fprintln(w, "No roaming clients in the local mirror; nothing was checked. Run: dnsfilter-cli sync --full --resources organizations,networks,user-agents")
				return nil
			}
			if len(incomplete) > 0 {
				// Findings stand; the rest of the fleet was not fully checked.
				printRows()
				fmt.Fprintln(w, humanText(incompleteSyncNote(incomplete)))
				return nil
			}
			if scanned == 0 {
				return nil
			}
			if len(rows) == 0 {
				fmt.Fprintf(w, "No roaming clients silent for more than %s or unprotected.\n", humanText(olderThan))
				return nil
			}
			printRows()
			return nil
		},
	}
	cmd.Flags().StringVar(&olderThan, "older-than", "7d", "Flag clients whose last check-in is older than this (e.g. 24h, 7d, 2w)")
	cmd.Flags().StringVar(&orgFilter, "org", "", "Only this client org (name substring or id)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror path (default: the CLI's synced database)")
	return cmd
}

// computeStale is the pure scan behind `agents stale`. scanned counts the
// installed clients that passed the org filter, so an empty result can be told
// apart from "nothing was checked".
func computeStale(agents, networks, orgs []jaRes, orgFilter string, window time.Duration, now time.Time) (out []staleOrgRow, scanned int) {
	netOrg := map[string]string{}
	for _, n := range networks {
		netOrg[n.ID] = n.relID("organization")
	}
	orgByID := byID(orgs)
	rows := map[string]*staleOrgRow{}
	for _, a := range agents {
		if isUninstalled(a) {
			continue
		}
		orgID := netOrg[a.relID("network")]
		org := orgByID[orgID]
		if orgFilter != "" && orgID != orgFilter && !strings.Contains(strings.ToLower(org.str("name")), strings.ToLower(orgFilter)) {
			continue
		}
		scanned++
		reason, bucket := "", ""
		last, hasSync := parseSyncTime(a.str("last_sync"))
		switch {
		case !hasSync:
			reason, bucket = "never_synced", "never"
		case now.Sub(last) > window:
			reason, bucket = "silent", ageBucket(now.Sub(last))
		case a.str("agent_state") == "unprotected" || a.str("agent_state") == "bypassed":
			reason, bucket = a.str("agent_state"), ageBucket(now.Sub(last))
		default:
			continue
		}
		row := rows[orgID]
		if row == nil {
			row = &staleOrgRow{OrgID: orgID, OrgName: org.str("name"), Agents: make([]staleAgent, 0)}
			if orgID == "" {
				row.OrgName = "(site not synced)"
			}
			rows[orgID] = row
		}
		row.Count++
		row.Agents = append(row.Agents, staleAgent{ID: a.ID, Hostname: a.str("hostname"), FriendlyName: a.str("friendly_name"), AgentType: a.str("agent_type"), AgentVersion: a.str("agent_version"), AgentState: a.str("agent_state"), LastSync: a.str("last_sync"), Reason: reason, AgeBucket: bucket})
	}
	out = make([]staleOrgRow, 0, len(rows))
	for _, r := range rows {
		sort.Slice(r.Agents, func(i, j int) bool { return r.Agents[i].LastSync < r.Agents[j].LastSync })
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].OrgName < out[j].OrgName
	})
	return out, scanned
}

// parseSyncTime accepts the spec's `format: date` plus full timestamps.
func parseSyncTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func ageBucket(d time.Duration) string {
	switch {
	case d < 24*time.Hour:
		return "<1d"
	case d < 7*24*time.Hour:
		return "1-7d"
	case d < 30*24*time.Hour:
		return "7-30d"
	default:
		return "30d+"
	}
}
