// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command (Phase 3); survives regeneration as a whole file.

// pp:data-source live

package cli

import (
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"gradient-pp-cli/internal/ledger"
)

// pushRow is one accountId x serviceId x unitCount entry parsed from the
// input file.
type pushRow struct {
	AccountID string  `json:"accountId"`
	ServiceID string  `json:"serviceId"`
	UnitCount float64 `json:"unitCount"`
}

type pushFailure struct {
	AccountID string `json:"account_id"`
	ServiceID string `json:"service_id"`
	Error     string `json:"error"`
}

type pushView struct {
	RunID                   string        `json:"run_id,omitempty"`
	Rows                    int           `json:"rows"`
	Pushed                  int           `json:"pushed"`
	Failed                  int           `json:"failed"`
	Services                []string      `json:"services,omitempty"`
	BillingRebuildTriggered bool          `json:"billing_rebuild_triggered"`
	DryRun                  bool          `json:"dry_run,omitempty"`
	FetchFailures           []pushFailure `json:"fetch_failures,omitempty"`
	Note                    string        `json:"note,omitempty"`
}

func newNovelUsagePushCmd(flags *rootFlags) *cobra.Command {
	var flagFile string
	var flagNoBuild bool

	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push a file of unit counts with a single billing rebuild",
		Long: strings.Trim(`
Use this command to push many unit counts from a file with a single billing rebuild.
Do NOT use this command to send one ad-hoc count; use the 'billing' command.
Do NOT use it to compare against the prior push; use 'usage drift'.

Reads a CSV (header: accountId,serviceId,unitCount) or a JSON array of
{accountId, serviceId, unitCount} objects, fans out one
POST /vendor-api/service/{serviceId}/count per row with no_build=true on every
call except the last (so Gradient rebuilds billing exactly once), and records
each row in the local push ledger that powers 'usage drift'.
`, "\n"),
		Example: strings.Trim(`
  gradient-cli usage push --file ./counts.csv --agent
  gradient-cli usage push --file ./counts.json --dry-run
  gradient-cli usage push --file ./counts.csv --no-build`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":       "false",
			"pp:typed-exit-codes": "0,1,2",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if len(args) > 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("usage push takes no positional arguments; pass the counts file via --file"))
			}

			if dryRunOK(flags) {
				if flagFile == "" {
					return printJSONFiltered(cmd.OutOrStdout(), pushView{DryRun: true, Note: "would push counts from --file; pass a counts file to validate its rows"}, flags)
				}
				rows, err := parsePushFile(flagFile)
				if err != nil {
					if os.IsNotExist(err) {
						// Dry-run on a not-yet-written file is a plan, not a failure.
						return printJSONFiltered(cmd.OutOrStdout(), pushView{DryRun: true, Note: fmt.Sprintf("file %s not found (dry-run): would parse and push its counts", flagFile)}, flags)
					}
					// Validation IS the dry-run feature: malformed rows fail loudly.
					return usageErr(fmt.Errorf("validating %s: %w", flagFile, err))
				}
				services := map[string]bool{}
				for _, r := range rows {
					services[r.ServiceID] = true
				}
				view := pushView{
					Rows:     len(rows),
					Services: sortedKeysOf(services),
					DryRun:   true,
					Note:     fmt.Sprintf("would push %d counts across %d service(s); no_build on all but the last call (single billing rebuild)", len(rows), len(services)),
				}
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}

			if flagFile == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--file is required"))
			}
			if flags.dataSource == "local" {
				return fmt.Errorf("usage push has no local data source: it sends counts to the live API (use --dry-run to validate the file offline)")
			}

			rows, err := parsePushFile(flagFile)
			if err != nil {
				return usageErr(fmt.Errorf("parsing %s: %w", flagFile, err))
			}
			services := map[string]bool{}
			for _, r := range rows {
				services[r.ServiceID] = true
			}
			serviceList := sortedKeysOf(services)
			if len(rows) == 0 {
				return printJSONFiltered(cmd.OutOrStdout(), pushView{Note: "input file has no rows; nothing pushed"}, flags)
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			runID := newPushRunID()
			view := pushView{RunID: runID, Rows: len(rows), Services: serviceList, FetchFailures: []pushFailure{}}
			records := make([]ledger.PushRecord, 0, len(rows))

			for i, row := range rows {
				last := i == len(rows)-1
				noBuild := !last || flagNoBuild
				path := replacePathParam("/vendor-api/service/{serviceId}/count", "serviceId", row.ServiceID)
				params := map[string]string{}
				if noBuild {
					params["no_build"] = "true"
				}
				body := map[string]any{"accountId": row.AccountID, "unitCount": row.UnitCount}
				rec := ledger.PushRecord{
					RunID: runID, At: time.Now().UTC(),
					ServiceID: row.ServiceID, AccountID: row.AccountID,
					UnitCount: row.UnitCount, NoBuild: noBuild, Status: "sent",
				}
				if _, _, err := c.PostWithParams(cmd.Context(), path, params, body); err != nil {
					rec.Status = "failed"
					rec.Error = err.Error()
					view.Failed++
					view.FetchFailures = append(view.FetchFailures, pushFailure{AccountID: row.AccountID, ServiceID: row.ServiceID, Error: err.Error()})
				} else {
					view.Pushed++
					if !noBuild {
						view.BillingRebuildTriggered = true
					}
				}
				records = append(records, rec)
			}

			// Rebuild recovery: the single billing rebuild rides on the LAST
			// row's call (no_build=false). If that row failed while earlier
			// rows landed, the pushed counts would sit in Gradient with no
			// rebuild ever triggered - the exact reconciliation failure this
			// command exists to prevent. Recover by re-pushing the most
			// recent successful row with no_build=false (same count, so the
			// re-push is value-idempotent and only exists to fire the
			// rebuild).
			if view.Pushed > 0 && !view.BillingRebuildTriggered && !flagNoBuild {
				for i := len(records) - 1; i >= 0; i-- {
					if records[i].Status != "sent" {
						continue
					}
					rrec := records[i]
					rpath := replacePathParam("/vendor-api/service/{serviceId}/count", "serviceId", rrec.ServiceID)
					rbody := map[string]any{"accountId": rrec.AccountID, "unitCount": rrec.UnitCount}
					rrec.At = time.Now().UTC()
					rrec.NoBuild = false
					if _, _, rerr := c.PostWithParams(cmd.Context(), rpath, map[string]string{}, rbody); rerr != nil {
						rrec.Status = "failed"
						rrec.Error = rerr.Error()
						view.Note = "counts landed but billing was NOT rebuilt (last row and rebuild re-push both failed); re-run the failed rows or push any single count without --no-build to trigger the rebuild"
					} else {
						view.BillingRebuildTriggered = true
						view.Note = fmt.Sprintf("last row failed, billing rebuild recovered by re-pushing account %s / service %s", rrec.AccountID, rrec.ServiceID)
					}
					records = append(records, rrec)
					break
				}
			}
			if view.Pushed > 0 && !view.BillingRebuildTriggered && flagNoBuild && view.Note == "" {
				view.Note = "billing rebuild deferred (--no-build): trigger it later by pushing any count without --no-build"
			}

			dir, err := ledger.Dir()
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: push sent but ledger unavailable: %v\n", err)
			} else if err := ledger.AppendPushes(dir, records); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: push sent but ledger write failed: %v\n", err)
			}

			if view.Failed > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d pushes failed; %d counts landed\n", view.Failed, view.Rows, view.Pushed)
			}
			if printErr := printJSONFiltered(cmd.OutOrStdout(), view, flags); printErr != nil {
				return printErr
			}
			if view.Pushed == 0 {
				return fmt.Errorf("all %d pushes failed", view.Rows)
			}
			// Partial failure is a real write failure: exit non-zero so
			// exit-code-gated callers see it; the JSON envelope carries the
			// per-row detail in fetch_failures.
			if view.Failed > 0 {
				return fmt.Errorf("%d of %d pushes failed (see fetch_failures)", view.Failed, view.Rows)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagFile, "file", "", "Counts file: CSV with header accountId,serviceId,unitCount, or a JSON array of {accountId,serviceId,unitCount}")
	cmd.Flags().BoolVar(&flagNoBuild, "no-build", false, "Set no_build on EVERY call (defer the billing rebuild entirely; default rebuilds once after the last row)")
	return cmd
}

