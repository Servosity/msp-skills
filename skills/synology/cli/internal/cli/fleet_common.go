// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Shared plumbing for the fleet/abb novel commands.

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"synology-pp-cli/internal/cliutil"
	"synology-pp-cli/internal/config"
	"synology-pp-cli/internal/dsm"
	"synology-pp-cli/internal/fleet"
	"synology-pp-cli/internal/store"
)

// exitFindings is the typed exit code for "the command ran and found
// something that needs attention" (failed backups, stale devices, storage
// problems, a NAS that stopped reporting). RMM checks alert on it.
const exitFindings = 6

func findingsErr(n int) error {
	return &cliError{code: exitFindings, err: fmt.Errorf("%d row(s) need attention (exit 6; --no-fail exits 0)", n)}
}

// redactErr masks every credential and session value c has used in err's
// text (response-derived values can echo them), keeping its exit code.
func redactErr(c *dsm.Client, err error) error {
	if err == nil || c == nil {
		return err
	}
	var ce *cliError
	if errors.As(err, &ce) {
		return &cliError{code: ce.code, err: errors.New(c.Redact(ce.err.Error()))}
	}
	return errors.New(c.Redact(err.Error()))
}

// dsmDryRun answers --dry-run for a generated DSM write command
// (HAND-FIX(dsm-dry-run)) with the standard dry-run object, sending nothing.
// An address carrying a user or password is refused even in a preview.
func dsmDryRun(cmd *cobra.Command, flags *rootFlags, action string) error {
	c, err := flags.newClient()
	if err != nil {
		return err
	}
	if err := dsm.CheckOrigin(c.BaseURL); err != nil {
		return configErr(err)
	}
	return writeDryRun(cmd.OutOrStdout(), flags, action)
}

// fleetNAS returns the registered NAS units. With an empty registry it falls
// back to the single NAS configured through SYNOLOGY_BASE_URL and
// SYNOLOGY_ACCOUNT/SYNOLOGY_PASSWORD, named "default", so a one-NAS user needs
// no registration.
func fleetNAS(flags *rootFlags) ([]fleet.NAS, *fleet.Registry, string, error) {
	path, err := fleet.RegistryPath()
	if err != nil {
		return nil, nil, "", configErr(err)
	}
	reg, err := fleet.LoadRegistry(path)
	if err != nil {
		return nil, nil, "", configErr(err)
	}
	if len(reg.NAS) > 0 {
		return reg.NAS, reg, path, nil
	}
	d, ok, err := defaultNAS(flags)
	if err != nil {
		return nil, nil, "", configErr(err)
	}
	if ok {
		return []fleet.NAS{d}, reg, path, nil
	}
	return nil, reg, path, nil
}

// defaultNAS builds the implicit "default" fleet entry from the CLI config.
// A SYNOLOGY_BASE_URL carrying a user or password, or a path, is an error: it
// would be stored in the mirror and printed.
func defaultNAS(flags *rootFlags) (fleet.NAS, bool, error) {
	cfg, err := config.Load(flags.configPath)
	if err != nil {
		return fleet.NAS{}, false, fmt.Errorf("reading the CLI configuration: %w", err)
	}
	if cfg.BaseURL == "" || doctorBaseURLIsPlaceholder(cfg.BaseURL) {
		return fleet.NAS{}, false, nil
	}
	origin, err := fleet.OriginURL(cfg.BaseURL)
	if err != nil {
		return fleet.NAS{}, false, fmt.Errorf("SYNOLOGY_BASE_URL: %w", err)
	}
	pin, skip := dsm.EnvTLS()
	n := fleet.NAS{Name: "default", URL: origin, EnvPrefix: "SYNOLOGY", TLSFingerprint: pin, Insecure: skip}
	n.Override = &dsm.Credentials{Account: cfg.SynologyAccount, Password: cfg.SynologyPassword, OTPCode: cfg.SynologyOtpCode, DeviceID: cfg.SynologyDeviceId}
	return n, true, nil
}

