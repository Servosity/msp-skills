// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): `services down`. Recorded in handfixes.json
// (fixed-scripts).
// pp:data-source live
// pp:client-call the API call is a fixed WinRM script run through winsrv.Runner, not the HTTP client.

package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"windows-server-pp-cli/internal/winsrv"
)

func newNovelServicesDownCmd(flags *rootFlags) *cobra.Command {
	var days int
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Stopped automatic services on every server, each paired with the Service Control Manager event that explains the stop.",
		Long: `Lists automatic services that are not running on every configured server
(or the one --server names), each with the newest Service Control Manager
event about it from the System log (7000, 7009, 7011, 7023, 7024, 7031, 7034)
in the last --days days.

Trigger-start services stop by design and are not listed, unless the stop
has a failure exit code. Neither is a delayed automatic service the Service Control Manager has not
started yet while the server has been up less than 10 minutes; health names
those as not started yet. A server that cannot be read is a row with an
error field.`,
		Example: "  windows-server-cli services down --agent\n  windows-server-cli services down --server fs01 --days 3 --agent",
		Args:    cobra.NoArgs,
		Annotations: map[string]string{
			"mcp:read-only": "true", "pp:data-source": "live",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if days < 1 || days > 30 {
				return usageErr(fmt.Errorf("--days must be from 1 to 30"))
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "services down on "+winsrvTarget()+"")
			}
			profiles, credErrs, err := winsrvServers(flags, true)
			if err != nil {
				return err
			}
			runner := newWinsrvRunner(flags)
			now := winsrvNow()
			results := eachServer(cmd.Context(), profiles, credErrs, func(ctx context.Context, p winsrv.Profile) (any, error) {
				services, err := winsrv.Rows(ctx, runner, p, "services", nil, now)
				if err != nil {
					return nil, err
				}
				// The uptime decides only a delayed automatic service that has
				// not started since boot, so the system script is read only
				// then. A failed read leaves the uptime unknown, and an
				// unknown uptime lists the service (fail toward reporting).
				var boot winsrv.Boot
				if winsrv.NeedsBoot(services) {
					if sys, err := winsrv.Rows(ctx, runner, p, "system", nil, now); err == nil {
						boot = winsrv.BootOf(sys)
					}
				}
				var down []winsrv.Row
				for _, s := range services {
					if winsrv.ServiceDown(s, boot) {
						down = append(down, s)
					}
				}
				if len(down) > 0 {
					events, err := winsrv.Rows(ctx, runner, p, "scm_events", map[string]any{"days": days, "max": scmEventCap}, now)
					if err != nil {
						return nil, err
					}
					capped := len(events) >= scmEventCap
					for _, s := range down {
						ev := latestSCMEvent(events, s)
						s["last_scm_event"] = ev
						if ev == nil {
							s["reason"] = quietStopReason(s, days, capped)
						}
					}
				}
				return down, nil
			})
			rows := []winsrv.Row{}
			for _, r := range results {
				if r.err != nil {
					rows = append(rows, errorRow(r.profile.Name, r.err))
					continue
				}
				rows = append(rows, r.value.([]winsrv.Row)...)
			}
			if err := printWinsrv(cmd, flags, rows, "server", "name", "exit_code", "last_scm_event", "reason", "error"); err != nil {
				return err
			}
			return scriptExit(allFailed(results))
		},
	}
	cmd.Flags().IntVar(&days, "days", 7, "How far back to look for the Service Control Manager event (1 to 30)")
	return cmd
}

// scmEventCap is how many Service Control Manager events services down reads,
// newest first.
const scmEventCap = 500

// quietStopReason explains a stop the Service Control Manager logged no error
// for. A read that hit its cap proves nothing about older events, so it never
// says there were none (round-1 review).
func quietStopReason(s winsrv.Row, days int, capped bool) string {
	code, _ := winsrv.Num(s["exit_code"])
	if capped {
		return fmt.Sprintf("exit code %d; no event about it among the newest %d Service Control Manager events of the last %d days, but the read hit its cap, so an older one may exist (try a smaller --days)", int(code), scmEventCap, days)
	}
	if code == 0 {
		return fmt.Sprintf("stopped cleanly (exit code 0) with no Service Control Manager error in the last %d days: stopped by design or by someone, not by a crash", days)
	}
	return fmt.Sprintf("exit code %d, with no Service Control Manager error in the last %d days", int(code), days)
}

// latestSCMEvent returns the newest SCM event about service s. Events name
// the service by its key (the binary-data property) or by display name.
func latestSCMEvent(events []winsrv.Row, s winsrv.Row) winsrv.Row {
	name, display := strings.ToLower(winsrv.Str(s["name"])), strings.ToLower(winsrv.Str(s["display_name"]))
	var best winsrv.Row
	for _, e := range events {
		match := strings.ToLower(winsrv.Str(e["service_key"])) == name
		if !match {
			if params, ok := e["params"].([]any); ok && len(params) > 0 {
				p0 := strings.ToLower(winsrv.Str(params[0]))
				match = p0 == display || p0 == name
			}
		}
		if match && (best == nil || winsrv.Str(e["time_created"]) > winsrv.Str(best["time_created"])) {
			best = e
		}
	}
	if best == nil {
		return nil
	}
	return winsrv.Row{"event_id": best["event_id"], "time_created": best["time_created"], "message": best["message"]}
}
