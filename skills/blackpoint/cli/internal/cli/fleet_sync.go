// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: fleet-sync.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"blackpoint-pp-cli/internal/client"
	"blackpoint-pp-cli/internal/cliutil"
	"blackpoint-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

// fleetSyncKinds are the --kinds values, in fetch order.
var fleetSyncKinds = []string{"detections", "devices", "vulnerabilities", "scans", "darkweb", "posture", "contacts", "cloud"}

// fleetIndexed maps a fleet row kind to the generated store resource that
// `search` reads. Tenant-scoped rows are indexed under tenant-qualified ids
// ("<tenant>:<id>") so equal API ids in two tenants never merge.
var fleetIndexed = map[string]string{
	fleetKindDetection: "detections",
	fleetKindDevice:    "assets",
	fleetKindVuln:      "vulnerabilities",
	fleetKindScan:      "scans",
	fleetKindDarkweb:   "darkweb_exposures",
	fleetKindPosture:   "posture",
}

type fleetSyncFailure struct {
	Tenant string `json:"tenant,omitempty"`
	Kind   string `json:"kind"`
	Error  string `json:"error"`
}

type fleetSyncSummary struct {
	DB            string             `json:"db"`
	Tenants       int                `json:"tenants"`
	Rows          map[string]int     `json:"rows"`
	Truncated     []string           `json:"truncated,omitempty"`
	FetchFailures []fleetSyncFailure `json:"fetch_failures"`
}

type fleetSyncer struct {
	ctx      context.Context
	c        *client.Client
	db       *store.Store
	pageSize int
	maxPages int
	now      time.Time
	sum      *fleetSyncSummary
	// storeErr is the first local write failure; fleet-sync fails when set so
	// a stale healthy status never survives a failed refresh silently.
	storeErr error
}

// errFleetIncomplete marks a fetch whose rows are real but whose set is not
// known to be complete, so nothing may be pruned on its account.
// errFleetTruncated (stopped at --max-pages) and an empty page before the
// advertised end both wrap it.
var (
	errFleetIncomplete = errors.New("incomplete fetch")
	errFleetTruncated  = fmt.Errorf("%w: stopped at --max-pages", errFleetIncomplete)
	errFleetShortPage  = fmt.Errorf("%w: empty page before the advertised end of the list", errFleetIncomplete)
)

