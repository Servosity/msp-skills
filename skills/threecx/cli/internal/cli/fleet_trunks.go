// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet trunks lists every SIP trunk on every PBX.
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

// trunkSelect asks the PBX for status fields only, so trunk credentials
// (AuthID, AuthPassword) never cross the wire.
const trunkSelect = "Id,Number,IsOnline,ConfigurationIssue,Direction,SimultaneousCalls,CertificateExpirationDate,Gateway"

func newNovelFleetTrunksCmd(flags *rootFlags) *cobra.Command {
	var scope fleetScope
	var offline, failOnOffline, noSave bool
	var certWithin string

	cmd := &cobra.Command{
		Use:   "trunks",
		Short: "List every SIP trunk on every client phone system with online state, certificate days left and configuration issues (--offline for problems only)",
		Long: `Every SIP trunk on every client phone system with online state, certificate days
left and configuration issues, offline trunks first.

Use this command to list individual SIP trunks on every PBX with online state, certificate expiry and configuration issues. Do NOT use this command for the one-row-per-PBX pass/warn/fail summary; use 'fleet health' instead. Do NOT use it for what went offline since the last run; use 'fleet changes' instead.

Each run is saved locally for 'fleet changes' (skip with --no-save). With
--fail-on-offline the command exits 8 when any trunk is offline or misconfigured.`,
		Example: strings.Trim(`
  threecx-cli fleet trunks --offline --agent
  threecx-cli fleet trunks --cert-within 30d --json
  threecx-cli fleet trunks --fail-on-offline`, "\n"),
		Annotations: map[string]string{"mcp:local-write": "true", "pp:data-source": "live", "pp:typed-exit-codes": "0,8"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet trunks")
			}
			certDays, err := optDays("cert-within", certWithin)
			if err != nil {
				return err
			}
			pbxes, full, err := scope.resolve()
			if err != nil {
				return err
			}
			if len(pbxes) == 0 && emptyFleet(cmd, flags) {
				return nil
			}
			now := time.Now().UTC()
			all, failed := collect(cmd, &scope, flags, pbxes, []fleet.Request{{Path: "/Trunks", Params: map[string]string{"$select": trunkSelect}}},
				func(p fleet.PBX, bodies []json.RawMessage) ([]fleet.TrunkRow, error) {
					var trunks []fleet.Trunk
					if err := decodeInto(bodies, &trunks); err != nil {
						return nil, fmt.Errorf("decoding /Trunks: %w", err)
					}
					return fleet.TrunkRows(p, trunks, now), nil
				}, fleet.ErrorTrunkRow)
			fleet.SortTrunks(all)
			saveRun(cmd, noSave, fleet.Snapshot{RunAt: now, Kind: fleet.KindTrunks, Scope: fleet.NamesOf(pbxes), Full: full, Trunks: all})
			rows := make([]fleet.TrunkRow, 0, len(all))
			down := 0
			for _, r := range all {
				switch r.State {
				case "offline", "issue", "unknown":
					down++
				}
				switch {
				case offline && r.State == "online":
					continue
				case certDays >= 0 && r.Error == "" && (r.CertDaysLeft == nil || *r.CertDaysLeft > certDays):
					continue
				}
				rows = append(rows, r)
			}
			err = render(cmd, flags, rows, []string{"pbx", "state", "trunk", "name", "online",
				"configuration_issue", "cert_days_left", "cert_expires", "error"}, "No matching trunks.",
				func(r fleet.TrunkRow) map[string]any {
					state := r.State
					if state != "online" {
						state = red(state)
					}
					return map[string]any{"pbx": r.PBX, "trunk": r.Trunk, "name": r.Name, "state": state,
						"issue": fleet.FirstNonEmpty(r.ConfigurationIssue, r.Error), "cert_days": intOrDash(r.CertDaysLeft)}
				})
			if err != nil {
				return err
			}
			if err := allFailed(failed, len(pbxes)); err != nil {
				return err
			}
			if failOnOffline && down > 0 {
				return fleetProblemsErr("%d trunk(s) offline, misconfigured or in an unknown state", down)
			}
			return nil
		},
	}
	addFleetScopeFlags(cmd, &scope)
	cmd.Flags().BoolVar(&offline, "offline", false, "Only trunks that are offline, misconfigured or in an unknown state (plus unreadable PBXes)")
	cmd.Flags().StringVar(&certWithin, "cert-within", "", "Only trunks whose TLS certificate expires within this many days (e.g. 30d)")
	cmd.Flags().BoolVar(&failOnOffline, "fail-on-offline", false, "Exit 8 when any trunk is offline, misconfigured or in an unknown state")
	cmd.Flags().BoolVar(&noSave, "no-save", false, "Do not save this run to the local history used by 'fleet changes'")
	return cmd
}
