// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Absorbed feature (manifest row 113): one local join replacing the N+1 call
// pattern of SPSImmyBot's Get-ImmyComputerFullReport.
// pp:data-source local

package cli

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"immybot-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

type dossierSoftware struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type dossierAction struct {
	Action string `json:"action"`
	Status string `json:"status,omitempty"`
	When   string `json:"when,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type computerDossierView struct {
	ComputerID    string            `json:"computer_id"`
	ComputerName  string            `json:"computer_name,omitempty"`
	Tenant        string            `json:"tenant,omitempty"`
	TenantID      string            `json:"tenant_id,omitempty"`
	Online        bool              `json:"online"`
	ExcludedMaint bool              `json:"excluded_from_maintenance"`
	SoftwareCount int               `json:"software_count"`
	Software      []dossierSoftware `json:"software"`
	RecentActions []dossierAction   `json:"recent_actions"`
	FailedActions int               `json:"failed_actions"`
	Note          string            `json:"note,omitempty"`
}

func newNovelComputerDossierCmd(flags *rootFlags) *cobra.Command {
	var (
		flagLimit int
		dbPath    string
	)
	cmd := &cobra.Command{
		Use:   "computer-dossier [computer]",
		Short: "Everything the mirror knows about one computer, in one join",
		Long: "Identity, tenant, installed software, and recent maintenance history for one " +
			"computer, assembled from the local mirror in a single pass instead of one API call " +
			"per section.",
		Example: strings.Trim(`
  immybot-cli computer-dossier 4821
  immybot-cli computer-dossier WS-01 --agent
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "computer=1",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "computer-dossier")
			}
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a computer id or name is required"))
			}
			needle := strings.TrimSpace(args[0])
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			dbPath = immyMirrorPath(dbPath)
			view := computerDossierView{
				ComputerID:    needle,
				Software:      make([]dossierSoftware, 0),
				RecentActions: make([]dossierAction, 0),
			}
			if immyMirrorMissing(cmd, dbPath, "computers,tenants-software-from-inventory-dx,maintenance-actions") {
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				return nil
			}
			db, err := store.OpenWithContext(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "computers") {
				hintIfStale(cmd, db, "computers", flags.maxAge)
			}

			crows, err := db.DB().QueryContext(ctx, `
				SELECT id, json_extract(data,'$.name'), json_extract(data,'$.tenant'),
				       json_extract(data,'$.tenantId'), json_extract(data,'$.online'),
				       json_extract(data,'$.excludeFromMaintenance')
				FROM resources WHERE resource_type IN ('computers','computers-paged','computers-dx','computers-my')`)
			if err != nil {
				return fmt.Errorf("querying computers: %w", err)
			}
			found := false
			for crows.Next() {
				var id, name, tenant, tenantID, online, excluded sql.NullString
				if err := crows.Scan(&id, &name, &tenant, &tenantID, &online, &excluded); err != nil {
					_ = crows.Close()
					return fmt.Errorf("scanning computer: %w", err)
				}
				if found {
					continue
				}
				if nullStr(id) == needle || strings.EqualFold(nullStr(name), needle) {
					view.ComputerID = nullStr(id)
					view.ComputerName = nullStr(name)
					view.Tenant = nullStr(tenant)
					view.TenantID = nullStr(tenantID)
					view.Online = truthy(nullStr(online))
					view.ExcludedMaint = truthy(nullStr(excluded))
					found = true
				}
			}
			if err := crows.Err(); err != nil {
				_ = crows.Close()
				return fmt.Errorf("iterating computers: %w", err)
			}
			if err := crows.Close(); err != nil {
				return fmt.Errorf("closing computers: %w", err)
			}
			if !found {
				view.Note = fmt.Sprintf("no computer with id or name %q in the local mirror", needle)
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				return nil
			}

			srows, err := db.DB().QueryContext(ctx, `
				SELECT json_extract(data,'$.softwareName'), json_extract(data,'$.version')
				FROM resources
				WHERE resource_type = 'tenants-software-from-inventory-dx'
				  AND (json_extract(data,'$.computerId') = ? OR json_extract(data,'$.computerName') = ?)`,
				view.ComputerID, view.ComputerName)
			if err == nil {
				for srows.Next() {
					var name, version sql.NullString
					if err := srows.Scan(&name, &version); err != nil {
						break
					}
					view.Software = append(view.Software, dossierSoftware{Name: nullStr(name), Version: nullStr(version)})
				}
				_ = srows.Err()
				_ = srows.Close()
			}
			sort.Slice(view.Software, func(i, j int) bool { return view.Software[i].Name < view.Software[j].Name })
			view.SoftwareCount = len(view.Software)

			arows, err := db.DB().QueryContext(ctx, `
				SELECT COALESCE(json_extract(data,'$.maintenanceDisplayName'), json_extract(data,'$.actionTypeName')),
				       COALESCE(json_extract(data,'$.statusName'), json_extract(data,'$.status')),
				       COALESCE(json_extract(data,'$.startTime'), json_extract(data,'$.endTime')),
				       COALESCE(json_extract(data,'$.reason'), json_extract(data,'$.result'))
				FROM resources
				WHERE resource_type IN ('maintenance-actions','maintenance-actions-maintenance-item')
				  AND (json_extract(data,'$.computerId') = ? OR json_extract(data,'$.computerName') = ?)`,
				view.ComputerID, view.ComputerName)
			if err == nil {
				for arows.Next() {
					var action, status, when, reason sql.NullString
					if err := arows.Scan(&action, &status, &when, &reason); err != nil {
						break
					}
					a := dossierAction{
						Action: nullStr(action), Status: nullStr(status),
						When: nullStr(when), Reason: nullStr(reason),
					}
					if immyIsFailureStatus(a.Status) {
						view.FailedActions++
					}
					view.RecentActions = append(view.RecentActions, a)
				}
				_ = arows.Err()
				_ = arows.Close()
			}
			sort.Slice(view.RecentActions, func(i, j int) bool { return view.RecentActions[i].When > view.RecentActions[j].When })
			if flagLimit > 0 {
				if len(view.RecentActions) > flagLimit {
					view.RecentActions = view.RecentActions[:flagLimit]
				}
				if len(view.Software) > flagLimit {
					view.Software = view.Software[:flagLimit]
				}
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s (%s)\n  tenant   %s\n  online   %t\n  excluded %t\n  software %d titles\n  actions  %d (%d failed)\n",
				firstNonEmpty(view.ComputerName, view.ComputerID), view.ComputerID, view.Tenant,
				view.Online, view.ExcludedMaint, view.SoftwareCount, len(view.RecentActions), view.FailedActions)
			return nil
		},
	}
	cmd.Flags().IntVar(&flagLimit, "limit", 50, "Maximum software titles and actions to list (0 for all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror database path")
	return cmd
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		addNovelCommandIfAbsent(root, newNovelComputerDossierCmd(flags))
	})
}