func newNovelFleetSyncCmd(flags *rootFlags) *cobra.Command {
	var tenantFlags []string
	var kindsFlag string
	var pageSize, maxPages int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "fleet-sync",
		Short: "Pull every tenant's detections, devices, vulnerabilities, scans, exposures, posture, contacts and geo policy into the local store",
		Long: `Use this command to refresh the local per-tenant data that triage, gaps,
stale-agents, cve-spread, overview, leaderboard and geo-audit read.
Do NOT use it for tenants or accounts alone; the built-in 'sync' covers those.

Tenant-scoped CompassOne routes need an x-tenant-id header per call, which the
generic sync cannot send. fleet-sync lists tenants, then fetches each kind per
tenant and replaces that tenant's slice wholesale, so a detection resolved since
the last run no longer shows as open. A fetch that fails (for example 403 when
the account is not entitled to vulnerability management) keeps its previous rows,
is recorded so the readers can warn, and is listed under fetch_failures. A fetch
that stops at --max-pages keeps what it read without pruning anything.

Kinds: detections (open alert groups), devices (DEVICE assets), vulnerabilities
(unresolved findings), scans, darkweb (exposures; passwords are never stored),
posture (rating), contacts (contact groups per account), cloud (M365, Cisco Duo
and Google connections, users and approved countries).

The key quota is 2000 requests per 15 minutes; limit the fan-out with --tenant
and --kinds on large fleets.`,
		Example: strings.Trim(`
  blackpoint-cli fleet-sync
  blackpoint-cli fleet-sync --kinds detections,devices --agent
  blackpoint-cli fleet-sync --tenant "Acme Dental" --tenant "Northwind Legal"`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "false", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet-sync: list tenants, then GET each kind per tenant with x-tenant-id and store it locally")
			}
			kinds, err := parseFleetKinds(kindsFlag)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(err)
			}
			if pageSize < 1 || maxPages < 1 {
				return usageErr(fmt.Errorf("--page-size and --max-pages must be at least 1"))
			}
			if dbPath == "" {
				dbPath = defaultDBPath("blackpoint-cli")
			}
			if cliutil.IsDogfoodEnv() {
				maxPages = 1
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			// Never write fleet responses (dark-web exposures included) to the
			// on-disk response cache.
			c.NoCache = true
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			db, err := store.OpenWithContext(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening local store %s: %w", dbPath, err)
			}
			defer db.Close()
			if err := ensureFleetSchema(db.DB()); err != nil {
				return err
			}

			sum := &fleetSyncSummary{DB: dbPath, Rows: map[string]int{}, FetchFailures: []fleetSyncFailure{}}
			s := &fleetSyncer{ctx: ctx, c: c, db: db, pageSize: pageSize, maxPages: maxPages, now: time.Now().UTC(), sum: sum}

			tenants, err := s.syncTenants()
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), fmt.Errorf("listing tenants: %w", err), flags)
			}
			selected := tenants
			if len(tenantFlags) > 0 {
				selected = nil
				for _, q := range tenantFlags {
					t, rerr := resolveFleetTenant(tenants, q)
					if rerr != nil {
						return usageErr(rerr)
					}
					selected = append(selected, t)
				}
			}
			if cliutil.IsDogfoodEnv() && len(selected) > 2 {
				selected = selected[:2]
			}
			sum.Tenants = len(selected)

			attempts, failures := 0, 0
			for _, kind := range kinds {
				if kind == "contacts" {
					attempts++
					if !s.syncContacts() {
						failures++
					}
					continue
				}
				total := 0
				for _, t := range selected {
					attempts++
					n, ok := s.syncTenantKind(kind, t)
					total += n
					if !ok {
						failures++
					}
				}
				sum.Rows[kind] = total
			}

			if len(sum.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d fetches failed or were incomplete; their previous local rows were kept (see fetch_failures)\n", len(sum.FetchFailures), attempts)
			}
			if err := printFleetSyncSummary(cmd, flags, sum); err != nil {
				return err
			}
			if s.storeErr != nil {
				return s.storeErr
			}
			if attempts > 0 && failures == attempts {
				return apiErr(fmt.Errorf("every fleet fetch failed; first error: %s", sum.FetchFailures[0].Error))
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&tenantFlags, "tenant", nil, "Limit to this tenant (name or id; repeatable). Default: every tenant the token can see")
	cmd.Flags().StringVar(&kindsFlag, "kinds", strings.Join(fleetSyncKinds, ","), "Comma-separated kinds to fetch: "+strings.Join(fleetSyncKinds, ", "))
	cmd.Flags().IntVar(&pageSize, "page-size", 200, "Rows per API page")
	cmd.Flags().IntVar(&maxPages, "max-pages", 25, "Maximum pages fetched per tenant and kind; hitting it is reported under truncated and prunes nothing")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database path (default: the CLI's local data.db)")
	return cmd
}

func parseFleetKinds(raw string) ([]string, error) {
	valid := map[string]bool{}
	for _, k := range fleetSyncKinds {
		valid[k] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, k := range strings.Split(raw, ",") {
		k = strings.TrimSpace(strings.ToLower(k))
		if k == "" || seen[k] {
			continue
		}
		if !valid[k] {
			return nil, fmt.Errorf("unknown --kinds value %q (valid: %s)", k, strings.Join(fleetSyncKinds, ", "))
		}
		seen[k] = true
		out = append(out, k)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--kinds is empty (valid: %s)", strings.Join(fleetSyncKinds, ", "))
	}
	return out, nil
}

func printFleetSyncSummary(cmd *cobra.Command, flags *rootFlags, sum *fleetSyncSummary) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), sum, flags)
	}
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "fleet-sync: %d tenants -> %s\n", sum.Tenants, sum.DB)
	keys := make([]string, 0, len(sum.Rows))
	for k := range sum.Rows {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "  %-16s %d rows\n", k, sum.Rows[k])
	}
	for _, t := range sum.Truncated {
		fmt.Fprintf(w, "  truncated: %s (raise --max-pages)\n", t)
	}
	for _, f := range sum.FetchFailures {
		fmt.Fprintf(w, "  failed: %s %s: %s\n", f.Tenant, f.Kind, f.Error)
	}
	return nil
}

