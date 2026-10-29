// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet licences is the renewals pipeline across every PBX.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"threecx-pp-cli/internal/fleet"
)

func newNovelFleetLicencesCmd(flags *rootFlags) *cobra.Command {
	var scope fleetScope
	var within, reseller string

	cmd := &cobra.Command{
		Use:     "licences",
		Aliases: []string{"licenses"},
		Short:   "List licence and maintenance expiry, edition, seat headroom and reseller for every client phone system, soonest first (--within 60d)",
		Long: `Licence and maintenance expiry, edition, seat headroom and reseller of record for
every client phone system, soonest expiry first. The licence key itself is never
printed.

Use this command for licence and maintenance expiry dates, edition, seat headroom and reseller of record per PBX. Do NOT use this command to find PBXes behind on 3CX builds or blocked from updating; use 'fleet versions' instead.

--within keeps only PBXes whose licence or maintenance expires inside the window
(already-expired ones included), plus any PBX not known to be outside it: an
inactive licence, or an activation or expiry it did not report (named in
"unknown"). --reseller flags PBXes whose reseller of record differs from the
name you give.`,
		Example: strings.Trim(`
  threecx-cli fleet licences --within 60d --csv
  threecx-cli fleet licences --reseller "Acme MSP" --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet licences")
			}
			windowDays, err := optDays("within", within)
			if err != nil {
				return err
			}
			pbxes, _, err := scope.resolve()
			if err != nil {
				return err
			}
			if len(pbxes) == 0 && emptyFleet(cmd, flags) {
				return nil
			}
			now := time.Now().UTC()
			rows, failed := collect(cmd, &scope, flags, pbxes, []fleet.Request{{Path: "/LicenseStatus"}, {Path: "/SystemStatus"}},
				func(p fleet.PBX, bodies []json.RawMessage) ([]fleet.LicenceRow, error) {
					var l fleet.LicenseStatus
					var s fleet.SystemStatus
					if err := decodeInto(bodies, &l, &s); err != nil {
						return nil, fmt.Errorf("decoding licence status: %w", err)
					}
					return []fleet.LicenceRow{fleet.LicenceRowFor(p, l, s, reseller, now)}, nil
				}, func(p fleet.PBX, err error) fleet.LicenceRow {
					return fleet.LicenceRow{PBX: p.Name, Error: err.Error()}
				})
			fleet.SortLicences(rows)
			if windowDays >= 0 {
				kept := make([]fleet.LicenceRow, 0, len(rows))
				for _, r := range rows {
					// Dropped only when known to need no renewal in the
					// window: every input reported, active, both expiries
					// beyond it (fleet.LicenceRow.OutsideWindow).
					if !r.OutsideWindow(windowDays) {
						kept = append(kept, r)
					}
				}
				rows = kept
			}
			err = render(cmd, flags, rows, []string{"pbx", "edition", "license_expires", "license_days_left",
				"maintenance_expires", "maintenance_days_left", "seat_headroom", "reseller", "reseller_mismatch", "unknown", "error"},
				"Nothing expiring in that window.", func(r fleet.LicenceRow) map[string]any {
					return map[string]any{"pbx": r.PBX, "edition": r.Edition,
						"licence_expires": r.LicenseExpires, "licence_days": intOrDash(r.LicenseDaysLeft),
						"maintenance_days": intOrDash(r.MaintenanceDaysLeft), "seat_headroom": intOrDash(r.SeatHeadroom),
						"reseller": r.Reseller, "reseller_mismatch": r.ResellerMismatch,
						"unknown": strings.Join(r.Unknown, ","), "error": r.Error}
				})
			if err != nil {
				return err
			}
			return allFailed(failed, len(pbxes))
		},
	}
	addFleetScopeFlags(cmd, &scope)
	cmd.Flags().StringVar(&within, "within", "", "Only PBXes whose licence or maintenance expires within this many days (e.g. 60d)")
	cmd.Flags().StringVar(&reseller, "reseller", "", "Flag PBXes whose reseller of record is not this name")
	return cmd
}
