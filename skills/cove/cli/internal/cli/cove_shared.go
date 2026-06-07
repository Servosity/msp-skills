// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Shared plumbing for the hand-built Cove commands: client construction from
// config, the EnumerateAccountStatistics pager, and snapshot guidance notes.
package cli

import (
	"context"
	"fmt"
	"sort"
	"time"

	"cove-pp-cli/internal/cliutil"
	"cove-pp-cli/internal/config"
	"cove-pp-cli/internal/coverpc"

	"github.com/spf13/cobra"
)

// newCoveRPC builds the JSON-RPC client honoring --config / COVE_BASE_URL.
func newCoveRPC(flags *rootFlags) (*coverpc.Client, error) {
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
			if v, ok := row["AccountId"].(float64); ok {
				d.AccountID = int64(v)
			}
			if v, ok := row["PartnerId"].(float64); ok {
				d.PartnerID = int64(v)
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
const snapshotPairNote = "trend commands diff local snapshots — run `cove-cli snapshot` at least twice, some time apart"

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
