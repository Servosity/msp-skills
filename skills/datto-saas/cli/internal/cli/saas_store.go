// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Shared local-store helpers for the hand-written Datto SaaS Protection
// commands (seats roster/summary/reconcile/gaps/bulk-plan, backup-health,
// exit-inventory). Every loader decodes the synced JSON `data` column so the
// live path (exit-inventory) and the local path share one parser.

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"datto-saas-pp-cli/internal/store"
)

// flexNum accepts a JSON number or a numeric string. Datto publishes no public
// schema, so counts are decoded defensively; ok=false means absent or not
// numeric, never zero.
type flexNum struct {
	v  float64
	ok bool
}

func (n *flexNum) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*n = flexNum{}
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		*n = flexNum{}
		return nil
	}
	*n = flexNum{v: f, ok: true}
	return nil
}

// validCount is a present, non-negative count; anything else is missing data.
func validCount(n flexNum) bool { return n.ok && n.v >= 0 }

// validPair is a usable (total, part) count pair such as (active, recent).
func validPair(total, part flexNum) bool {
	return validCount(total) && validCount(part) && part.v <= total.v
}

func (n flexNum) ptr() *float64 {
	if !n.ok {
		return nil
	}
	v := n.v
	return &v
}

// flexStr accepts a JSON string, number or bool for fields whose type no
// public source pins down (timeWindow, status, dateAdded).
type flexStr string

func (f *flexStr) UnmarshalJSON(b []byte) error {
	t := strings.TrimSpace(string(b))
	if t == "null" {
		*f = ""
		return nil
	}
	if strings.HasPrefix(t, `"`) {
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		*f = flexStr(str)
		return nil
	}
	*f = flexStr(t)
	return nil
}

// flexID renders a numeric or string identifier as a string ("123456").
type flexID string

func (id *flexID) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*id = ""
		return nil
	}
	if strings.HasPrefix(s, `"`) {
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		*id = flexID(strings.TrimSpace(str))
		return nil
	}
	// Keep integer tokens exactly; a float64 round trip corrupts ids above 2^53.
	if _, err := strconv.ParseInt(s, 10, 64); err == nil {
		*id = flexID(s)
		return nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil && f == float64(int64(f)) && f < 1<<53 && f > -(1<<53) {
		*id = flexID(strconv.FormatInt(int64(f), 10))
		return nil
	}
	*id = flexID(s)
	return nil
}

type saasDomain struct {
	CustomerID     flexID  `json:"saasCustomerId"`
	CustomerName   string  `json:"saasCustomerName"`
	Domain         string  `json:"domain"`
	ProductType    string  `json:"productType"`
	SubscriptionID string  `json:"externalSubscriptionId"`
	RetentionType  string  `json:"retentionType"`
	SeatsUsed      flexNum `json:"seatsUsed"`
	OrgID          flexID  `json:"organizationId"`
	OrgName        string  `json:"organizationName"`
	BackupStats    struct {
		Active  flexNum `json:"activeServicesCount"`
		Recent  flexNum `json:"activeServicesWithRecentBackupCount"`
		Percent flexNum `json:"backupPercentage"`
	} `json:"backupStats"`
}

type saasSeat struct {
	CustomerID string          `json:"-"`
	MainID     string          `json:"mainId"`
	Name       string          `json:"name"`
	SeatType   string          `json:"seatType"`
	SeatState  string          `json:"seatState"`
	BillableJS json.RawMessage `json:"billable"`
	DateAdded  flexStr         `json:"dateAdded"`
	RemoteID   string          `json:"remoteId"`
}

type saasWindow struct {
	TimeWindow    flexStr `json:"timeWindow"`
	Status        flexStr `json:"status"`
	StartTime     flexNum `json:"startTime"`
	EndTime       flexNum `json:"endTime"`
	Total         flexNum `json:"totalServiceCount"`
	Active        flexNum `json:"activeServiceCount"`
	WithBackup    flexNum `json:"activeServiceWithBackupCount"`
	PerfectBackup flexNum `json:"activeServiceWithPerfectBackupCount"`
}

type saasAppReport struct {
	CustomerID   flexID  `json:"customerId"`
	CustomerName string  `json:"customerName"`
	UsedBytes    flexNum `json:"usedBytes"`
	Suites       []struct {
		SuiteType string `json:"suiteType"`
		AppTypes  []struct {
			AppType       string       `json:"appType"`
			BackupHistory []saasWindow `json:"backupHistory"`
		} `json:"appTypes"`
	} `json:"suites"`
	// parentID is the saasCustomerId the report was fetched under.
	parentID string
}