// resolveNAS picks one NAS by name, or the only one when name is empty.
func resolveNAS(flags *rootFlags, name string) (fleet.NAS, error) {
	list, _, _, err := fleetNAS(flags)
	if err != nil {
		return fleet.NAS{}, err
	}
	if len(list) == 0 {
		return fleet.NAS{}, configErr(errors.New("no NAS configured: run 'synology-cli fleet add <name> --url https://<nas>:5001' or set SYNOLOGY_BASE_URL, SYNOLOGY_ACCOUNT and SYNOLOGY_PASSWORD"))
	}
	if name == "" {
		if len(list) == 1 {
			return list[0], nil
		}
		return fleet.NAS{}, usageErr(fmt.Errorf("%d NAS units are registered; pick one with --nas", len(list)))
	}
	for _, n := range list {
		if n.Name == name {
			return n, nil
		}
	}
	return fleet.NAS{}, notFoundErr(fmt.Errorf("no registered NAS named %q (see 'synology-cli fleet list')", name))
}

// dsmClient builds the per-NAS RPC client.
func dsmClient(flags *rootFlags, n fleet.NAS) (*dsm.Client, error) {
	timeout := flags.timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	hc, err := n.HTTPClient(timeout)
	if err != nil {
		return nil, configErr(err)
	}
	baseURL := strings.TrimRight(n.URL, "/")
	creds := n.Credentials()
	return &dsm.Client{HTTP: hc, BaseURL: baseURL, Creds: creds, OnDeviceID: func(did string) error {
		// Cache the trusted-device token so the next login skips the one-time code.
		dir, err := cliutil.DataDir()
		if err != nil {
			return err
		}
		return dsm.SaveDeviceID(dir, baseURL, creds.Account, did)
	}}, nil
}

func fleetDBPath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	p, err := fleet.DBPath()
	if err != nil {
		return "", configErr(err)
	}
	return p, nil
}

// openFleetWrite opens (creating if needed) the fleet mirror for writing.
func openFleetWrite(ctx context.Context, path string) (*store.Store, error) {
	s, err := store.OpenWithContext(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := fleet.EnsureSchema(ctx, s.DB()); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

// openFleetRead opens the mirror read-only so reads never contend with a
// running fleet sync. A missing mirror is created first (schema only) when NAS
// units are registered, so their rows read as "never synced", never as empty.
func openFleetRead(ctx context.Context, path string, haveNAS bool) (*sql.DB, func(), error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if !haveNAS {
			return nil, func() {}, nil
		}
		s, err := openFleetWrite(ctx, path)
		if err != nil {
			return nil, nil, err
		}
		_ = s.Close()
	}
	s, err := store.OpenReadOnlyContext(ctx, path)
	if err != nil {
		return nil, nil, err
	}
	// A mirror from an older version is migrated once, through a short write
	// open; reads stay read-only otherwise.
	if stale, err := fleet.NeedsMigration(ctx, s.DB()); err != nil || stale {
		_ = s.Close()
		if err != nil {
			return nil, nil, err
		}
		w, err := openFleetWrite(ctx, path)
		if err != nil {
			return nil, nil, err
		}
		_ = w.Close()
		if s, err = store.OpenReadOnlyContext(ctx, path); err != nil {
			return nil, nil, err
		}
	}
	return s.DB(), func() { _ = s.Close() }, nil
}

// printFleetRows writes machine output through the generated helpers and a
// table for humans; rows must be a slice.
func printFleetRows(cmd *cobra.Command, flags *rootFlags, rows any, empty string) error {
	w := cmd.OutOrStdout()
	if !wantsHumanTable(w, flags) {
		return printJSONFiltered(w, rows, flags)
	}
	data, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	var items []map[string]any
	_ = json.Unmarshal(data, &items)
	if len(items) == 0 {
		_, err := fmt.Fprintln(w, empty)
		return err
	}
	for _, it := range items {
		for k, v := range it {
			f, ok := v.(float64)
			if !ok || f <= 0 {
				continue
			}
			if strings.HasSuffix(k, "_at") || k == "last_success" || k == "time_start" || k == "time_end" {
				it[k] = time.Unix(int64(f), 0).Local().Format("2006-01-02 15:04")
			}
		}
	}
	return printAutoTable(w, items)
}

// checkLimits rejects threshold flags that would silently disable a check: a
// non-finite or out-of-range percentage or multiple, or a negative
// temperature. 0 keeps its documented meaning (check off).
func checkLimits(fullPct, overrun float64, diskTemp int64) error {
	switch {
	case math.IsNaN(fullPct) || math.IsInf(fullPct, 0) || fullPct < 0 || fullPct > 100:
		return usageErr(fmt.Errorf("--full-pct must be between 0 and 100, got %v", fullPct))
	case math.IsNaN(overrun) || math.IsInf(overrun, 0) || overrun < 0:
		return usageErr(fmt.Errorf("--overrun must be a finite number of 0 or more, got %v", overrun))
	case diskTemp < 0:
		return usageErr(fmt.Errorf("--disk-temp must be 0 or more, got %d", diskTemp))
	}
	return nil
}

func parseWindow(name, v string) (time.Duration, error) {
	d, err := cliutil.ParseDurationLoose(v)
	if err != nil || d <= 0 {
		return 0, usageErr(fmt.Errorf("--%s %q is not a positive duration (use 24h, 36h, 7d, 2w)", name, v))
	}
	return d, nil
}

// noFleetHint tells a human how to populate the mirror.
func noFleetHint(w io.Writer) {
	fmt.Fprintln(w, "hint: no NAS configured; run 'synology-cli fleet add <name> --url https://<nas>:5001' then 'synology-cli fleet sync'")
}

// apiInfo asks SYNO.API.Info which of the given spec paths' APIs this NAS
// exposes, keyed by path.
func apiInfo(ctx context.Context, c *dsm.Client, paths ...string) (map[string]bool, error) {
	raw, err := c.Get(ctx, fleet.PathAPIInfo, nil)
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parsing API list: %w", err)
	}
	out := map[string]bool{}
	for _, p := range paths {
		_, out[p] = m[fleet.APIOf(p)]
	}
	return out, nil
}

