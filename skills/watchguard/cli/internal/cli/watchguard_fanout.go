// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"watchguard-pp-cli/internal/client"
	"watchguard-pp-cli/internal/cliutil"
)

// Shared plumbing for the cross-tenant (Service Provider -> Subscriber) commands.
// Field names and code tables below come from WatchGuard's published Endpoint
// Security, Accounts v2, Allocation v2, Firebox and ThreatSync OpenAPI files.

func init() {
	// Keeps audience-scoped requests scoped (WATCHGUARD_AUDIENCE and fan-out tokens).
	registerClientHook(client.InstallScopeTransport)
}

const (
	maxFanoutConcurrency = 16
	maxListPages         = 200
)

type subscriber struct {
	ID   string `json:"account_id"`
	Name string `json:"account_name,omitempty"`
}

func (s subscriber) label() string {
	if s.Name != "" {
		return s.Name
	}
	return s.ID
}

type fetchFailure struct {
	AccountID string `json:"account_id,omitempty"`
	Source    string `json:"source,omitempty"`
	Error     string `json:"error"`
}

func fanoutFailures(errs []cliutil.FanoutError) []fetchFailure {
	out := make([]fetchFailure, 0, len(errs))
	for _, e := range errs {
		out = append(out, fetchFailure{AccountID: e.Source, Error: client.ScrubScoped(e.Err).Error()})
	}
	return out
}

// tenantFlags are the account-selection flags every fan-out command shares.
type tenantFlags struct {
	accounts    string
	includeSelf bool
	concurrency int
	maxPages    int
}

func (t *tenantFlags) register(cmdFlags interface {
	StringVar(*string, string, string, string)
	BoolVar(*bool, string, bool, string)
	IntVar(*int, string, int, string)
}) {
	cmdFlags.StringVar(&t.accounts, "accounts", "", "Comma-separated account IDs to check (default: every account directly managed by --account-id, including delegated ones)")
	cmdFlags.BoolVar(&t.includeSelf, "include-self", false, "Also check the --account-id account itself, not only the accounts it manages")
	cmdFlags.IntVar(&t.concurrency, "concurrency", 4, fmt.Sprintf("Accounts fetched in parallel (1-%d)", maxFanoutConcurrency))
	cmdFlags.IntVar(&t.maxPages, "max-pages", 20, fmt.Sprintf("Maximum list pages read per account and per lookup (1-%d); hitting it is reported as truncated", maxListPages))
}

func (t *tenantFlags) workers() int { return clampInt(t.concurrency, 1, maxFanoutConcurrency) }

func (t *tenantFlags) pages() int { return listPages(t.maxPages) }

func listPages(n int) int {
	if cliutil.IsDogfoodEnv() {
		return 1
	}
	return clampInt(n, 1, maxListPages)
}

