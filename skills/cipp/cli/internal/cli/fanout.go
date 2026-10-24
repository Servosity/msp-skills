// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"cipp-pp-cli/internal/client"
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
	Tenant   string          `json:"tenant"`
	Endpoint string          `json:"endpoint,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
	Count    int             `json:"count"`
	Error    string          `json:"error,omitempty"`
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
	// The state dir follows CIPP_HOME / CIPP_STATE_DIR relocation like every
	// other path this CLI writes.
	state, err := cliutil.StateDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(state, "checkpoints")
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

// itemsFromResponse normalizes a CIPP read response into its rows: a bare
// array as-is, or the array of objects inside a {"Results": [...]} (or
// data/value/items) envelope; any other object is one row (shown; fanoutSaveRows
// refuses to store it). CIPP answers HTTP 200 with a message on failure
// ({"Results": "Access denied"}), so a Results value that is a string, or a
// list of strings, is never rows: it is a tenant error, and the tenant's stored
// data is kept. Hand-fix (handfixes.json tenant-list-message-is-not-a-fleet):
// the whole response, envelope and every row, goes through the one document
// walker (cippScan) first, and any failure signal anywhere refuses it. In a
// list, failure wording counts only under a status-like key or in a bare
// message, so a group named "Denied Access" is a row. The trade-off is
// deliberate: a list whose rows report their own failed status is refused; run
// fanout with --json to see what CIPP returned.
func itemsFromResponse(data json.RawMessage) ([]json.RawMessage, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil
	}
	doc, ok, derr := cippDocument(data)
	if derr != nil {
		return nil, derr
	}
	if ok {
		if failure := cippResponseFailure(doc, nil); failure != "" {
			return nil, fmt.Errorf("CIPP returned a failure in the response: %s", failure)
		}
	}
	var arr []json.RawMessage
	if json.Unmarshal(data, &arr) == nil {
		if err := rowsCarryMessages(arr); err != nil {
			return nil, err
		}
		return arr, nil
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(data, &obj) != nil {
		return []json.RawMessage{data}, nil
	}
	for _, want := range tenantEnvelopeKeys {
		for k, v := range obj {
			if !strings.EqualFold(k, want) {
				continue
			}
			if json.Unmarshal(v, &arr) == nil {
				var msgs []string
				if len(arr) > 0 && json.Unmarshal(v, &msgs) == nil {
					return nil, fmt.Errorf("CIPP returned messages instead of rows: %s", strings.Join(msgs, "; "))
				}
				if err := rowsCarryMessages(arr); err != nil {
					return nil, err
				}
				return arr, nil
			}
			var msg string
			if json.Unmarshal(v, &msg) == nil {
				return nil, fmt.Errorf("CIPP returned a message instead of rows: %s", msg)
			}
		}
	}
	return []json.RawMessage{data}, nil
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
		if !hasUniqueIDField(obj) {
			// The store keys a row on id, then _id, then slug/name. CIPP report
			// rows (mailbox forwarding, inbox rules) often carry no id and a
			// Name that repeats across mailboxes, so give them a stable _id.
			obj["_id"] = syntheticRowKey(obj)
		}
		if b, err := json.Marshal(obj); err == nil {
			out = append(out, b)
		} else {
			out = append(out, it)
		}
	}
	return out
}

// rowsCarryMessages fails a rows array that mixes objects with strings: CIPP
// appends a message to a partial result ([{...}, "Retrieved 50 of 120"]), and
// saving the partial set as complete would delete the rows it did not return.
// Failure wording anywhere was already refused by the document walker; an
// all-string array without it passes through as data.
func rowsCarryMessages(arr []json.RawMessage) error {
	var msgs []string
	objects := 0
	for _, it := range arr {
		var s string
		if json.Unmarshal(it, &s) == nil {
			msgs = append(msgs, s)
			continue
		}
		objects++
	}
	if objects > 0 && len(msgs) > 0 {
		return fmt.Errorf("CIPP returned a partial result with messages: %s", strings.Join(msgs, "; "))
	}
	return nil
}

// fanoutIdentityKeys name what makes an element a row of an endpoint for
// --save, matched case-insensitively (hand-fix handfixes.json
// fanout-save-integrity). The default is an id-class field (fanoutDefaultIdentity);
// endpoints whose rows carry none are named from the CIPP-API source
// (Invoke-ListMailboxForwarding: UPN; Invoke-ListStandards: displayName;
// Invoke-ListGDAPRoles: GroupId; Get-CIPPLicenseOverview: skuId).
var fanoutIdentityKeys = map[string][]string{
	"/ListMailboxForwarding": {"UPN", "PrimarySmtpAddress"},
	"/ListStandards":         {"displayName"},
	"/ListGDAPRoles":         {"GroupId"},
	"/ListLicenses":          {"skuId"},
}

var fanoutDefaultIdentity = []string{"id", "_id", "id_", "uuid", "guid", "Identity", "RowKey"}

// fanoutSaveRows is what fanout --save may store for one tenant: only a
// well-formed list, a JSON array or an envelope whose Results is an array,
// whose every element is an object carrying the endpoint's identity key
// (fanoutIdentityKeys). The body is decoded strictly (a duplicate key refuses
// it). JSON null, an empty body, a lone object, a string or any other shape is
// refused:
// it is shown, and the tenant's stored rows stay (hand-fix handfixes.json
// fanout-save-integrity).
func fanoutSaveRows(endpoint string, data json.RawMessage) ([]json.RawMessage, error) {
	notList := fmt.Errorf("not saved: CIPP's answer is not a list of rows (a JSON array, or an envelope whose Results is an array); the stored rows are kept")
	doc, ok, err := cippDocument(data)
	if err != nil {
		return nil, fmt.Errorf("not saved: %w; the stored rows are kept", err)
	}
	if !ok {
		return nil, notList
	}
	var rows []any
	switch x := doc.(type) {
	case []any:
		rows = x
	case map[string]any:
		found := false
		for k, v := range x {
			if strings.EqualFold(k, "Results") {
				rows, found = v.([]any)
				break
			}
		}
		if !found {
			return nil, notList
		}
	default:
		// JSON null, a string, a number or a boolean.
		return nil, notList
	}
	keys := fanoutIdentityKeys[endpoint]
	if len(keys) == 0 {
		keys = fanoutDefaultIdentity
	}
	out := make([]json.RawMessage, 0, len(rows))
	for _, it := range rows {
		m, isObj := it.(map[string]any)
		if !isObj || tenantFieldLookup(m, keys...) == "" {
			return nil, fmt.Errorf("not saved: an element of CIPP's answer is not a row with %s: %s; the stored rows are kept", strings.Join(keys, "/"), cippEntryText(it))
		}
		b, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

// saveTenantRefs replaces the stored tenant list with the one just resolved
// from /ListTenants.
func saveTenantRefs(db *store.Store, tenants []tenantRef) error {
	rows := make([]json.RawMessage, 0, len(tenants))
	for _, t := range tenants {
		b, err := json.Marshal(map[string]any{"id": t.DefaultDomainName, "defaultDomainName": t.DefaultDomainName, "displayName": t.DisplayName, "customerId": t.CustomerID})
		if err != nil {
			return err
		}
		rows = append(rows, b)
	}
	if _, err := db.DB().Exec(`DELETE FROM resources WHERE resource_type = 'tenants'`); err != nil {
		return err
	}
	_, _, err := db.UpsertBatch("tenants", rows)
	return err
}

// hasUniqueIDField reports whether a row carries an id-class key the store
// would extract ahead of _id (lookupField is case-insensitive, so "id" covers
// Id and ID).
func hasUniqueIDField(obj map[string]any) bool {
	for _, k := range []string{"id", "_id", "id_", "uuid"} {
		if v, ok := lookupField(obj, k); ok && v != nil && fmt.Sprint(v) != "" {
			return true
		}
	}
	return false
}

// syntheticRowKey derives a key for a row with no id: an Exchange Identity or
// table RowKey when present (unique per object), else a hash of the row. A
// mailbox UPN is NOT a key: one mailbox has many inbox rules. The store adds
// _tenant on top, and fanout replaces a tenant's rows on every save, so a
// content hash never accumulates stale rows.
func syntheticRowKey(obj map[string]any) string {
	for _, k := range []string{"Identity", "RowKey"} {
		if v, ok := lookupField(obj, k); ok {
			if s := strings.TrimSpace(fmt.Sprint(v)); s != "" && v != nil {
				return k + ":" + s
			}
		}
	}
	raw, _ := json.Marshal(obj)
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:12])
}

// fanoutEndpointParams are query parameters fanout always sends for an
// endpoint. /ListUsers omits sign-in activity unless asked, and without it
// 'users stale' and 'licenses waste' cannot tell idle from unknown.
var fanoutEndpointParams = map[string]map[string]string{
	"/ListUsers": {"IncludeLogonDetails": "true"},
}

// fanoutParams is the query for one tenant's request.
func fanoutParams(endpoint, tenant string) map[string]string {
	params := map[string]string{"tenantFilter": tenant}
	for k, v := range fanoutEndpointParams[endpoint] {
		params[k] = v
	}
	return params
}

// fanoutPresets are named endpoint bundles for populating the local store.
//
// CIPP exposes no bulk-list endpoints, so `sync` is a structural no-op and
// fan-out is the only population path. A live-verification report (issue #201)
// found an operator had to derive that mechanism from scratch, so the common
// bundles are named here rather than left to be reassembled by hand.
//
// Every endpoint below is present in the current CIPP OpenAPI surface.
var fanoutPresets = map[string][]string{
	// /ListTenants is partner-wide and POST-only, so it is not fanned out per
	// tenant; fanout --save stores the resolved tenant list itself.
	"core": {
		"/ListUsers",
		"/ListLicenses",
	},
	"posture": {
		"/ListMFAUsers",
		"/ListConditionalAccessPolicies",
		"/ListStandards",
		"/ListBPA",
	},
	"compliance": {
		"/ListDlpCompliancePolicy",
		"/ListRetentionCompliancePolicy",
		"/ListSensitivityLabel",
		"/ListSensitiveInfoType",
	},
	"gdap": {
		"/ListGDAPRelationships",
		"/ListGDAPInvite",
		"/ListGDAPRoles",
	},
	// What 'users offboard-check' joins: disabled users against forwarding,
	// inbox rules and directory role membership.
	"offboarding": {
		"/ListUsers",
		"/ListMailboxForwarding",
		"/ListMailboxRules",
		"/ListRoles",
	},
}

// fanoutReadEndpoint matches the CIPP read endpoints fanout may drive. CIPP
// names its reads List*/Get*; 39 of its GET endpoints are actions (Exec*,
// Remove*) that change state, and fanout would otherwise run one of them once
// per tenant with no confirmation. A strict path shape also keeps a query
// string or traversal segment out of the request.
var fanoutReadEndpoint = regexp.MustCompile(`^/(?:(?:List|list|Get)[A-Za-z0-9_]*|BestPracticeAnalyser_List|PublicPing)$`)

// checkFanoutEndpoint refuses any endpoint that is not a CIPP read.
func checkFanoutEndpoint(e string) error {
	if fanoutReadEndpoint.MatchString(e) {
		return nil
	}
	return fmt.Errorf("refusing %s: fanout only drives CIPP read endpoints (List*, Get*); action endpoints change state, so call the typed command for that endpoint instead", e)
}

// checkCippReadPath refuses a resource path that is not a CIPP read, for the
// generated commands (export, tail) that GET a resource by name.
func checkCippReadPath(path string) error {
	if fanoutReadEndpoint.MatchString(path) {
		return nil
	}
	return fmt.Errorf("refusing %s: only CIPP read endpoints (List*, Get*) can be exported or tailed; action endpoints change state, so call the typed command for that endpoint instead", path)
}

// fanoutPresetNames returns the preset keys in stable order for help text.
func fanoutPresetNames() []string {
	names := make([]string, 0, len(fanoutPresets))
	for k := range fanoutPresets {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// normalizeEndpointPath ensures a leading slash so `/ListUsers` and `ListUsers`
// behave identically.
func normalizeEndpointPath(e string) string {
	e = strings.TrimSpace(e)
	if e == "" {
		return ""
	}
	if !strings.HasPrefix(e, "/") {
		return "/" + e
	}
	return e
}

// resolveFanoutEndpoints expands --preset and --endpoint into the ordered,
// de-duplicated endpoint list to fan out. Both may be supplied together; the
// explicit --endpoint values come first.
func resolveFanoutEndpoints(endpointCSV, preset string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	add := func(e string) {
		e = normalizeEndpointPath(e)
		if e == "" || seen[e] {
			return
		}
		seen[e] = true
		out = append(out, e)
	}
	for _, e := range cliutil.SplitCSV(endpointCSV) {
		add(e)
	}
	for _, e := range out {
		if err := checkFanoutEndpoint(e); err != nil {
			return nil, err
		}
	}
	if preset != "" {
		endpoints, ok := fanoutPresets[strings.ToLower(strings.TrimSpace(preset))]
		if !ok {
			return nil, fmt.Errorf("unknown --preset %q: must be one of %s", preset, strings.Join(fanoutPresetNames(), ", "))
		}
		for _, e := range endpoints {
			add(e)
		}
	}
	return out, nil
}

func newNovelFanoutCmd(flags *rootFlags) *cobra.Command {
	var flagEndpoint string
	var flagPreset string
	var flagAllTenants bool
	var flagTenant string
	var flagConcurrency int
	var flagSave bool
	var flagResume bool
	var flagLocal bool
	var flagDB string

	cmd := &cobra.Command{
		Use:         "fanout",
		Short:       "Run any CIPP read endpoint across every client tenant at once and save the results locally.",
		Annotations: map[string]string{"mcp:read-only": "false", "pp:data-source": "live"},
		Long: `Fan a CIPP read endpoint out across every tenant in the fleet.

