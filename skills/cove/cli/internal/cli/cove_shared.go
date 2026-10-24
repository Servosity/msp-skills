// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Shared plumbing for the hand-built Cove commands: client construction from
// config, the EnumerateAccountStatistics pager, and snapshot guidance notes.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"cove-pp-cli/internal/cliutil"
	"cove-pp-cli/internal/config"
	"cove-pp-cli/internal/coverpc"
	"cove-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

// newCoveRPC builds the JSON-RPC client for a command that reads the live
// API, honoring --config / COVE_BASE_URL. It refuses --data-source local: the
// hand-built live commands have no local equivalent, so they must not dial
// Cove when the operator asked for local data only.
func newCoveRPC(flags *rootFlags) (*coverpc.Client, error) {
	if err := validateDataSourceStrategy(flags, "live"); err != nil {
		return nil, usageErr(err)
	}
	return newCoveRPCLocal(flags)
}

// newCoveRPCLocal builds the client for a command that only touches the local
// session cache (auth status, auth logout) and sends nothing.
func newCoveRPCLocal(flags *rootFlags) (*coverpc.Client, error) {
	cfg, err := config.Load(flags.configPath)
	if err != nil {
		return nil, configErr(err)
	}
	c := coverpc.New(cfg.BaseURL)
	if flags.timeout > 0 {
		c.HTTP.Timeout = flags.timeout
	}
	return c, nil
}

// fleetDevice is one EnumerateAccountStatistics row with flattened settings.
type fleetDevice struct {
	AccountID int64             `json:"account_id"`
	PartnerID int64             `json:"partner_id"`
	Settings  map[string]string `json:"settings,omitempty"`
	// AccountKnown is false when AccountId was absent or not an exact
	// integer: AccountID then holds 0, which is no device's id.
	AccountKnown bool `json:"-"`
}

// fleetStatsQuery drives the EnumerateAccountStatistics pager.
type fleetStatsQuery struct {
	PartnerID int64
	Filter    string
	Columns   []string
	OrderBy   string
	PageSize  int
	MaxPages  int
}

// fetchFleetStats pages EnumerateAccountStatistics under the partner subtree
// and returns flattened device rows plus the number of records scanned.
// Scan effort is bounded by MaxPages independently of result size; callers
// surface the cap in their JSON envelopes per the scan-and-filter rule.
func fetchFleetStats(ctx context.Context, c *coverpc.Client, q fleetStatsQuery) ([]fleetDevice, int, bool, error) {
	if q.PageSize <= 0 {
		q.PageSize = 300
	}
	if q.MaxPages <= 0 {
		q.MaxPages = 10
	}
	if cliutil.IsDogfoodEnv() && q.MaxPages > 1 {
		q.MaxPages = 1
	}
	devices := make([]fleetDevice, 0, q.PageSize)
	scanned := 0
	capHit := true
	for page := 0; page < q.MaxPages; page++ {
		params := map[string]any{
			"query": map[string]any{
				"PartnerId":         q.PartnerID,
				"Filter":            q.Filter,
				"SelectionMode":     "Merged",
				"StartRecordNumber": page * q.PageSize,
				"RecordsCount":      q.PageSize,
				"OrderBy":           q.OrderBy,
				"Columns":           q.Columns,
				"Totals":            []string{},
			},
		}
		result, err := c.Call(ctx, "EnumerateAccountStatistics", params)
		if err != nil {
			return nil, scanned, false, fmt.Errorf("enumerating device statistics (page %d): %w", page+1, err)
		}
		rows, err := coverpc.Rows(result)
		if err != nil {
			return nil, scanned, false, fmt.Errorf("decoding device statistics (page %d): %w", page+1, err)
		}
		for _, row := range rows {
			scanned++
			d := fleetDevice{Settings: coverpc.FlattenSettings(row["Settings"])}
			if v, ok := coverpc.RowInt(row, "AccountId"); ok {
				d.AccountID, d.AccountKnown = v, true
			}
			if v, ok := coverpc.RowInt(row, "PartnerId"); ok {
				d.PartnerID = v
			}
			devices = append(devices, d)
		}
		if len(rows) < q.PageSize {
			capHit = false
			break
		}
	}
	return devices, scanned, capHit, nil
}

// resolvePartnerID returns the explicit --partner-id when set, else the
// logged-in user's root partner.
func resolvePartnerID(ctx context.Context, c *coverpc.Client, explicit int64) (int64, error) {
	if explicit > 0 {
		return explicit, nil
	}
	return c.RootPartnerID(ctx)
}

// scanCapFields carries the shared scan-and-filter accounting every fleet
// command embeds in its JSON envelope.
type scanCapFields struct {
	ScannedDevices int    `json:"scanned_devices"`
	MaxScanPages   int    `json:"max_scan_pages"`
	Note           string `json:"note,omitempty"`
}

func scanCapNote(capHit bool, scanned, maxPages int, what string) string {
	if !capHit {
		return ""
	}
	return fmt.Sprintf("scanned %d devices across the first %d pages without exhausting the fleet; raise --max-scan-pages to widen the %s scan", scanned, maxPages, what)
}

// snapshotPairNote explains the two-snapshot requirement for drift commands.
const snapshotPairNote = "trend commands diff local snapshots of the same partner tree and endpoint; run `cove-cli snapshot` at least twice, some time apart"