func clampInt(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

func ownAccount(c *client.Client) (string, error) {
	if own := c.OwnAccountID(); own != "" {
		return own, nil
	}
	return "", usageErr(fmt.Errorf("an account ID is required: set WATCHGUARD_ACCOUNT_ID or pass --account-id"))
}

// resolveSubscribers returns the accounts to fan out over: --accounts when given,
// otherwise every account directly managed by the caller's account (one level,
// delegated accounts included). An account with no managed accounts checks itself.
// truncated reports that --max-pages stopped the managed-account listing.
func resolveSubscribers(ctx context.Context, c *client.Client, t tenantFlags) (subs []subscriber, truncated bool, err error) {
	// EnvOverride treats an unresolved .mcpb ${user_config...} placeholder as unset.
	if strings.TrimSpace(cliutil.EnvOverride("WATCHGUARD_AUDIENCE")) != "" {
		return nil, false, usageErr(fmt.Errorf("%s pins this process to one managed account; unset it to check several accounts (use --accounts to pick them)", client.AudienceEnvVar))
	}
	own, err := ownAccount(c)
	if err != nil {
		return nil, false, err
	}
	seen := map[string]bool{}
	add := func(s subscriber) {
		if s.ID != "" && !seen[s.ID] {
			seen[s.ID] = true
			subs = append(subs, s)
		}
	}
	if t.includeSelf {
		add(subscriber{ID: own})
	}
	if strings.TrimSpace(t.accounts) != "" {
		for _, id := range strings.Split(t.accounts, ",") {
			add(subscriber{ID: strings.TrimSpace(id)})
		}
	} else {
		children, trunc, err := listChildren(ctx, c, own, t.pages())
		if err != nil {
			return nil, false, err
		}
		truncated = trunc
		for _, ch := range children {
			add(ch)
		}
		if len(subs) == 0 {
			add(subscriber{ID: own})
		}
	}
	// Mint the caller's own token once before workers share the client, so the
	// generated token refresh does not race at fan-out start.
	if err := c.WarmAuth(ctx); err != nil {
		return nil, false, err
	}
	return subs, truncated, nil
}

// accountsTruncatedNote is the shared warning when the managed-account listing was cut short.
func accountsTruncatedNote(w io.Writer, truncated bool) string {
	if !truncated {
		return ""
	}
	note := "the managed-account list was cut short by --max-pages; some accounts were not checked"
	fmt.Fprintln(w, "warning: "+note)
	return note
}

func listChildren(ctx context.Context, c *client.Client, own string, maxPages int) ([]subscriber, bool, error) {
	rows, truncated, err := pageLimitOffset(ctx, c, "/rest/platform/accounts/v2/accounts/"+url.PathEscape(own)+"/children",
		map[string]string{"includeDelegatedAccounts": "true"}, nil, "items", 100, maxPages)
	if err != nil {
		return nil, false, fmt.Errorf("listing managed accounts of %s: %w", own, err)
	}
	out := make([]subscriber, 0, len(rows))
	for _, r := range rows {
		if id := strField(r, "accountId"); id != "" {
			out = append(out, subscriber{ID: id, Name: strField(r, "name")})
		}
	}
	return out, truncated, nil
}

// tenantGet issues a GET for a managed account with its audience-scoped token.
func tenantGet(ctx context.Context, c *client.Client, accountID, path string, params map[string]string) (json.RawMessage, error) {
	headers, err := c.TenantHeaders(ctx, accountID)
	if err != nil {
		return nil, client.ScrubScoped(err)
	}
	data, err := c.GetWithHeaders(ctx, path, params, headers)
	return data, client.ScrubScoped(err)
}

// pageOData walks an Endpoint Security list ({"data":[...],"total_items":N})
// with OData $top/$skip. truncated reports rows left unread.
func pageOData(ctx context.Context, c *client.Client, accountID, path string, maxPages int) (rows []map[string]any, truncated bool, err error) {
	const top = 100
	rows = make([]map[string]any, 0)
	for page := 0; page < maxPages; page++ {
		params := map[string]string{"$top": strconv.Itoa(top), "$skip": strconv.Itoa(len(rows)), "$count": "true"}
		data, err := tenantGet(ctx, c, accountID, path, params)
		if err != nil {
			return rows, false, err
		}
		batch, err := listRows(data, "data")
		if err != nil {
			return rows, false, err
		}
		rows = append(rows, batch...)
		total := totalCount(data)
		if pagingDone(len(batch), top, total, len(rows)) {
			return rows, total > 0 && len(rows) < total, nil
		}
	}
	return rows, true, nil
}

// pageLimitOffset walks a platform list ({"<key>":[...],"pageControls":{totalItems}})
// with limit/offset. headers may be nil for the caller's own account.
func pageLimitOffset(ctx context.Context, c *client.Client, path string, extra, headers map[string]string, key string, limit, maxPages int) (rows []map[string]any, truncated bool, err error) {
	rows = make([]map[string]any, 0)
	for page := 0; page < maxPages; page++ {
		params := map[string]string{"limit": strconv.Itoa(limit), "offset": strconv.Itoa(len(rows))}
		for k, v := range extra {
			params[k] = v
		}
		data, err := c.GetWithHeaders(ctx, path, params, headers)
		if err != nil {
			return rows, false, client.ScrubScoped(err)
		}
		batch, err := listRows(data, key)
		if err != nil {
			return rows, false, err
		}
		rows = append(rows, batch...)
		total := totalCount(data)
		if pagingDone(len(batch), limit, total, len(rows)) {
			return rows, total > 0 && len(rows) < total, nil
		}
	}
	return rows, true, nil
}

// pagingDone decides whether a paged walk is complete: an empty page always ends
// it; a reported total wins over page size (servers may cap pages below the ask).
func pagingDone(got, asked, total, have int) bool {
	if got == 0 {
		return true
	}
	if total > 0 {
		return have >= total
	}
	return got < asked
}

// totalCount reads total_items (Endpoint Security) or pageControls.totalItems (platform).
func totalCount(raw json.RawMessage) int {
	var env struct {
		TotalItems   int `json:"total_items"`
		PageControls struct {
			TotalItems int `json:"totalItems"`
		} `json:"pageControls"`
	}
	if json.Unmarshal(raw, &env) != nil {
		var wrapped []json.RawMessage // [{"data":{...},"total_items":N}, ...]
		if json.Unmarshal(raw, &wrapped) == nil && len(wrapped) > 0 {
			return totalCount(wrapped[0])
		}
		return 0
	}
	if env.TotalItems > 0 {
		return env.TotalItems
	}
	return env.PageControls.TotalItems
}

// listRows is asMaps for a read whose answer feeds a report: a 2xx body that
// carries neither a top-level list nor the expected key is a failed read, never
// an empty one. Hand-fix (handfixes.json fanout-partial-data-exit): an
// {"unexpected": ...} body used to count as zero devices, zero endpoints or
// zero incidents and let accounts health report the account complete.
func listRows(raw json.RawMessage, key string) ([]map[string]any, error) {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil, fmt.Errorf("unexpected response: not JSON")
	}
	var list []any
	switch t := v.(type) {
	case []any:
		list = t
	case map[string]any:
		inner, ok := t[key]
		if !ok {
			return nil, fmt.Errorf("unexpected response: no %q list", key)
		}
		// Hand-fix (handfixes.json: fanout-malformed-collection-is-failed-read):
		// {"data":null} or {"data":"unavailable"} is not an empty fleet.
		if list, ok = inner.([]any); !ok {
			return nil, fmt.Errorf("unexpected response: %q is not a list", key)
		}
	default:
		return nil, fmt.Errorf("unexpected response: no %q list", key)
	}
	for _, e := range list {
		if _, ok := e.(map[string]any); !ok {
			return nil, fmt.Errorf("unexpected response: %q list holds a non-object row", key)
		}
	}
	return asMaps(raw, key), nil
}

