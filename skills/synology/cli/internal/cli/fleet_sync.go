// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. fleet sync: mirror ABB job status and storage health from
// every registered NAS into the local fleet database.
// pp:data-source live

package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"synology-pp-cli/internal/cliutil"
	"synology-pp-cli/internal/dsm"
	"synology-pp-cli/internal/fleet"
)

type fleetSyncRow struct {
	NAS          string   `json:"nas"`
	OK           bool     `json:"ok"`
	Error        string   `json:"error,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
	ABBInstalled *bool    `json:"abb_installed,omitempty"`
	Tasks        int      `json:"tasks"`
	Results      int      `json:"results"`
	DeviceRuns   int      `json:"device_runs"`
	StorageItems int      `json:"storage_items"`
	DurationMS   int64    `json:"duration_ms"`
}

func newNovelFleetSyncCmd(flags *rootFlags) *cobra.Command {
	var nasName, dbPath, window string
	var full bool
	var concurrency int
	var perNAS time.Duration
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Mirror ABB job status and storage health from every registered NAS into the local fleet database",
		Long: strings.Trim(`
Use this command to refresh the local mirror for every NAS registered with
'fleet add' (or the single NAS configured through SYNOLOGY_BASE_URL). Do NOT
use this command for a one-off resource refresh against the default NAS; use
'sync' instead.

Per NAS it logs in, reads Active Backup for Business tasks, job results since
the last sync (with the first error line of recent failed runs), per-device
transfers and a Storage Manager snapshot, then logs out. One unreachable NAS is
recorded and does not stop the others; the exit code is 5 when any NAS failed.`, "\n"),
		Example: strings.Trim(`
  synology-cli fleet sync
  synology-cli fleet sync --nas acme --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "false", "pp:data-source": "live", "pp:typed-exit-codes": "0,5,10"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet sync")
			}
			initial, err := parseWindow("since", window)
			if err != nil {
				return err
			}
			if perNAS <= 0 {
				return usageErr(fmt.Errorf("--nas-timeout must be positive, got %s", perNAS))
			}
			list, _, _, err := fleetNAS(flags)
			if err != nil {
				return err
			}
			if nasName != "" {
				n, err := resolveNAS(flags, nasName)
				if err != nil {
					return err
				}
				list = []fleet.NAS{n}
			}
			if len(list) == 0 {
				noFleetHint(cmd.ErrOrStderr())
				return configErr(errors.New("no NAS to sync"))
			}
			path, err := fleetDBPath(dbPath)
			if err != nil {
				return err
			}
			// Each NAS gets its own --nas-timeout budget; an explicit --timeout
			// also bounds the whole run. (The default --timeout is per request.)
			ctx, cancel := cmd.Context(), context.CancelFunc(func() {})
			if flags.timeoutExplicit {
				ctx, cancel = boundCtx(cmd.Context(), flags)
			}
			defer cancel()
			s, err := openFleetWrite(ctx, path)
			if err != nil {
				return err
			}
			defer s.Close()

			opts := fleet.DefaultOptions
			opts.InitialWindow, opts.IgnoreCursor = initial, full
			if cliutil.IsDogfoodEnv() {
				opts.MaxResultPages, opts.MaxDetails = 1, 2
			}
			if concurrency < 1 {
				concurrency = 1
			}
			rows := make([]fleetSyncRow, len(list))
			var writeMu sync.Mutex // SQLite takes one writer; fetches run in parallel
			sem := make(chan struct{}, concurrency)
			var wg sync.WaitGroup
			for i, n := range list {
				wg.Add(1)
				go func(i int, n fleet.NAS) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					nasCtx, cancelNAS := context.WithTimeout(ctx, perNAS)
					defer cancelNAS()
					rows[i] = syncOneNAS(nasCtx, flags, s.DB(), &writeMu, n, opts)
				}(i, n)
			}
			wg.Wait()

			failed := 0
			for _, r := range rows {
				if !r.OK {
					failed++
				}
			}
			if err := printFleetRows(cmd, flags, rows, "nothing synced"); err != nil {
				return err
			}
			if failed > 0 {
				return apiErr(fmt.Errorf("%d of %d NAS failed to sync; see each row's error", failed, len(rows)))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&nasName, "nas", "", "Sync only this registered NAS")
	cmd.Flags().StringVar(&dbPath, "db", "", "Fleet mirror path (default <data dir>/fleet.db)")
	cmd.Flags().StringVar(&window, "since", "30d", "History to pull on a NAS's first sync, or with --full (later syncs continue from the newest mirrored run)")
	cmd.Flags().BoolVar(&full, "full", false, "Re-pull the whole --since window instead of continuing from the newest mirrored run (backfill, or recover from a truncated sync)")
	cmd.Flags().IntVar(&concurrency, "concurrency", 4, "NAS units fetched in parallel")
	cmd.Flags().DurationVar(&perNAS, "nas-timeout", 10*time.Minute, "Time budget for one NAS (login, history, storage)")
	return cmd
}

