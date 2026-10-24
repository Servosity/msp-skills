// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
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
	// MultiTenant marks an endpoint that accepts a `selectedTenants` array and
	// fans out SERVER-SIDE. For these, rows sharing an action and params are
	// collapsed into ONE call carrying every tenant, instead of one call per
	// tenant. That is both faster and materially less likely to trip Microsoft
	// Graph throttling, which is the dominant failure mode for large batches.
	MultiTenant bool
}

// bulkActions is the documented action→endpoint map.
//
// The MultiTenant entries are the endpoints whose OpenAPI request body declares
// a `selectedTenants` property ("Comma-separated list of tenant domains for bulk
// operations"). All 21 such endpoints in the current spec are represented below
// or reachable via the generic `deploy-*` names; every one is a deploy/apply
// shape rather than a per-user mutation, which is why the collapse is safe.
var bulkActions = map[string]bulkAction{
	// Per-user mutations: one call per tenant, no server-side fan-out.
	"offboard":        {Method: "POST", Path: "/ExecOffboardUser"},
	"add-user":        {Method: "POST", Path: "/AddUser"},
	"remove-user":     {Method: "POST", Path: "/RemoveUser"},
	"set-forwarding":  {Method: "POST", Path: "/ExecEmailForward"},
	"reset-password":  {Method: "POST", Path: "/ExecResetPass"},
	"reset-mfa":       {Method: "POST", Path: "/ExecResetMFA"},
	"revoke-sessions": {Method: "POST", Path: "/ExecRevokeSessions"},
	"disable-user":    {Method: "POST", Path: "/ExecDisableUser"},

	// Multi-tenant deploys: collapse to one server-side fan-out call.
	"deploy-dlp-policy":          {Method: "POST", Path: "/AddDlpCompliancePolicy", MultiTenant: true},
	"deploy-retention-policy":    {Method: "POST", Path: "/AddRetentionCompliancePolicy", MultiTenant: true},
	"deploy-sensitivity-label":   {Method: "POST", Path: "/AddSensitivityLabel", MultiTenant: true},
	"deploy-sensitive-info-type": {Method: "POST", Path: "/AddSensitiveInfoType", MultiTenant: true},
	"deploy-named-location":      {Method: "POST", Path: "/AddNamedLocation", MultiTenant: true},
	"deploy-transport-rule":      {Method: "POST", Path: "/AddTransportRule", MultiTenant: true},
	"deploy-spam-filter":         {Method: "POST", Path: "/AddSpamFilter", MultiTenant: true},
	"deploy-connection-filter":   {Method: "POST", Path: "/AddConnectionFilter", MultiTenant: true},
	"deploy-quarantine-policy":   {Method: "POST", Path: "/AddQuarantinePolicy", MultiTenant: true},
	"deploy-safelinks-policy":    {Method: "POST", Path: "/AddSafeLinksPolicyFromTemplate", MultiTenant: true},
	"deploy-exchange-connector":  {Method: "POST", Path: "/AddExConnector", MultiTenant: true},
	"deploy-defender":            {Method: "POST", Path: "/AddDefenderDeployment", MultiTenant: true},
	"deploy-app-template":        {Method: "POST", Path: "/ExecDeployAppTemplate", MultiTenant: true},
	"deploy-contact-template":    {Method: "POST", Path: "/DeployContactTemplates", MultiTenant: true},
	"deploy-autopilot-config":    {Method: "POST", Path: "/AddAutopilotConfig", MultiTenant: true},
	"deploy-enrollment":          {Method: "POST", Path: "/AddEnrollment", MultiTenant: true},
	"deploy-office-app":          {Method: "POST", Path: "/AddOfficeApp", MultiTenant: true},
	"deploy-choco-app":           {Method: "POST", Path: "/AddChocoApp", MultiTenant: true},
	"deploy-store-app":           {Method: "POST", Path: "/AddStoreApp", MultiTenant: true},
	"deploy-msp-app":             {Method: "POST", Path: "/AddMSPApp", MultiTenant: true},
	"deploy-win32-script-app":    {Method: "POST", Path: "/AddWin32ScriptApp", MultiTenant: true},
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

// bulkBatch is one outgoing request. A per-user action produces one batch per
// row; a MultiTenant deploy action produces one batch for every row sharing the
// same action and params, carrying all their tenants in `selectedTenants` so
// CIPP fans out server-side.
type bulkBatch struct {
	Action   string
	Endpoint string
	Method   string
	Params   map[string]any
	Tenants  []string
	Indexes  []int
	// MultiTenant selects the request shape: selectedTenants in the body versus
	// a single tenantFilter query parameter.
	MultiTenant bool
}

// paramsFingerprint renders a row's params to a stable key so identical deploys
// across tenants collapse together. Marshal failures fall back to a unique key,
// which degrades to per-row calls rather than wrongly merging distinct payloads.
func paramsFingerprint(index int, params map[string]any) string {
	b, err := json.Marshal(params)
	if err != nil {
		return fmt.Sprintf("unmergeable-%d", index)
	}
	return string(b)
}

// collapseBulkRows groups rows into outgoing requests. Rows whose action is
// MultiTenant and whose params are identical merge into one call carrying every
// tenant; everything else stays one call per row.
//
// This is the difference between N throttled calls and one server-side fan-out
// for the deploy family, and Microsoft Graph throttling is the dominant failure
// mode for large batches.
func collapseBulkRows(rows []bulkRow) []bulkBatch {
	batches := make([]bulkBatch, 0, len(rows))
	merged := map[string]int{} // action+params -> index into batches

	for _, r := range rows {
		if r.parseError != "" {
			continue
		}
		def, known := bulkActions[strings.ToLower(r.Action)]
		if known && def.MultiTenant {
			key := strings.ToLower(r.Action) + "|" + paramsFingerprint(r.Index, r.Params)
			if at, ok := merged[key]; ok {
				batches[at].Tenants = append(batches[at].Tenants, r.Tenant)
				batches[at].Indexes = append(batches[at].Indexes, r.Index)
				continue
			}
			merged[key] = len(batches)
			batches = append(batches, bulkBatch{
				Action: r.Action, Endpoint: r.Endpoint, Method: r.Method,
				Params: r.Params, Tenants: []string{r.Tenant}, Indexes: []int{r.Index},
				MultiTenant: true,
			})
			continue
		}
		batches = append(batches, bulkBatch{
			Action: r.Action, Endpoint: r.Endpoint, Method: r.Method,
			Params: r.Params, Tenants: []string{r.Tenant}, Indexes: []int{r.Index},
		})
	}
	return batches
}

// bulkCheckpoint records completed row indices for resume, and the rows whose
// write came back conflicting (hand-fix handfixes.json bulk-conflict-needs-review):
// those are never resent automatically, with or without --resume. It holds
// only hashes and row numbers: the sha256 of the CSV bytes, the sha256 of the
// CSV's resolved path, completed row indices and needs-review row indices. No
// CIPP response and no CSV row content is written, in any mode (so nothing of
// CIPP's data lands on disk under CIPP_NO_LOCAL_DATA=1 either).
type bulkCheckpoint struct {
	CSVSHA256   string `json:"csv_sha256"`
	PathSHA256  string `json:"path_sha256"`
	Completed   []int  `json:"completed"`
	NeedsReview []int  `json:"needs_review,omitempty"`
}

// bulkResolvedPath is the CSV's absolute path with symlinks resolved.
func bulkResolvedPath(csvPath string) string {
	resolved := csvPath
	if abs, err := filepath.Abs(csvPath); err == nil {
		resolved = abs
	}
	if real, err := filepath.EvalSymlinks(resolved); err == nil {
		resolved = real
	}
	return resolved
}

func bulkCheckpointPath(csvPath string, contentSum []byte) (string, error) {
	dir, err := fanoutCacheDir()
	if err != nil {
		return "", err
	}
	base := strings.TrimSuffix(filepath.Base(csvPath), filepath.Ext(csvPath))
	if base == "" {
		base = "bulk"
	}
	// Hand-fix (handfixes.json bulk-checkpoint-per-file): a checkpoint belongs
	// to one CSV's exact bytes at one resolved path. Size and modification time
	// are not identity: an edit that keeps both must still start from row 0.
	h := sha256.New()
	_, _ = h.Write([]byte(bulkResolvedPath(csvPath)))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(contentSum)
	return filepath.Join(dir, "bulk-"+checkpointSlug(base)+"-"+hex.EncodeToString(h.Sum(nil)[:8])+".json"), nil
}

// bulkCheckpointWritable proves this CSV's checkpoint (or, before it exists,
// its directory) can be written before any write is sent, so a needs-review
// hold can never silently fail to persist.
func bulkCheckpointWritable(cpPath string) error {
	// An existing checkpoint is rewritten in place (os.WriteFile), so the file
	// itself must open for writing; O_WRONLY without O_TRUNC leaves it intact.
	// cpPath comes from bulkCheckpointPath. #nosec G304 -- internal cache path.
	if f, err := os.OpenFile(cpPath, os.O_WRONLY, 0); err == nil { // #nosec G304
		return f.Close()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(cpPath), ".bulk-probe-*")
	if err != nil {
		return err
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}

func readBulkCheckpoint(path string) (map[int]bool, map[int]bool, error) {
	done := map[int]bool{}
	review := map[int]bool{}
	// path comes from bulkCheckpointPath: a slugified filename under the
	// per-CLI cache dir, never user-supplied. #nosec G304 -- internal cache path.
	data, err := os.ReadFile(path) // #nosec G304

	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return done, review, nil
		}
		return nil, nil, err
	}
	var cp bulkCheckpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, nil, fmt.Errorf("parsing checkpoint %s: %w", path, err)
	}
	for _, i := range cp.Completed {
		done[i] = true
	}
	for _, i := range cp.NeedsReview {
		review[i] = true
	}
	return done, review, nil
}

