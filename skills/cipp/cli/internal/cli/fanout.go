// Copyright 2026 damienstevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"cipp-pp-cli/internal/cliutil"
	"cipp-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

// resourceTypeForEndpoint derives the store resource_type from a CIPP endpoint
// path: strip the leading slash, strip a leading "List" verb, lowercase.
// "/ListUsers" -> "users", "/ListConditionalAccessPolicies" ->
// "conditionalaccesspolicies". Endpoints with no recognizable noun fall back
// to the lowercased trailing path segment.
func resourceTypeForEndpoint(endpoint string) string {
	e := strings.TrimSpace(endpoint)
	e = strings.TrimLeft(e, "/")
	// Use the last path segment if the endpoint has multiple.
	if i := strings.LastIndex(e, "/"); i >= 0 {
		e = e[i+1:]
	}
	// Drop a query string if one slipped in.
	if i := strings.Index(e, "?"); i >= 0 {
		e = e[:i]
	}
	if strings.HasPrefix(e, "List") {
		e = strings.TrimPrefix(e, "List")
	}
	return strings.ToLower(e)
}

// fanoutTenantResult is one tenant's slice of the aggregated fan-out output.
type fanoutTenantResult struct {
	Tenant string          `json:"tenant"`
	Data   json.RawMessage `json:"data,omitempty"`
	Count  int             `json:"count"`
	Error  string          `json:"error,omitempty"`
}

// checkpointSlug renders an endpoint into a filesystem-safe slug for the
// resume checkpoint filename.
func checkpointSlug(endpoint string) string {
	s := strings.ToLower(strings.Trim(endpoint, "/"))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "endpoint"
	}
	return out
}

// cacheDir returns the per-CLI cache directory used for resume checkpoints,
// creating it if necessary.
func fanoutCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".cache", "cipp-cli")
	// Resume checkpoints can record tenant domains, so keep the cache dir
	// user-private (0700) rather than world-readable.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// checkpoint is the on-disk resume state: which tenant domains already
// completed for a given endpoint.
type fanoutCheckpoint struct {
	Endpoint  string   `json:"endpoint"`
	Completed []string `json:"completed"`
}

func fanoutCheckpointPath(endpoint string) (string, error) {
	dir, err := fanoutCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "fanout-"+checkpointSlug(endpoint)+".json"), nil
}

// readFanoutCheckpoint loads the set of already-completed tenant domains. A
// missing file is not an error — it means "nothing done yet".
func readFanoutCheckpoint(path string) (map[string]bool, error) {
	done := map[string]bool{}
	// path comes from fanoutCheckpointPath: a slugified filename under the
	// per-CLI cache dir, never user-supplied. #nosec G304 -- internal cache path.
	data, err := os.ReadFile(path) // #nosec G304

	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return done, nil
		}
		return nil, err
	}
	var cp fanoutCheckpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("parsing checkpoint %s: %w", path, err)
	}
	for _, d := range cp.Completed {
		done[d] = true
	}
	return done, nil
}

// writeFanoutCheckpoint persists the completed tenant set for resume.
func writeFanoutCheckpoint(path, endpoint string, done map[string]bool) error {
	completed := make([]string, 0, len(done))
	for d := range done {
		completed = append(completed, d)
	}
	sort.Strings(completed)
	data, err := json.MarshalIndent(fanoutCheckpoint{Endpoint: endpoint, Completed: completed}, "", "  ")
	if err != nil {
		return err
	}
	// Checkpoint records tenant domains; keep it user-private (0600).
	return os.WriteFile(path, data, 0o600)
}

// itemsFromResponse normalizes a CIPP response into a slice of items: a bare
// array is returned as-is; a single object becomes a one-element slice.
func itemsFromResponse(data json.RawMessage) []json.RawMessage {
	var arr []json.RawMessage
	if json.Unmarshal(data, &arr) == nil {
		return arr
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}
	return []json.RawMessage{data}
}