// partialSnapshotNote qualifies a comparison that involves a snapshot whose
// scan stopped at its cap (or that predates completeness tracking): a device
// missing from it was not necessarily removed.
const partialSnapshotNote = "a compared snapshot is partial (its scan stopped at --max-scan-pages, it skipped devices without a readable AccountId, or it predates completeness tracking), so added and removed devices are not reported and totals cover only devices present in both; re-run `cove-cli snapshot` with a higher --max-scan-pages"

// snapshotBaseline picks the snapshot to compare latest against: the newest
// one at least window older (default: any older one) of the SAME partner tree
// at the SAME endpoint. Every comparison command picks its baseline here and
// every pair it returns passes store.CoveSnapshotsComparable, the one rule.
// When no comparable baseline exists, refusal says why in plain words and the
// caller prints no delta at all.
func snapshotBaseline(ctx context.Context, db *store.Store, latest store.CoveSnapshot, window time.Duration) (baseline store.CoveSnapshot, refusal string, err error) {
	if !latest.KnownScope() {
		return store.CoveSnapshot{}, fmt.Sprintf("nothing compared: the latest snapshot (id %d) has no recorded endpoint or partner (it predates endpoint tracking), so it could have come from another endpoint and is never compared with any snapshot; %s", latest.ID, snapshotPairNote), nil
	}
	cutoff := latest.TakenAt.Add(-time.Second)
	if window > 0 {
		cutoff = latest.TakenAt.Add(-window)
	}
	baseline, ok, err := db.CoveSnapshotBeforeInScope(ctx, cutoff, latest.RootPartnerID, latest.Endpoint)
	if err != nil {
		return store.CoveSnapshot{}, "", err
	}
	if !ok || !store.CoveSnapshotsComparable(latest, baseline) {
		return store.CoveSnapshot{}, fmt.Sprintf("only one qualifying snapshot for partner %d at %s; %s", latest.RootPartnerID, latest.Endpoint, snapshotPairNote), nil
	}
	return baseline, "", nil
}

// snapshotSetting reads one integer column of a stored device row from its
// settings map through the one exact parser. ok is false when the server did
// not report the column or it is not exactly an integer: the row's hot column
// holds 0 then, which must never be read as a real zero.
func snapshotSetting(r store.CoveDeviceStat, code string) (int64, bool) {
	var m map[string]string
	if err := json.Unmarshal([]byte(r.SettingsJSON), &m); err != nil {
		return 0, false
	}
	return coverpc.SettingInt(m, code)
}

// snapshotCount is snapshotSetting for a count or byte size: a negative
// stored value is not a measurement either.
func snapshotCount(r store.CoveDeviceStat, code string) (int64, bool) {
	var m map[string]string
	if err := json.Unmarshal([]byte(r.SettingsJSON), &m); err != nil {
		return 0, false
	}
	return coverpc.SettingCount(m, code)
}

// addExact adds v to *sum. It reports false and leaves *sum unchanged when
// the result would overflow int64, so a total is refused, never wrapped.
func addExact(sum *int64, v int64) bool {
	r := *sum + v
	if (v > 0 && r < *sum) || (v < 0 && r > *sum) {
		return false
	}
	*sum = r
	return true
}

// errTotalOverflow refuses to print a total that does not fit in int64.
func errTotalOverflow(what string, accountID int64) error {
	return fmt.Errorf("%s overflows a 64-bit integer at account %d: a reported value is implausible, so no total is printed rather than a wrapped one", what, accountID)
}

// emitCoveJSON renders a hand-built command's envelope through the generated
// output helpers so --select/--compact/--quiet behave like every other
// command. When --csv is set and items is non-nil, the item rows are emitted
// as CSV instead of the wrapper envelope.
func emitCoveJSON(cmd *cobra.Command, flags *rootFlags, view any, items any) error {
	if flags.csv && items != nil {
		return printJSONFiltered(cmd.OutOrStdout(), items, flags)
	}
	return flags.printJSON(cmd, view)
}

// sortedByCustomer orders map keys for deterministic per-customer rollups.
func sortedKeys[M ~map[string]V, V any](m M) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// parseCoveSince parses a --since value with day/week shorthand support.
func parseCoveSince(raw string) (time.Duration, error) {
	if raw == "" {
		return 0, nil
	}
	d, err := cliutil.ParseDurationLoose(raw)
	if err != nil {
		return 0, usageErr(fmt.Errorf("invalid --since %q: use forms like 24h, 7d, 1w", raw))
	}
	return d, nil
}

// authErr marks a credential failure (exit code 4). Press 4.34.0 omits it for a
// spec with auth type none; Cove's hand-built commands sign in through coverpc.
func authErr(err error) error { return &cliError{code: 4, err: err} }

// coveDryRun reports a hand-built command's dry run. Under --json (and --agent)
// it prints the same envelope as writeDryRun, so a caller parsing stdout never
// gets prose.
func coveDryRun(cmd *cobra.Command, flags *rootFlags, would string) error {
	if flags != nil && flags.asJSON {
		return writeEncodedJSON(cmd.OutOrStdout(), dryRunResult{DryRun: true, Action: cmd.CommandPath(), Would: would}, "", "")
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "dry-run: %s\n", would)
	return err
}
