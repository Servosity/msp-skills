// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature (reprint 20260606): enriched single-incident view.

package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelIncidentDetailCmd(flags *rootFlags) *cobra.Command {
	var id int
	var dbPath string
	cmd := &cobra.Command{
		Use:   "incident-detail [id]",
		Short: "One incident enriched with its remediations, agent, and org name",
		Long: "Joins one incident report with its remediations, the affected agent, and the\n" +
			"organization name in a single local lookup — data the API returns across 3+ calls.\n" +
			"Do NOT use it to list or filter incidents across orgs; use 'fleet-incidents' instead.\n" +
			"Do NOT use it for the raw single API record; use the generated incident-reports get instead.\n" +
			"Reads the local store; run `sync` first.",
		Example:     "  huntress-cli incident-detail 123456 --json",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:happy-args": "id=1"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			incidentID, given, err := idFromFlagOrArg(cmd, "id", id, args)
			if err != nil {
				return usageErr(err)
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would look up one incident with remediations, agent, and org joined")
				return nil
			}
			if !given {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("an incident id is required (positional or --id)"))
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, entIncidents, entAgents, entOrgs)
			if err != nil {
				return err
			}
			defer st.Close()

			rows, err := queryMaps(ctx, st.DB(),
				fmt.Sprintf(`SELECT i.data AS data FROM %s i WHERE %s = ?`, st.src(entIncidents), jx("i", "id")), incidentID)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				return fmt.Errorf("incident %d not found in the local store; run `sync` first or check the id", incidentID)
			}
			blob, _ := rows[0]["data"].(string)
			incident := map[string]interface{}{}
			dec := json.NewDecoder(strings.NewReader(blob))
			dec.UseNumber()
			if err := dec.Decode(&incident); err != nil {
				return fmt.Errorf("decoding stored incident: %w", err)
			}

			// Remediations ride inside the incident blob as a
			// {total_count, has_more, items} envelope holding only the first
			// items; say so when the list is not the whole set. A field that
			// is missing or cannot be read is unknown: null, never zero.
			remediations, total, complete, known := remediationsOf(incident["remediations"])
			delete(incident, "remediations")

			result := map[string]interface{}{
				"incident":              incident,
				"remediations":          remediations,
				"remediation_count":     total,
				"remediations_complete": complete,
			}
			switch {
			case !known:
				result["remediations"] = nil
				result["remediation_count"] = nil
				result["note"] = fmt.Sprintf("the stored incident has no readable remediations list, so its remediations are unknown; list them with the remediations command for incident %d", incidentID)
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: incident %d has no readable remediations list; remediation_count is withheld (null).\n", incidentID)
			case !complete:
				result["note"] = fmt.Sprintf("the incident lists %d of its %d remediations; list the rest with the remediations command for incident %d", len(remediations), total, incidentID)
			}

			if orgID, ok := toFloat(incident["organization_id"]); ok && orgID > 0 {
				orgRows, e := queryMaps(ctx, st.DB(),
					fmt.Sprintf(`SELECT %s AS id, %s AS name, %s AS key FROM %s o WHERE %s = ?`,
						jx("o", "id"), jx("o", "name"), jx("o", "key"), st.src(entOrgs), jx("o", "id")), int64(orgID))
				if e != nil {
					return fmt.Errorf("reading the incident's organization: %w", e)
				}
				if len(orgRows) > 0 {
					result["organization"] = orgRows[0]
				}
			}
			if agentID, ok := toFloat(incident["agent_id"]); ok && agentID > 0 {
				agRows, e := queryMaps(ctx, st.DB(),
					fmt.Sprintf(`SELECT %s AS id, %s AS hostname, %s AS platform, %s AS last_callback_at, %s AS external_ip, %s AS ipv4_address
					FROM %s a WHERE %s = ?`,
						jx("a", "id"), jx("a", "hostname"), jx("a", "platform"), jx("a", "last_callback_at"),
						jx("a", "external_ip"), jx("a", "ipv4_address"), st.src(entAgents), jx("a", "id")), int64(agentID))
				if e != nil {
					return fmt.Errorf("reading the incident's agent: %w", e)
				}
				if len(agRows) > 0 {
					result["agent"] = agRows[0]
				}
			}
			return printJSONFilteredKeep(cmd.OutOrStdout(), result, flags, "remediation_count", "remediations_complete")
		},
	}
	cmd.Flags().IntVar(&id, "id", 0, "Incident report ID to enrich")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}