// billable parses the seat billable flag, which public sources type as a
// string (S4) and a boolean (S6). nil means unknown, never false.
func (s saasSeat) billable() *bool {
	raw := strings.ToLower(strings.Trim(strings.TrimSpace(string(s.BillableJS)), `"`))
	var v bool
	switch raw {
	case "true", "1", "yes", "y":
		v = true
	case "false", "0", "no", "n":
		v = false
	default:
		return nil
	}
	return &v
}

// seatDateLayouts covers the date shapes seen in public examples; dateAdded
// has no documented format, so unparseable values are reported, not dropped.
var seatDateLayouts = []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"}

func parseSeatDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range seatDateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
		if n > 1e12 { // milliseconds
			return time.UnixMilli(n).UTC(), true
		}
		return time.Unix(n, 0).UTC(), true
	}
	return time.Time{}, false
}

// openSaasStore resolves --db and opens the mirror read-only. missing=true
// means there is no local mirror yet (an ordinary empty state, not an error).
func openSaasStore(cmd *cobra.Command, dbPath string) (db *store.Store, missing bool, err error) {
	if dbPath == "" {
		dbPath = defaultDBPath("datto-saas-cli")
	}
	if _, statErr := os.Stat(dbPath); os.IsNotExist(statErr) {
		// The "hint:" prefix is what the MCP server forwards from stderr, so an
		// agent sees why the result is empty instead of a bare [].
		fmt.Fprintf(cmd.ErrOrStderr(), "hint: no local mirror at %s, so the results are empty, not an all-clear. Run: datto-saas-cli sync --db %s\n", dbPath, dbPath)
		return nil, true, nil
	}
	db, err = store.OpenReadOnlyContext(cmd.Context(), dbPath)
	if err != nil {
		return nil, false, fmt.Errorf("opening database: %w", err)
	}
	return db, false, nil
}

// mirrorIncomplete returns the resources whose last sync did not finish
// (sync_state.last_attempt_complete = 0) or that were never synced.
func mirrorIncomplete(ctx context.Context, q queryer, resources ...string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT resource_type, last_attempt_complete FROM sync_state`)
	if err != nil {
		return nil, fmt.Errorf("query sync_state: %w", err)
	}
	complete := map[string]bool{}
	for rows.Next() {
		var name string
		var done int
		if err := rows.Scan(&name, &done); err != nil {
			_ = rows.Close()
			return nil, err
		}
		complete[name] = done == 1
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	out := make([]string, 0)
	for _, r := range resources {
		if !complete[r] {
			out = append(out, r)
		}
	}
	return out, nil
}

// syncHints prints the generated stale/unsynced hints for each resource read.
// queryer is satisfied by *sql.DB and *sql.Tx.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// saasRead is one read transaction over the local store. Every local command
// reads sync completeness and its rows through it, so a concurrent sync can
// never mix generations (the store uses a rollback journal: the open read
// transaction holds a shared lock until Close, and a sync's commit waits).
type saasRead struct {
	tx *sql.Tx
	// partial lists resources whose last sync did not finish or never ran.
	partial []string
}

// snapshotHook is a test seam, called inside the read transaction after the
// completeness check and before any rows are read.
var snapshotHook func()

// beginSaasRead prints one stale/unsynced hint (the generated helpers print
// the same line per resource), opens the read transaction, and warns on
// stderr, independently of the freshness hint, when the mirror is incomplete.
// An unreadable sync_state fails closed: every resource counts as incomplete.
func beginSaasRead(cmd *cobra.Command, db *store.Store, flags *rootFlags, resources ...string) (*saasRead, error) {
	for _, r := range resources {
		if hintIfUnsynced(cmd, db, r) || hintIfStale(cmd, db, r, flags.maxAge) {
			break
		}
	}
	tx, err := db.DB().BeginTx(cmd.Context(), nil)
	if err != nil {
		return nil, fmt.Errorf("opening read transaction: %w", err)
	}
	partial, err := mirrorIncomplete(cmd.Context(), tx, resources...)
	if err != nil {
		partial = append([]string(nil), resources...)
	}
	if len(partial) > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: the last sync of %s did not finish or never ran; results may be incomplete. Run: datto-saas-cli sync\n", strings.Join(partial, ", "))
	}
	if snapshotHook != nil {
		snapshotHook()
	}
	return &saasRead{tx: tx, partial: partial}, nil
}

func (r *saasRead) Close() { _ = r.tx.Rollback() }

func containsStr(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// emptyMsg keeps an unsynced store from reading as an all-clear in human output.
func emptyMsg(domains []saasDomain, normal string) string {
	if len(domains) == 0 {
		return "No synced data yet, so nothing can be reported. Run: datto-saas-cli sync"
	}
	return normal
}

// queryData drains `SELECT <cols>, data FROM ...` into decoded rows before
// returning, so callers never hold an open *sql.Rows (SQLite single-conn rule).
func queryData(ctx context.Context, db queryer, q string, args ...any) ([][2]string, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	out := make([][2]string, 0)
	for rows.Next() {
		var key, data string
		if err := rows.Scan(&key, &data); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan row: %w", err)
		}
		out = append(out, [2]string{key, data})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	return out, rows.Close()
}

func loadDomains(ctx context.Context, db queryer) ([]saasDomain, error) {
	raw, err := queryData(ctx, db, `SELECT id, data FROM domains ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query domains: %w", err)
	}
	out := make([]saasDomain, 0, len(raw))
	for _, r := range raw {
		var d saasDomain
		if err := json.Unmarshal([]byte(r[1]), &d); err != nil {
			return nil, fmt.Errorf("decode domain %q: %w", r[0], err)
		}
		out = append(out, d)
	}
	return out, nil
}

