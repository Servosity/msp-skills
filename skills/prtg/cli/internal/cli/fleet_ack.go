// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet ack acknowledges matching alarms on every core after a preview.
// pp:data-source live

package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

func newNovelFleetAckCmd(flags *rootFlags) *cobra.Command {
	var filter, message, duration string
	var scope fleetScope

	cmd := &cobra.Command{
		Use:   "ack",
		Short: "Acknowledge matching alarms on every core at once with a ticket message, after a per-core preview",
		Long: `Acknowledge matching alarms on every core at once with a ticket message, after a per-core preview.

Use this command to acknowledge down alarms across cores so they stop alerting while monitoring continues. Do NOT use this command to stop monitoring for a maintenance window; use 'fleet pause' instead.

Your filter is narrowed to down sensors: (<filter>) and (status = down or
status = partialdown), so already-acknowledged alarms keep their owner's
acknowledgement. Without --yes it only previews: each core lists how many
sensors that scoped filter matches (GET /experimental/sensors). With --yes it
sends POST /sensors/acknowledge with the same scoped filter, message and
duration to every core with a match, and reports per core how many alarms
were acknowledged and which failed; any failure exits non-zero. Each core's
preview is cross-checked: a core that returns sensors outside the down scope
is not acted on. The API key needs the Acknowledge access level. --duration 0 (the
default) acknowledges indefinitely; PRTG rounds up to whole minutes.`,
		Example: strings.Trim(`
  prtg-cli fleet ack --filter 'name = "Ping"' --message 'TKT-4411 ISP outage' --duration 2h
  prtg-cli fleet ack --filter 'basic.tags contains "wan"' --message 'TKT-4411' --cores acme,globex --yes`, "\n"),
		Annotations: map[string]string{
			"prtg:sends-with-yes":   "true", // MCP: previews until confirm (cobratree/prtg_confirm.go)
			"pp:data-source":        "live",
			"pp:preview-happy-path": "true",
			"pp:happy-args":         "--filter=name = \"Ping\";--message=TKT-4411",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet ack")
			}
			secs, err := durationSeconds(duration)
			if err != nil {
				return err
			}
			a := fleetAction{name: "fleet ack", verb: "acknowledge", kind: "sensors", filter: filter,
				body: map[string]any{"message": message, "duration": secs}}
			return a.run(cmd, flags, &scope)
		},
	}
	cmd.Flags().StringVar(&filter, "filter", "", "PRTG filter selecting the sensors to acknowledge, e.g. 'name = \"Ping\"' (required)")
	cmd.Flags().StringVar(&message, "message", "", "Acknowledgement message shown on each sensor, e.g. a ticket number")
	cmd.Flags().StringVar(&duration, "duration", "0", "How long the acknowledgement lasts, e.g. 30m, 2h (0 = indefinite)")
	addFleetScopeFlags(cmd, &scope)
	return cmd
}
