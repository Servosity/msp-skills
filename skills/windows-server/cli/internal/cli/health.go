// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): `health`. Recorded in handfixes.json (health-events).
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"windows-server-pp-cli/internal/store"
	"windows-server-pp-cli/internal/winsrv"
)

// Event types `health` writes and `events list` returns.
const (
	EventVerdictChanged = "windows_server.health.verdict_changed"
	EventCheckChanged   = "windows_server.health.check_changed"
)

type healthResult struct {
	Server          string         `json:"server"`
	Verdict         string         `json:"verdict"`
	CheckedAt       string         `json:"checked_at"`
	PreviousVerdict string         `json:"previous_verdict,omitempty"`
	Checks          []winsrv.Check `json:"checks"`
	EventCursors    []string       `json:"event_cursors,omitempty"`
	Error           string         `json:"error,omitempty"`
}

func newNovelHealthCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "health",
		Short: "One ok, warn, crit or unknown verdict per server",
		Long: `Runs every health check on every configured server (or the one --server
names) and returns one verdict per server: the worst check, ranked crit,
unknown, warn, ok. Each failing check carries its reason and failing_since,
the first run it was seen failing.

Checks: reboot (pending reboot), disks (free space and volume health),
services (stopped automatic services), system_events (critical and error
System events in 24h), patch_age (newest installed update), pending_updates,
defender, time_sync, certificates (LocalMachine\My with a private key),
scheduled_tasks (non-Microsoft failures) and hyperv. A check that does not
apply (Defender passive, no Hyper-V) is n/a and never moves the verdict.

Every run is recorded in the local store; a verdict or check that changed
since the last run writes an event that 'events list' returns.`,
		Example: "  windows-server-cli health --agent\n  windows-server-cli health --server fs01 --agent --select server,verdict,checks.name,checks.status,checks.reason",
		Args:    cobra.NoArgs,
		// Writes the local store (health runs and events): not read-only.
		// Writes only the local store, so dogfood may run it for real.
		Annotations: map[string]string{"mcp:read-only": "false", "pp:data-source": "live", "pp:live-happy-path": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "health on "+winsrvTarget()+" and record the result")
			}
			profiles, credErrs, err := winsrvServers(flags, true)
			if err != nil {
				return err
			}
			// WINDOWS_SERVER_NO_LOCAL_DATA=1: the verdicts print, nothing is recorded.
			// The store opens first, so a locked store costs no server reads.
			var db *store.Store
			if !localDataDisabled() {
				if db, err = openWinsrvStore(cmd.Context()); err != nil {
					return err
				}
				defer db.Close()
			}
			runner := newWinsrvRunner(flags)
			now := winsrvNow().UTC()
			results := eachServer(cmd.Context(), profiles, credErrs, func(ctx context.Context, p winsrv.Profile) (any, error) {
				g := winsrv.Gather(ctx, runner, p, now)
				return winsrv.Evaluate(g, now), g.Down
			})
			out := make([]healthResult, 0, len(results))
			for _, r := range results {
				res := healthResult{Server: r.profile.Name, CheckedAt: now.Format(time.RFC3339)}
				checks, _ := r.value.([]winsrv.Check)
				if r.err != nil {
					res.Error = r.err.Error()
					if checks == nil { // no credential: the server was not called
						checks = winsrv.Evaluate(winsrv.Unreachable(r.err), now)
					}
				}
				if err := recordHealth(cmd.Context(), db, &res, checks, now); err != nil {
					return err
				}
				out = append(out, res)
			}
			if err := printWinsrv(cmd, flags, out, "server", "verdict", "checks", "error"); err != nil {
				return err
			}
			return scriptExit(allFailed(results))
		},
	}
	return cmd
}

// recordHealth fills res from checks and the previous run, stores the run and
// writes an event for every change since the previous run.
func recordHealth(ctx context.Context, db *store.Store, res *healthResult, checks []winsrv.Check, now time.Time) error {
	var prev store.HealthRun
	var havePrev bool
	var err error
	if db != nil {
		if prev, havePrev, err = db.LastHealthRun(ctx, res.Server); err != nil {
			return err
		}
	}
	prevChecks := map[string]winsrv.Check{}
	if havePrev {
		var list []winsrv.Check
		_ = json.Unmarshal(prev.Checks, &list)
		for _, c := range list {
			prevChecks[c.Name] = c
		}
		res.PreviousVerdict = prev.Verdict
	}
	stamp := now.Format(time.RFC3339)
	var events []store.VendorEvent
	event := func(typ, resType, resID string, data map[string]any) {
		b, _ := json.Marshal(data)
		events = append(events, store.VendorEvent{
			EventID: fmt.Sprintf("health:%s:{run}:%s", res.Server, resID), Type: typ, Source: "poll",
			OccurredAt: stamp, ReceivedAt: stamp, ResourceType: resType, ResourceID: res.Server + "/" + resID,
			Verified: true, Data: b,
		})
	}
	for i := range checks {
		c := &checks[i]
		old, seen := prevChecks[c.Name]
		if winsrv.Failing(c.Status) {
			c.FailingSince = stamp
			if seen && winsrv.Failing(old.Status) && old.FailingSince != "" {
				c.FailingSince = old.FailingSince
			}
		}
		// A first run reports only what is failing, from null, so a server
		// that is already broken when it is added still reaches the feed.
		if havePrev && (!seen || old.Status != c.Status) || !havePrev && winsrv.Failing(c.Status) {
			var from any
			if seen {
				from = old.Status
			}
			event(EventCheckChanged, "check", "check/"+c.Name, map[string]any{
				"server": res.Server, "check": c.Name, "from": from, "to": c.Status, "reason": c.Reason,
			})
		}
	}
	res.Checks = checks
	res.Verdict = winsrv.Verdict(checks)
	switch {
	case havePrev && prev.Verdict != res.Verdict:
		event(EventVerdictChanged, "server", "verdict", map[string]any{"server": res.Server, "from": prev.Verdict, "to": res.Verdict})
	case !havePrev && res.Verdict != winsrv.StatusOK:
		event(EventVerdictChanged, "server", "verdict", map[string]any{"server": res.Server, "from": nil, "to": res.Verdict})
	}
	if db == nil {
		return nil
	}
	raw, err := json.Marshal(checks)
	if err != nil {
		return err
	}
	_, seqs, err := db.RecordHealth(ctx, store.HealthRun{Server: res.Server, RanAt: stamp, Verdict: res.Verdict, Checks: raw}, events)
	if err != nil {
		return err
	}
	for _, s := range seqs {
		res.EventCursors = append(res.EventCursors, fmt.Sprint(s))
	}
	return nil
}