// stampTenant adds a "_tenant" field to each item's JSON so saved rows carry
// the tenant they came from (the posture/waste/stale analytics group by it).
func stampTenant(items []json.RawMessage, tenant string) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(items))
	for _, it := range items {
		var obj map[string]any
		if json.Unmarshal(it, &obj) != nil {
			out = append(out, it)
			continue
		}
		obj["_tenant"] = tenant
		if b, err := json.Marshal(obj); err == nil {
			out = append(out, b)
		} else {
			out = append(out, it)
		}
	}
	return out
}

func newNovelFanoutCmd(flags *rootFlags) *cobra.Command {
	var flagEndpoint string
	var flagAllTenants bool
	var flagTenant string
	var flagConcurrency int
	var flagSave bool
	var flagResume bool
	var flagLocal bool
	var flagDB string

	cmd := &cobra.Command{
		Use:         "fanout",
		Short:       "Run any read command across every client tenant at once, with throttle-aware backoff and resume after a halt.",
		Annotations: map[string]string{"mcp:read-only": "false"},
		Long: `Fan a CIPP read endpoint out across every tenant in the fleet.

CIPP scopes nearly every endpoint by a tenantFilter query param. The UI forces
one tenant at a time; fanout amplifies a single read across the whole fleet
concurrently, with adaptive rate limiting, optional persistence to the local
store, and resume-after-halt checkpointing.

Examples:
  cipp-cli fanout --endpoint /ListUsers --all-tenants
  cipp-cli fanout --endpoint /ListUsers --tenant contoso.onmicrosoft.com,fabrikam.onmicrosoft.com
  cipp-cli fanout --endpoint /ListUsers --all-tenants --save
  cipp-cli fanout --endpoint /ListUsers --all-tenants --resume`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && flagEndpoint == "" {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if flagEndpoint == "" {
				return usageErr(fmt.Errorf("--endpoint is required"))
			}
			if flagTenant == "" && !flagLocal && !flagAllTenants {
				return usageErr(fmt.Errorf("specify --all-tenants to fan out across every tenant, or --tenant <csv> to target specific tenants"))
			}
			endpoint := flagEndpoint
			if !strings.HasPrefix(endpoint, "/") {
				endpoint = "/" + endpoint
			}

			ctx := cmd.Context()

			// Resolve the tenant list.
			var tenants []tenantRef
			switch {
			case flagTenant != "":
				for _, d := range cliutil.SplitCSV(flagTenant) {
					tenants = append(tenants, tenantRef{DefaultDomainName: d, DisplayName: d})
				}
			case flagLocal:
				ts, err := loadTenantsFromStore(ctx, flagDB)
				if err != nil {
					return err
				}
				tenants = ts
			default:
				c, err := flags.newClient()
				if err != nil {
					return err
				}
				ts, err := fetchTenants(ctx, c)
				if err != nil {
					return classifyAPIError(err, flags)
				}
				tenants = ts
			}
			if len(tenants) == 0 {
				return fmt.Errorf("no tenants resolved; pass --tenant <csv>, sync tenants for --local, or check API access")
			}

			// Resume: skip already-completed tenants.
			var cpPath string
			completed := map[string]bool{}
			if flagResume {
				p, err := fanoutCheckpointPath(endpoint)
				if err != nil {
					return err
				}
				cpPath = p
				done, err := readFanoutCheckpoint(cpPath)
				if err != nil {
					return err
				}
				completed = done
			}

			pending := make([]tenantRef, 0, len(tenants))
			for _, t := range tenants {
				if flagResume && completed[t.DefaultDomainName] {
					continue
				}
				pending = append(pending, t)
			}

			conc := flagConcurrency
			if conc <= 0 {
				conc = 5
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			// Fan out.
			results, errs := cliutil.FanoutRun(
				ctx,
				pending,
				func(t tenantRef) string { return t.DefaultDomainName },
				func(ctx context.Context, t tenantRef) (json.RawMessage, error) {
					return c.Get(ctx, endpoint, map[string]string{"tenantFilter": t.DefaultDomainName})
				},
				cliutil.WithConcurrency(conc),
			)

			// Aggregate in tenant order. Index results/errs by source.
			resByTenant := map[string]json.RawMessage{}
			for _, r := range results {
				resByTenant[r.Source] = r.Value
			}
			errByTenant := map[string]error{}
			for _, e := range errs {
				errByTenant[e.Source] = e.Err
			}

			agg := make([]fanoutTenantResult, 0, len(pending))
			var saveErr error
			var db *store.Store
			if flagSave {
				dbPath := flagDB
				if dbPath == "" {
					dbPath = defaultDBPath("cipp-cli")
				}
				d, err := store.OpenWithContext(ctx, dbPath)
				if err != nil {
					return fmt.Errorf("opening local database: %w", err)
				}
				db = d
				defer db.Close()
			}
			resourceType := resourceTypeForEndpoint(endpoint)
			// One shared snapshot timestamp per fanout run so the standards
			// history groups every tenant's rows under the same snapshot.
			snapshotTS := time.Now().UTC().Format(time.RFC3339)

			for _, t := range pending {
				row := fanoutTenantResult{Tenant: t.DefaultDomainName}
				if err, ok := errByTenant[t.DefaultDomainName]; ok && err != nil {
					var rle *cliutil.RateLimitError
					if errors.As(err, &rle) {
						row.Error = rle.Error()
					} else {
						row.Error = err.Error()
					}
					agg = append(agg, row)
					continue
				}
				data := resByTenant[t.DefaultDomainName]
				items := itemsFromResponse(data)
				row.Data = data
				row.Count = len(items)

				saveFailed := false
				if flagSave && db != nil && len(items) > 0 {
					stamped := stampTenant(items, t.DefaultDomainName)
					if _, _, err := db.UpsertBatch(resourceType, stamped); err != nil {
						saveErr = err
						saveFailed = true
					} else if resourceType == "standards" {
						// Append-only history so `standards drift` has two
						// comparable snapshots; the generic upsert above
						// overwrites in place and cannot carry history.
						if err := appendStandardsSnapshot(ctx, db, snapshotTS, t.DefaultDomainName, items); err != nil {
							saveErr = err
							saveFailed = true
						}
					}
				}
				// Never checkpoint a tenant whose save failed: a resumed run
				// must retry it, otherwise its data is silently lost forever.
				if flagResume && !saveFailed {
					completed[t.DefaultDomainName] = true
				}
				agg = append(agg, row)
			}

			// Surface per-tenant errors to stderr so none are silently dropped.
			cliutil.FanoutReportErrors(cmd.ErrOrStderr(), errs)

			// Persist checkpoint.
			if flagResume && cpPath != "" {
				if err := writeFanoutCheckpoint(cpPath, endpoint, completed); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warn: writing checkpoint: %v\n", err)
				}
			}
			if saveErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warn: some tenant data failed to save: %v\n", saveErr)
			}

			if flags.asJSON {
				return flags.printJSON(cmd, agg)
			}
			headers := []string{"TENANT", "ITEMS", "ERROR"}
			rows := make([][]string, 0, len(agg))
			for _, r := range agg {
				rows = append(rows, []string{r.Tenant, fmt.Sprintf("%d", r.Count), r.Error})
			}
			return flags.printTable(cmd, headers, rows)
		},
	}
	cmd.Flags().StringVar(&flagEndpoint, "endpoint", "", "CIPP read endpoint path to fan out (e.g. /ListUsers)")
	cmd.Flags().BoolVar(&flagAllTenants, "all-tenants", false, "Run across every tenant in the fleet")
	cmd.Flags().StringVar(&flagTenant, "tenant", "", "Comma-separated tenant defaultDomainNames to target (overrides --all-tenants)")
	cmd.Flags().IntVar(&flagConcurrency, "concurrency", 5, "Number of tenants to query concurrently")
	cmd.Flags().BoolVar(&flagSave, "save", false, "Persist each tenant's returned items into the local store")
	cmd.Flags().BoolVar(&flagResume, "resume", false, "Skip tenants already completed in a prior run (checkpoint)")
	cmd.Flags().BoolVar(&flagLocal, "local", false, "Resolve the tenant list from the local store instead of the live API")
	cmd.Flags().StringVar(&flagDB, "db", "", "Path to the local SQLite database (default: standard location)")
	return cmd
}