// loadSeats returns seats for one customer, or every customer when id is "".
func loadSeats(ctx context.Context, db queryer, customerID string) ([]saasSeat, error) {
	q := `SELECT COALESCE(parent_id, ''), data FROM seats`
	var args []any
	if customerID != "" {
		q += ` WHERE parent_id = ?`
		args = append(args, customerID)
	}
	raw, err := queryData(ctx, db, q+` ORDER BY parent_id, id`, args...)
	if err != nil {
		return nil, fmt.Errorf("query seats: %w", err)
	}
	out := make([]saasSeat, 0, len(raw))
	for _, r := range raw {
		var s saasSeat
		if err := json.Unmarshal([]byte(r[1]), &s); err != nil {
			return nil, fmt.Errorf("decode seat: %w", err)
		}
		s.CustomerID = r[0]
		out = append(out, s)
	}
	return out, nil
}

func loadApplications(ctx context.Context, db queryer, customerID string) ([]saasAppReport, error) {
	q := `SELECT COALESCE(parent_id, ''), data FROM applications`
	var args []any
	if customerID != "" {
		q += ` WHERE parent_id = ?`
		args = append(args, customerID)
	}
	raw, err := queryData(ctx, db, q+` ORDER BY parent_id, id`, args...)
	if err != nil {
		return nil, fmt.Errorf("query applications: %w", err)
	}
	out := make([]saasAppReport, 0, len(raw))
	for _, r := range raw {
		var a saasAppReport
		if err := json.Unmarshal([]byte(r[1]), &a); err != nil {
			return nil, fmt.Errorf("decode application report: %w", err)
		}
		a.parentID = r[0]
		if a.parentID == "" {
			a.parentID = string(a.CustomerID)
		}
		out = append(out, a)
	}
	return out, nil
}

// saasCustomer groups a customer's domain rows; one customer can own several.
type saasCustomer struct {
	ID            string
	Name          string
	OrgName       string
	Domains       []saasDomain
	SeatsUsed     float64
	SeatsUsedSeen bool
	// seatsUsedDomains counts domains that reported a valid seatsUsed;
	// SeatsUsed is a complete customer total only when every domain did.
	seatsUsedDomains int
}

func (c saasCustomer) seatsUsedComplete() bool {
	return c.SeatsUsedSeen && c.seatsUsedDomains == len(c.Domains)
}

func (c saasCustomer) domainNames() []string {
	out := make([]string, 0, len(c.Domains))
	for _, d := range c.Domains {
		out = append(out, d.Domain)
	}
	return out
}