// asMaps extracts a list of objects from an envelope: {key:[...]}, [...] or
// [{key:{...}}, ...]. A top-level object without key is not a row.
func asMaps(raw json.RawMessage, key string) []map[string]any {
	out := make([]map[string]any, 0)
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return out
	}
	var visit func(any)
	visit = func(x any) {
		switch t := x.(type) {
		case []any:
			for _, e := range t {
				visit(e)
			}
		case map[string]any:
			if inner, ok := t[key]; ok && len(t) <= 2 {
				visit(inner)
				return
			}
			out = append(out, t)
		}
	}
	switch t := v.(type) {
	case []any:
		visit(t)
	case map[string]any:
		if inner, ok := t[key]; ok {
			visit(inner)
		}
	}
	return out
}

// firstObject reads a single-record response that may be a bare object or a
// one-element list (the transaction GET is documented both ways).
func firstObject(raw json.RawMessage, key string) map[string]any {
	if rows := asMaps(raw, key); len(rows) > 0 && len(rows[0]) > 0 {
		return rows[0]
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) == nil && len(m) > 0 {
		if _, envelope := m[key]; !envelope {
			return m
		}
	}
	return nil
}

func strField(m map[string]any, k string) string {
	switch v := m[k].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	}
	return ""
}

func intField(m map[string]any, k string) (int, bool) {
	switch v := m[k].(type) {
	case float64:
		return int(v), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		return n, err == nil
	}
	return 0, false
}

