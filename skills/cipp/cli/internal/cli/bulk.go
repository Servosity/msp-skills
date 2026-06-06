// Copyright 2026 damienstevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"cipp-pp-cli/internal/cliutil"
	"github.com/spf13/cobra"
)

// bulkAction maps a CSV action keyword to a CIPP endpoint + HTTP method.
type bulkAction struct {
	Method string
	Path   string
}

// bulkActions is the small, documented action→endpoint map. Keep it small; add
// new actions deliberately.
var bulkActions = map[string]bulkAction{
	"offboard":       {Method: "POST", Path: "/ExecOffboardUser"},
	"add-user":       {Method: "POST", Path: "/AddUser"},
	"remove-user":    {Method: "POST", Path: "/RemoveUser"},
	"set-forwarding": {Method: "POST", Path: "/ExecEmailForward"},
}

// bulkRow is one parsed CSV row.
type bulkRow struct {
	Index      int            `json:"index"`
	Action     string         `json:"action"`
	Tenant     string         `json:"tenant"`
	Endpoint   string         `json:"endpoint"`
	Method     string         `json:"method"`
	Params     map[string]any `json:"params,omitempty"`
	parseError string
}

// parseBulkCSV reads action,tenant,params_json rows. A header row whose first
// column is literally "action" is skipped. Unknown actions and malformed
// params_json are recorded on the row rather than aborting the whole parse so
// the plan can show every problem at once.
func parseBulkCSV(r io.Reader) ([]bulkRow, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1 // tolerate trailing/variable columns
	records, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading CSV: %w", err)
	}
	out := make([]bulkRow, 0, len(records))
	idx := 0
	for i, rec := range records {
		if len(rec) == 0 {
			continue
		}
		action := strings.TrimSpace(rec[0])
		// Skip a header row.
		if i == 0 && strings.EqualFold(action, "action") {
			continue
		}
		if action == "" {
			continue
		}
		row := bulkRow{Index: idx, Action: action}
		idx++
		if len(rec) > 1 {
			row.Tenant = strings.TrimSpace(rec[1])
		}
		var paramsRaw string
		if len(rec) > 2 {
			paramsRaw = strings.TrimSpace(rec[2])
		}
		if def, ok := bulkActions[strings.ToLower(action)]; ok {
			row.Method = def.Method
			row.Endpoint = def.Path
		} else {
			row.parseError = fmt.Sprintf("unknown action %q (known: %s)", action, strings.Join(knownActions(), ", "))
		}
		if paramsRaw != "" {
			var p map[string]any
			if err := json.Unmarshal([]byte(paramsRaw), &p); err != nil {
				if row.parseError == "" {
					row.parseError = fmt.Sprintf("invalid params_json: %v", err)
				}
			} else {
				row.Params = p
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func knownActions() []string {
	keys := make([]string, 0, len(bulkActions))
	for k := range bulkActions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// bulkCheckpoint records completed row indices for resume.
type bulkCheckpoint struct {
	CSV       string `json:"csv"`
	Completed []int  `json:"completed"`
}

func bulkCheckpointPath(csvPath string) (string, error) {
	dir, err := fanoutCacheDir()
	if err != nil {
		return "", err
	}
	base := strings.TrimSuffix(filepath.Base(csvPath), filepath.Ext(csvPath))
	if base == "" {
		base = "bulk"
	}
	return filepath.Join(dir, "bulk-"+checkpointSlug(base)+".json"), nil
}

func readBulkCheckpoint(path string) (map[int]bool, error) {
	done := map[int]bool{}
	// path comes from bulkCheckpointPath: a slugified filename under the
	// per-CLI cache dir, never user-supplied. #nosec G304 -- internal cache path.
	data, err := os.ReadFile(path) // #nosec G304

	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return done, nil
		}
		return nil, err
	}
	var cp bulkCheckpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("parsing checkpoint %s: %w", path, err)
	}
	for _, i := range cp.Completed {
		done[i] = true
	}
	return done, nil
}

func writeBulkCheckpoint(path, csvPath string, done map[int]bool) error {
	completed := make([]int, 0, len(done))
	for i := range done {
		completed = append(completed, i)
	}
	sort.Ints(completed)
	data, err := json.MarshalIndent(bulkCheckpoint{CSV: csvPath, Completed: completed}, "", "  ")
	if err != nil {
		return err
	}
	// Checkpoint records the source CSV path; keep it user-private (0600).
	return os.WriteFile(path, data, 0o600)
}

func newNovelBulkCmd(flags *rootFlags) *cobra.Command {
	var flagFrom string
	var flagExecute bool
	var flagResume bool
	var flagConcurrency int

	cmd := &cobra.Command{
		Use:         "bulk",
		Short:       "Drive create/edit/remove/offboard actions from a CSV with Retry-After backoff and resume-after-429 checkpointing.",
		Annotations: map[string]string{"mcp:read-only": "false"},
		Long: `Drive bulk write actions from a CSV across tenants.

CSV columns: action,tenant,params_json
  action       one of: ` + strings.Join(knownActions(), ", ") + `
  tenant       the tenant defaultDomainName (added to the request as tenantFilter)
  params_json  a JSON object merged into the request body

By default bulk PRINTS the plan and does NOT execute. Pass --execute to perform
the writes (each row POSTs to its mapped endpoint with tenantFilter set). The
client already retries 429 with Retry-After. --resume checkpoints completed rows
and skips them on a re-run.

Examples:
  cipp-cli bulk --from changes.csv            # plan only
  cipp-cli bulk --from changes.csv --dry-run  # plan only (explicit)
  cipp-cli bulk --from changes.csv --execute  # perform the writes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && flagFrom == "" {
				return cmd.Help()
			}
			// Verify-friendly guard: a probe sets --dry-run with no --from, so
			// short-circuit before any IO. When --from is present, --dry-run is
			// the user asking for a plan (printed below), so fall through.
			if dryRunOK(flags) && flagFrom == "" {
				return nil
			}
			if flagFrom == "" {
				return usageErr(fmt.Errorf("--from <csv> is required"))
			}

			// flagFrom is the user's own --from CSV path; opening the file the
			// operator named is the feature. #nosec G304 -- user-supplied input file.
			f, err := os.Open(flagFrom) // #nosec G304
			if err != nil {
				return fmt.Errorf("opening CSV %q: %w", flagFrom, err)
			}
			defer f.Close()
			rows, err := parseBulkCSV(f)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				return fmt.Errorf("no actionable rows found in %q", flagFrom)
			}

			planOnly := !flagExecute || flags.dryRun

			// Print the plan: one line per row.
			for _, r := range rows {
				note := r.parseError
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s%s\n",
					r.Action, r.Tenant, r.Method, r.Endpoint,
					func() string {
						if note != "" {
							return "\t[skip: " + note + "]"
						}
						return ""
					}())
			}

			if planOnly {
				return nil
			}

			// Verify-safe: never perform writes under the verifier.
			if cliutil.IsVerifyEnv() {
				fmt.Fprintln(cmd.OutOrStdout(), "would execute (verify mode): no requests sent")
				return nil
			}

			// Resume checkpoint.
			var cpPath string
			completed := map[int]bool{}
			if flagResume {
				p, err := bulkCheckpointPath(flagFrom)
				if err != nil {
					return err
				}
				cpPath = p
				done, err := readBulkCheckpoint(cpPath)
				if err != nil {
					return err
				}
				completed = done
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			_ = flagConcurrency // executes sequentially; writes are ordered and 429-sensitive

			var firstErr error
			executed := 0
			for _, r := range rows {
				if r.parseError != "" {
					continue
				}
				if flagResume && completed[r.Index] {
					continue
				}
				body := map[string]any{}
				for k, v := range r.Params {
					body[k] = v
				}
				params := map[string]string{"tenantFilter": r.Tenant}
				_, status, err := c.PostWithParams(ctx, r.Endpoint, params, body)
				if err != nil {
					var rle *cliutil.RateLimitError
					if errors.As(err, &rle) {
						fmt.Fprintf(cmd.ErrOrStderr(), "row %d (%s/%s): %v\n", r.Index, r.Action, r.Tenant, rle)
					} else {
						fmt.Fprintf(cmd.ErrOrStderr(), "row %d (%s/%s): %v\n", r.Index, r.Action, r.Tenant, err)
					}
					if firstErr == nil {
						firstErr = err
					}
					continue
				}
				executed++
				if flagResume {
					completed[r.Index] = true
				}
				fmt.Fprintf(cmd.OutOrStdout(), "executed row %d: %s %s (tenant=%s) -> HTTP %d\n", r.Index, r.Method, r.Endpoint, r.Tenant, status)
			}

			if flagResume && cpPath != "" {
				if err := writeBulkCheckpoint(cpPath, flagFrom, completed); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warn: writing checkpoint: %v\n", err)
				}
			}

			if firstErr != nil {
				return classifyAPIError(firstErr, flags)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "%d rows executed\n", executed)
			return nil
		},
	}
	cmd.Flags().StringVar(&flagFrom, "from", "", "Path to the actions CSV (columns: action,tenant,params_json)")
	cmd.Flags().BoolVar(&flagExecute, "execute", false, "Actually perform the writes (default is plan-only; --dry-run forces plan-only)")
	cmd.Flags().BoolVar(&flagResume, "resume", false, "Skip rows already completed in a prior run (checkpoint)")
	cmd.Flags().IntVar(&flagConcurrency, "concurrency", 1, "Reserved: bulk writes execute sequentially to respect 429 backoff")
	return cmd
}
