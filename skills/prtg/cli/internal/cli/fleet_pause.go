// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet pause pauses matching objects on every core for maintenance.
// pp:data-source live

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelFleetPauseCmd(flags *rootFlags) *cobra.Command {
	var kind, filter, message, duration string
	var scope fleetScope

	cmd := &cobra.Command{
		Use:   "pause",
		Short: "Pause matching devices, groups, sensors or probes on every core for a maintenance window",
		Long: `Pause matching devices, groups, sensors or probes on every core for a maintenance window, after a per-core preview.

Use 'fleet pause' to stop monitoring matching objects across cores for a maintenance window and 'fleet resume' to end it. Do NOT use these to acknowledge an alarm that is already down; use 'fleet ack' instead. Do NOT use 'fleet resume' to find what is paused; use 'fleet silenced' instead.

Your filter is narrowed to objects nobody has paused already: (<filter>) and
not(status = pausedbyuser or status = pausedbyuseruntil), so an indefinite
pause is never shortened into one that auto-resumes. Without --yes it only
previews: each core lists how many objects of --kind that scoped filter
matches (GET /experimental/<kind>s). With --yes it sends POST
/<kind>s/pause with the filter, message and duration to every core where the
filter matched. The API key needs the Write access level. Any failed core or object exits
non-zero after the rows print. --duration is required; 0 pauses until someone
resumes. PRTG rounds up to whole minutes. Each core's preview is cross-checked:
a core that returns objects outside the scope is not acted on.`,
		Example: strings.Trim(`
  prtg-cli fleet pause --kind device --filter 'name = "fw01"' --duration 2h --message 'Patch night'
  prtg-cli fleet pause --kind group --filter 'name = "Branch Office"' --duration 90m --message 'CHG-1020' --tag gold --yes`, "\n"),
		Annotations: map[string]string{
			"prtg:sends-with-yes":   "true", // MCP: previews until confirm (cobratree/prtg_confirm.go)
			"pp:data-source":        "live",
			"pp:preview-happy-path": "true",
			"pp:happy-args":         "--kind=device;--filter=name = \"fw01\";--duration=2h;--message=Patch night",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet pause")
			}
			k, err := parseKind(kind)
			if err != nil {
				return usageErr(err)
			}
			if strings.TrimSpace(duration) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--duration is required, e.g. 2h for a maintenance window (0 pauses until someone resumes)"))
			}
			secs, err := durationSeconds(duration)
			if err != nil {
				return err
			}
			a := fleetAction{name: "fleet pause", verb: "pause", kind: k, filter: filter,
				body: map[string]any{"message": message, "duration": secs}}
			return a.run(cmd, flags, &scope)
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "device", "Object kind to pause: device, group, sensor or probe")
	cmd.Flags().StringVar(&filter, "filter", "", "PRTG filter selecting the objects, e.g. 'name = \"fw01\"' (required)")
	cmd.Flags().StringVar(&message, "message", "", "Pause message shown on each object, e.g. a change number")
	cmd.Flags().StringVar(&duration, "duration", "", "How long the pause lasts, e.g. 90m, 2h (required; 0 = until someone resumes, which 'fleet resume' then needs --include-indefinite for)")
	addFleetScopeFlags(cmd, &scope)
	return cmd
}