func boolField(m map[string]any, k string) bool {
	b, _ := m[k].(bool)
	return b
}

func strList(m map[string]any, k string) []string {
	out := make([]string, 0)
	if arr, ok := m[k].([]any); ok {
		for _, e := range arr {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// parseTime accepts RFC 3339 and the space-separated form ThreatSync documents
// ("2023-03-02 18:52:20+00:00").
func parseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999Z07:00", "2006-01-02 15:04:05Z07:00", "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// Endpoint Security integer code tables (management-1.5.2.yaml descriptions).
var (
	protectionCodes  = map[int]string{0: "no protection or no information", 1: "activated", 2: "deactivated", 3: "error", 4: "error installing", 5: "without license", 6: "not available"}
	isolationCodes   = map[int]string{0: "not isolated", 1: "isolated", 2: "applying isolation", 3: "removing isolation"}
	engineCodes      = map[int]string{0: "undefined", 1: "outdated", 2: "updated", 3: "waiting for reboot"}
	licenseCodes     = map[int]string{0: "assigned", 1: "not assigned", 2: "manual deallocation", 3: "auto deallocation"}
	unmanagedCodes   = map[int]string{0: "undefined", 1: "discovered", 2: "installing", 3: "error"}
	platformCodes    = map[int]string{1: "Windows", 2: "Linux", 3: "macOS", 4: "Android", 5: "iOS"}
	criticalityCodes = map[int]string{101: "other (non-security)", 201: "critical", 202: "important", 203: "moderate", 204: "low", 205: "unspecified security", 301: "service pack"}
	// Security criticality first (201 critical .. 205), then service packs and other patches.
	criticalityRank = map[int]int{201: 7, 202: 6, 203: 5, 204: 4, 205: 3, 301: 2, 101: 1}
	criticalityName = map[string]int{"critical": 7, "important": 6, "moderate": 5, "low": 4}
)

func decodeCode(table map[int]string, m map[string]any, k string) string {
	n, ok := intField(m, k)
	if !ok {
		return ""
	}
	if label, ok := table[n]; ok {
		return label
	}
	return "code " + strconv.Itoa(n)
}

// protectionIssues lists why a protection-status row needs attention; empty means healthy.
func protectionIssues(m map[string]any) []string {
	issues := make([]string, 0)
	// Hand-fix (handfixes.json fanout-partial-data-exit): a row with none of
	// the status fields is unknown, not healthy.
	known := false
	for _, k := range []string{"accumulated_protection_status", "isolation_state", "protection_engine_update_status", "license_status"} {
		if _, ok := intField(m, k); ok {
			known = true
		}
	}
	if !known {
		issues = append(issues, "protection status unknown (no status fields in the response)")
	}
	if n, ok := intField(m, "accumulated_protection_status"); ok && n != 1 {
		issues = append(issues, "protection "+decodeCode(protectionCodes, m, "accumulated_protection_status"))
	}
	if n, ok := intField(m, "isolation_state"); ok && n != 0 {
		issues = append(issues, decodeCode(isolationCodes, m, "isolation_state"))
	}
	if n, ok := intField(m, "protection_engine_update_status"); ok && (n == 1 || n == 3) {
		issues = append(issues, "engine "+decodeCode(engineCodes, m, "protection_engine_update_status"))
	}
	if n, ok := intField(m, "license_status"); ok && n != 0 {
		issues = append(issues, "license "+decodeCode(licenseCodes, m, "license_status"))
	}
	for _, k := range []string{"reboot_requested", "reinstall_protection_requested", "reinstall_agent_requested"} {
		if boolField(m, k) {
			issues = append(issues, strings.ReplaceAll(k, "_", " "))
		}
	}
	return issues
}

var severityRank = map[string]int{"critical": 4, "high": 3, "medium": 2, "low": 1}

func sevRank(s string) int { return severityRank[strings.ToLower(strings.TrimSpace(s))] }

func isOffline(state string) bool { return state == "offline" || state == "never connected" }

func serialKey(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

// sortedMapKeys keeps map-derived output deterministic.
func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

var durationDays = regexp.MustCompile(`^([0-9]+)[dD]$`)

// incidentDuration normalizes --duration to ThreatSync's "<n>D" form.
func incidentDuration(s string) (string, error) {
	m := durationDays.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", usageErr(fmt.Errorf("--duration %q must be a number of days like 7D or 30d", s))
	}
	return m[1] + "D", nil
}

// fetchIncidents pages /incidents with sortBy=timestamp,id and startAfter set to
// the last record's "timestamp,id" (the documented cursor contract).
func fetchIncidents(ctx context.Context, c *client.Client, own, duration string, tenants bool, maxPages int) ([]map[string]any, bool, error) {
	path := "/rest/threatsync/management/v1/" + url.PathEscape(own) + "/incidents"
	all := make([]map[string]any, 0)
	after := ""
	for page := 0; page < maxPages; page++ {
		params := map[string]string{
			"limit": "500", "sortBy": "timestamp,id", "sortOrder": "asc",
			"includeActions": "true", "includeComments": "true", "tenants": strconv.FormatBool(tenants),
		}
		if duration != "" {
			params["duration"] = duration
		}
		if after != "" {
			params["startAfter"] = after
		}
		data, err := c.Get(ctx, path, params)
		if err != nil {
			return all, false, err
		}
		batch, err := incidentRows(data)
		if err != nil {
			return all, false, err
		}
		all = append(all, batch...)
		if len(batch) < 500 {
			return all, false, nil
		}
		last := batch[len(batch)-1]
		after = strField(last, "timestamp") + "," + strField(last, "id")
	}
	return all, true, nil
}

// incidentRows reads {"items":[...]} and the grouped {"groups":{"<g>":[...]}} form the docs also show.
func incidentRows(raw json.RawMessage) ([]map[string]any, error) {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil, fmt.Errorf("unexpected response: not JSON")
	}
	objects := func(name string, list any) ([]map[string]any, error) {
		items, ok := list.([]any)
		if !ok {
			return nil, fmt.Errorf("unexpected response: %q is not a list", name)
		}
		out := make([]map[string]any, 0, len(items))
		for _, e := range items {
			m, ok := e.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("unexpected response: %q list holds a non-object row", name)
			}
			out = append(out, m)
		}
		return out, nil
	}
	switch t := v.(type) {
	case []any:
		if _, err := objects("items", t); err != nil {
			return nil, err
		}
		return asMaps(raw, "items"), nil
	case map[string]any:
		items, hasItems := t["items"]
		groups, hasGroups := t["groups"]
		if !hasItems && !hasGroups {
			return nil, fmt.Errorf("unexpected response: no \"items\" or \"groups\" list")
		}
		rows := make([]map[string]any, 0)
		if hasItems {
			// Hand-fix (handfixes.json: fanout-malformed-collection-is-failed-read).
			got, err := objects("items", items)
			if err != nil {
				return nil, err
			}
			rows = append(rows, got...)
		}
		if hasGroups {
			gm, ok := groups.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("unexpected response: \"groups\" is not an object")
			}
			for _, g := range sortedMapKeys(gm) {
				got, err := objects("groups."+g, gm[g])
				if err != nil {
					return nil, err
				}
				rows = append(rows, got...)
			}
		}
		return rows, nil
	}
	return nil, fmt.Errorf("unexpected response: no \"items\" or \"groups\" list")
}