type fleetPageMode int

const (
	pageByNumber  fleetPageMode = iota // page / pageSize, meta.currentPage / totalPages
	pageBySkip                         // skip / take, meta.totalItems or total
	pageNone                           // single request returning a list
	pageSingleton                      // single request returning one object
)

// fetchAll walks a list route. It returns an errFleetIncomplete error (with
// the rows read so far) when --max-pages stopped it or the pages contradict
// themselves, and any HTTP or decoding error as is.
func (s *fleetSyncer) fetchAll(path string, params, headers map[string]string, mode fleetPageMode) ([]json.RawMessage, error) {
	var all []json.RawMessage
	pt := newPageTracker(mode, s.pageSize)
	for page := 1; page <= s.maxPages; page++ {
		p := make(map[string]string, len(params)+2)
		for k, v := range params {
			p[k] = v
		}
		switch mode {
		case pageByNumber:
			p["page"] = strconv.Itoa(page)
			p["pageSize"] = strconv.Itoa(s.pageSize)
		case pageBySkip:
			p["skip"] = strconv.Itoa(len(all))
			p["take"] = strconv.Itoa(s.pageSize)
		}
		raw, err := s.c.GetWithHeadersNoCache(s.ctx, path, p, headers)
		if err != nil {
			return all, err
		}
		items, meta, err := splitFleetPage(raw, mode == pageSingleton)
		if err != nil {
			return all, fmt.Errorf("GET %s: %w", path, err)
		}
		all = append(all, items...)
		if mode == pageSingleton {
			return all, nil
		}
		done, err := pt.observe(page, items, meta)
		if err != nil {
			return all, err
		}
		if done {
			return all, nil
		}
	}
	return all, errFleetTruncated
}

// pageTracker decides when a list walk is complete. It keeps the strictest
// totals any page advertised, checks that page numbers and skip offsets
// advance as requested, and rejects repeated row ids. Any contradiction is an
// errFleetIncomplete error: the rows are kept, but nothing is pruned on them.
type pageTracker struct {
	mode     fleetPageMode
	pageSize int
	rows     int
	expTotal float64
	hasTotal bool
	expPages float64
	hasPages bool
	seen     map[string]bool
}

func newPageTracker(mode fleetPageMode, pageSize int) *pageTracker {
	return &pageTracker{mode: mode, pageSize: pageSize, seen: map[string]bool{}}
}

func fleetIncomplete(why string) error { return fmt.Errorf("%w: %s", errFleetIncomplete, why) }