func syncOneNAS(ctx context.Context, flags *rootFlags, db *sql.DB, writeMu *sync.Mutex, n fleet.NAS, opts fleet.Options) fleetSyncRow {
	start := time.Now()
	row := fleetSyncRow{NAS: n.Name}
	var client *dsm.Client // set once built; its Redact masks what the error may echo
	finish := func(snap *fleet.Snapshot, info fleet.SystemInfo, err error) fleetSyncRow {
		writeMu.Lock()
		defer writeMu.Unlock()
		if err != nil && client != nil {
			// The error is printed and stored in fleet_sync.error: mask every
			// credential and session value used before either happens.
			err = errors.New(client.Redact(err.Error()))
		}
		// Record the outcome even when the NAS's time budget ran out; data and
		// outcome commit together.
		wctx := context.WithoutCancel(ctx)
		if snap != nil {
			row.Warnings = snap.Warnings
			row.Tasks, row.Results, row.DeviceRuns, row.StorageItems = len(snap.Tasks), len(snap.Results), len(snap.DeviceRuns), len(snap.Storage)
		}
		row.ABBInstalled = info.ABBInstalled
		if cerr := fleet.CommitSync(wctx, db, n, start, snap, info, err); errors.Is(cerr, fleet.ErrSuperseded) {
			// A sync that started later already committed; it is the answer.
			row.Warnings = append(row.Warnings, cerr.Error())
		} else if cerr != nil {
			// The data could not be stored: record the failure on its own.
			err = errors.Join(err, fmt.Errorf("writing mirror: %w", cerr))
			if client != nil { // a commit error can quote mirrored ids: redact before storing
				err = errors.New(client.Redact(err.Error()))
			}
			if rerr := fleet.RecordAttempt(wctx, db, n, start, info, err); rerr != nil {
				err = errors.Join(err, rerr)
			}
		}
		row.OK = err == nil
		if err != nil {
			row.Error = err.Error()
		}
		row.DurationMS = time.Since(start).Milliseconds()
		return row
	}
	if !n.Credentials().Complete() {
		return finish(nil, fleet.SystemInfo{}, fmt.Errorf("credentials not set: set %[1]s_ACCOUNT and %[1]s_PASSWORD (macOS/Linux: export %[1]s_ACCOUNT=<account>; PowerShell: $env:%[1]s_ACCOUNT = \"<account>\")", n.EnvPrefix))
	}
	c, err := dsmClient(flags, n)
	if err != nil {
		return finish(nil, fleet.SystemInfo{}, err)
	}
	client = c
	defer func() { _ = c.Logout(context.Background()) }()
	cursor, err := fleet.ResultCursor(ctx, db, n)
	if err != nil {
		return finish(nil, fleet.SystemInfo{}, err)
	}
	snap, info, err := fleet.Fetch(ctx, c, cursor, start, opts)
	if c.DeviceIDErr != nil && snap != nil {
		snap.Warnings = append(snap.Warnings, "could not cache the trusted-device token; the next login will need a one-time code again: "+c.DeviceIDErr.Error())
	}
	return finish(snap, info, err)
}
