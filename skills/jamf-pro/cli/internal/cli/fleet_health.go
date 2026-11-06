// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet health reports service-plane risk per client instance.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"jamf-pro-pp-cli/internal/fleet"
)

func newNovelFleetHealthCmd(flags *rootFlags) *cobra.Command {
	var within, failOn string
	var scope fleetScope

	cmd := &cobra.Command{
		Use:   "health",
		Short: "ADE and VPP token expiry, failed enrollment syncs, health check and Jamf Pro version spread for every client",
		Long: `ADE and VPP token expiry, failed enrollment syncs, health check and Jamf Pro version spread for every client.

Use this command for service-plane risk across clients: ADE and VPP token expiry, failed enrollment syncs, VPP tokens in use by another server, health-check status, and Jamf Pro version spread. Do NOT use this command to debug one registration's credentials; use 'instance test' instead. Do NOT use it for device-level MDM profile expiry; use 'fleet posture' instead.

Rows per instance:
  version       GET /api/v1/jamf-pro-version, compared with the newest in the fleet
  health_check  GET /api/v1/health-check (204 = ok)
  ade_token     tokenExpirationDate of each /api/v1/device-enrollments instance
  ade_sync      syncState of each instance's /syncs/latest
  vpp_token     tokenExpiration of each /api/v1/volume-purchasing-locations entry
                (clientContextMismatch true = the token is in use by another server)
Status is ok, warn (inside --within, or no data) or fail (expired, failed).
APNs push certificate expiry is not exposed by the endpoints this CLI ships.`,
		Example: strings.Trim(`
  jamf-pro-cli fleet health --within 30d --agent
  jamf-pro-cli fleet health --fail-on warn --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:typed-exit-codes": "0,8"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet health")
			}
			withinDays, err := parseDays("within", within)
			if err != nil {
				return err
			}
			switch failOn {
			case "", fleet.StatusWarn, fleet.StatusFail:
			default:
				return usageErr(fmt.Errorf("--fail-on %q: use warn or fail", failOn))
			}
			instances, err := scope.resolve()
			if err != nil {
				return err
			}
			if len(instances) == 0 && emptyFleet(cmd, flags) {
				return nil
			}
			now := time.Now().UTC()
			rows, failed := collectFn(cmd, &scope, flags, instances,
				func(fetch fetchFunc, p fleet.Instance) ([]fleet.HealthRow, error) {
					bodies, err := fetch(
						jamfGet("/api/v1/jamf-pro-version"),
						jamfGet("/api/v1/device-enrollments", "page-size", "100"),
						jamfGet("/api/v1/volume-purchasing-locations", "page-size", "100"),
					)
					if err != nil {
						return nil, err
					}
					var ver struct {
						Version string `json:"version"`
					}
					if err := fleet.DecodeObject(bodies[0], &ver, "version"); err != nil {
						return nil, fmt.Errorf("jamf-pro-version: unexpected shape: %w", err)
					}
					enrollments, err := fleet.Results[fleet.DeviceEnrollment](bodies[1], "id")
					if err != nil {
						return nil, fmt.Errorf("device-enrollments: %w", err)
					}
					vpp, err := fleet.Results[fleet.VPPLocation](bodies[2], "id")
					if err != nil {
						return nil, fmt.Errorf("volume-purchasing-locations: %w", err)
					}
					out := []fleet.HealthRow{{Instance: p.Name, Item: "version", Name: ver.Version, Status: fleet.StatusOK}}
					// The health check and each sync read run on their own so one
					// failing item becomes a row instead of failing the instance.
					hc := fleet.HealthRow{Instance: p.Name, Item: "health_check", Name: "/api/v1/health-check", Status: fleet.StatusOK, Detail: "working"}
					if _, err := fetch(jamfGet("/api/v1/health-check")); err != nil {
						hc.Status, hc.Detail = fleet.StatusFail, err.Error()
					}
					out = append(out, hc)
					for _, e := range enrollments {
						out = append(out, fleet.ExpiryRow(p.Name, "ade_token", e.Name, e.TokenExpirationDate, withinDays, now))
						sb, err := fetch(jamfGet("/api/v1/device-enrollments/" + e.ID + "/syncs/latest"))
						if err != nil {
							out = append(out, fleet.HealthRow{Instance: p.Name, Item: "ade_sync", Name: e.Name, Status: fleet.StatusWarn, Detail: err.Error()})
							continue
						}
						var s fleet.EnrollmentSync
						if err := json.Unmarshal(sb[0], &s); err != nil {
							out = append(out, fleet.HealthRow{Instance: p.Name, Item: "ade_sync", Name: e.Name, Status: fleet.StatusWarn, Detail: "unexpected sync response"})
							continue
						}
						out = append(out, fleet.SyncRow(p.Name, e.Name, s))
					}
					for _, l := range vpp {
						r := fleet.ExpiryRow(p.Name, "vpp_token", l.Name, l.TokenExpiration, withinDays, now)
						switch {
						case l.ClientContextMismatch == nil:
							// Unknown ownership is never ok.
							if r.Status == fleet.StatusOK {
								r.Status = fleet.StatusWarn
							}
							r.Detail = "token ownership unknown (clientContextMismatch not reported); " + r.Detail
						case *l.ClientContextMismatch:
							r.Status, r.Detail = fleet.StatusFail, "token is in use by another server (clientContextMismatch); "+r.Detail
						}
						out = append(out, r)
					}
					return out, nil
				},
				func(p fleet.Instance, err error) fleet.HealthRow {
					return fleet.HealthRow{Instance: p.Name, Item: "instance", Status: fleet.StatusFail, Error: err.Error()}
				})
			fleet.MarkVersionDrift(rows)
			if err := render(cmd, flags, rows, []string{"instance", "item", "status", "error"}, "No instances.",
				[]string{"instance", "item", "name", "status", "days_left", "detail", "error"}, func(r fleet.HealthRow) map[string]any {
					days := ""
					if r.DaysLeft != nil {
						days = fmt.Sprint(*r.DaysLeft)
					}
					return map[string]any{"instance": r.Instance, "item": r.Item, "name": r.Name, "status": r.Status, "days_left": days, "detail": r.Detail, "error": r.Error}
				}); err != nil {
				return err
			}
			if err := allFailed(failed, len(instances)); err != nil {
				return err
			}
			if failOn != "" {
				n := 0
				for _, r := range rows {
					if r.Status == fleet.StatusFail || (failOn == fleet.StatusWarn && r.Status == fleet.StatusWarn) {
						n++
					}
				}
				if n > 0 {
					return fleetProblemsErr("%d health row(s) at or above %s", n, failOn)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&within, "within", "30d", "Warn when a token expires within this window (e.g. 30d, 6w)")
	cmd.Flags().StringVar(&failOn, "fail-on", "", "Exit 8 when any row is at this level or worse: warn or fail")
	addFleetScopeFlags(cmd, &scope)
	return cmd
}