// observe records one fetched page (1-based) and reports whether the walk is done.
func (pt *pageTracker) observe(page int, items []json.RawMessage, meta map[string]any) (bool, error) {
	if meta != nil {
		if cp, ok := fieldNum(meta, "currentPage"); ok && pt.mode != pageBySkip && int(cp) != page {
			return false, fleetIncomplete(fmt.Sprintf("asked for page %d, got page %v", page, cp))
		}
		if sk, ok := fieldNum(meta, "skip"); ok && pt.mode == pageBySkip && int(sk) != pt.rows {
			return false, fleetIncomplete(fmt.Sprintf("asked to skip %d, got skip %v", pt.rows, sk))
		}
		if tp, ok := fieldNum(meta, "totalPages"); ok && (!pt.hasPages || tp > pt.expPages) {
			pt.expPages, pt.hasPages = tp, true
		}
		if ti, ok := fieldNum(meta, "totalItems"); ok && (!pt.hasTotal || ti > pt.expTotal) {
			pt.expTotal, pt.hasTotal = ti, true
		}
	}
	for _, it := range items {
		var obj map[string]any
		if json.Unmarshal(it, &obj) != nil {
			continue
		}
		if id := fieldStr(obj, "id"); id != "" {
			if pt.seen[id] {
				return false, fleetIncomplete(fmt.Sprintf("row %q repeated across pages", id))
			}
			pt.seen[id] = true
		}
	}
	pt.rows += len(items)

	if pt.hasTotal || pt.hasPages {
		if len(items) == 0 {
			// Only a first page that advertises an empty list may be empty.
			if page == 1 && pt.rows == 0 && (!pt.hasTotal || pt.expTotal == 0) && (!pt.hasPages || pt.expPages <= 1) {
				return true, nil
			}
			return false, errFleetShortPage
		}
		if pt.mode == pageNone && pt.hasPages && pt.expPages > 1 {
			return false, errFleetShortPage
		}
		// Every advertised bound must hold before the walk counts as complete.
		totalOK := !pt.hasTotal || float64(pt.rows) >= pt.expTotal
		pagesOK := !pt.hasPages || float64(page) >= pt.expPages
		if totalOK && pagesOK {
			return true, nil
		}
		if pt.mode == pageNone {
			return false, errFleetShortPage
		}
		return false, nil
	}
	// No metadata on any page: a single request, an empty page, or a short
	// page ends the list.
	return pt.mode == pageNone || len(items) < pt.pageSize, nil
}

// splitFleetPage accepts a bare array, {data: [...], meta} or {items: [...],
// total}. A bare object is one row only for singleton routes (the posture
// rating); on a collection route it is an error. Anything malformed is an
// error rather than an empty page, so a bad response never wipes stored rows.
func splitFleetPage(raw json.RawMessage, singleton bool) ([]json.RawMessage, map[string]any, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil, fmt.Errorf("response body is empty or null")
	}
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) == nil {
		return arr, nil, nil
	}
	var env map[string]json.RawMessage
	if err := json.Unmarshal(raw, &env); err != nil || env == nil {
		return nil, nil, fmt.Errorf("response is not a JSON array or object")
	}
	for _, key := range []string{"data", "items"} {
		v, ok := env[key]
		if !ok {
			continue
		}
		if err := json.Unmarshal(v, &arr); err != nil || arr == nil {
			return nil, nil, fmt.Errorf("response %q is not an array", key)
		}
		var meta map[string]any
		if m, ok := env["meta"]; ok {
			_ = json.Unmarshal(m, &meta)
		}
		if t, ok := env["total"]; ok {
			var n float64
			if json.Unmarshal(t, &n) == nil {
				if meta == nil {
					meta = map[string]any{}
				}
				// Conflicting totals keep the larger claim (fails closed).
				if prev, ok := fieldNum(meta, "totalItems"); !ok || n > prev {
					meta["totalItems"] = n
				}
			}
		}
		if len(meta) == 0 {
			meta = nil
		}
		return arr, meta, nil
	}
	if !singleton {
		return nil, nil, fmt.Errorf("expected a list response, got an object without data or items")
	}
	return []json.RawMessage{raw}, nil, nil
}

func fleetStatus(err error) int {
	var ae *client.APIError
	if errors.As(err, &ae) {
		return ae.StatusCode
	}
	return 0
}

// fail records a failed or incomplete fetch. When fkind is set, the (fkind,
// scope) sync state is marked so readers warn about the gap.
func (s *fleetSyncer) fail(tenant, kind, fkind, scope string, err error) {
	msg := fleetErrText(err)
	switch {
	case errors.Is(err, errFleetTruncated):
		msg = fmt.Sprintf("incomplete: stopped at --max-pages=%d; raise it", s.maxPages)
		s.sum.Truncated = append(s.sum.Truncated, strings.TrimSpace(kind+" "+tenant))
	case errors.Is(err, errFleetIncomplete):
		msg = errFleetShortPage.Error()
	case fleetStatus(err) == 403:
		msg = "not entitled (403): " + msg
	}
	s.sum.FetchFailures = append(s.sum.FetchFailures, fleetSyncFailure{Tenant: tenant, Kind: kind, Error: msg})
	if fkind != "" {
		if err := recordFleetFailure(s.db.DB(), fkind, scope, errors.New(msg), s.now); err != nil && s.storeErr == nil {
			s.storeErr = fmt.Errorf("recording %s failure for %q: %w", kind, scope, err)
		}
	}
}

