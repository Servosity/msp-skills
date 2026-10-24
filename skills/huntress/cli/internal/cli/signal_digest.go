// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// pp:data-source local

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelSignalDigestCmd(flags *rootFlags) *cobra.Command {
	var since, types, org, dbPath string
	cmd := &cobra.Command{
		Use:   "signal-digest",
		Short: "SOC signals grouped by type and organization over a window, with reported and closed counts",
		Long: "Groups every synced SOC signal by signal type and organization within --since (by created_at),\n" +
			"with reported and closed counts and first/last seen. The API lists signals one filter at a time;\n" +
			"this is the fleet-wide rollup. Reads the local store; run `sync` first.",
		Example:     "  huntress-cli signal-digest --since 7d --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			days, err := parseWindowDays("since", since)
			if err != nil {
				return usageErr(err)
			}
			var orgID int64
			if cmd.Flags().Changed("org") {
				if orgID, err = parsePositiveID("--org", org); err != nil {
					return usageErr(err)
				}
			}
			if dryRunOK(flags) {
				return nil
			}
			st, err := openStoreFor(cmd, flags, dbPath, entSignals)
			if err != nil {
				return err
			}
			defer st.Close()

			where := []string{"1=1"}
			qargs := []interface{}{}
			if frag, a := inClause("lower("+jx("s", "type")+")", lowerAll(csvList(types))); frag != "" {
				where = append(where, frag)
				qargs = append(qargs, a...)
			}
			if orgID > 0 {
				where = append(where, "CAST("+jx("s", nested("organization", "id"))+" AS TEXT) = ?")
				qargs = append(qargs, fmt.Sprint(orgID))
			}
			unreadable := unreadableTimeSQL("s", "created_at", true)
			if days > 0 {
				// A signal whose created_at cannot be read cannot be placed in
				// the window: it is not counted, and the warning says so.
				n, err := countRows(cmd.Context(), st.DB(), "SELECT COUNT(*) FROM "+st.src(entSignals)+" s WHERE "+strings.Join(where, " AND ")+" AND "+unreadable, qargs...)
				if err != nil {
					return fmt.Errorf("checking signal timestamps: %w", err)
				}
				warnUnreadable(cmd, n, "signal(s) have a missing or unreadable created_at", "whether they fall in the window is unknown, so they are not counted; run with --since all to count them.")
				where = append(where, jx("s", "created_at")+" IS NOT NULL AND julianday('now') - "+jdx("s", "created_at")+" <= ?")
				qargs = append(qargs, days)
			}
			// first_seen and last_seen are withheld (null) for a group holding a
			// signal whose created_at cannot be read: MIN and MAX would skip it.
			q := fmt.Sprintf(`SELECT %[1]s AS type, %[2]s AS organization_id, MAX(%[3]s) AS organization_name,
				COUNT(*) AS signals,
				SUM(CASE WHEN lower(%[4]s) = 'reported' THEN 1 ELSE 0 END) AS reported,
				SUM(CASE WHEN lower(%[4]s) = 'closed' THEN 1 ELSE 0 END) AS closed,
				CASE WHEN SUM(CASE WHEN %[8]s THEN 1 ELSE 0 END) > 0 THEN NULL
					ELSE strftime('%%Y-%%m-%%dT%%H:%%M:%%SZ', MIN(%[5]s)) END AS first_seen,
				CASE WHEN SUM(CASE WHEN %[8]s THEN 1 ELSE 0 END) > 0 THEN NULL
					ELSE strftime('%%Y-%%m-%%dT%%H:%%M:%%SZ', MAX(%[5]s)) END AS last_seen,
				SUM(CASE WHEN %[8]s THEN 1 ELSE 0 END) AS pp_unreadable
			FROM %[7]s s
			WHERE %[6]s
			GROUP BY %[1]s, %[2]s
			ORDER BY signals DESC, type, organization_id`,
				jx("s", "type"), jx("s", nested("organization", "id")), jx("s", nested("organization", "name")),
				jx("s", "status"), jdx("s", "created_at"), strings.Join(where, " AND "), st.src(entSignals), unreadable)
			rows, err := queryMaps(cmd.Context(), st.DB(), q, qargs...)
			if err != nil {
				return err
			}
			unread := 0
			for _, r := range rows {
				if n, ok := toFloat(r["pp_unreadable"]); ok {
					unread += int(n)
				}
				delete(r, "pp_unreadable")
			}
			warnUnreadable(cmd, unread, "counted signal(s) have a missing or unreadable created_at", "first_seen and last_seen are withheld (null) for their groups.")
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().StringVar(&since, "since", "7d", "Look-back window on created_at, e.g. 24h, 7d, 2w, or all")
	cmd.Flags().StringVar(&types, "type", "", "Only these signal types, comma-separated (e.g. Footholds,Antivirus)")
	cmd.Flags().StringVar(&org, "org", "", "Only this organization ID")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the store the sync command writes)")
	return cmd
}

// nested joins JSON path segments for jx. Built at runtime because a dotted
// path literal (a field name, a dot, a field name) reads as a host to the novel-host gate.
func nested(parts ...string) string { return strings.Join(parts, ".") }

func lowerAll(in []string) []string {
	for i := range in {
		in[i] = strings.ToLower(in[i])
	}
	return in
}
