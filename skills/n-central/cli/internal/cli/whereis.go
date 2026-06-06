// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature. Not generated.

package cli

import (
	"database/sql"
	"fmt"

	"github.com/spf13/cobra"
	"n-central-pp-cli/internal/store"
)

// whereisMatch is one located device with its full org path.
type whereisMatch struct {
	DeviceID     string `json:"deviceId"`
	LongName     string `json:"longName"`
	Path         string `json:"path"`
	SoName       string `json:"soName"`
	CustomerName string `json:"customerName"`
	SiteName     string `json:"siteName"`
	OrgUnitID    string `json:"orgUnitId"`
}

func newNovelWhereisCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "whereis <device>",
		Short: "Given a device name fragment, return its full path — server, service org, customer, site — from the local mirror.",
		Long: `Locate a device in the local SQLite mirror by name fragment (matched against
longName, discoveredName, or exact id) and print its full org path:

    server > serviceOrg > customer > site

Local-only: whereis never hits the network. Run 'sync' first to populate the
mirror. An empty result is not an error — it prints a hint and exits 0.`,
		Example: `  n-central-cli whereis EXCHANGE01
  n-central-cli whereis web --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			frag := args[0]

			// Derive the server label up front so every path starts with it.
			serverHost := ""
			if c, err := flags.newClient(); err == nil {
				serverHost = hostFromBaseURL(c.BaseURL)
			}

			dbPath := defaultDBPath("n-central-cli")
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w\nRun 'n-central-cli sync' first.", err)
			}
			defer db.Close()

			like := "%" + frag + "%"
			rows, err := db.DB().QueryContext(cmd.Context(),
				`SELECT data, so_name, customer_name, site_name, org_unit_id, long_name
				 FROM devices
				 WHERE long_name LIKE ? OR discovered_name LIKE ? OR id = ?`,
				like, like, frag,
			)
			if err != nil {
				return fmt.Errorf("querying devices: %w", err)
			}
			defer rows.Close()

			var matches []whereisMatch
			for rows.Next() {
				var data string
				var soName, customerName, siteName, longName sql.NullString
				var orgUnitID sql.NullInt64
				if err := rows.Scan(&data, &soName, &customerName, &siteName, &orgUnitID, &longName); err != nil {
					return fmt.Errorf("scanning device row: %w", err)
				}
				orgID := ""
				if orgUnitID.Valid {
					orgID = fmt.Sprintf("%d", orgUnitID.Int64)
				}
				deviceID := ""
				if obj := decodeObj([]byte(data)); obj != nil {
					deviceID = asString(firstField(obj, "deviceId", "device_id", "id"))
				}
				m := whereisMatch{
					DeviceID:     deviceID,
					LongName:     longName.String,
					SoName:       soName.String,
					CustomerName: customerName.String,
					SiteName:     siteName.String,
					OrgUnitID:    orgID,
				}
				m.Path = joinPath(serverHost, m.SoName, m.CustomerName, m.SiteName)
				matches = append(matches, m)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating device rows: %w", err)
			}

			if len(matches) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "no device matching %q in the local store; run 'sync' first\n", frag)
				return nil
			}

			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				for _, m := range matches {
					name := m.LongName
					if name == "" {
						name = m.DeviceID
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s  ->  %s\n", name, m.Path)
				}
				return nil
			}
			return flags.printJSON(cmd, matches)
		},
	}
	return cmd
}