// expiryOf returns when a row expires: the allocation/license expiry date, or
// now + daysUntilExpiry when only the day count is given.
func expiryOf(m map[string]any, now time.Time) (time.Time, string, bool) {
	date := strField(m, "allocationExpiryDate")
	if date == "" {
		date = strField(m, "licenseExpiryDate")
	}
	if t, ok := parseTime(date); ok {
		return t, date, true
	}
	if n, ok := intField(m, "daysUntilExpiry"); ok {
		return now.Add(time.Duration(n) * 24 * time.Hour), date, true
	}
	return time.Time{}, date, false
}

type expiringItem struct {
	ResourceType string `json:"resource_type"`
	Product      string `json:"product,omitempty"`
	Serial       string `json:"serial_or_license,omitempty"`
	AccountID    string `json:"account_id,omitempty"`
	AccountName  string `json:"account_name,omitempty"`
	ExpiryDate   string `json:"expiry_date"`
	DaysLeft     int    `json:"days_left"`
	Expired      bool   `json:"expired"`
}

// expiringWithin lists rows that expire by now+window; already-expired rows are kept, flagged expired.
func expiringWithin(resourceType string, rows []map[string]any, window time.Duration, now time.Time) []expiringItem {
	out := make([]expiringItem, 0)
	for _, m := range rows {
		at, date, ok := expiryOf(m, now)
		if !ok || at.After(now.Add(window)) {
			continue
		}
		if date == "" {
			date = at.UTC().Format(time.RFC3339)
		}
		out = append(out, expiringItem{ResourceType: resourceType, Product: strField(m, "productName"), Serial: strField(m, "serialOrLicense"),
			AccountID: strField(m, "accountId"), AccountName: strField(m, "accountName"), ExpiryDate: date,
			DaysLeft: int(math.Floor(at.Sub(now).Hours() / 24)), Expired: at.Before(now)})
	}
	return out
}

