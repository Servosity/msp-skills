// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature (reprint 20260606): enriched single-incident view.

package cli

import (
	"encoding/json"
	"fmt"

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
			if id == 0 && len(args) > 0 {
				// Ignore parse errors: a non-numeric arg leaves id==0 and the
				// guards below return help / a usage error, which is intended.
				_, _ = fmt.Sscanf(args[0], "%d", &id)
			}
			if id == 0 && len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would look up one incident with remediations, agent, and org joined")
				return nil
			}
			if id == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("an incident id is required (positional or --id)"))
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, rtIncidents, rtAgents, rtOrgs)
			if err != nil {
				return err
			}
			defer st.Close()

			rows, err := queryMaps(ctx, st.DB(),
				fmt.Sprintf(`SELECT i.data AS data FROM resources i WHERE i.resource_type='%s' AND %s = ?`, rtIncidents, jx("i", "id")), id)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				return fmt.Errorf("incident %d not found in the local store; run `sync` first or check the id", id)
			}
			blob, _ := rows[0]["data"].(string)
			incident := map[string]interface{}{}
			if err := json.Unmarshal([]byte(blob), &incident); err != nil {
				return fmt.Errorf("decoding stored incident: %w", err)
			}

			// Remediations ride inside the incident blob.
			remediations, _ := incident["remediations"].([]interface{})
			if remediations == nil {
				remediations = []interface{}{}
			}
			delete(incident, "remediations")

			result := map[string]interface{}{
				"incident":          incident,
				"remediations":      remediations,
				"remediation_count": len(remediations),
			}

			if orgID, ok := toFloat(incident["organization_id"]); ok && orgID > 0 {
				if orgRows, e := queryMaps(ctx, st.DB(),
					fmt.Sprintf(`SELECT %s AS id, %s AS name, %s AS key FROM resources o WHERE o.resource_type='%s' AND %s = ?`,
						jx("o", "id"), jx("o", "name"), jx("o", "key"), rtOrgs, jx("o", "id")), int(orgID)); e == nil && len(orgRows) > 0 {
					result["organization"] = orgRows[0]
				}
			}
			if agentID, ok := toFloat(incident["agent_id"]); ok && agentID > 0 {
				if agRows, e := queryMaps(ctx, st.DB(),
					fmt.Sprintf(`SELECT %s AS id, %s AS hostname, %s AS platform, %s AS last_callback_at, %s AS external_ip, %s AS ipv4_address
					FROM resources a WHERE a.resource_type='%s' AND %s = ?`,
						jx("a", "id"), jx("a", "hostname"), jx("a", "platform"), jx("a", "last_callback_at"),
						jx("a", "external_ip"), jx("a", "ipv4_address"), rtAgents, jx("a", "id")), int(agentID)); e == nil && len(agRows) > 0 {
					result["agent"] = agRows[0]
				}
			}
			return flags.printJSON(cmd, result)
		},
	}
	cmd.Flags().IntVar(&id, "id", 0, "Incident report ID to enrich")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}
