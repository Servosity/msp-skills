// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local
package cli

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelSoftwareFindCmd(flags *rootFlags) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:         "find [name]",
		Short:       "Find which agents have a software package installed",
		Long:        "Searches synced per-agent software inventory for a package name (case-insensitive substring) and lists each agent, version and publisher. Reads only the local store. To find agents where a Windows service is stopped instead, use 'services down'.",
		Example:     "  tactical-rmm-cli software find openssl\n  tactical-rmm-cli software find --name chrome --json",
		Annotations: map[string]string{"mcp:read-only": "true"}, // read-only: queries the local store only
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would search synced software inventory for the named package")
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			q := name
			if len(args) > 0 {
				q = args[0]
			}
			if strings.TrimSpace(q) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a package name is required (positional or --name)"))
			}
			type row struct {
				AgentID   string `json:"agent_id"`
				Name      string `json:"name"`
				Version   string `json:"version"`
				Publisher string `json:"publisher"`
			}
			type envelope struct {
				Query string `json:"query"`
				Items []row  `json:"items"`
				Total int    `json:"total"`
				Note  string `json:"note,omitempty"`
			}
			items := make([]row, 0)
			if s := novelLocalRead(cmd, flags, "software"); s != nil {
				defer s.Close()
				sq := "SELECT json_extract(s.data,'$.agent'), json_extract(je.value,'$.name'), json_extract(je.value,'$.version'), json_extract(je.value,'$.publisher') FROM resources s, json_each(json_extract(s.data,'$.software')) je WHERE s.resource_type='software' AND lower(json_extract(je.value,'$.name')) LIKE '%'||lower(?)||'%'"
				if rows, qe := s.DB().QueryContext(cmd.Context(), sq, q); qe == nil {
					defer rows.Close()
					for rows.Next() {
						var ag, nm, ver, pub sql.NullString
						if rows.Scan(&ag, &nm, &ver, &pub) == nil {
							items = append(items, row{ag.String, nm.String, ver.String, pub.String})
						}
					}
				}
			}
			out := envelope{Query: q, Items: items, Total: len(items)}
			if len(items) == 0 {
				out.Note = fmt.Sprintf("no installed software matching %q in the local store; run 'tactical-rmm-cli sync' to refresh inventory", q)
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Package name to search for")
	return cmd
}