// fanoutVerdict reports partial and total failure after a fan-out's output is written:
// every account failed -> exit 5; some failed -> exit 6 unless --allow-partial-failure.
// readVerdict is the one exit rule every fan-out report follows (hand-fix,
// handfixes.json fanout-partial-data-exit): 0 when every read the report
// depends on succeeded, 5 when none did, 6 when some failed - an account, a
// product read inside an account, or a shared lookup - with
// --allow-partial-failure turning 6 into 0. WatchGuard documents no separate
// "product not licensed" answer (403 means "no permission"), so a product an
// account does not have counts as a failed read: fail closed.
func readVerdict(w io.Writer, flags *rootFlags, ok, failed int, what string) error {
	if failed == 0 {
		return nil
	}
	if ok == 0 {
		return apiErr(fmt.Errorf("every %s failed; see the errors in the output", what))
	}
	fmt.Fprintf(w, "warning: %d of %d %s failed; the output marks what is missing\n", failed, ok+failed, what)
	if flags.allowPartialFailure {
		return nil
	}
	return partialFailureErr(fmt.Errorf("%d of %d %s failed (pass --allow-partial-failure to exit 0)", failed, ok+failed, what))
}

func fanoutVerdict(w io.Writer, flags *rootFlags, failed, total int) error {
	if failed == 0 {
		return nil
	}
	if failed == total {
		return apiErr(fmt.Errorf("all %d account(s) failed; see fetch_failures", total))
	}
	fmt.Fprintf(w, "warning: %d of %d accounts failed; results cover the remaining %d (see fetch_failures)\n", failed, total, total-failed)
	if flags.allowPartialFailure {
		return nil
	}
	return partialFailureErr(fmt.Errorf("%d of %d accounts failed (pass --allow-partial-failure to exit 0)", failed, total))
}
