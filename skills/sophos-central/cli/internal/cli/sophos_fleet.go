// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written: cross-tenant plumbing shared by the fleet commands.
//
// Sophos has no cross-tenant API. A partner or organization credential lists
// its tenants, then calls each tenant's regional host with X-Tenant-ID (the
// routing transport in sophos_routing.go does the host/header work). A tenant
// credential is a fleet of one. Per-tenant 403s are normal (trial tenants,
// term customers without Partner Assistance), so a fan-out records each
// tenant's outcome and keeps going instead of aborting.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"sophos-central-pp-cli/internal/client"
	"sophos-central-pp-cli/internal/cliutil"
	"sophos-central-pp-cli/internal/store"
)

// fleetTenant is the subset of a partner/organization tenant record the fleet
// commands use.
type fleetTenant struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	APIHost     string `json:"apiHost,omitempty"`
	DataRegion  string `json:"dataRegion,omitempty"`
	BillingType string `json:"billingType,omitempty"`
	Status      string `json:"status,omitempty"`
}

// fleetOutcome is one tenant's result in a fan-out.
type fleetOutcome struct {
	TenantID   string `json:"tenant_id"`
	TenantName string `json:"tenant_name"`
	Status     string `json:"status"` // ok | forbidden | no_api_host | error
	Error      string `json:"error,omitempty"`
	Items      int    `json:"items"`
	// Freshness is per tenant and per resource: a --tenant or --resources
	// refresh must not make another tenant's or resource's data look fresh.
	At     time.Time `json:"at,omitzero"`      // this resource's last successful fetch
	PrevAt time.Time `json:"prev_at,omitzero"` // the fetch before that ("new" alerts)
	FullAt time.Time `json:"full_at,omitzero"` // last fetch not limited by --since
	Window string    `json:"window,omitempty"` // set when the last fetch used --since
}

