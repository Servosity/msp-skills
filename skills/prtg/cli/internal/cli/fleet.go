// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: the fleet command group (every registered PRTG core at once).
// pp:data-source live

package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

func newNovelFleetCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "fleet",
		Short: "Query and act on every registered PRTG core at once (one core per client)",
		Long: `Query and act on every registered PRTG core at once, usually one core per client.

Register cores with 'core add'. Reads (alarms, status, handoff, silenced, drift,
exec, sync) run on every core in parallel and merge the rows with a core
column; a core that cannot be read becomes a row with an error. Actions (ack,
pause, resume) preview by default and only send with --yes. Limit any fleet
command with --cores or --tag.`,
		Example: strings.Trim(`
  prtg-cli fleet status --agent
  prtg-cli fleet alarms --status down --unacked --agent
  prtg-cli fleet ack --filter 'name = "Ping"' --message 'TKT-4411 ISP outage' --duration 2h`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:typed-exit-codes": "0,2", "pp:parent-group": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelFleetAckCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetAlarmsCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetDriftCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetExecCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetHandoffCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetPauseCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetResumeCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetSilencedCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetStatusCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetSyncCmd(flags))
	return cmd
}
