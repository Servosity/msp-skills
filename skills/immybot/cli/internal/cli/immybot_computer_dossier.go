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

	"github.com/spf13/cobra"
)

type dossierSoftware struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type dossierAction struct {
	Action string `json:"action"`
	// Status is the outcome (resultName); Lifecycle is statusName.
	Status    string `json:"status,omitempty"`
	Lifecycle string `json:"lifecycle,omitempty"`
	// When is the action's time as mirrored: the first of
	// immyActionTimeFields that parses, else the first one present.
	When   string `json:"when,omitempty"`
	Reason string `json:"reason,omitempty"`
	at     immyRowTime
}

// computerDossierView is one computer's dossier. SoftwareCount counts the
// detected software rows (DetectedComputerSoftwareResponse, one per install, so
// a title detected twice counts twice); ActionCount counts the computer's
// recorded actions and FailedActions the failures among them. All three are
// totals: --limit caps only the software and recent_actions lists.
type computerDossierView struct {
	ComputerID    string            `json:"computer_id"`
	ComputerName  string            `json:"computer_name,omitempty"`
	Tenant        string            `json:"tenant,omitempty"`
	TenantID      string            `json:"tenant_id,omitempty"`
	Online        bool              `json:"online"`
	ExcludedMaint bool              `json:"excluded_from_maintenance"`
	SoftwareCount int               `json:"software_count"`
	Software      []dossierSoftware `json:"software"`
	ActionCount   int               `json:"action_count"`
	RecentActions []dossierAction   `json:"recent_actions"`
	FailedActions int               `json:"failed_actions"`
	// UnidentifiedActions and UnidentifiedSoftware count the mirrored rows
	// that carry no computer id: no computer can claim them, and a name never
	// stands for one.
	UnidentifiedActions  int    `json:"unidentified_actions"`
	UnidentifiedSoftware int    `json:"unidentified_software"`
	Note                 string `json:"note,omitempty"`
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
			"per section. The computer is an id, or a name only one mirrored computer carries. " +
			"Software and actions join the computer by computer id only; mirrored rows that carry no " +
			"computer id are counted in unidentified_actions and unidentified_software, never matched by name. " +
			"software_count, action_count and failed_actions are totals; --limit caps only the lists, " +
			"and recent_actions is newest first by each action's time.",
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
			db, err := immyOpenMirror(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "computers") {
				hintIfStale(cmd, db, "computers", flags.maxAge)
			}

			// Identity from every computer resource, each read with its own
			// schema's field names (immyComputerShapes).
			roster, err := immyLoadComputers(ctx, db, immyComputerIdentityTypes)
			if err != nil {
				return err
			}
			match, ambiguous, found := roster.find(needle)
			if found {
				view.ComputerID = match.ID
				view.ComputerName = match.Name
				view.TenantID = match.TenantID
				view.Online = truthy(match.Online)
				view.ExcludedMaint = truthy(match.Excluded)
				if view.Tenant, err = immyComputerTenantName(ctx, db, match); err != nil {
					return err
				}
			}
			if !found {
				view.Note = immyComputerNotFound(needle, ambiguous)
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				return nil
			}

			join, joinArgs := immyComputerJoinFor("tenants-software-from-inventory-dx", match.ID)
			srows, err := db.DB().QueryContext(ctx, `
				SELECT json_extract(data,'$.softwareName'), json_extract(data,'$.version')
				FROM resources
				WHERE resource_type = 'tenants-software-from-inventory-dx'
				  AND `+join, joinArgs...)
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

			join, joinArgs = immyComputerJoinFor("maintenance-actions", match.ID)
			arows, err := db.DB().QueryContext(ctx, `
				SELECT id, COALESCE(json_extract(data,'$.maintenanceDisplayName'), json_extract(data,'$.actionTypeName')),
				       `+immyTimeColumns(immyActionTimeFields)+`,
				       `+immyActionOutcomeColumns+`
				`+immyActionFrom+`
				  AND `+join+immyActionOrder, joinArgs...)
			if err == nil {
				seen := map[string]struct{}{}
				for arows.Next() {
					var rowID, action sql.NullString
					var oc immyActionOutcomeRow
					ts := newImmyTimeScan(immyActionTimeFields)
					dest := append([]any{&rowID, &action}, ts.dest()...)
					if err := arows.Scan(append(dest, oc.dest()...)...); err != nil {
						break
					}
					if immyActionSeen(seen, rowID) {
						continue
					}
					o := oc.outcome()
					a := dossierAction{
						Action: nullStr(action), Status: o.Label, Lifecycle: o.Lifecycle,
						Reason: o.Cause, at: ts.time(),
					}
					a.When = a.at.Raw
					if !a.at.Known {
						for _, f := range immyActionTimeFields {
							if a.When = ts.raw(f); a.When != "" {
								break
							}
						}
					}
					if o.Failed {
						view.FailedActions++
					}
					view.RecentActions = append(view.RecentActions, a)
				}
				_ = arows.Err()
				_ = arows.Close()
			}
			view.ActionCount = len(view.RecentActions)
			if err := db.DB().QueryRowContext(ctx, `SELECT COUNT(DISTINCT id) `+immyActionFrom+
				` AND `+immyComputerNoID("maintenance-actions")).Scan(&view.UnidentifiedActions); err != nil && !immyNoSuchTableErr(err) {
				return fmt.Errorf("counting actions with no computer id: %w", err)
			}
			if err := db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM resources WHERE resource_type = 'tenants-software-from-inventory-dx' AND `+
				immyComputerNoID("tenants-software-from-inventory-dx")).Scan(&view.UnidentifiedSoftware); err != nil && !immyNoSuchTableErr(err) {
				return fmt.Errorf("counting software rows with no computer id: %w", err)
			}
			// Newest first, compared as instants (offsets and the offset-less
			// .NET form differ as text); an action with no usable time last.
			sort.SliceStable(view.RecentActions, func(i, j int) bool {
				a, b := view.RecentActions[i].at, view.RecentActions[j].at
				if a.Known != b.Known {
					return a.Known
				}
				return a.Known && a.At.After(b.At)
			})
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
			fmt.Fprintf(cmd.OutOrStdout(), "%s (%s)\n  tenant   %s\n  online   %t\n  excluded %t\n  software %d detected installs\n  actions  %d (%d failed)\n",
				firstNonEmpty(view.ComputerName, view.ComputerID), view.ComputerID, view.Tenant,
				view.Online, view.ExcludedMaint, view.SoftwareCount, view.ActionCount, view.FailedActions)
			if view.UnidentifiedActions+view.UnidentifiedSoftware > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "  not counted: %d action(s) and %d software row(s) in the mirror carry no computer id\n",
					view.UnidentifiedActions, view.UnidentifiedSoftware)
			}
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
