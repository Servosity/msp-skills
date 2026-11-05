// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: devices unassign.
// pp:data-source live

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelDevicesUnassignCmd(flags *rootFlags) *cobra.Command {
	var file string
	var o activityOpts
	cmd := &cobra.Command{
		Use:   "unassign [serial...]",
		Short: "Remove serials from whatever MDM server they are on now, grouped per server, with wait and verify",
		Long: `Use this command to remove serials from whatever MDM server they are on now. Do NOT use this command to move serials to a different server; use 'devices assign' instead.

Each serial's current device management service is read first, serials are
grouped per server, and one UNASSIGN_DEVICES activity is sent per server.
Serials not assigned anywhere are reported as skipped (ok). A serial whose
current server cannot be read, or a server group whose request fails, is
reported and makes the command exit non-zero after the other groups run.
With --wait, exits non-zero when any serial is still assigned.`,
		Example: strings.Trim(`
  apple-business-cli devices unassign C02XL0ABJGH5 --wait --dry-run
  apple-business-cli devices unassign --file returns.csv --wait --json`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "pp:happy-args": "serial=XABC123X0ABC123X0"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if err := o.parse(); err != nil {
				return err
			}
			if dryRunOK(flags) {
				serials, err := readSerials(args, existingFile(file), cmd.InOrStdin())
				if err != nil && !requiredInputWaived() {
					return err
				}
				body := activityBody("UNASSIGN_DEVICES", "<each serial's current server>", serials, "")
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"dry_run": true, "action": "devices unassign", "would": "POST /orgDeviceActivities once per current server", "serials": len(serials), "body": body}, flags)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "dry-run: would look up the current server of %d serial(s) and POST one UNASSIGN_DEVICES activity per server\n", len(serials))
				return nil
			}
			serials, err := readSerials(args, file, cmd.InOrStdin())
			if err != nil {
				return err
			}
			if len(serials) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("no serials: pass them as arguments or --file"))
			}
			ctx := cmd.Context()
			c, _, err := frozenClient(flags) // pins the store the device IDs come from
			if err != nil {
				return err
			}
			c.NoCache = true // a write resolves servers and assignments from live reads only
			devIDs := loadDeviceIDs(ctx, storeDBPath(""))
			out := assignOutput{Activities: []activityReport{}}
			byServer := map[string][]string{}
			for _, s := range serials {
				id, err := assignedServerID(ctx, c, devIDs.of(s))
				switch {
				case err != nil:
					out.Skipped = append(out.Skipped, serialVerdict{Serial: s, Error: "could not read current server: " + err.Error()})
				case id == "":
					out.Skipped = append(out.Skipped, serialVerdict{Serial: s, OK: true, Error: "not assigned to any device management service"})
				default:
					byServer[id] = append(byServer[id], s)
				}
			}
			ids := make([]string, 0, len(byServer))
			for id := range byServer {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			var firstErr error
			for _, id := range ids {
				rep := activityReport{ActivityType: "UNASSIGN_DEVICES", ServerID: id, Serials: byServer[id]}
				if err := runActivity(ctx, c, devIDs, &rep, "", o, func(got string) bool { return got == "" }); err != nil {
					rep.Error = err.Error()
					if firstErr == nil {
						firstErr = err
					}
				}
				out.Activities = append(out.Activities, rep)
			}
			return emitActivityOutput(cmd, flags, out, firstErr)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "File of serials (one per line, first CSV column, or this CLI's JSON); - reads stdin")
	cmd.Flags().BoolVar(&o.wait, "wait", false, "Poll the activity until it finishes, then re-read each serial's assignment and report ok or mismatch")
	cmd.Flags().StringVar(&o.waitTimeout, "wait-timeout", "10m", "Give up waiting after this long (e.g. 10m, 1h)")
	cmd.Flags().StringVar(&o.poll, "poll", "5s", "Interval between activity status checks (minimum 1s)")
	return cmd
}
