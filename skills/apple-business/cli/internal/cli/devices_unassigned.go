// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: devices unassigned.
// pp:data-source local

package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"apple-business-pp-cli/internal/cliutil"
	"apple-business-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

type unassignedRow struct {
	Org                string `json:"org,omitempty"`
	Serial             string `json:"serial"`
	Model              string `json:"model"`
	Family             string `json:"family"`
	AddedToOrg         string `json:"added_to_org"`
	DaysInOrg          int    `json:"days_in_org"`
	OrderNumber        string `json:"order_number,omitempty"`
	PurchaseSourceType string `json:"purchase_source_type,omitempty"`
	PurchaseSourceID   string `json:"purchase_source_id,omitempty"`
	NoDefaultServer    bool   `json:"no_default_server"`
}

func newNovelDevicesUnassignedCmd(flags *rootFlags) *cobra.Command {
	var family, addedWithin, dbPath string
	var allOrgs bool
	cmd := &cobra.Command{
		Use:   "unassigned",
		Short: "Org-owned devices not assigned to any MDM server, with days since it joined the org and a missing-default-server flag",
		Long: `Use this command to list org-owned devices not assigned to any MDM server, with days since it joined the org and a missing-default-server diagnosis. Do NOT use this command to perform the assignment; pipe its serials into 'devices assign' instead.

Reads the local store: run 'sync --resources devices,mdm_servers' first.
no_default_server is true when no ACTIVE device management service lists the
device's product family in defaultProductFamilies, the usual reason a new
device lands unassigned. days_in_org counts from addedToOrgDateTime (the API
records no unassigned-since date); rows sort oldest first. --all-orgs reads
every customer org's store.`,
		Example: strings.Trim(`
  apple-business-cli devices unassigned --added-within 7d --agent
  apple-business-cli devices unassigned --family Mac --all-orgs --csv`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "devices unassigned")
			}
			rows, err := unassignedRows(cmd, flags, orgStorePaths(dbPath, allOrgs), allOrgs, storeDBPath(dbPath), family, addedWithin)
			if err != nil {
				return err
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No unassigned devices in the local store.")
				return nil
			}
			for _, r := range rows {
				diag := ""
				if r.NoDefaultServer {
					diag = "  (no server defaults this family)"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s%s  %s  %s  %dd  order %s%s\n", prefixOrg(r.Org), r.Serial, r.Family, r.Model, r.DaysInOrg, firstNonEmpty(r.OrderNumber, "-"), diag)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&family, "family", "", "Only this product family (Mac, iPhone, iPad, AppleTV, Watch, Vision)")
	cmd.Flags().StringVar(&addedWithin, "added-within", "", "Only devices added to the org within this window (e.g. 7d, 4w)")
	cmd.Flags().BoolVar(&allOrgs, "all-orgs", false, "Read every customer org's local store, not just the active API account's")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: the active API account's store)")
	return cmd
}

func prefixOrg(org string) string {
	if org == "" {
		return ""
	}
	return org + "  "
}

func unassignedRows(cmd *cobra.Command, flags *rootFlags, paths []string, labelOrgs bool, current, family, addedWithin string) ([]unassignedRow, error) {
	var window time.Duration
	if addedWithin != "" {
		d, err := cliutil.ParseDurationLoose(addedWithin)
		if err != nil || d <= 0 {
			return nil, usageErr(fmt.Errorf("--added-within must be a positive duration such as 7d or 4w"))
		}
		window = d
	}
	now := time.Now()
	multi := len(paths) > 1 || labelOrgs
	rows := make([]unassignedRow, 0)
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "no local store at %s\nrun: apple-business-cli sync --resources devices,mdm_servers\n", path)
			continue
		}
		db, err := store.OpenReadOnlyContext(cmd.Context(), path)
		if err != nil {
			return nil, fmt.Errorf("opening %s: %w", path, err)
		}
		if !multi && !hintIfUnsynced(cmd, db, "devices") {
			hintIfStale(cmd, db, "devices", flags.maxAge)
		}
		devices, err := loadLocalDevices(cmd.Context(), db)
		if err == nil {
			var servers []mdmServer
			servers, err = loadLocalServers(cmd.Context(), db)
			// With the server list synced (even empty), a family no ACTIVE
			// server defaults is a real gap; never synced means unknown.
			serversKnown := len(servers) > 0 || db.GetLastSyncedAt("mdm_servers") != ""
			covered := map[string]bool{}
			for _, s := range servers {
				if strings.EqualFold(s.Attributes.Status, "ACTIVE") {
					for _, f := range s.Attributes.DefaultProductFamilies {
						covered[familyKey(f)] = true
					}
				}
			}
			label := ""
			skip := false
			if multi {
				skip = path != current && !storeHasData(cmd.Context(), db)
				label = orgLabel(cmd.Context(), db, path)
				if !skip {
					hintStoreUnsynced(cmd, db, label)
				}
			}
			for _, d := range devices {
				a := d.Attributes
				if skip || !strings.EqualFold(a.Status, "UNASSIGNED") || d.released() {
					continue
				}
				if family != "" && familyKey(a.ProductFamily) != familyKey(family) {
					continue
				}
				added, ok := parseTime(a.AddedToOrgDateTime)
				if window > 0 && (!ok || now.Sub(added) > window) {
					continue
				}
				r := unassignedRow{Org: label, Serial: d.serial(), Model: a.DeviceModel, Family: a.ProductFamily, AddedToOrg: a.AddedToOrgDateTime,
					OrderNumber: a.OrderNumber, PurchaseSourceType: a.PurchaseSourceType, PurchaseSourceID: a.PurchaseSourceID,
					NoDefaultServer: serversKnown && !covered[familyKey(a.ProductFamily)]}
				if ok {
					r.DaysInOrg = int(now.Sub(added).Hours() / 24)
				}
				rows = append(rows, r)
			}
		}
		_ = db.Close()
		if err != nil {
			return nil, err
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].DaysInOrg > rows[j].DaysInOrg })
	return rows, nil
}
