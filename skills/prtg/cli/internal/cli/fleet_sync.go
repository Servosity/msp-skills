// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet sync refreshes every core's local mirror in one run.
// pp:data-source live

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"prtg-pp-cli/internal/cliutil"
	"prtg-pp-cli/internal/fleet"
)

// SyncRow is one core's sync result.
type SyncRow struct {
	Core         string   `json:"core"`
	Records      *int     `json:"records,omitempty"`
	Resources    *int     `json:"resources,omitempty"`
	Succeeded    *int     `json:"succeeded,omitempty"`
	Warned       *int     `json:"warned,omitempty"`
	Errored      *int     `json:"errored,omitempty"`
	DurationSecs string   `json:"duration_seconds,omitempty"`
	SyncErrors   []string `json:"sync_errors,omitempty"`
	Error        string   `json:"error,omitempty"`
}

func newNovelFleetSyncCmd(flags *rootFlags) *cobra.Command {
	var resources []string
	var full bool
	var scope fleetScope

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Refresh the offline mirror of every registered core in one run, each into its own isolated store",
		Long: `Refresh the offline mirror of every registered core in one run, each into its own isolated store.

Use this command to refresh the local mirror of every registered core in one run. Do NOT use it to refresh a single core; use 'sync' instead (with --core <name>).

Runs the sync command once per core with that core's key and home, with
bounded concurrency, and reports records, resources and errors per core. One
unreachable core does not stop the rest. Afterwards read a core's mirror with
'--core <name> search', 'analytics' or '--data-source local'.`,
		Example: strings.Trim(`
  prtg-cli fleet sync --resources sensors,devices --agent
  prtg-cli fleet sync --full --cores acme,globex --json`, "\n"),
		Annotations: map[string]string{
			"pp:data-source": "live",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet sync")
			}
			for _, r := range resources {
				if strings.ContainsAny(r, " \t=") || strings.HasPrefix(r, "-") {
					return usageErr(fmt.Errorf("--resources %q: use resource names like sensors,devices", r))
				}
			}
			cores, done, err := scope.resolveOrEmpty(cmd, flags)
			if err != nil || done {
				return err
			}
			args = []string{"sync"}
			if len(resources) > 0 {
				args = append(args, "--resources", strings.Join(resources, ","))
			}
			if full {
				args = append(args, "--full")
			}
			if cliutil.IsDogfoodEnv() {
				args = append(args, "--max-pages", "1")
			}
			if flags.timeout > 0 {
				args = append(args, "--timeout", flags.timeout.String())
			}
			r := scope.runner(flags)
			rows := make([]SyncRow, len(cores))
			fleet.Each(cmd.Context(), scope.concurrency, cores, func(ctx context.Context, i int, c fleet.Core) {
				start := time.Now()
				out, err := r.Run(ctx, c, nil, args...)
				rows[i] = syncRowFor(c, out, err, time.Since(start))
			})
			failed := 0
			for _, row := range rows {
				if row.Error != "" {
					failed++
				}
			}
			if failed > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d cores failed to sync; their rows carry the error\n", failed, len(cores))
			}
			if err := render(cmd, flags, rows, []string{"core", "records", "resources", "succeeded", "warned", "errored", "duration_seconds", "error", "sync_errors"}, "No cores in scope."); err != nil {
				return err
			}
			return allFailed(failed, len(cores))
		},
	}
	cmd.Flags().StringSliceVar(&resources, "resources", nil, "Resources to sync on every core, e.g. sensors,devices,groups,probes (default all)")
	cmd.Flags().BoolVar(&full, "full", false, "Full resync on every core (ignore previous checkpoints)")
	addFleetScopeFlags(cmd, &scope)
	return cmd
}

// syncRowFor reads the child's sync summary (the last JSON object on stdout
// that carries total_records) and its sync_error events. A run that stored
// nothing and reported errors is a failed core, whatever its exit code.
func syncRowFor(c fleet.Core, out []byte, err error, took time.Duration) SyncRow {
	row := SyncRow{Core: c.Name, DurationSecs: fmt.Sprintf("%.1f", took.Seconds())}
	var summary struct {
		Total     *int `json:"total_records"`
		Resources *int `json:"resources"`
		Success   *int `json:"success"`
		Warned    *int `json:"warned"`
		Errored   *int `json:"errored"`
	}
	found := false
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var obj map[string]json.RawMessage
		if err := dec.Decode(&obj); err != nil {
			break
		}
		if string(obj["event"]) == `"sync_error"` {
			var msg string
			if json.Unmarshal(obj["error"], &msg) == nil && len(row.SyncErrors) < 5 {
				row.SyncErrors = append(row.SyncErrors, msg)
			}
			continue
		}
		if _, ok := obj["total_records"]; !ok {
			continue
		}
		data, _ := json.Marshal(obj)
		if json.Unmarshal(data, &summary) == nil {
			found = true
		}
	}
	if err != nil {
		// The child failed: its first sync_error names the cause (a TLS or
		// auth failure) better than the exit summary does.
		row.Error = err.Error()
		if len(row.SyncErrors) > 0 {
			row.Error = row.SyncErrors[0]
		}
		return row
	}
	if !found {
		row.Error = "sync finished without a summary"
		return row
	}
	row.Records, row.Resources, row.Succeeded, row.Warned, row.Errored = summary.Total, summary.Resources, summary.Success, summary.Warned, summary.Errored
	if len(row.SyncErrors) > 0 && (summary.Total == nil || *summary.Total == 0) {
		row.Error = row.SyncErrors[0]
	}
	return row
}