// fleetErrText describes an error without the HTTP response body, which can
// echo sensitive data (a dark-web exposure's password, for one). The text is
// stored in fleet_sync_state and printed under fetch_failures.
func fleetErrText(err error) string {
	var ae *client.APIError
	if errors.As(err, &ae) {
		return fmt.Sprintf("%s %s returned HTTP %d", ae.Method, ae.Path, ae.StatusCode)
	}
	return err.Error()
}

// requireIDs rejects inventory rows whose payload lacks a non-empty string
// "id"; pruning keyed on a fallback key would delete real tenants or accounts.
func requireIDs(rows []fleetRow) error {
	for i, r := range rows {
		var obj map[string]any
		if err := json.Unmarshal(r.Data, &obj); err != nil {
			return fmt.Errorf("inventory row %d: %w", i, err)
		}
		if id, ok := obj["id"].(string); !ok || strings.TrimSpace(id) == "" {
			return fmt.Errorf("inventory row %d has no id", i)
		}
	}
	return nil
}

// toFleetRows keys each item by prefix+id, tags it with its scope, and applies
// redact before storing. A non-object row is an error.
func toFleetRows(items []json.RawMessage, tags map[string]string, prefix string, redact func(map[string]any)) ([]fleetRow, error) {
	rows := make([]fleetRow, 0, len(items))
	for i, it := range items {
		var obj map[string]any
		if err := json.Unmarshal(it, &obj); err != nil || obj == nil {
			return nil, fmt.Errorf("row %d is not a JSON object", i)
		}
		if redact != nil {
			redact(obj)
		}
		id := fieldStr(obj, "id", "code")
		if id == "" {
			id = "#" + strconv.Itoa(i)
		}
		for k, v := range tags {
			obj[k] = v
		}
		data, err := json.Marshal(obj)
		if err != nil {
			return nil, err
		}
		rows = append(rows, fleetRow{ID: prefix + id, Data: data})
	}
	return rows, nil
}

// put stores one fetched slice. complete=false (a truncated fetch) merges the
// rows without pruning and records the slice as incomplete.
func (s *fleetSyncer) put(fkind, scope string, rows []fleetRow, complete bool) error {
	if complete {
		return replaceFleetScope(s.db.DB(), fkind, scope, rows, s.now)
	}
	return mergeFleetRows(s.db.DB(), fkind, scope, rows, s.now, "incomplete fetch; rows kept without pruning")
}

// index feeds tenant-scoped rows to the generated store under tenant-qualified
// ids so `search` covers them, and on a complete fetch prunes that tenant's
// rows the API no longer returns.
func (s *fleetSyncer) index(fkind, tenantID string, rows []fleetRow, complete bool) error {
	resource, ok := fleetIndexed[fkind]
	if !ok {
		return nil
	}
	items := make([]json.RawMessage, 0, len(rows))
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		var obj map[string]any
		if err := json.Unmarshal(r.Data, &obj); err != nil {
			return err
		}
		qid := tenantID + ":" + r.ID
		obj["api_id"] = obj["id"]
		obj["id"] = qid
		b, err := json.Marshal(obj)
		if err != nil {
			return err
		}
		items = append(items, b)
		ids = append(ids, qid)
	}
	if len(items) > 0 {
		if _, _, err := s.db.UpsertBatch(resource, items); err != nil {
			return fmt.Errorf("indexing %s for search: %w", resource, err)
		}
	}
	if complete {
		table, _ := store.TypedListTable(resource)
		if _, err := s.db.ReconcilePartition(resource, "$._tenant_id", tenantID, ids, table, nil); err != nil {
			return fmt.Errorf("pruning %s search index: %w", resource, err)
		}
	}
	return nil
}

