// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet resume ends a maintenance pause on every core, optionally scanning.
// pp:data-source live

package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

func newNovelFleetResumeCmd(flags *rootFlags) *cobra.Command {
	var kind, filter string
	var scan, includeIndefinite bool
	var scope fleetScope

	cmd := &cobra.Command{
		Use:   "resume",
		Short: "Resume matching objects on every core after maintenance and optionally trigger an immediate scan",
		Long: `Resume matching objects on every core after maintenance and optionally trigger an immediate scan.

Use 'fleet pause' to stop monitoring matching objects across cores for a maintenance window and 'fleet resume' to end it. Do NOT use these to acknowledge an alarm that is already down; use 'fleet ack' instead. Do NOT use 'fleet resume' to find what is paused; use 'fleet silenced' instead.

Your filter is narrowed to time-limited pauses: (<filter>) and (status =
pausedbyuseruntil), so an object someone paused indefinitely (for example a
sensor waiting for decommission) is left alone; --include-indefinite also
resumes pauses with no end time. Each core's preview is cross-checked: a core
that returns objects outside that scope is not acted on. Without --yes it only
previews the matches per core. With --yes it sends POST /<kind>s/resume with
the scoped filter to every core with a match, and with --scan also scans now
each object PRTG reports as resumed, so recovery is confirmed right away. The API key needs the Write access level. Any failed core or object exits
non-zero after the rows print.`,
		Example: strings.Trim(`
  prtg-cli fleet resume --kind device --filter 'name = "fw01"' --scan
  prtg-cli fleet resume --kind group --filter 'name = "Branch Office"' --scan --yes`, "\n"),
		Annotations: map[string]string{
			"prtg:sends-with-yes":   "true", // MCP: previews until confirm (cobratree/prtg_confirm.go)
			"pp:data-source":        "live",
			"pp:preview-happy-path": "true",
			"pp:happy-args":         "--kind=device;--filter=name = \"fw01\";--scan",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet resume")
			}
			k, err := parseKind(kind)
			if err != nil {
				return usageErr(err)
			}
			a := fleetAction{name: "fleet resume", verb: "resume", kind: k, filter: filter, scan: scan,
				includeIndefinite: includeIndefinite}
			return a.run(cmd, flags, &scope)
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "device", "Object kind to resume: device, group, sensor or probe")
	cmd.Flags().StringVar(&filter, "filter", "", "PRTG filter selecting the objects, e.g. 'name = \"fw01\"' (required)")
	cmd.Flags().BoolVar(&scan, "scan", false, "Also trigger scan now for each object that was resumed")
	cmd.Flags().BoolVar(&includeIndefinite, "include-indefinite", false, "Also resume objects paused with no end time (by default only time-limited pauses are resumed)")
	addFleetScopeFlags(cmd, &scope)
	return cmd
}