func writeBulkCheckpoint(path string, csvSum []byte, csvPath string, done, review map[int]bool) error {
	sorted := func(m map[int]bool) []int {
		out := make([]int, 0, len(m))
		for i := range m {
			out = append(out, i)
		}
		sort.Ints(out)
		return out
	}
	pathSum := sha256.Sum256([]byte(bulkResolvedPath(csvPath)))
	cp := bulkCheckpoint{
		CSVSHA256:  hex.EncodeToString(csvSum),
		PathSHA256: hex.EncodeToString(pathSum[:]),
		Completed:  sorted(done),
	}
	if len(review) > 0 {
		cp.NeedsReview = sorted(review)
	}
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	// Checkpoint holds only hashes and row numbers; keep it user-private (0600).
	return os.WriteFile(path, data, 0o600)
}

func newNovelBulkCmd(flags *rootFlags) *cobra.Command {
	var flagFrom string
	var flagExecute bool
	var flagResume bool
	var flagConcurrency int

	cmd := &cobra.Command{
		Use:   "bulk",
		Short: "Drive create/edit/remove/offboard actions from a CSV with Retry-After backoff and resume-after-429 checkpointing.",
		// Not an MCP tool: --from reads a CSV off this machine's disk and
		// --execute writes without the MCP confirm step. MCP writes go through
		// cipp_execute, which previews until confirm is true.
		Annotations: map[string]string{"mcp:read-only": "false", "mcp:hidden": "true", "mcp:read-flags": "from", "pp:data-source": "live"},
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

			// Verify-safe: the verifier runs documented examples verbatim, and
			// the CSV they name does not exist in its sandbox. Report the plan
			// shape and exit 0 rather than failing on a missing fixture — a
			// filesystem read must never happen before the dry-run/verify
			// short-circuit.
			if cliutil.IsVerifyEnv() {
				fmt.Fprintf(cmd.OutOrStdout(), "would read actions from %s and plan one request per row (no writes)\n", flagFrom)
				return nil
			}

			// flagFrom is the user's own --from CSV path; opening the file the
			// operator named is the feature. #nosec G304 -- user-supplied input file.
			f, err := os.Open(flagFrom) // #nosec G304
			if err != nil {
				return fmt.Errorf("opening CSV %q: %w", flagFrom, err)
			}
			defer f.Close()
			// Hand-fix (handfixes.json bulk-checkpoint-per-file): hash every
			// byte of the CSV as it is parsed; the resume checkpoint is keyed
			// on it, so any edit to the file starts a fresh checkpoint.
			csvHash := sha256.New()
			rows, err := parseBulkCSV(io.TeeReader(f, csvHash))
			if err != nil {
				return err
			}
			if _, err := io.Copy(csvHash, f); err != nil {
				return fmt.Errorf("reading CSV %q: %w", flagFrom, err)
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

			// Checkpoint. Completed rows are skipped only with --resume; rows
			// marked needs-review are never resent, with or without it.
			csvSum := csvHash.Sum(nil)
			cpPath, err := bulkCheckpointPath(flagFrom, csvSum)
			if err != nil {
				return err
			}
			// Hand-fix (handfixes.json bulk-conflict-needs-review): refuse to
			// start when a needs-review hold could not be saved.
			if err := bulkCheckpointWritable(cpPath); err != nil {
				return fmt.Errorf("bulk --execute needs a writable checkpoint (%s) to hold rows that need review; nothing was sent: %w", cpPath, err)
			}
			fileCompleted, review, err := readBulkCheckpoint(cpPath)
			if err != nil {
				return err
			}
			completed := map[int]bool{}
			if flagResume {
				completed = fileCompleted
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			_ = flagConcurrency // executes sequentially; writes are ordered and 429-sensitive

			var firstErr error
			executed := 0
			for _, b := range collapseBulkRows(rows) {
				// Hand-fix (handfixes.json bulk-conflict-needs-review): a row
				// whose write came back conflicting may already have taken
				// effect, so it is never sent again automatically.
				var held []int
				for _, idx := range b.Indexes {
					if review[idx] {
						held = append(held, idx)
					}
				}
				if len(held) > 0 {
					fmt.Fprintf(cmd.ErrOrStderr(), "rows %v (%s): needs review, not sent again (CIPP reported success and a failure for this write; check it in CIPP, then edit the CSV to send it again)\n",
						held, b.Action)
					if firstErr == nil {
						firstErr = partialFailureErr(fmt.Errorf("rows %v need review: CIPP reported success and a failure for this write", b.Indexes))
					}
					continue
				}
				// A batch is skipped only when EVERY row in it is already
				// checkpointed; a partially-completed merged batch re-sends to
				// the whole set, which is safe because these deploys are
				// idempotent applies.
				if flagResume {
					allDone := true
					for _, idx := range b.Indexes {
						if !completed[idx] {
							allDone = false
							break
						}
					}
					if allDone {
						continue
					}
				}

				body := map[string]any{}
				for k, v := range b.Params {
					body[k] = v
				}
				params := map[string]string{}
				label := b.Tenants[0]
				if b.MultiTenant {
					// One server-side fan-out instead of len(Tenants) calls.
					body["selectedTenants"] = b.Tenants
					label = fmt.Sprintf("%d tenants", len(b.Tenants))
				} else {
					params["tenantFilter"] = b.Tenants[0]
				}

				data, status, err := c.PostWithParams(ctx, b.Endpoint, params, body)
				if err != nil {
					var rle *cliutil.RateLimitError
					if errors.As(err, &rle) {
						fmt.Fprintf(cmd.ErrOrStderr(), "rows %v (%s/%s): %v\n", b.Indexes, b.Action, label, rle)
					} else {
						fmt.Fprintf(cmd.ErrOrStderr(), "rows %v (%s/%s): %v\n", b.Indexes, b.Action, label, err)
					}
					if firstErr == nil {
						firstErr = err
					}
					continue
				}

				// CIPP hardcodes HTTP 200 on several write entrypoints and
				// reports the failure as prose inside `Results`. Without this
				// check a failed deploy is indistinguishable from a successful
				// one and the batch would report success.
				verdict, msg, _ := cippResultsVerdict(data)
				if verdict == cippConflicting {
					for _, idx := range b.Indexes {
						review[idx] = true
					}
					fmt.Fprintf(cmd.ErrOrStderr(), "rows %v (%s/%s): HTTP %d but %s; marked needs-review and will not be resent\n",
						b.Indexes, b.Action, label, status, msg)
					// Save the hold now: a later failure must not lose it.
					keep := fileCompleted
					if flagResume {
						keep = completed
					}
					if err := writeBulkCheckpoint(cpPath, csvSum, flagFrom, keep, review); err != nil {
						return fmt.Errorf("rows %v need review, and the hold could not be saved (%v); stopping so nothing is resent", b.Indexes, err)
					}
					if firstErr == nil {
						firstErr = partialFailureErr(fmt.Errorf("%s", msg))
					}
					continue
				}
				if verdict == cippFailure {
					fmt.Fprintf(cmd.ErrOrStderr(), "rows %v (%s/%s): HTTP %d but %s\n",
						b.Indexes, b.Action, label, status, msg)
					if firstErr == nil {
						firstErr = partialFailureErr(fmt.Errorf("%s", msg))
					}
					continue
				}

				executed += len(b.Indexes)
				if flagResume {
					for _, idx := range b.Indexes {
						completed[idx] = true
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "executed rows %v: %s %s (%s) -> HTTP %d\n",
					b.Indexes, b.Method, b.Endpoint, label, status)
			}

			if flagResume {
				if err := writeBulkCheckpoint(cpPath, csvSum, flagFrom, completed, review); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warn: writing checkpoint: %v\n", err)
				}
			}

			if firstErr != nil {
				return classifyAPIError(cmd.ErrOrStderr(), firstErr, flags)
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