func (s *fleetSyncer) syncTenants() ([]fleetTenant, error) {
	items, err := s.fetchAll("/tenants", nil, nil, pageByNumber)
	complete := err == nil
	if err != nil && !errors.Is(err, errFleetIncomplete) {
		return nil, err
	}
	if !complete {
		s.fail("", "tenants", "", "", err)
	}
	rows, err := toFleetRows(items, nil, "", nil)
	if err == nil {
		err = requireIDs(rows)
	}
	if err != nil {
		return nil, fmt.Errorf("GET /tenants: %w", err)
	}
	if complete {
		if err := s.pruneRemovedTenants(rows); err != nil {
			return nil, err
		}
	}
	if err := s.put(fleetKindTenant, "", rows, complete); err != nil {
		return nil, err
	}
	raw := make([]json.RawMessage, 0, len(rows))
	for _, r := range rows {
		raw = append(raw, r.Data)
	}
	if len(raw) > 0 {
		if _, _, err := s.db.UpsertBatch("tenants", raw); err != nil {
			return nil, fmt.Errorf("indexing tenants: %w", err)
		}
	}
	if complete {
		ids := make([]string, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.ID)
		}
		table, _ := store.TypedListTable("tenants")
		if _, err := s.db.ReconcileAll("tenants", ids, table, nil); err != nil {
			return nil, fmt.Errorf("pruning tenants: %w", err)
		}
		_ = s.db.SaveSyncState("tenants", "", len(rows))
	}
	return loadFleetTenants(s.db.DB())
}

