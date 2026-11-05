// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet silenced lists what people paused or acknowledged across cores.
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

// silencedFilter asks each core only for objects a person paused or
// acknowledged (documented status tokens); rows are still checked locally.
const silencedFilter = "status = pausedbyuser or status = pausedbyuseruntil or status = acknowledged"

// silencedSources are the typed lists read for every core.
var silencedSources = []struct{ path, kind string }{
	{"/probes", "probe"}, {"/groups", "group"}, {"/devices", "device"}, {"/sensors", "sensor"},
}

func newNovelFleetSilencedCmd(flags *rootFlags) *cobra.Command {
	var olderThan string
	var indefinite bool
	var scope fleetScope

	cmd := &cobra.Command{
		Use:   "silenced",
		Short: "Everything someone paused or acknowledged across all cores, with who, when, until when and why",
		Long: `Everything someone paused or acknowledged across all cores, with who, when, until when and why.

Use this command to find objects someone paused or acknowledged and left that way, with who, when, and until when. Do NOT use it for current unacknowledged alarms; use 'fleet alarms' instead. To end a pause, use 'fleet resume'.

Reads each core's probes, groups, devices and sensors (GET /probes, /groups,
/devices, /sensors, paged, filtered on the core to the paused and acknowledged
states) and lists those in PAUSED_BY_USER,
PAUSED_BY_USER_UNTIL or DOWN_ACKNOWLEDGED state (objects paused only because a
parent is paused are left out). Each row carries status_info: the user, the
time stamp, valid_until and the message. An item with no end time is
indefinite; with --older-than, indefinite items younger than the threshold
are left out and older ones are marked stale.`,
		Example: strings.Trim(`
  prtg-cli fleet silenced --indefinite --older-than 7d --agent
  prtg-cli fleet silenced --cores acme --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet silenced")
			}
			older, err := parseWindow("older-than", olderThan)
			if err != nil {
				return err
			}
			cores, done, err := scope.resolveOrEmpty(cmd, flags)
			if err != nil || done {
				return err
			}
			reqs := make([]fleet.Request, 0, len(silencedSources))
			for _, src := range silencedSources {
				reqs = append(reqs, fleet.Request{Method: fleet.MethodGetAll, Path: src.path,
					Params: map[string]string{"filter": silencedFilter}})
			}
			opts := fleet.SilencedOptions{OlderThan: older, Indefinite: indefinite, Now: time.Now().UTC()}
			rows, failed := collect(cmd, &scope, flags, cores, reqs,
				func(c fleet.Core, bodies []json.RawMessage) ([]fleet.SilencedRow, error) {
					var nodes []fleet.Node
					var unread []string
					for i, b := range bodies {
						body, err := bodyOrErr(b)
						if err != nil {
							unread = append(unread, silencedSources[i].kind+"s: "+err.Error())
							continue
						}
						page, err := fleet.DecodeNodes(body)
						if err != nil {
							return nil, fmt.Errorf("unexpected %s body: %v", silencedSources[i].path, err)
						}
						for _, n := range page {
							n.Type = silencedSources[i].kind
							nodes = append(nodes, n)
						}
					}
					rows := fleet.Silenced(c.Name, nodes, opts)
					if len(unread) > 0 {
						// Say which lists are missing rather than imply they are clean.
						rows = append(rows, fleet.SilencedRow{Core: c.Name, Error: "partial: could not read " + strings.Join(unread, "; ")})
					}
					return rows, nil
				},
				func(c fleet.Core, err error) fleet.SilencedRow {
					return fleet.SilencedRow{Core: c.Name, Error: err.Error()}
				})
			fleet.SortSilenced(rows)
			if err := render(cmd, flags, rows, []string{"core", "type", "name", "status", "by", "since", "until", "age_days", "stale", "message", "error"}, "Nothing is paused or acknowledged by a person on any core in scope."); err != nil {
				return err
			}
			return allFailed(failed, len(cores))
		},
	}
	cmd.Flags().StringVar(&olderThan, "older-than", "", "Mark indefinite items at least this old as stale and hide younger ones, e.g. 7d")
	cmd.Flags().BoolVar(&indefinite, "indefinite", false, "Only items with no end time")
	addFleetScopeFlags(cmd, &scope)
	return cmd
}