// fleetTenants enumerates the tenants this credential can reach. tenantFilter
// (from --tenant) narrows to one ID. includeTrial keeps trial tenants, which
// usually 403 for partners.
func fleetTenants(ctx context.Context, c *client.Client, tenantFilter string, includeTrial bool) ([]fleetTenant, string, error) {
	who, err := callerIdentity(ctx, c)
	if err != nil {
		return nil, "", err
	}
	if who.IDType == "tenant" {
		// A tenant credential only ever reaches its own tenant; an explicit
		// selector for any other tenant must fail, never fall back to self.
		if tenantFilter != "" && tenantFilter != who.ID {
			return nil, who.IDType, usageErr(fmt.Errorf("this tenant credential can only act on tenant %s; --tenant %s does not match", who.ID, tenantFilter))
		}
		return []fleetTenant{{ID: who.ID, Name: "(this tenant)", APIHost: who.APIHosts.DataRegion}}, who.IDType, nil
	}
	listPath := "/partner/v1/tenants"
	if who.IDType == "organization" {
		listPath = "/organization/v1/tenants"
	}
	var tenants []fleetTenant
	totalPages := 0
	for page := 1; ; page++ {
		params := map[string]string{"page": strconv.Itoa(page), "pageSize": "100"}
		if page == 1 {
			params["pageTotal"] = "true" // Sophos: only on the first page
		}
		raw, err := c.Get(ctx, listPath, params)
		if err != nil {
			return nil, who.IDType, fmt.Errorf("listing tenants (page %d): %w", page, err)
		}
		var env struct {
			Items *[]fleetTenant `json:"items"`
			Pages struct {
				Current int `json:"current"`
				Total   int `json:"total"`
			} `json:"pages"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			return nil, who.IDType, fmt.Errorf("decoding tenant list: %w", err)
		}
		// No items array is not "no tenants": the fleet would silently shrink.
		if env.Items == nil {
			return nil, who.IDType, fmt.Errorf("listing tenants (page %d): response has no items array", page)
		}
		pageItems := *env.Items
		for _, t := range pageItems {
			if t.ID == "" {
				return nil, who.IDType, fmt.Errorf("listing tenants (page %d): a tenant row has no id", page)
			}
			rememberSophosTenantHost(t.ID, t.APIHost)
			if tenantFilter != "" && t.ID != tenantFilter {
				continue
			}
			if !includeTrial && strings.EqualFold(t.BillingType, "trial") && tenantFilter == "" {
				continue
			}
			tenants = append(tenants, t)
		}
		// pageTotal is only sent on page 1, so remember that page's total;
		// stop on an empty page or once the known total is reached.
		if page == 1 {
			totalPages = env.Pages.Total
		}
		if len(pageItems) == 0 && page < totalPages {
			return nil, who.IDType, fmt.Errorf("listing tenants: page %d of %d came back empty; tenant list is incomplete", page, totalPages)
		}
		if len(pageItems) == 0 || (totalPages > 0 && page >= totalPages) {
			break
		}
		if page >= 1000 {
			return nil, who.IDType, fmt.Errorf("listing tenants: stopped at %d pages with more reported; tenant list is incomplete", page)
		}
	}
	if tenantFilter != "" && len(tenants) == 0 {
		return nil, who.IDType, usageErr(fmt.Errorf("tenant %s is not managed by this %s credential", tenantFilter, who.IDType))
	}
	return tenants, who.IDType, nil
}

// callerIdentity returns the whoami answer for the credential this run uses.
func callerIdentity(ctx context.Context, c *client.Client) (*sophosIdentity, error) {
	// Always ask: the routing transport records the answer for the exact
	// credential that sent it, so later calls in this run reuse it.
	raw, err := c.GetNoCache(ctx, "/whoami/v1", nil)
	if err != nil {
		return nil, fmt.Errorf("resolving caller identity: %w", err)
	}
	var who sophosIdentity
	if err := json.Unmarshal(raw, &who); err != nil || who.IDType == "" {
		return nil, errors.New("resolving caller identity: /whoami/v1 returned no idType")
	}
	return &who, nil
}

// classifyTenantErr maps a per-tenant failure to a fleet status.
func classifyTenantErr(err error) string {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.StatusCode == 403:
			return "forbidden"
		case apiErr.StatusCode == 400 && strings.Contains(apiErr.Body, "noApiHost"):
			return "no_api_host"
		}
	}
	return "error"
}

// tenantHeaders scopes one request to a tenant; the header is also part of
// the response-cache key, so tenants never share cached rows.
func tenantHeaders(tenantID string) map[string]string {
	return map[string]string{headerTenantID: tenantID}
}

// checkItemObjects rejects list items that are not JSON objects: a null
// item decodes into a zero value without error and would be counted as data.
func checkItemObjects(items []json.RawMessage) error {
	for i, it := range items {
		var m map[string]any
		if json.Unmarshal(it, &m) != nil || m == nil {
			return fmt.Errorf("item %d is not a JSON object", i)
		}
	}
	return nil
}

// fetchKeyPaged walks a Sophos key-paged list (pageFromKey -> pages.nextKey)
// for one tenant (empty tenantID = no tenant header, for partner-global
// lists) and returns every item, up to maxItems (0 = no cap).
func fetchKeyPaged(ctx context.Context, c *client.Client, path string, params map[string]string, tenantID string, maxItems int) ([]json.RawMessage, error) {
	q := map[string]string{}
	for k, v := range params {
		q[k] = v
	}
	var headers map[string]string
	if tenantID != "" {
		headers = tenantHeaders(tenantID)
	}
	var out []json.RawMessage
	visited := map[string]bool{}
	for {
		// No response cache: a cached page could replay a cursor and make a
		// partial listing look complete.
		raw, err := c.GetWithHeadersNoCache(ctx, path, q, headers)
		if err != nil {
			return out, err
		}
		var env struct {
			Items *[]json.RawMessage `json:"items"`
			Pages struct {
				NextKey string `json:"nextKey"`
			} `json:"pages"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			return out, fmt.Errorf("decoding %s: %w", path, err)
		}
		// A body without an items array ({} or null) is not an empty
		// listing; treating it as one would let a refresh delete rows.
		if env.Items == nil {
			return out, fmt.Errorf("%s: response has no items array; listing is incomplete", path)
		}
		if err := checkItemObjects(*env.Items); err != nil {
			return out, fmt.Errorf("%s: %w; listing is incomplete", path, err)
		}
		out = append(out, (*env.Items)...)
		if maxItems > 0 && len(out) >= maxItems {
			return out[:maxItems], nil
		}
		next := env.Pages.NextKey
		if next == "" {
			return out, nil
		}
		// A repeated key means the listing never reached its end. Report it
		// as an error so callers never treat (and reconcile) a partial
		// listing as complete.
		if visited[next] || next == q["pageFromKey"] {
			return out, fmt.Errorf("%s: pagination repeated key %q after %d items; listing is incomplete", path, next, len(out))
		}
		visited[next] = true
		q["pageFromKey"] = next
	}
}