// pruneRemovedTenants drops every fleet slice and search row of tenants that a
// complete inventory no longer lists.
func (s *fleetSyncer) pruneRemovedTenants(current []fleetRow) error {
	prev, err := loadFleetRows(s.db.DB(), fleetKindTenant)
	if err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, r := range current {
		keep[r.ID] = true
	}
	for _, p := range prev {
		if keep[p.ID] {
			continue
		}
		if err := deleteFleetTenantScope(s.db.DB(), p.ID); err != nil {
			return err
		}
		for _, resource := range fleetIndexed {
			table, _ := store.TypedListTable(resource)
			if _, err := s.db.ReconcilePartition(resource, "$._tenant_id", p.ID, nil, table, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *fleetSyncer) syncContacts() bool {
	accounts, err := s.fetchAll("/accounts", nil, nil, pageByNumber)
	accComplete := err == nil
	if err != nil {
		s.fail("", "contacts", fleetKindAccount, "", fmt.Errorf("listing accounts: %w", err))
		if !errors.Is(err, errFleetIncomplete) {
			return false
		}
	}
	accRows, err := toFleetRows(accounts, nil, "", nil)
	if err == nil {
		err = requireIDs(accRows)
	}
	if err == nil && accComplete {
		err = s.pruneRemovedAccounts(accRows)
	}
	if err == nil {
		err = s.put(fleetKindAccount, "", accRows, accComplete)
	}
	if err != nil {
		s.fail("", "contacts", fleetKindAccount, "", fmt.Errorf("accounts: %w", err))
		return false
	}
	ok, total := accComplete, 0
	for _, a := range accRows {
		path := "/accounts/" + a.ID + "/contact-groups"
		items, ferr := s.fetchAll(path, map[string]string{"membersPageSize": "50", "tenantsPageSize": "0"}, nil, pageByNumber)
		complete := ferr == nil
		if ferr != nil {
			s.fail("", "contacts", fleetKindContactGroup, a.ID, fmt.Errorf("account %s: %w", a.ID, ferr))
			ok = false
			if !errors.Is(ferr, errFleetIncomplete) {
				continue
			}
		}
		rows, rerr := toFleetRows(items, map[string]string{"_account_id": a.ID}, "", nil)
		if rerr == nil {
			rerr = s.put(fleetKindContactGroup, a.ID, rows, complete)
		}
		if rerr != nil {
			s.fail("", "contacts", fleetKindContactGroup, a.ID, fmt.Errorf("account %s: %w", a.ID, rerr))
			ok = false
			continue
		}
		total += len(rows)
	}
	s.sum.Rows["contacts"] = total
	return ok
}

// pruneRemovedAccounts drops contact groups of accounts a complete inventory
// no longer lists.
func (s *fleetSyncer) pruneRemovedAccounts(current []fleetRow) error {
	prev, err := loadFleetRows(s.db.DB(), fleetKindAccount)
	if err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, r := range current {
		keep[r.ID] = true
	}
	for _, p := range prev {
		if !keep[p.ID] {
			if err := deleteFleetScope(s.db.DB(), fleetKindContactGroup, p.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func redactDarkweb(obj map[string]any) {
	// Keep whether a password leaked, never the value.
	if v, ok := obj["password"]; ok {
		obj["passwordExposed"] = v != nil && v != "" && v != false
		delete(obj, "password")
	}
}

// syncTenantKind fetches one kind for one tenant. It returns the stored row
// count and whether the fetch was complete and stored.
func (s *fleetSyncer) syncTenantKind(kind string, t fleetTenant) (int, bool) {
	hdr := map[string]string{"x-tenant-id": t.ID}
	var (
		fkind, path string
		params      map[string]string
		mode        = pageByNumber
		redact      func(map[string]any)
	)
	switch kind {
	case "detections":
		fkind, path, mode = fleetKindDetection, "/alert-groups", pageBySkip
		params = map[string]string{"status": "OPEN"}
	case "devices":
		fkind, path = fleetKindDevice, "/assets"
		params = map[string]string{"class": "DEVICE"}
	case "vulnerabilities":
		fkind, path = fleetKindVuln, "/vulnerability-management/vulnerabilities"
		params = map[string]string{"hideResolvedVulnerabilities": "true"}
	case "scans":
		fkind, path = fleetKindScan, "/vulnerability-management/scans"
		params = map[string]string{"sortBy": "createdOn", "sortOrder": "DESC"}
	case "darkweb":
		fkind, path, redact = fleetKindDarkweb, "/vulnerability-management/darkweb/scan/exposures", redactDarkweb
	case "posture":
		fkind, path, mode = fleetKindPosture, "/security-posture/rating", pageSingleton
	case "cloud":
		return s.syncCloud(t)
	}
	items, err := s.fetchAll(path, params, hdr, mode)
	complete := err == nil
	if err != nil {
		s.fail(t.Name, kind, fkind, t.ID, err)
		if !errors.Is(err, errFleetIncomplete) {
			return 0, false
		}
	}
	rows, err := toFleetRows(items, map[string]string{"_tenant_id": t.ID, "_tenant_name": t.Name}, "", redact)
	if err == nil {
		err = s.put(fkind, t.ID, rows, complete)
	}
	if err == nil {
		err = s.index(fkind, t.ID, rows, complete)
	}
	if err != nil {
		s.fail(t.Name, kind, fkind, t.ID, fmt.Errorf("%s: %w", path, err))
		return 0, false
	}
	return len(rows), complete
}

// syncCloud stores a tenant's Cloud Response state: M365 connections and their
// users, Cisco Duo and Google onboardings, and each onboarding connection's
// approved countries and users. A 404 means the tenant has no such connection.
// All parts land together: if any part fails or is truncated, no cloud slice
// for the tenant is replaced and the tenant's cloud status records the gap.
func (s *fleetSyncer) syncCloud(t fleetTenant) (int, bool) {
	slices, err := s.fetchCloud(t)
	if err != nil {
		s.fail(t.Name, "cloud", fleetKindCloudStatus, t.ID, err)
		return 0, false
	}
	total := 0
	all := map[string][]fleetRow{fleetKindCloudStatus: nil}
	for _, kind := range []string{fleetKindM365Conn, fleetKindM365User, fleetKindOnboarding, fleetKindConnCountry, fleetKindConnUser} {
		all[kind] = slices[kind]
		total += len(slices[kind])
	}
	if err := replaceFleetScopes(s.db.DB(), t.ID, all, s.now); err != nil {
		s.fail(t.Name, "cloud", fleetKindCloudStatus, t.ID, err)
		return 0, false
	}
	return total, true
}

func (s *fleetSyncer) fetchCloud(t fleetTenant) (map[string][]fleetRow, error) {
	q := map[string]string{"tenantId": t.ID}
	tags := func(extra ...string) map[string]string {
		m := map[string]string{"_tenant_id": t.ID, "_tenant_name": t.Name}
		for i := 0; i+1 < len(extra); i += 2 {
			m[extra[i]] = extra[i+1]
		}
		return m
	}
	notFound := func(err error) bool { return fleetStatus(err) == 404 }
	out := map[string][]fleetRow{}

	raw, err := s.c.GetWithHeadersNoCache(s.ctx, "/cloud/ms365/customer", q, nil)
	var packages []json.RawMessage
	switch {
	case err == nil:
		var cust map[string]json.RawMessage
		if derr := json.Unmarshal(raw, &cust); derr != nil || cust == nil {
			return nil, fmt.Errorf("ms365 customer: response is not a JSON object")
		}
		if p, ok := cust["ms365DefensePackages"]; ok && string(p) != "null" {
			if derr := json.Unmarshal(p, &packages); derr != nil {
				return nil, fmt.Errorf("ms365 customer: ms365DefensePackages is not an array")
			}
		}
	case notFound(err):
	default:
		return nil, fmt.Errorf("ms365 customer: %w", err)
	}
	conns, err := toFleetRows(packages, tags(), "m365/", nil)
	if err != nil {
		return nil, fmt.Errorf("ms365 connections: %w", err)
	}
	out[fleetKindM365Conn] = conns
	for _, conn := range conns {
		connID := strings.TrimPrefix(conn.ID, "m365/")
		items, ferr := s.fetchAll("/cloud/ms365/connections/"+connID+"/users", q, nil, pageBySkip)
		if ferr != nil {
			return nil, fmt.Errorf("ms365 users %s: %w", connID, ferr)
		}
		users, rerr := toFleetRows(items, tags("_connectionId", connID), "m365/"+connID+"/", nil)
		if rerr != nil {
			return nil, fmt.Errorf("ms365 users %s: %w", connID, rerr)
		}
		out[fleetKindM365User] = append(out[fleetKindM365User], users...)
	}

	for _, provider := range []string{"cisco", "google"} {
		items, ferr := s.fetchAll("/cloud/"+provider+"/onboardings", q, nil, pageNone)
		if ferr != nil {
			if notFound(ferr) {
				continue
			}
			return nil, fmt.Errorf("%s onboardings: %w", provider, ferr)
		}
		onbs, rerr := toFleetRows(items, tags("_provider", provider), provider+"/", nil)
		if rerr != nil {
			return nil, fmt.Errorf("%s onboardings: %w", provider, rerr)
		}
		out[fleetKindOnboarding] = append(out[fleetKindOnboarding], onbs...)
		for _, onb := range onbs {
			var obj map[string]any
			_ = json.Unmarshal(onb.Data, &obj)
			connID := fieldStr(obj, "connectionId")
			if connID == "" {
				continue
			}
			base := "/cloud/connections/" + connID
			prefix := provider + "/" + connID + "/"
			citems, cerr := s.fetchAll(base+"/approved-countries", q, nil, pageBySkip)
			if cerr != nil {
				return nil, fmt.Errorf("connection %s countries: %w", connID, cerr)
			}
			crows, rerr := toFleetRows(citems, tags("_connectionId", connID, "_provider", provider), prefix, nil)
			if rerr != nil {
				return nil, fmt.Errorf("connection %s countries: %w", connID, rerr)
			}
			out[fleetKindConnCountry] = append(out[fleetKindConnCountry], crows...)
			uitems, uerr := s.fetchAll(base+"/users", q, nil, pageBySkip)
			if uerr != nil {
				return nil, fmt.Errorf("connection %s users: %w", connID, uerr)
			}
			urows, rerr := toFleetRows(uitems, tags("_connectionId", connID, "_provider", provider), prefix, nil)
			if rerr != nil {
				return nil, fmt.Errorf("connection %s users: %w", connID, rerr)
			}
			out[fleetKindConnUser] = append(out[fleetKindConnUser], urows...)
		}
	}
	return out, nil
}