// parsePushFile reads CSV (with header) or a JSON array into push rows.
func parsePushFile(path string) ([]pushRow, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- user explicitly supplies the --file path to push; reading it is the command's purpose
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return []pushRow{}, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var rows []pushRow
		if err := json.Unmarshal([]byte(trimmed), &rows); err != nil {
			return nil, fmt.Errorf("invalid JSON array: %w", err)
		}
		return validatePushRows(rows)
	}
	r := csv.NewReader(strings.NewReader(trimmed))
	r.TrimLeadingSpace = true
	recs, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("invalid CSV: %w", err)
	}
	if len(recs) < 1 {
		return []pushRow{}, nil
	}
	col := map[string]int{}
	for i, h := range recs[0] {
		col[normalizeHeader(h)] = i
	}
	ai, aok := col["accountid"]
	si, sok := col["serviceid"]
	ui, uok := col["unitcount"]
	if !aok || !sok || !uok {
		return nil, fmt.Errorf("CSV header must include accountId, serviceId, unitCount (got: %s)", strings.Join(recs[0], ","))
	}
	rows := make([]pushRow, 0, len(recs)-1)
	for n, rec := range recs[1:] {
		if len(rec) <= ai || len(rec) <= si || len(rec) <= ui {
			return nil, fmt.Errorf("CSV row %d has too few columns", n+2)
		}
		count, err := strconv.ParseFloat(strings.TrimSpace(rec[ui]), 64)
		if err != nil {
			return nil, fmt.Errorf("CSV row %d: unitCount %q is not a number", n+2, rec[ui])
		}
		rows = append(rows, pushRow{
			AccountID: strings.TrimSpace(rec[ai]),
			ServiceID: strings.TrimSpace(rec[si]),
			UnitCount: count,
		})
	}
	return validatePushRows(rows)
}

func normalizeHeader(h string) string {
	return strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(h), "_", ""), " ", ""))
}

// sortedKeysOf returns the map's keys sorted, so JSON output is stable.
func sortedKeysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func validatePushRows(rows []pushRow) ([]pushRow, error) {
	for i, r := range rows {
		if r.ServiceID == "" {
			return nil, fmt.Errorf("row %d: serviceId is empty", i+1)
		}
		if r.AccountID == "" {
			return nil, fmt.Errorf("row %d: accountId is empty", i+1)
		}
		if r.UnitCount < 0 {
			return nil, fmt.Errorf("row %d: unitCount %v is negative", i+1, r.UnitCount)
		}
	}
	return rows, nil
}

// newPushRunID returns a collision-proof run id: UTC second timestamp plus a
// random suffix, so back-to-back pushes within the same second remain
// distinct runs in the drift ledger.
func newPushRunID() string {
	buf := make([]byte, 3)
	if _, err := rand.Read(buf); err != nil {
		// Timestamp-only fallback; collision window is one second.
		return time.Now().UTC().Format("20060102-150405")
	}
	return time.Now().UTC().Format("20060102-150405") + "-" + hex.EncodeToString(buf)
}