const fleetCLIName = "sophos-central-cli"

// Local store resource types owned by the fleet commands. Endpoints, alerts
// and tenants reuse the generated tables so search/analytics see fleet rows.
const (
	fleetLedgerType     = "fleet_ledger"      // one row per tenant: last fleet refresh outcome
	fleetHealthType     = "fleet_health"      // one row per tenant: latest Account Health Check
	fleetHealthPrevType = "fleet_health_prev" // the snapshot before the latest one
	fleetLicensesType   = "fleet_licenses"    // one row per tenant: GET /licenses/v1/licenses
	fleetMetaType       = "fleet_meta"        // id "sync": started/finished timestamps
	fleetSiemCursorType = "fleet_siem_cursor" // id "<tenant>:<stream>"
)

func fleetDBPath(p string) string {
	if p != "" {
		return p
	}
	return defaultDBPath(fleetCLIName)
}

// fleetTenantFilter returns the --tenant / SOPHOS_CENTRAL_TENANT_ID value as a
// set. Fleet commands accept a comma-separated list; unset means all tenants.
// A selector that names no tenant (",", " ") is an error, never "all".
func fleetTenantFilter() (map[string]bool, error) {
	raw := configuredSophosTenant()
	if raw == "" {
		// A blank flag is rejected at parse time; a blank-but-set env var
		// is the same mistake and must not mean "all tenants" either.
		if env := cliutil.EnvOverride(sophosTenantEnv); env != "" {
			return nil, usageErr(fmt.Errorf("%s=%q names no tenant ID", sophosTenantEnv, env))
		}
		return nil, nil
	}
	set := map[string]bool{}
	for _, id := range strings.Split(raw, ",") {
		if id = strings.TrimSpace(id); id != "" {
			set[id] = true
		}
	}
	if len(set) == 0 {
		return nil, usageErr(fmt.Errorf("--tenant %q names no tenant ID", raw))
	}
	return set, nil
}

// fleetLedgerRow is one tenant's outcome from the last fleet refresh.
type fleetLedgerRow struct {
	ID         string                  `json:"id"`
	TenantName string                  `json:"tenant_name"`
	Status     string                  `json:"status"` // ok | partial | forbidden | no_api_host | error
	SyncedAt   time.Time               `json:"synced_at"`
	Resources  map[string]fleetOutcome `json:"resources"`
}

// fleetSyncMeta brackets fleet refresh runs so reads can flag what is new.
type fleetSyncMeta struct {
	ID              string    `json:"id"`
	StartedAt       time.Time `json:"started_at"`
	FinishedAt      time.Time `json:"finished_at"`
	PrevStartedAt   time.Time `json:"prev_started_at"`
	TenantsAttempts int       `json:"tenants_attempted"`
}