func (c saasCustomer) unique(field func(saasDomain) string) []string {
	seen := map[string]bool{}
	out := make([]string, 0)
	for _, d := range c.Domains {
		v := strings.TrimSpace(field(d))
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func (c saasCustomer) subscriptions() []string {
	return c.unique(func(d saasDomain) string { return d.SubscriptionID })
}

func (c saasCustomer) hasDomainWithoutSubscription() bool {
	for _, d := range c.Domains {
		if strings.TrimSpace(d.SubscriptionID) == "" {
			return true
		}
	}
	return false
}

func (c saasCustomer) productTypes() []string {
	return c.unique(func(d saasDomain) string { return d.ProductType })
}

// groupCustomers folds domain rows into customers, ordered by name then id.
func groupCustomers(domains []saasDomain) []saasCustomer {
	idx := map[string]int{}
	out := make([]saasCustomer, 0)
	for _, d := range domains {
		id := string(d.CustomerID)
		if id == "" {
			continue
		}
		i, ok := idx[id]
		if !ok {
			idx[id] = len(out)
			out = append(out, saasCustomer{ID: id, Name: d.CustomerName, OrgName: d.OrgName})
			i = len(out) - 1
		}
		c := &out[i]
		if c.Name == "" {
			c.Name = d.CustomerName
		}
		if c.OrgName == "" {
			c.OrgName = d.OrgName
		}
		c.Domains = append(c.Domains, d)
		if validCount(d.SeatsUsed) {
			c.SeatsUsed += d.SeatsUsed.v
			c.SeatsUsedSeen = true
			c.seatsUsedDomains++
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if !strings.EqualFold(out[a].Name, out[b].Name) {
			return strings.ToLower(out[a].Name) < strings.ToLower(out[b].Name)
		}
		return out[a].ID < out[b].ID
	})
	return out
}

func customerIndex(customers []saasCustomer) map[string]saasCustomer {
	m := make(map[string]saasCustomer, len(customers))
	for _, c := range customers {
		m[c.ID] = c
	}
	return m
}

// resolveCustomer matches an exact saasCustomerId first, then a
// case-insensitive customer name or domain. Ambiguity is an error that lists
// the candidates rather than a guess.
func resolveCustomer(customers []saasCustomer, arg string) (saasCustomer, error) {
	arg = strings.TrimSpace(arg)
	for _, c := range customers {
		if c.ID == arg {
			return c, nil
		}
	}
	var matches []saasCustomer
	for _, c := range customers {
		hit := strings.EqualFold(c.Name, arg)
		for _, d := range c.Domains {
			if strings.EqualFold(d.Domain, arg) {
				hit = true
			}
		}
		if hit {
			matches = append(matches, c)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		// Shared by the live exit-inventory path (domains fetched from the API)
		// and the local commands, so the message must not claim the list was
		// synced.
		return saasCustomer{}, notFoundErr(fmt.Errorf("no SaaS customer matches %q (use a saasCustomerId, customer name, or domain from 'datto-saas-cli domains')", arg))
	}
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, fmt.Sprintf("%s (%s)", m.ID, m.Name))
	}
	return saasCustomer{}, usageErr(fmt.Errorf("%q matches several customers: %s; pass the saasCustomerId", arg, strings.Join(names, ", ")))
}

// customerFilter resolves an optional --customer flag to a saasCustomerId.
func customerFilter(customers []saasCustomer, arg string) (string, error) {
	if strings.TrimSpace(arg) == "" {
		return "", nil
	}
	if len(customers) == 0 {
		// Nothing synced yet: filtering by the raw value matches no rows, so
		// callers return an ordinary empty result (the sync hint is on stderr).
		return strings.TrimSpace(arg), nil
	}
	c, err := resolveCustomer(customers, arg)
	if err != nil {
		return "", err
	}
	return c.ID, nil
}

// splitCSVSet parses a comma-separated flag into a set; empty means "any".
func splitCSVSet(v string) map[string]bool {
	out := map[string]bool{}
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out[p] = true
		}
	}
	return out
}

// Case-sensitive seat vocabularies from the Datto help pages (S1).
var (
	saasSeatTypes  = []string{"User", "SharedMailbox", "SharedDrive", "Site", "TeamSite", "Team"}
	saasSeatStates = []string{"Active", "Paused", "Archived", "Unprotected"}
)

// validateEnumSet rejects values outside allowed, naming the correctly cased
// spelling when only the case is wrong.
func validateEnumSet(flag string, set map[string]bool, allowed []string) error {
	for v := range set {
		ok := false
		hint := ""
		for _, a := range allowed {
			if v == a {
				ok = true
			} else if strings.EqualFold(v, a) {
				hint = a
			}
		}
		if ok {
			continue
		}
		if hint != "" {
			return usageErr(fmt.Errorf("--%s %q is case-sensitive; use %q", flag, v, hint))
		}
		return usageErr(fmt.Errorf("--%s %q is not one of %s", flag, v, strings.Join(allowed, ", ")))
	}
	return nil
}

func roundPct(v float64) float64 {
	return float64(int64(v*10+0.5)) / 10
}

// pctOf returns part/whole as a percentage, or nil when it is not meaningful
// (whole is 0 or missing, or part exceeds whole, per spektrum-labs' rule).
func pctOf(part, whole flexNum) *float64 {
	if !part.ok || !whole.ok || whole.v <= 0 || part.v > whole.v || part.v < 0 {
		return nil
	}
	p := roundPct(part.v / whole.v * 100)
	return &p
}

// printSaasRows routes machine formats through printJSONFiltered (so --select,
// --csv, --compact apply) and renders the generated human table otherwise.
func printSaasRows(cmd *cobra.Command, flags *rootFlags, rows any, empty string) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
	}
	b, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	var items []map[string]any
	if err := json.Unmarshal(b, &items); err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), empty)
		return nil
	}
	return printAutoTable(cmd.OutOrStdout(), items)
}
