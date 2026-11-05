// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet drift reports PRTG estate hygiene problems across cores.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"prtg-pp-cli/internal/fleet"
)

func newNovelFleetDriftCmd(flags *rootFlags) *cobra.Command {
	var within string
	var scope fleetScope

	cmd := &cobra.Command{
		Use:   "drift",
		Short: "PRTG estate hygiene across cores: old versions, expiring licenses, sensor overuse, disconnected probes",
		Long: `PRTG estate hygiene across cores: outdated versions, expiring licenses or maintenance, sensor overuse, disconnected probes and unreachable servers.

Use this command for PRTG estate hygiene across cores: version skew, license and maintenance expiry, sensor-license overuse, disconnected probes, unreachable application servers. Do NOT use it for sensor alarm counts; use 'fleet status' instead.

Reads GET /health, /version, /experimental/license, /probes and
/sensor-status-summary on every core and compares them across the fleet: a
core whose PRTG version is behind the newest core is flagged, as is a license
or maintenance end date inside --within, more sensors in use than the license
allows, any probe whose probe_status is not CONNECTED (DISCONNECTED and BANNED
are critical, transient states a warning), a rejected API key, and an
application server that is unreachable or license-inactive.
A source the key may not read (for example the license for a non-admin key)
becomes an info finding instead of hiding the rest.`,
		Example: strings.Trim(`
  prtg-cli fleet drift --within 30d --agent
  prtg-cli fleet drift --cores acme --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet drift")
			}
			if strings.TrimSpace(within) == "" {
				within = "30d"
			}
			win, err := parseWindow("within", within)
			if err != nil {
				return err
			}
			cores, done, err := scope.resolveOrEmpty(cmd, flags)
			if err != nil || done {
				return err
			}
			reqs := []fleet.Request{
				{Path: "/health"},
				{Path: "/version"},
				{Path: "/experimental/license"},
				{Method: fleet.MethodGetAll, Path: "/probes"},
				{Path: "/sensor-status-summary"},
			}
			facts, failed := collect(cmd, &scope, flags, cores, reqs,
				func(c fleet.Core, bodies []json.RawMessage) ([]fleet.CoreFacts, error) {
					f := fleet.CoreFacts{Core: c.Name, Health: fleet.HealthState(bodies[0]), Errors: map[string]string{}}
					for _, b := range bodies[1:] {
						if e, ok := fleet.RequestError(b); ok && e.Status == 401 {
							f.AuthFailed = true
						}
					}
					read := func(check string, body json.RawMessage, into func(json.RawMessage) error) {
						if f.AuthFailed {
							return // one auth finding, not three unreadable-source rows
						}
						b, err := bodyOrErr(body)
						if err == nil {
							err = into(b)
						}
						if err != nil {
							f.Errors[check] = err.Error()
						}
					}
					read("version", bodies[1], func(b json.RawMessage) error {
						v := new(fleet.Versions)
						if err := json.Unmarshal(b, v); err != nil {
							return fmt.Errorf("unexpected /version body: %w", err)
						}
						f.Version = v
						return nil
					})
					read("license", bodies[2], func(b json.RawMessage) error {
						l := new(fleet.License)
						if err := json.Unmarshal(b, l); err != nil {
							return fmt.Errorf("unexpected /experimental/license body: %w", err)
						}
						f.License = l
						return nil
					})
					read("sensor_usage", bodies[4], func(b json.RawMessage) error {
						var sum fleet.SensorSummary
						if err := json.Unmarshal(b, &sum); err != nil {
							return fmt.Errorf("unexpected /sensor-status-summary body: %w", err)
						}
						f.SensorsInUse = &sum.Total
						f.PausedByLicense = &sum.PausedByLicense
						return nil
					})
					read("probes", bodies[3], func(b json.RawMessage) error {
						nodes, err := fleet.DecodeNodes(b)
						if err != nil {
							return fmt.Errorf("unexpected /probes body: %w", err)
						}
						f.Probes = nodes
						return nil
					})
					return []fleet.CoreFacts{f}, nil
				},
				func(c fleet.Core, err error) fleet.CoreFacts {
					return fleet.CoreFacts{Core: c.Name, Unread: true, ReadErr: err.Error()}
				})
			findings := fleet.Drift(facts, fleet.DriftOptions{Within: win, Now: time.Now().UTC()})
			// A core whose key was rejected answered only the public /health.
			for _, f := range facts {
				if f.AuthFailed && !f.Unread {
					failed++
				}
			}
			if err := render(cmd, flags, findings, []string{"core", "severity", "check", "object", "detail"}, "No hygiene findings on any core in scope."); err != nil {
				return err
			}
			return allFailed(failed, len(cores))
		},
	}
	cmd.Flags().StringVar(&within, "within", "30d", "Flag license and maintenance end dates inside this window, e.g. 30d, 8w")
	addFleetScopeFlags(cmd, &scope)
	return cmd
}
