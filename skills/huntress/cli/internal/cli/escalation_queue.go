// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// pp:data-source local

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelEscalationQueueCmd(flags *rootFlags) *cobra.Command {
	var severity, org, dbPath string
	cmd := &cobra.Command{
		Use:   "escalation-queue",
		Short: "Unresolved escalations across the fleet, aged and ordered by severity",
		Long: "Lists every synced escalation that is not resolved (status other than \"resolved\" and no\n" +
			"resolved_at), with hours open from created_at and the organizations it names, ordered\n" +
			"critical first, then oldest first. Escalations are a separate list from incident reports,\n" +
			"so this is the backlog fleet-incidents does not show. Reads the local store; run `sync` first.",
		Example:     "  huntress-cli escalation-queue --severity critical --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			sevs := lowerAll(csvList(severity))
			for _, s := range sevs {
				if s != "low" && s != "high" && s != "critical" {
					return usageErr(fmt.Errorf("--severity %q: want low, high or critical", s))
				}
			}
			var orgID int64
			if cmd.Flags().Changed("org") {
				id, err := parsePositiveID("--org", org)
				if err != nil {
					return usageErr(err)
				}
				orgID = id
			}
			if dryRunOK(flags) {
				return nil
			}
			st, err := openStoreFor(cmd, flags, dbPath, entEscal)
			if err != nil {
				return err
			}
			defer st.Close()

			// Unresolved means BOTH signals say so: a resolved status or a
			// resolved_at timestamp each take the row out.
			where := []string{
				"lower(COALESCE(" + jx("e", "status") + ", '')) != 'resolved'",
				"COALESCE(" + jx("e", "resolved_at") + ", '') = ''"}
			qargs := []interface{}{}
			if frag, a := inClause("lower("+jx("e", "severity")+")", sevs); frag != "" {
				where = append(where, frag)
				qargs = append(qargs, a...)
			}
			if orgID > 0 {
				// organizations items are {id,name} objects in practice; the spec
				// types them as strings, so a bare id string matches too.
				where = append(where, `EXISTS (SELECT 1 FROM json_each(e.data, '$.organizations') j
					WHERE CAST(CASE WHEN j.type = 'object' THEN json_extract(j.value, '$.id') ELSE j.value END AS TEXT) = ?)`)
				qargs = append(qargs, fmt.Sprint(orgID))
			}
			q := fmt.Sprintf(`SELECT %s AS id, lower(%s) AS severity, %s AS status, %s AS type, %s AS subtype,
				%s AS subject, %s AS created_at,
				ROUND((julianday('now') - %s) * 24, 1) AS hours_open,
				%s AS organizations
			FROM %s e
			WHERE %s
			ORDER BY CASE lower(%s) WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'low' THEN 2 ELSE 3 END,
				%s IS NULL, %s ASC, e.id`,
				jx("e", "id"), jx("e", "severity"), jx("e", "status"), jx("e", "type"), jx("e", "subtype"),
				jx("e", "subject"), jx("e", "created_at"), jdx("e", "created_at"), jx("e", "organizations"),
				st.src(entEscal), strings.Join(where, " AND "), jx("e", "severity"), jdx("e", "created_at"), jdx("e", "created_at"))
			rows, err := queryMaps(cmd.Context(), st.DB(), q, qargs...)
			if err != nil {
				return err
			}
			// organizations is a list; one that is missing or cannot be read
			// is unknown (null), never "names no organization" ([]).
			unreadable := 0
			for _, r := range rows {
				var orgs []interface{}
				if s, ok := r["organizations"].(string); ok {
					orgs, err = decodeJSONList([]byte(s))
				} else {
					err = fmt.Errorf("missing")
				}
				if err != nil {
					r["organizations"] = nil
					unreadable++
					continue
				}
				r["organizations"] = orgs
			}
			warnUnreadable(cmd, unreadable, "escalation(s) have a missing or unreadable organizations list", "their organizations are shown as null (unknown), and --org cannot match them.")
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().StringVar(&severity, "severity", "", "Only these severities, comma-separated: low, high, critical")
	cmd.Flags().StringVar(&org, "org", "", "Only escalations naming this organization ID")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the store the sync command writes)")
	return cmd
}
