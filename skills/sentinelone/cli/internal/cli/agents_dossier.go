// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type dossierAgent struct {
	ComputerName string `json:"computer_name,omitempty"`
	UUID         string `json:"uuid,omitempty"`
	ID           string `json:"id,omitempty"`
	OS           string `json:"os_name,omitempty"`
	AgentVersion string `json:"agent_version,omitempty"`
	Site         string `json:"site,omitempty"`
	Group        string `json:"group,omitempty"`
	// Liveness is "online" (isActive true), "offline" (isActive false) or
	// "unknown". NetworkStatus is the network-isolation state, not liveness.
	Liveness      string `json:"liveness"`
	NetworkStatus string `json:"network_status,omitempty"`
	Infected      bool   `json:"infected"`
	UpToDate      bool   `json:"is_up_to_date"`
	InProtect     bool   `json:"in_protect"`
	LastActive    string `json:"last_active_date,omitempty"`
}

type dossierThreat struct {
	Threat     string `json:"threat,omitempty"`
	SHA1       string `json:"sha1,omitempty"`
	Verdict    string `json:"verdict,omitempty"`
	Mitigation string `json:"mitigation_status,omitempty"`
	Created    string `json:"created_at,omitempty"`
}

type dossierActivity struct {
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
}

// newNovelAgentsDossierCmd pulls one endpoint's full profile onto a single
// card — agent state, its threat history, recent activity, and membership —
// joining agents/threats/activities the console only shows on separate screens.
// pp:data-source local
func newNovelAgentsDossierCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "dossier [name-or-uuid]",
		Short: "Everything about one endpoint: agent state, threat history, recent activity, membership",
		Long: `Use this command to pull one endpoint's full profile — its agent state,
the threats seen on it, its recent activity, and its site/group membership —
all on one card. Do NOT use this command to scope one threat across many
endpoints; use 'threats blast-radius' instead.

Identify the endpoint by computer name, UUID, or agent id (case-insensitive).`,
		Example: `  # Profile an endpoint by name
  sentinelone-cli agents dossier WIN-DC01

  # By UUID, as JSON
  sentinelone-cli agents dossier 5f3c... --agent`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Bare invocation (no arg, no flags) → show help.
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if len(args) == 0 {
				return usageErr(fmt.Errorf("dossier requires an endpoint name, UUID, or id"))
			}
			query := args[0]

			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openS1Store(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			// Multi-entity command: hint on the freshest of the entities it joins.
			if !hintIfUnsynced(cmd, db, "agents") {
				hintIfStale(cmd, db, "agents", flags.maxAge)
			}

			agents, err := loadAgents(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("loading agents: %w", err)
			}

			// An exact id or uuid names one agent. A computer name can name several
			// (two customers' "DC01"): then no agent is picked for the caller.
			var matches []map[string]any
			for _, a := range agents {
				if strings.EqualFold(gstr(a, "uuid"), query) || strings.EqualFold(gstr(a, "id"), query) {
					matches = append(matches, a)
				}
			}
			if len(matches) == 0 {
				for _, a := range agents {
					if strings.EqualFold(gstr(a, "computerName"), query) {
						matches = append(matches, a)
					}
				}
			}
			if len(matches) == 0 {
				return honestEmptyJSON(cmd, flags,
					fmt.Sprintf("No agent in the local store matched %q (by computer name, UUID, or id). Run 'sentinelone-cli sync --resources agents' if the endpoint is new.", query),
					map[string]any{"query": query})
			}
			if len(matches) > 1 {
				type candidate struct {
					ID           string `json:"id"`
					UUID         string `json:"uuid,omitempty"`
					ComputerName string `json:"computer_name"`
					Site         string `json:"site"`
					SiteID       string `json:"site_id,omitempty"`
				}
				cands := make([]candidate, 0, len(matches))
				for _, m := range matches {
					r := agentSiteRef(m)
					cands = append(cands, candidate{ID: gstr(m, "id"), UUID: gstr(m, "uuid"), ComputerName: gstr(m, "computerName"), Site: r.Label(), SiteID: r.ID})
				}
				sort.SliceStable(cands, func(i, j int) bool { return cands[i].ID < cands[j].ID })
				return honestEmptyJSON(cmd, flags,
					fmt.Sprintf("%d agents are named %q; ask again by id or uuid: %s", len(matches), query, describeCandidates(cands, func(c candidate) string { return c.ID + " (" + c.Site + ")" })),
					map[string]any{"query": query, "ambiguous": true, "candidates": cands})
			}

			a := matches[0]
			agentName := gstr(a, "computerName")
			agentID := gstr(a, "id")
			detail := dossierAgent{
				ComputerName:  agentName,
				UUID:          gstr(a, "uuid"),
				ID:            agentID,
				OS:            gstr(a, "osName"),
				AgentVersion:  gstr(a, "agentVersion"),
				Site:          agentSite(a),
				Group:         gstr(a, "groupName"),
				Liveness:      agentLiveness(a).String(),
				NetworkStatus: gstr(a, "networkStatus"),
				Infected:      gbool(a, "infected"),
				UpToDate:      gbool(a, "isUpToDate"),
				InProtect:     agentInProtect(a),
				LastActive:    gstr(a, "lastActiveDate"),
			}

			// Threats on this endpoint: by agent id/uuid; a detection that names no
			// agent matches by name only when that name is unique in the store.
			nameUnique := true
			if agentName != "" {
				n := 0
				for _, other := range agents {
					if strings.EqualFold(gstr(other, "computerName"), agentName) {
						n++
					}
				}
				nameUnique = n == 1
			}
			threats, err := loadThreats(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("loading threats: %w", err)
			}
			threatRows := []dossierThreat{}
			for _, t := range threats {
				if threatOnAgent(t, a, nameUnique) {
					threatRows = append(threatRows, dossierThreat{
						Threat:     threatName(t),
						SHA1:       clip(threatSHA1(t), 12),
						Verdict:    threatVerdict(t),
						Mitigation: threatMitigation(t),
						Created:    threatCreatedAt(t),
					})
				}
			}

			// Recent activities for this endpoint: an activity that carries an
			// agentId belongs to that agent only; the name fallback is for
			// activities with no agentId, and only for a unique name.
			activities, err := loadResourceObjects(cmd.Context(), db, "activities")
			if err != nil {
				return fmt.Errorf("loading activities: %w", err)
			}
			var actMatches []map[string]any
			for _, act := range activities {
				if actAgent := gstr(act, "agentId"); actAgent != "" {
					if agentID != "" && actAgent == agentID {
						actMatches = append(actMatches, act)
					}
					continue
				}
				if nameUnique && agentName != "" && strings.EqualFold(gstr(act, "data.computerName"), agentName) {
					actMatches = append(actMatches, act)
				}
			}
			// Most recent first by createdAt, take 10.
			sort.SliceStable(actMatches, func(i, j int) bool {
				return gstr(actMatches[i], "createdAt") > gstr(actMatches[j], "createdAt")
			})
			if len(actMatches) > 10 {
				actMatches = actMatches[:10]
			}
			actRows := []dossierActivity{}
			for _, act := range actMatches {
				actRows = append(actRows, dossierActivity{
					Description: gstrFirst(act, "primaryDescription", "data.primaryDescription"),
					CreatedAt:   gstr(act, "createdAt"),
				})
			}

			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{
					"agent":             detail,
					"threats":           threatRows,
					"recent_activities": actRows,
					"matches":           len(matches),
				})
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Endpoint dossier: %s\n\n", orUnknown(detail.ComputerName))
			fmt.Fprintf(w, "  uuid             %s\n", orUnknown(detail.UUID))
			fmt.Fprintf(w, "  id               %s\n", orUnknown(detail.ID))
			fmt.Fprintf(w, "  os               %s\n", orUnknown(detail.OS))
			fmt.Fprintf(w, "  agent version    %s\n", orUnknown(detail.AgentVersion))
			fmt.Fprintf(w, "  site             %s\n", detail.Site)
			fmt.Fprintf(w, "  group            %s\n", orUnknown(detail.Group))
			fmt.Fprintf(w, "  liveness         %s\n", detail.Liveness)
			fmt.Fprintf(w, "  network status   %s (isolation state, not liveness)\n", orUnknown(detail.NetworkStatus))
			fmt.Fprintf(w, "  infected         %t\n", detail.Infected)
			fmt.Fprintf(w, "  up-to-date       %t\n", detail.UpToDate)
			fmt.Fprintf(w, "  in protect       %t\n", detail.InProtect)
			fmt.Fprintf(w, "  last active      %s\n", orUnknown(detail.LastActive))

			fmt.Fprintf(w, "\nThreats (%d):\n", len(threatRows))
			for _, tr := range threatRows {
				fmt.Fprintf(w, "  %-30s %-13s %-12s %-12s %s\n",
					clip(orUnknown(tr.Threat), 30), tr.SHA1, clip(orUnknown(tr.Verdict), 12),
					clip(orUnknown(tr.Mitigation), 12), tr.Created)
			}

			fmt.Fprintf(w, "\nRecent activity (%d):\n", len(actRows))
			for _, ar := range actRows {
				fmt.Fprintf(w, "  %-19s %s\n", clip(ar.CreatedAt, 19), clip(orUnknown(ar.Description), 70))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/sentinelone-cli/data.db)")
	return cmd
}

// describeCandidates joins up to five candidates for an ambiguity message.
func describeCandidates[T any](items []T, show func(T) string) string {
	parts := make([]string, 0, len(items))
	for i, it := range items {
		if i == 5 {
			parts = append(parts, fmt.Sprintf("and %d more", len(items)-5))
			break
		}
		parts = append(parts, show(it))
	}
	return strings.Join(parts, ", ")
}