// blindTenant is a tenant a fleet read could not see on the last sync.
type blindTenant struct {
	TenantID   string `json:"tenant_id"`
	TenantName string `json:"tenant_name"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
}

func loadFleetLedger(db fleetReader) (map[string]fleetLedgerRow, error) {
	out := map[string]fleetLedgerRow{}
	err := db.ListScan(fleetLedgerType, func(key string, data json.RawMessage) bool {
		var row fleetLedgerRow
		if json.Unmarshal(data, &row) != nil || row.ID == "" {
			// Keep the tenant visible as blind instead of dropping it.
			row = fleetLedgerRow{ID: key, Status: "unreadable", Resources: map[string]fleetOutcome{
				"ledger": {Status: "error", Error: "stored fleet ledger row could not be read; run 'sophos-central-cli fleet refresh'"}}}
		}
		out[row.ID] = row
		return true
	})
	return out, err
}

func loadFleetMeta(db fleetReader) fleetSyncMeta {
	var meta fleetSyncMeta
	if raw, err := db.Get(fleetMetaType, "sync"); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &meta)
	}
	return meta
}

// blindTenantsFrom lists tenants a fleet read cannot fully see: ledger
// tenants whose last refresh of a resource this view reads was not ok,
// missing, or limited to a --since window, plus --tenant IDs the store has
// never seen (status not_in_store), so a filtered read never drops a
// requested tenant silently. A failure in a resource the view does not read
// does not make the tenant blind.
func blindTenantsFrom(ledger map[string]fleetLedgerRow, filter map[string]bool, needed ...string) []blindTenant {
	out := make([]blindTenant, 0)
	for id, row := range ledger {
		if filter != nil && !filter[id] {
			continue
		}
		var unfetched, windowed []string
		var failed *fleetOutcome
		for _, rt := range needed {
			o, ok := row.Resources[rt]
			switch {
			case !ok:
				unfetched = append(unfetched, rt)
			case o.Status != "ok":
				if failed == nil {
					failed = &o
				}
			case o.Window != "" && o.FullAt.IsZero():
				windowed = append(windowed, rt+" (only the last "+o.Window+")")
			}
		}
		switch {
		case failed != nil:
			out = append(out, blindTenant{TenantID: id, TenantName: row.TenantName, Status: failed.Status, Error: failed.Error})
		case len(unfetched) > 0 && row.Status != "ok":
			// The refresh stopped before these resources (e.g. no API host).
			b := blindTenant{TenantID: id, TenantName: row.TenantName, Status: row.Status}
			for _, o := range row.Resources {
				if o.Error != "" {
					b.Error = o.Error
					break
				}
			}
			out = append(out, b)
		case len(unfetched) > 0:
			out = append(out, blindTenant{TenantID: id, TenantName: row.TenantName, Status: "not_refreshed",
				Error: "fleet refresh has not fetched " + strings.Join(unfetched, ", ") + " for this tenant; run 'sophos-central-cli fleet refresh --resources " + strings.Join(unfetched, ",") + "'"})
		case len(windowed) > 0:
			out = append(out, blindTenant{TenantID: id, TenantName: row.TenantName, Status: "partial_window",
				Error: "fleet refresh has fetched " + strings.Join(windowed, ", ") + " for this tenant; run 'sophos-central-cli fleet refresh' without --since once for the full list"})
		}
	}
	for id := range filter {
		if _, ok := ledger[id]; !ok {
			out = append(out, blindTenant{TenantID: id, Status: "not_in_store", Error: "no fleet refresh has recorded this tenant; run 'sophos-central-cli fleet refresh --tenant " + id + "'"})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TenantName != out[j].TenantName {
			return out[i].TenantName < out[j].TenantName
		}
		return out[i].TenantID < out[j].TenantID
	})
	return out
}

const fleetNeverRefreshedNote = "no fleet refresh has run yet; run 'sophos-central-cli fleet refresh' to fill the local store"

// fleetFreshness returns when the last fleet refresh finished (nil if never)
// and a note for empty or stale results. It also writes the hint to stderr,
// naming 'fleet refresh': plain 'sync' does not fill the fleet tables.
func fleetFreshness(w io.Writer, meta fleetSyncMeta, ledger map[string]fleetLedgerRow, filter map[string]bool, maxAge time.Duration, needed ...string) (*time.Time, string) {
	if meta.FinishedAt.IsZero() {
		fmt.Fprintln(w, "hint: "+fleetNeverRefreshedNote)
		return nil, fleetNeverRefreshedNote
	}
	// The view is as fresh as its oldest input: the oldest successful fetch of
	// a needed resource across the tenants in scope. Tenants that were never
	// fetched are reported in blind_tenants.
	// A resource that failed this run still shows its last good fetch, so
	// that fetch counts; a resource never attempted makes the view incomplete.
	var at time.Time
	attempted, fetched := map[string]bool{}, map[string]bool{}
	for id, row := range ledger {
		if filter != nil && !filter[id] {
			continue
		}
		for _, rt := range needed {
			o, ok := row.Resources[rt]
			if !ok {
				continue
			}
			attempted[rt] = true
			// A --since fetch on top of a full one does not refresh what lies
			// outside its window, so the full fetch dates the view.
			t := o.At
			if !o.FullAt.IsZero() {
				t = o.FullAt
			}
			if t.IsZero() && o.Status == "ok" {
				t = row.SyncedAt
			}
			if t.IsZero() {
				continue
			}
			fetched[rt] = true
			if at.IsZero() || t.Before(at) {
				at = t
			}
		}
	}
	var missing, failed []string
	for _, rt := range needed {
		switch {
		case !attempted[rt]:
			missing = append(missing, rt)
		case !fetched[rt]:
			failed = append(failed, rt)
		}
	}
	if len(missing) > 0 {
		note := "fleet refresh has not fetched " + strings.Join(missing, ", ") + " yet, so this view is incomplete; run 'sophos-central-cli fleet refresh --resources " + strings.Join(missing, ",") + "'"
		fmt.Fprintln(w, "hint: "+note)
		return nil, note
	}
	if len(failed) > 0 {
		note := "fleet refresh could not fetch " + strings.Join(failed, ", ") + " for any tenant in scope; see blind_tenants"
		fmt.Fprintln(w, "hint: "+note)
		if at.IsZero() {
			return nil, note
		}
		return &at, note
	}
	if maxAge > 0 && time.Since(at) > maxAge {
		note := fmt.Sprintf("local fleet data is %s old (older than --max-age=%s); run 'sophos-central-cli fleet refresh' to update it", time.Since(at).Round(time.Minute), maxAge)
		fmt.Fprintln(w, "hint: "+note)
		return &at, note
	}
	return &at, ""
}

// tenantNames maps tenant ID to display name from the tenants table and ledger.
func tenantNames(db fleetReader, ledger map[string]fleetLedgerRow) map[string]string {
	names := map[string]string{}
	_ = db.ListScan("tenants", func(_ string, data json.RawMessage) bool {
		var t fleetTenant
		if json.Unmarshal(data, &t) == nil && t.ID != "" && t.Name != "" {
			names[t.ID] = t.Name
		}
		return true
	})
	for id, row := range ledger {
		if names[id] == "" && row.TenantName != "" {
			names[id] = row.TenantName
		}
	}
	return names
}

// unrefreshedTenants lists stored non-trial tenants that have no ledger row
// (a tenants-only refresh, or a run cut off before reaching them), so an
// unfiltered fleet read never presents a partial book as complete. Filtered
// reads already report unknown --tenant IDs as not_in_store.
func unrefreshedTenants(db fleetReader, ledger map[string]fleetLedgerRow, filter map[string]bool) []blindTenant {
	out := make([]blindTenant, 0)
	if filter != nil {
		return out
	}
	_ = db.ListScan("tenants", func(_ string, data json.RawMessage) bool {
		var t fleetTenant
		// Rows without a name are not tenants (a mutation response cached
		// under the tenants group).
		if json.Unmarshal(data, &t) != nil || t.ID == "" || t.Name == "" || strings.EqualFold(t.BillingType, "trial") {
			return true
		}
		if _, ok := ledger[t.ID]; !ok {
			out = append(out, blindTenant{TenantID: t.ID, TenantName: t.Name, Status: "not_refreshed",
				Error: "fleet refresh has not reached this tenant yet; run 'sophos-central-cli fleet refresh'"})
		}
		return true
	})
	return out
}

// sophosLocalSyncedAt dates a local read. With one tenant selected it is that
// tenant's own last fetch of the resource (its fleet ledger row, then its sync
// checkpoint), never the fleet-wide stamp another tenant's refresh could move.
// Several selected tenants have no single date.
func sophosLocalSyncedAt(db *store.Store, resourceType string) (time.Time, error) {
	getState := func(key string) (time.Time, error) {
		_, at, _, err := db.GetSyncState(key)
		return at, err
	}
	t := sophosExplicitTenant()
	if t == "" {
		if strings.Contains(configuredSophosTenant(), ",") {
			return time.Time{}, nil
		}
		return getState(resourceType)
	}
	if raw, err := db.Get(fleetLedgerType, t); err == nil && len(raw) > 0 {
		var row fleetLedgerRow
		if json.Unmarshal(raw, &row) == nil {
			if o, ok := row.Resources[resourceType]; ok {
				if !o.FullAt.IsZero() {
					return o.FullAt, nil
				}
				return o.At, nil
			}
		}
	}
	return getState(sophosSyncStateKey(resourceType))
}

// openFleetStore opens the local store read-only for a fleet read and reports
// whether it exists. A missing mirror is an empty result, not an error.
func openFleetStore(ctx context.Context, dbPath string) (*store.Store, bool, error) {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, false, nil
	}
	db, err := store.OpenReadOnlyContext(ctx, dbPath)
	if err != nil {
		return nil, true, fmt.Errorf("opening local store %s: %w", dbPath, err)
	}
	return db, true, nil
}

// healthCheck is one scored Account Health Check node, e.g. path
// "endpoint.protection.computer" with its score and snooze state.
type healthCheck struct {
	Check        string         `json:"check"`
	Score        int            `json:"score"`
	Snoozed      bool           `json:"snoozed"`
	SnoozeExpiry string         `json:"snooze_expiry,omitempty"`
	Detail       map[string]int `json:"detail,omitempty"`
}

// healthDetailKeys are the counters worth carrying next to a score.
var healthDetailKeys = []string{"total", "notFullyProtected", "disabled", "notOnRecommended", "numberOfSecurityRisks"}

// walkHealthChecks collects every node that carries a numeric "score". The
// Account Health Check response nests the same {score, snoozed, snoozeDetail}
// shape at every level, so walking it keeps up with checks Sophos adds.
func walkHealthChecks(prefix string, v any, out *[]healthCheck) {
	obj, ok := v.(map[string]any)
	if !ok {
		return
	}
	if score, ok := obj["score"].(float64); ok {
		hc := healthCheck{Check: prefix, Score: int(score)}
		hc.Snoozed, _ = obj["snoozed"].(bool)
		if sd, ok := obj["snoozeDetail"].(map[string]any); ok {
			if exp, ok := sd["expiry"].(string); ok {
				hc.SnoozeExpiry = exp
			} else if end, ok := sd["end"].(string); ok {
				hc.SnoozeExpiry = end
			}
		}
		for _, k := range healthDetailKeys {
			if n, ok := obj[k].(float64); ok {
				if hc.Detail == nil {
					hc.Detail = map[string]int{}
				}
				hc.Detail[k] = int(n)
			}
		}
		*out = append(*out, hc)
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch k {
		case "tenant", "snoozeDetail", "policies", "scanningExclusions", "schedule":
			continue
		}
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		walkHealthChecks(path, obj[k], out)
	}
}

// overallHealth is the minimum score across non-snoozed checks; ok=false when
// no check reported a score.
func overallHealth(checks []healthCheck) (int, bool) {
	min, found := 0, false
	for _, c := range checks {
		if c.Snoozed {
			continue
		}
		if !found || c.Score < min {
			min, found = c.Score, true
		}
	}
	return min, found
}

// jsonPath walks decoded JSON by object keys, e.g. jsonPath(e, "health", "overall").
func jsonPath(v any, keys ...string) any {
	for _, k := range keys {
		obj, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = obj[k]
	}
	return v
}

func jsonString(v any, keys ...string) string {
	s, _ := jsonPath(v, keys...).(string)
	return s
}

// hasAny reports whether the user passed any sync filter parameter. Used by
// the filtered-prune hand-fix in the generated sync.go.
func (p *syncUserParams) hasAny() bool {
	return p != nil && len(p.flatGlobal)+len(p.trueGlobal)+len(p.perResource) > 0
}

// sophosTenantScopedPath reports whether a request path belongs to a tenant
// API (everything except the partner, organization and whoami services).
func sophosTenantScopedPath(path string) bool {
	p := strings.TrimPrefix(path, "/")
	return !strings.HasPrefix(p, "partner/") && !strings.HasPrefix(p, "organization/") && !strings.HasPrefix(p, "whoami")
}

// sophosLocalTenant is the tenant a local read must be scoped to.
func sophosLocalTenant() (string, error) {
	if t := configuredSophosTenant(); t != "" {
		if strings.Contains(t, ",") {
			return "", usageErr(fmt.Errorf("local reads take a single --tenant, not a list"))
		}
		return t, nil
	}
	if t := sophosActiveTenant(); t != "" {
		return t, nil
	}
	return "", usageErr(fmt.Errorf("the local store can hold rows for many tenants; pass --tenant <tenant-id> to read tenant-scoped data locally, or use --data-source live"))
}

// unreadableRows collects stored fleet rows a local report could not decode,
// keyed by tenant ID ("" when even that is unreadable), so the report lists
// them as blind instead of reading clean.
type unreadableRows map[string]map[string]bool

func (u unreadableRows) add(raw json.RawMessage, resource string) {
	var probe struct {
		TenantID string `json:"tenant_id"`
	}
	_ = json.Unmarshal(raw, &probe)
	tid := probe.TenantID
	if tid == "" {
		tid = rowTenantID(raw)
	}
	if u[tid] == nil {
		u[tid] = map[string]bool{}
	}
	u[tid][resource] = true
}

func (u unreadableRows) blind(names map[string]string, filter map[string]bool) []blindTenant {
	out := make([]blindTenant, 0, len(u))
	for tid, res := range u {
		if tid != "" && filter != nil && !filter[tid] {
			continue
		}
		var list []string
		for r := range res {
			list = append(list, r)
		}
		sort.Strings(list)
		out = append(out, blindTenant{TenantID: tid, TenantName: names[tid], Status: "unreadable",
			Error: fmt.Sprintf("stored %s data could not be read; run 'sophos-central-cli fleet refresh'", strings.Join(list, ", "))})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TenantID < out[j].TenantID })
	return out
}

// warnBlind tells a human reader that a local report does not cover every tenant.
func warnBlind(w io.Writer, n int) {
	if n > 0 {
		fmt.Fprintf(w, "warning: %d tenant(s) could not be read; see blind_tenants in --json\n", n)
	}
}

func rowTenantID(raw json.RawMessage) string {
	var row struct {
		Tenant struct {
			ID string `json:"id"`
		} `json:"tenant"`
	}
	if json.Unmarshal(raw, &row) != nil {
		return ""
	}
	return row.Tenant.ID
}

// sophosScopeLocalItems keeps only the selected tenant's rows for a
// tenant-scoped local list, and fails closed when a row does not record
// its tenant (it cannot be attributed safely).
func sophosScopeLocalItems(path string, items []json.RawMessage) ([]json.RawMessage, error) {
	if !sophosTenantScopedPath(path) {
		return items, nil
	}
	tenant, err := sophosLocalTenant()
	if err != nil {
		return nil, err
	}
	kept := make([]json.RawMessage, 0, len(items))
	for _, it := range items {
		tid := rowTenantID(it)
		if tid == "" {
			return nil, usageErr(fmt.Errorf("local rows for %s do not record their tenant, so they cannot be scoped to %s; use --data-source live", path, tenant))
		}
		if tid == tenant {
			kept = append(kept, it)
		}
	}
	return kept, nil
}

// sophosUnpagedParams strips paging from a tenant-scoped local list so the
// tenant filter runs before the page is cut (another tenant's rows must not
// use up the page). It returns the offset and limit (-1 = none) to apply
// afterwards with sophosPage. Malformed paging is left for the generated
// fail-closed check.
// ponytail: scoped local lists scan every stored row; push the tenant into the scan if that gets slow.
func sophosUnpagedParams(path string, params map[string]string) (map[string]string, int, int) {
	if !sophosTenantScopedPath(path) || len(params) == 0 {
		return params, 0, -1
	}
	q := parseLocalListQuery(params)
	if len(q.rowSelecting) > 0 {
		return params, 0, -1
	}
	out := make(map[string]string, len(params))
	for k, v := range params {
		c := localParamCanon(k)
		if localListLimitParams[c] || localListOffsetParams[c] || localListPageParams[c] {
			continue
		}
		out[k] = v
	}
	return out, q.offset, q.limit
}

// sophosPage applies an offset and limit (-1 = none) to scoped local rows.
func sophosPage(items []json.RawMessage, offset, limit int) []json.RawMessage {
	if offset >= len(items) {
		return items[:0]
	}
	items = items[offset:]
	if limit >= 0 && limit < len(items) {
		items = items[:limit]
	}
	return items
}

// sophosCheckLocalItem refuses a locally stored row that belongs to another
// tenant (or does not say which tenant it belongs to).
func sophosCheckLocalItem(path string, item json.RawMessage) error {
	if !sophosTenantScopedPath(path) {
		return nil
	}
	tenant, err := sophosLocalTenant()
	if err != nil {
		return err
	}
	tid := rowTenantID(item)
	if tid == "" {
		return usageErr(fmt.Errorf("the local row for %s does not record its tenant; use --data-source live", path))
	}
	if tid != tenant {
		return notFoundErr(fmt.Errorf("%s is not stored for tenant %s", path, tenant))
	}
	return nil
}

// loadFleetLedgerQuiet is loadFleetLedger for freshness: a read error leaves
// the view with no fresh inputs, which fleetFreshness reports.
func loadFleetLedgerQuiet(db fleetReader) map[string]fleetLedgerRow {
	ledger, _ := loadFleetLedger(db)
	return ledger
}

// alertIsNew reports whether an alert was raised after the tenant's previous
// alerts fetch; ledgers written before per-tenant times use the run-level start.
func alertIsNew(ledger map[string]fleetLedgerRow, tenantID string, raised time.Time, meta fleetSyncMeta) bool {
	if o, ok := ledger[tenantID].Resources["alerts"]; ok && !o.At.IsZero() {
		return !o.PrevAt.IsZero() && raised.After(o.PrevAt)
	}
	return !meta.PrevStartedAt.IsZero() && raised.After(meta.PrevStartedAt)
}
