// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet posture rolls up Mac compliance per client instance.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"jamf-pro-pp-cli/internal/fleet"
)

func newNovelFleetPostureCmd(flags *rootFlags) *cobra.Command {
	var staleDays, pageSize int
	var minOS, within string
	var failing bool
	var scope fleetScope

	cmd := &cobra.Command{
		Use:   "posture",
		Short: "Per-client counts of stale check-ins, Macs below a macOS floor, FileVault off, firewall off and expiring MDM profiles",
		Long: `Per-client counts of stale check-ins, Macs below a macOS floor, FileVault off, firewall off and expiring MDM profiles.

Use this command for cross-client Mac compliance: stale check-ins, macOS below a floor, FileVault off, firewall off, MDM profile expiring. Do NOT use this command to find one device; use 'fleet find' instead. Do NOT use it for patch titles; use 'fleet patch' instead.

Managed and unmanaged counts come from GET /api/v1/inventory-information.
Every page of /api/v4/computers-inventory (sections GENERAL, HARDWARE,
OPERATING_SYSTEM, SECURITY) is read, and each managed Mac is checked:
  stale_check_in        general.lastContact older than --stale-days
  never_contacted       no general.lastContact at all
  below_min_os          operatingSystem.version below --min-os (skipped when unset)
  filevault_off         operatingSystem.fileVault2Status NOT_ENCRYPTED or SOME_ENCRYPTED
  firewall_off          security.firewallEnabled false
  mdm_profile_expiring  general.mdmProfileExpiration within --within
A check inventory cannot answer (no OS version, FileVault status or firewall
state) is reported as unknown, never as a pass: such a Mac counts as
incomplete, not compliant. --failing prints one row per Mac that failed or
could not be fully judged.`,
		Example: strings.Trim(`
  jamf-pro-cli fleet posture --stale-days 14 --min-os 14.6 --agent
  jamf-pro-cli fleet posture --failing --tag gold --csv`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet posture")
			}
			if staleDays < 1 {
				return usageErr(fmt.Errorf("--stale-days must be at least 1"))
			}
			if pageSize < 1 || pageSize > 2000 {
				return usageErr(fmt.Errorf("--page-size must be between 1 and 2000"))
			}
			withinDays, err := parseDays("within", within)
			if err != nil {
				return err
			}
			if minOS != "" && fleet.CompareVersions(minOS, "0") <= 0 {
				return usageErr(fmt.Errorf("--min-os %q: give a macOS version like 14.6", minOS))
			}
			instances, err := scope.resolve()
			if err != nil {
				return err
			}
			if len(instances) == 0 && emptyFleet(cmd, flags) {
				return nil
			}
			opts := fleet.PostureOptions{StaleDays: staleDays, MinOS: minOS, WithinDays: withinDays, Now: time.Now().UTC()}
			reqs := []fleet.Request{
				jamfGet("/api/v1/inventory-information"),
				jamfGet("/api/v4/computers-inventory", "section", "GENERAL", "section", "HARDWARE", "section", "OPERATING_SYSTEM",
					"section", "SECURITY", "page-size", strconv.Itoa(pageSize)),
			}
			type result struct {
				row     fleet.PostureRow
				failing []fleet.FailingComputer
			}
			results, failed := collect(cmd, &scope, flags, instances, reqs,
				func(p fleet.Instance, bodies []json.RawMessage) ([]result, error) {
					var info fleet.InventoryInformation
					if err := fleet.DecodeObject(bodies[0], &info, fleet.InventoryInformationKeys...); err != nil {
						return nil, fmt.Errorf("inventory-information: unexpected shape: %w", err)
					}
					comps, err := fleet.Results[fleet.Computer](bodies[1], "id")
					if err != nil {
						return nil, fmt.Errorf("computers-inventory: %w", err)
					}
					row, bad := fleet.Posture(p.Name, info, comps, opts)
					return []result{{row: row, failing: bad}}, nil
				},
				func(p fleet.Instance, err error) result {
					return result{row: fleet.PostureRow{Instance: p.Name, Error: err.Error()}}
				})
			if failing {
				var rows []fleet.FailingComputer
				for _, r := range results {
					if r.row.Error != "" {
						rows = append(rows, fleet.FailingComputer{Instance: r.row.Instance, Error: r.row.Error})
						continue
					}
					// Unevaluated managed Macs are not a pass: say so per instance.
					if r.row.Coverage != "" {
						rows = append(rows, fleet.FailingComputer{Instance: r.row.Instance, Error: "coverage " + r.row.Coverage})
					}
					rows = append(rows, r.failing...)
				}
				if err := render(cmd, flags, rows, []string{"instance", "failed_checks", "unknown_checks", "error"}, "Every evaluated Mac passed every check.",
					[]string{"instance", "name", "serial", "last_contact", "os_version", "failed_checks", "unknown_checks", "error"}, func(r fleet.FailingComputer) map[string]any {
						return map[string]any{"instance": r.Instance, "name": r.Name, "serial": r.Serial, "last_contact": r.LastContact,
							"os_version": r.OSVersion, "failed_checks": r.FailedChecks, "unknown_checks": r.UnknownChecks, "error": r.Error}
					}); err != nil {
					return err
				}
				return allFailed(failed, len(instances))
			}
			// An instance that could not be read has no counts: its row holds
			// only the instance and the error, so it never reads as compliant.
			rows := make([]any, 0, len(results))
			for _, r := range results {
				if r.row.Error != "" {
					rows = append(rows, map[string]any{"instance": r.row.Instance, "error": r.row.Error})
					continue
				}
				rows = append(rows, r.row)
			}
			cols := []string{"instance", "evaluated", "compliant_pct", "compliant", "failing", "incomplete", "stale", "never_contacted", "below_os",
				"filevault_off", "firewall_off", "mdm_expiring", "coverage", "error"}
			if err := render(cmd, flags, rows, []string{"instance", "coverage", "error"}, "No instances.", cols, func(row any) map[string]any {
				if m, ok := row.(map[string]any); ok {
					return m
				}
				r := row.(fleet.PostureRow)
				pct := ""
				if r.PercentCompliant != nil {
					pct = strconv.Itoa(*r.PercentCompliant) + "%"
				}
				return map[string]any{"instance": r.Instance, "evaluated": r.Evaluated, "compliant_pct": pct, "compliant": r.Compliant,
					"failing": r.Failing, "incomplete": r.Incomplete, "stale": r.Stale, "never_contacted": r.NeverContacted, "below_os": r.BelowMinOS, "filevault_off": r.FileVaultOff,
					"firewall_off": r.FirewallOff, "mdm_expiring": r.MDMProfileExpiring, "coverage": r.Coverage, "error": r.Error}
			}); err != nil {
				return err
			}
			return allFailed(failed, len(instances))
		},
	}
	cmd.Flags().IntVar(&staleDays, "stale-days", 14, "Days without contact before a Mac counts as stale")
	cmd.Flags().StringVar(&minOS, "min-os", "", "Lowest acceptable macOS version, e.g. 14.6 (unset skips the check)")
	cmd.Flags().StringVar(&within, "within", "30d", "Flag MDM profiles expiring within this window (e.g. 30d, 6w)")
	cmd.Flags().BoolVar(&failing, "failing", false, "Print one row per failing Mac instead of the per-instance rollup")
	cmd.Flags().IntVar(&pageSize, "page-size", 100, "Computers per inventory page (every page is read; the vendor default is 100)")
	addFleetScopeFlags(cmd, &scope)
	return cmd
}
