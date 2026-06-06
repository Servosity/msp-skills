// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local

package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"microsoft-graph-pp-cli/internal/insights"
)

func newNovelManagedDevicesDriftCmd(flags *rootFlags) *cobra.Command {
	var flagDays int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "drift",
		Short: "Flag Intune devices that are non-compliant, unencrypted, or stale",
		Long: strings.Trim(`
Scans the locally synced Intune managed devices for compliance drift: devices
that are non-compliant, unencrypted, or have not checked in within the staleness
window, each attributed to its assigned user and tagged with every reason it was
flagged — the weekly compliance ticket queue in one command.

Requires managed devices synced: run 'microsoft-graph-cli pull' first.`, "\n"),
		Example: strings.Trim(`
  microsoft-graph-cli managed-devices drift --days 30 --agent
  microsoft-graph-cli managed-devices drift --days 14 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if flagDays < 0 {
				return usageErr(fmt.Errorf("--days must be >= 0, got %s", strconv.Itoa(flagDays)))
			}
			staleBefore := time.Now().UTC().AddDate(0, 0, -flagDays)
			devices, err := loadDomainRows(dbPath, `SELECT data FROM managed_devices`)
			if err != nil {
				return apiErr(fmt.Errorf("reading local store: %w", err))
			}
			result := insights.DeviceDrift(devices, staleBefore)
			hintUnsyncedIfEmpty(cmd, dbPath, len(result) == 0)
			return printJSONFiltered(cmd.OutOrStdout(), result, flags)
		},
	}
	cmd.Flags().IntVar(&flagDays, "days", 30, "Flag devices not synced within this many days as stale")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: ~/.local/share/microsoft-graph-cli/data.db)")
	return cmd
}