Use this command to run any CIPP read endpoint across tenants, or to populate
the local store with --save. Do NOT use this command for a computed fleet
rollup; use 'posture' or 'scorecard' instead.

Most CIPP endpoints take a tenantFilter query param: one tenant, or
AllTenants for CIPP's own estate-wide answer. fanout runs one read per
tenant across the tenants you choose, concurrently, with adaptive rate
limiting, optional persistence to the local store, and resume-after-halt
checkpointing. Only read endpoints (List*, Get*) are accepted.

This CLI has no sync command: fanout --save is how the local store that 'posture', 'scorecard', 'ca diff',
'licenses waste', 'users stale', 'users offboard-check', 'standards drift' and
'changes' read gets populated. Each save also appends a snapshot that
'changes' compares over time.

Presets (--preset) are named endpoint bundles: ` + strings.Join(fanoutPresetNames(), ", ") + `

Examples:
  cipp-cli fanout --endpoint /ListUsers --all-tenants
  cipp-cli fanout --endpoint /ListUsers,/ListLicenses --all-tenants --save
  cipp-cli fanout --preset core --all-tenants --save
  cipp-cli fanout --endpoint /ListUsers --tenant contoso.onmicrosoft.com,fabrikam.onmicrosoft.com
  cipp-cli fanout --endpoint /ListUsers --all-tenants --resume`,
		Example: "  cipp-cli fanout --endpoint /ListUsers --all-tenants --save --agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fanout")
			}
			// Hand-fix (handfixes.json fanout-save-honors-no-local-data): the
			// operator's no-local-copy switch outranks --save, which would
			// write every tenant's rows to the local store, and --resume,
			// which writes the tenant domains to a checkpoint file.
			if flagSave && localDataDisabled() {
				return usageErr(fmt.Errorf("--save is disabled while %s=1: it would save API data to the local store; run fanout without --save for a live read", noLocalDataEnvVar))
			}
			if flagResume && localDataDisabled() {
				return usageErr(fmt.Errorf("--resume is disabled while %s=1: it would save tenant names to a checkpoint file; run fanout without --resume", noLocalDataEnvVar))
			}
			endpoints, err := resolveFanoutEndpoints(flagEndpoint, flagPreset)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(err)
			}
			if len(endpoints) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--endpoint or --preset is required"))
			}
			if flagTenant == "" && !flagLocal && !flagAllTenants {
				return usageErr(fmt.Errorf("specify --all-tenants to fan out across every tenant, or --tenant <csv> to target specific tenants"))
			}

			ctx := cmd.Context()

			// Resolve the tenant list ONCE and reuse it for every endpoint, so a
			// multi-endpoint preset costs one tenant lookup rather than N.
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
					return classifyAPIError(cmd.ErrOrStderr(), err, flags)
				}
				tenants = ts
			}
			if len(tenants) == 0 {
				return fmt.Errorf("no tenants resolved; pass --tenant <csv>, sync tenants for --local, or check API access")
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			conc := flagConcurrency
			if conc <= 0 {
				conc = 5
			}

			// Open the store once for the whole run when saving. Key every saved
			// type on (id, _tenant) BEFORE the open, so the store's open-time
			// migration also re-keys rows an older build saved under a bare id.
			var db *store.Store
			if flagSave {
				for _, e := range endpoints {
					store.TenantScopeResource(resourceTypeForEndpoint(e))
				}
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
				// Store the tenant list this run resolved, so 'gdap expiring'
				// and 'scorecard' can map domains to customer tenant ids.
				if flagTenant == "" && !flagLocal {
					if err := saveTenantRefs(db, tenants); err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "warn: saving tenant list: %v\n", err)
					}
				}
			}

			allAgg := make([]fanoutTenantResult, 0, len(tenants)*len(endpoints))
			for _, endpoint := range endpoints {
				agg, err := runFanoutForEndpoint(ctx, cmd, flags, c, db, endpoint, tenants, conc, flagSave, flagResume)
				if err != nil {
					return err
				}
				allAgg = append(allAgg, agg...)
			}
			// Every tenant failing is a failed command, not an empty success.
			failed := 0
			for _, r := range allAgg {
				if r.Error != "" {
					failed++
				}
			}
			if len(allAgg) > 0 && failed == len(allAgg) {
				if flags.asJSON {
					_ = printNovelJSON(cmd, flags, allAgg)
				}
				return apiErr(fmt.Errorf("all %d tenant request(s) failed; see the per-tenant errors above", failed))
			}

			if flags.asJSON {
				return printNovelJSON(cmd, flags, allAgg)
			}
			headers := []string{"ENDPOINT", "TENANT", "ITEMS", "ERROR"}
			rows := make([][]string, 0, len(allAgg))
			for _, r := range allAgg {
				rows = append(rows, []string{r.Endpoint, r.Tenant, fmt.Sprintf("%d", r.Count), r.Error})
			}
			return flags.printTable(cmd, headers, rows)
		},
	}
	cmd.Flags().StringVar(&flagEndpoint, "endpoint", "", "CIPP read endpoint path(s) to fan out, comma-separated (e.g. /ListUsers)")
	cmd.Flags().StringVar(&flagPreset, "preset", "", "Named endpoint bundle to fan out: "+strings.Join(fanoutPresetNames(), ", "))
	cmd.Flags().BoolVar(&flagAllTenants, "all-tenants", false, "Run across every tenant in the fleet")
	cmd.Flags().StringVar(&flagTenant, "tenant", "", "Comma-separated tenant defaultDomainNames to target (overrides --all-tenants)")
	cmd.Flags().IntVar(&flagConcurrency, "concurrency", 5, "Number of tenants to query concurrently")
	cmd.Flags().BoolVar(&flagSave, "save", false, "Persist each tenant's returned items into the local store")
	cmd.Flags().BoolVar(&flagResume, "resume", false, "Skip tenants already completed in a prior run (checkpoint)")
	cmd.Flags().BoolVar(&flagLocal, "local", false, "Resolve the tenant list from the local store instead of the live API")
	cmd.Flags().StringVar(&flagDB, "db", "", "Path to the local SQLite database (default: standard location)")
	return cmd
}

// runFanoutForEndpoint fans one endpoint across the resolved tenant list and
// returns the per-tenant aggregate. Extracted from the command body so a
// --preset bundle can reuse a single tenant lookup, client, and store handle
// across every endpoint in the bundle.
func runFanoutForEndpoint(
	ctx context.Context,
	cmd *cobra.Command,
	flags *rootFlags,
	c *client.Client,
	db *store.Store,
	endpoint string,
	tenants []tenantRef,
	conc int,
	save bool,
	resume bool,
) ([]fanoutTenantResult, error) {
	// Resume: skip already-completed tenants. The checkpoint is per-endpoint,
	// so a halted preset resumes each endpoint independently.
	var cpPath string
	completed := map[string]bool{}
	if resume {
		p, err := fanoutCheckpointPath(endpoint)
		if err != nil {
			return nil, err
		}
		cpPath = p
		done, err := readFanoutCheckpoint(cpPath)
		if err != nil {
			return nil, err
		}
		completed = done
	}

	pending := make([]tenantRef, 0, len(tenants))
	for _, t := range tenants {
		if resume && completed[t.DefaultDomainName] {
			continue
		}
		pending = append(pending, t)
	}

	results, errs := cliutil.FanoutRun(
		ctx,
		pending,
		func(t tenantRef) string { return t.DefaultDomainName },
		func(ctx context.Context, t tenantRef) (json.RawMessage, error) {
			return c.Get(ctx, endpoint, fanoutParams(endpoint, t.DefaultDomainName))
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
	resourceType := resourceTypeForEndpoint(endpoint)
	// One shared snapshot timestamp per endpoint so the standards history
	// groups every tenant's rows under the same snapshot.
	// Fixed-width nanoseconds: two saves inside one second stay two
	// snapshots, and the strings still sort chronologically.
	snapshotTS := time.Now().UTC().Format(snapshotTimeFormat)

	for _, t := range pending {
		row := fanoutTenantResult{Tenant: t.DefaultDomainName, Endpoint: endpoint}
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
		items, parseErr := itemsFromResponse(data)
		row.Data = data
		if parseErr != nil {
			// Not checkpointed: a resumed run retries the tenant.
			row.Error = parseErr.Error()
			agg = append(agg, row)
			continue
		}
		row.Count = len(items)

		saveFailed := false
		var saveRows []json.RawMessage
		if save && db != nil {
			// Hand-fix (handfixes.json fanout-save-integrity): only a
			// well-formed list replaces the stored rows; anything else is
			// shown and not stored, and the tenant is not checkpointed.
			rows, shapeErr := fanoutSaveRows(endpoint, data)
			if shapeErr != nil {
				saveErr = shapeErr
				saveFailed = true
				row.Error = shapeErr.Error()
			}
			saveRows = rows
		}
		if save && db != nil && !saveFailed {
			// A successful response is the tenant's complete current set, so
			// it replaces what was stored (an empty set clears it) and is
			// recorded as a snapshot even when empty.
			items = saveRows
			stamped := stampTenant(items, t.DefaultDomainName)
			if _, err := db.ReplaceTenantRows(resourceType, t.DefaultDomainName, stamped); err != nil {
				// Rolled back: the previous rows are intact. Report it on the
				// tenant's row too, so the result is not a silent success.
				saveErr = err
				saveFailed = true
				row.Error = "save failed: " + err.Error()
			} else if err := appendResourceSnapshot(ctx, db, snapshotTS, resourceType, t.DefaultDomainName, stamped); err != nil {
				// History for 'changes'. A failed append is a failed save:
				// the tenant must not be checkpointed as done.
				saveErr = err
				saveFailed = true
			} else if resourceType == "standards" || resourceType == "standardscurrentstate" {
				// Append-only history so `standards drift` has two comparable
				// snapshots; the generic upsert above overwrites in place and
				// cannot carry history. Both /ListStandards and the newer
				// /ListStandardsCurrentState seed it, so a fleet on either
				// endpoint still accumulates comparable history.
				if err := appendStandardsSnapshot(ctx, db, snapshotTS, t.DefaultDomainName, items); err != nil {
					saveErr = err
					saveFailed = true
				}
			}
		}
		// Never checkpoint a tenant whose save failed: a resumed run must retry
		// it, otherwise its data is silently lost forever.
		if resume && !saveFailed {
			completed[t.DefaultDomainName] = true
		}
		agg = append(agg, row)
	}

	// Surface per-tenant errors to stderr so none are silently dropped.
	cliutil.FanoutReportErrors(cmd.ErrOrStderr(), errs)

	// Persist checkpoint.
	if resume && cpPath != "" {
		if err := writeFanoutCheckpoint(cpPath, endpoint, completed); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warn: writing checkpoint: %v\n", err)
		}
	}
	if saveErr != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warn: some tenant data failed to save for %s: %v\n", endpoint, saveErr)
	}

	return agg, nil
}