// fleetReport is the shared body of the local fleet/abb report commands: open
// the mirror read-only, run the query over the configured NAS units, print,
// and exit 6 when findings exist (unless --no-fail).
type fleetReport struct {
	dbPath     string
	maxSyncAge string
	noFail     bool
}

func (r *fleetReport) bind(cmd *cobra.Command, findings bool) {
	cmd.Flags().StringVar(&r.dbPath, "db", "", "Fleet mirror path (default <data dir>/fleet.db)")
	cmd.Flags().StringVar(&r.maxSyncAge, "max-sync-age", "26h", "A NAS whose last good sync is older than this reads as not synced, never as OK")
	if findings {
		cmd.Flags().BoolVar(&r.noFail, "no-fail", false, "Exit 0 even when findings exist (default: exit 6)")
	}
}

// run's empty is printed for humans when nothing needs attention; %d is the
// number of NAS units checked.
func (r *fleetReport) run(cmd *cobra.Command, flags *rootFlags, empty string,
	query func(ctx context.Context, db fleet.Querier, nas []fleet.NAS, maxSyncAge time.Duration) (rows any, findings int, err error)) error {
	maxSyncAge, err := parseWindow("max-sync-age", r.maxSyncAge)
	if err != nil {
		return err
	}
	nas, _, _, err := fleetNAS(flags)
	if err != nil {
		return err
	}
	path, err := fleetDBPath(r.dbPath)
	if err != nil {
		return err
	}
	if len(nas) == 0 {
		// An empty answer from an empty fleet would read as "all clear" to an
		// RMM check; nothing was checked, so it is a configuration error.
		detail := "a fleet mirror exists but no NAS is registered"
		if _, statErr := os.Stat(path); statErr != nil {
			detail = "fleet mirror: " + statErr.Error()
		}
		return configErr(fmt.Errorf("no NAS configured: run 'synology-cli fleet add <name> --url https://<nas>:5001' then 'synology-cli fleet sync', or set SYNOLOGY_BASE_URL, SYNOLOGY_ACCOUNT and SYNOLOGY_PASSWORD (%s)", detail))
	}
	db, closeDB, err := openFleetRead(cmd.Context(), path, true)
	if err != nil {
		return err
	}
	defer closeDB()
	// One read transaction: every query in the report sees the same snapshot,
	// even while a fleet sync commits.
	tx, err := db.BeginTx(cmd.Context(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	rows, findings, err := query(cmd.Context(), tx, nas, maxSyncAge)
	if err != nil {
		return err
	}

	if err := printFleetRows(cmd, flags, rows, fmt.Sprintf(empty, len(nas))); err != nil {
		return err
	}
	if findings == noMatch {
		return notFoundErr(errors.New("no match"))
	}
	if findings > 0 && !r.noFail {
		return findingsErr(findings)
	}
	return nil
}

// noMatch is a findings value a report returns to exit 3 after printing an
// empty result (abb device with no matching host on fresh mirrors).
const noMatch = -1
