// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Shared local-store helpers for the hand-written readers triage,
// cross-tenant, investigate and coverage. Everything here reads the
// generic `resources` table that sync fills (resource_type + data JSON);
// nothing writes. Field names follow the official specs:
//   - events:      Reports EventOmit / IEvent (_id, time, pullTime,
//     user{id,name}, customer{id,name}, product{id,name}, ip,
//     location{country,region,city,ipInfo{asn{asn,name},threat}},
//     alertStatus, jointType, jointDesc, eventId,
//     psaTicket[]{ticketNumber,psaTicketId})
//   - customers:   Reports ISCustomer (id, name, status, ipRanges[]{from,to},
//     asnList[]{name,...}, countries[]{code,label})

package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"net/netip"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"saas-alerts-pp-cli/internal/cliutil"
	"saas-alerts-pp-cli/internal/store"
)

// saasLocalNow is the clock the local readers use for window math. Tests pin it.
var saasLocalNow = time.Now

const saasLocalCLIName = "saas-alerts-cli"

// saasSeverityRank orders the three Reports alertStatus values. Anything
// else ranks 0 so it sorts after low.
func saasSeverityRank(s string) int {
	switch saasNormalizeSeverity(s) {
	case "critical":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	}
	return 0
}

func saasNormalizeSeverity(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// saasLocalEvent is the decoded subset of one synced Reports event.
type saasLocalEvent struct {
	Key      string
	DocID    string // IEvent _id: the Elasticsearch document id
	EventID  string
	Time     time.Time
	PullLag  *float64 // pullTime - time in seconds; nil when pullTime is absent or unparseable
	UserID   string
	UserName string
	// AccountIDs are the event's account ids: user.id and the top-level UserId.
	AccountIDs []string
	// AccountID is the stable account id under the storage rule
	// (saasEventAccountID); empty when the event names no stable account.
	AccountID string
	// AccountLabel is a display label only: the first account email, else
	// user.name, else AccountID.
	AccountLabel string
	// AccountEmails are the email addresses this event is attributable to:
	// top-level userEmail, the deprecated email field, and user.name when it
	// is email-shaped (the published example puts a display name there).
	AccountEmails []string
	CustomerID    string
	CustomerName  string
	ProductID     string
	ProductName   string
	IP            string
	Country       string
	Region        string
	City          string
	ASN           string
	ASNName       string
	Threat        any // location.ipInfo.threat; nil when absent
	Severity      string
	JointType     string
	JointDesc     string
	Tickets       []string
	HasTicket     bool
}

// saasEventIdentity identifies one event across duplicate stored copies. It
// mirrors the storage identity in internal/store/saas_events_id.go so the
// reader never merges events that storage keeps apart:
//  1. _id, the search-index document id declared on the Reports IEvent schema;
//  2. eventId scoped by ALL of customer id, product (id, else name) and the
//     stable account id (saasEventAccountID). The spec documents eventId as
//     the external product's raw id (example "logins_2021-03-24T07:36:46Z", a
//     type plus a timestamp), so it is not unique on its own;
//  3. a hash of the whole event document, so an event with no usable id only
//     collapses with an exact copy of itself.
//
// doc must be decoded with number preservation (store.DecodeJSONObject) so the
// content hash sees the stored digits.
func saasEventIdentity(doc map[string]any) string {
	str := func(v any) string { return strings.TrimSpace(store.ResourceIDString(v)) }
	nested := func(parent, key string) string {
		m, _ := doc[parent].(map[string]any)
		if m == nil {
			return ""
		}
		return str(m[key])
	}
	if id := str(doc["_id"]); id != "" {
		return "i|" + id
	}
	if eid := str(doc["eventId"]); eid != "" {
		customer := nested("customer", "id")
		product := nested("product", "id")
		if product == "" {
			product = nested("product", "name")
		}
		account := saasEventAccountID(doc)
		if customer != "" && product != "" && account != "" {
			return "e|" + strings.Join([]string{eid, customer, product, account}, "\x00")
		}
	}
	raw, err := json.Marshal(doc) // map keys marshal sorted, so this is stable
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return "h|" + hex.EncodeToString(sum[:])
}

// saasEventAccountID is the event's stable account id, as storage defines it:
// user.id, else the top-level UserId, else userEmail, else email, else an
// email-shaped user.name (emails lowercased). A plain display name (the
// published example has "Charles Buck") is never an account id.
func saasEventAccountID(doc map[string]any) string {
	str := func(v any) string { return strings.TrimSpace(store.ResourceIDString(v)) }
	if user, ok := doc["user"].(map[string]any); ok {
		if id := str(user["id"]); id != "" {
			return id
		}
	}
	if id := str(doc["UserId"]); id != "" {
		return id
	}
	for _, email := range []string{str(doc["userEmail"]), str(doc["email"])} {
		if email != "" {
			return strings.ToLower(email)
		}
	}
	if user, ok := doc["user"].(map[string]any); ok {
		if name := str(user["name"]); saasEmailShaped(name) {
			return strings.ToLower(name)
		}
	}
	return ""
}

// saasEmailShaped reports whether s looks like an email address: one "@" with
// text on both sides and no whitespace (same test as storage identity).
func saasEmailShaped(s string) bool {
	s = strings.TrimSpace(s)
	at := strings.Index(s, "@")
	return at > 0 && at == strings.LastIndex(s, "@") && at < len(s)-1 && !strings.ContainsAny(s, " \t\r\n")
}

// saasDecodeEvent maps one events document. ok is false when the document
// has no parseable `time`, because every reader windows or orders by it.
func saasDecodeEvent(doc map[string]any) (saasLocalEvent, bool) {
	t, ok := saasParseTime(doc["time"])
	if !ok {
		return saasLocalEvent{}, false
	}
	ev := saasLocalEvent{
		DocID:        strings.TrimSpace(saasStr(doc, "_id")),
		EventID:      strings.TrimSpace(saasStr(doc, "eventId")),
		Time:         t,
		UserID:       saasStr(doc, "user", "id"),
		UserName:     strings.TrimSpace(saasStr(doc, "user", "name")),
		CustomerID:   saasStr(doc, "customer", "id"),
		CustomerName: saasStr(doc, "customer", "name"),
		ProductID:    saasStr(doc, "product", "id"),
		ProductName:  saasStr(doc, "product", "name"),
		IP:           strings.TrimSpace(saasStr(doc, "ip")),
		Country:      strings.TrimSpace(saasStr(doc, "location", "country")),
		Region:       saasStr(doc, "location", "region"),
		City:         saasStr(doc, "location", "city"),
		ASN:          strings.TrimSpace(saasStr(doc, "location", "ipInfo", "asn", "asn")),
		ASNName:      strings.TrimSpace(saasStr(doc, "location", "ipInfo", "asn", "name")),
		Threat:       saasGet(doc, "location", "ipInfo", "threat"),
		Severity:     saasNormalizeSeverity(saasStr(doc, "alertStatus")),
		JointType:    saasStr(doc, "jointType"),
		JointDesc:    saasStr(doc, "jointDesc"),
	}
	for _, id := range []string{ev.UserID, saasStr(doc, "UserId")} {
		if id = strings.TrimSpace(id); id != "" {
			ev.AccountIDs = saasAppendUnique(ev.AccountIDs, id)
		}
	}
	for _, candidate := range []string{saasStr(doc, "userEmail"), saasStr(doc, "email"), ev.UserName} {
		candidate = strings.TrimSpace(candidate)
		if saasEmailShaped(candidate) {
			ev.AccountEmails = saasAppendUnique(ev.AccountEmails, candidate)
		}
	}
	ev.AccountID = saasEventAccountID(doc)
	switch {
	case len(ev.AccountEmails) > 0:
		ev.AccountLabel = ev.AccountEmails[0]
	case ev.UserName != "":
		ev.AccountLabel = ev.UserName
	default:
		ev.AccountLabel = ev.AccountID
	}
	if ev.IP == "" {
		// EventOmit carries the source address at the top level; Location has
		// no ip field in the spec. Accept location.ip defensively anyway.
		ev.IP = strings.TrimSpace(saasStr(doc, "location", "ip"))
	}
	if pt, ok := saasParseTime(doc["pullTime"]); ok {
		lag := pt.Sub(t).Seconds()
		ev.PullLag = &lag
	}
	// psaTicket is an array in EventOmit; tolerate a single object too.
	tickets := saasArr(doc, "psaTicket")
	if obj, isObj := doc["psaTicket"].(map[string]any); isObj {
		tickets = []any{obj}
	}
	for _, raw := range tickets {
		item, isObj := raw.(map[string]any)
		if !isObj {
			continue
		}
		ev.HasTicket = true
		num := strings.TrimSpace(saasStr(item, "ticketNumber"))
		if num == "" {
			// Some PSAs return only the internal id; better than nothing.
			num = strings.TrimSpace(saasStr(item, "psaTicketId"))
		}
		if num != "" {
			ev.Tickets = saasAppendUnique(ev.Tickets, num)
		}
	}
	ev.Key = saasEventIdentity(doc)
	return ev, true
}

// saasEventQuery bounds a local event load. A zero Start/End is unbounded.
type saasEventQuery struct {
	Start     time.Time
	End       time.Time
	UserEmail string // case-insensitive match on any of the event's account emails; empty = every user
	// AccountScopes are the synced user identities for UserEmail. An event
	// also matches when its user.id or UserId equals a scope's account id
	// inside that scope's customer and product (positive product agreement).
	AccountScopes []saasAccountScope
	// AmbiguousIDMatches, when non-nil, receives the number of distinct
	// in-window events that carry a scope's account id in its customer but
	// whose product cannot be compared; they are not attributed.
	AmbiguousIDMatches *int
}

// saasAccountScope is one synced user's identity. Account ids are only
// unique inside a tenant (and product), so the id never matches on its own.
type saasAccountScope struct {
	CustomerID  string
	ProductID   string
	ProductName string
	AccountID   string
}

// saasProductAgreement is the outcome of comparing a scope's product with an
// event's product.
type saasProductAgreement int

const (
	saasProductUnknown  saasProductAgreement = iota // nothing comparable on both sides
	saasProductAgree                                // positive agreement
	saasProductDisagree                             // comparable and different
)

// productAgreement compares ids when both sides carry an id, else names
// (case-insensitively) when both carry a name. Anything else, such as a
// missing event product or an id on one side and only a name on the other,
// is unknown: ID-only attribution needs positive agreement.
func (sc saasAccountScope) productAgreement(ev saasLocalEvent) saasProductAgreement {
	switch {
	case sc.ProductID != "" && ev.ProductID != "":
		if sc.ProductID == ev.ProductID {
			return saasProductAgree
		}
		return saasProductDisagree
	case sc.ProductName != "" && ev.ProductName != "":
		if strings.EqualFold(sc.ProductName, ev.ProductName) {
			return saasProductAgree
		}
		return saasProductDisagree
	}
	return saasProductUnknown
}

// saasJSONPath builds a SQLite JSON path from its segments, so no source
// literal reads as a dotted hostname.
func saasJSONPath(parts ...string) string { return "$." + strings.Join(parts, ".") }

// saasEventAccountPaths are the event fields an account email can live in.
var saasEventAccountPaths = []string{saasJSONPath("userEmail"), saasJSONPath("email"), saasJSONPath("user", "name")}

// JSON paths for the scoped account-id match, built from parts.
var (
	saasEventUserIDPath     = saasJSONPath("user", "id")
	saasEventTopUserIDPath  = saasJSONPath("UserId")
	saasEventCustomerIDPath = saasJSONPath("customer", "id")
	saasEventProductIDPath  = saasJSONPath("product", "id")
)

// saasEventMatchesAccount reports whether the event is attributable to the
// account: one of its email fields equals email, or (when the event names no
// email at all) its user.id or UserId equals a scope's account id within that
// scope's customer with positive product agreement. An event that names a
// different account by email is never attributed by id. ambiguous is true
// when the id and customer match but no scope can establish the product.
func saasEventMatchesAccount(ev saasLocalEvent, email string, scopes []saasAccountScope) (match, ambiguous bool) {
	email = strings.TrimSpace(email)
	for _, candidate := range ev.AccountEmails {
		if strings.EqualFold(candidate, email) {
			return true, false
		}
	}
	if len(ev.AccountEmails) > 0 {
		return false, false
	}
	for _, sc := range scopes {
		if sc.CustomerID == "" || sc.AccountID == "" || ev.CustomerID != sc.CustomerID {
			continue
		}
		idMatch := false
		for _, id := range ev.AccountIDs {
			if id == sc.AccountID {
				idMatch = true
				break
			}
		}
		if !idMatch {
			continue
		}
		switch sc.productAgreement(ev) {
		case saasProductAgree:
			return true, false
		case saasProductUnknown:
			ambiguous = true
		}
	}
	return false, ambiguous
}

// saasLoadLocalEvents returns deduped events inside [Start, End], oldest first.
// A missing resources table (never-synced store) yields no events and no error.
func saasLoadLocalEvents(ctx context.Context, db *store.Store, q saasEventQuery) ([]saasLocalEvent, error) {
	query := `SELECT data FROM resources WHERE resource_type = 'events'`
	var args []any
	if !q.Start.IsZero() {
		// Coarse SQL prefilter for ISO-8601 text times only (lexicographic
		// order is chronological there). A day of slack absorbs any UTC
		// offset; numeric or non-ISO times always pass and are filtered
		// exactly in Go below.
		query += ` AND (json_extract(data, '$.time') NOT GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]*' OR json_extract(data, '$.time') >= ?)`
		args = append(args, q.Start.UTC().Add(-26*time.Hour).Format("2006-01-02"))
	}
	if q.UserEmail != "" {
		want := strings.ToLower(strings.TrimSpace(q.UserEmail))
		var clauses []string
		for _, path := range saasEventAccountPaths {
			clauses = append(clauses, `lower(trim(json_extract(data, ?))) = ?`)
			args = append(args, path, want)
		}
		// Scoped id match: the id only counts inside the scope's customer.
		// This SQL is a superset on purpose: it still admits events with no
		// product id so the Go filter below can count them as ambiguous; the
		// Go filter requires positive product agreement and applies the
		// "no explicit other email" rule.
		for _, sc := range q.AccountScopes {
			if sc.CustomerID == "" || sc.AccountID == "" {
				continue
			}
			clause := `((trim(CAST(json_extract(data, ?) AS TEXT)) = ? OR trim(CAST(json_extract(data, ?) AS TEXT)) = ?) AND trim(CAST(json_extract(data, ?) AS TEXT)) = ?`
			args = append(args, saasEventUserIDPath, sc.AccountID, saasEventTopUserIDPath, sc.AccountID, saasEventCustomerIDPath, sc.CustomerID)
			if sc.ProductID != "" {
				clause += ` AND coalesce(trim(CAST(json_extract(data, ?) AS TEXT)), '') IN ('', ?)`
				args = append(args, saasEventProductIDPath, sc.ProductID)
			}
			clauses = append(clauses, clause+`)`)
		}
		query += ` AND (` + strings.Join(clauses, " OR ") + `)`
	}
	raws, err := saasQueryRaw(ctx, db, query, args...)
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]int, len(raws))
	ambiguousSeen := map[string]bool{}
	events := make([]saasLocalEvent, 0, len(raws))
	for _, raw := range raws {
		// Number-preserving decode, the same one storage identity uses.
		doc, err := store.DecodeJSONObject(json.RawMessage(raw))
		if err != nil || doc == nil {
			continue
		}
		ev, ok := saasDecodeEvent(doc)
		if !ok {
			continue
		}
		if !q.Start.IsZero() && ev.Time.Before(q.Start) {
			continue
		}
		if !q.End.IsZero() && ev.Time.After(q.End) {
			continue
		}
		if q.UserEmail != "" {
			match, ambiguous := saasEventMatchesAccount(ev, q.UserEmail, q.AccountScopes)
			if !match {
				if ambiguous && q.AmbiguousIDMatches != nil && !ambiguousSeen[ev.Key] {
					ambiguousSeen[ev.Key] = true
					*q.AmbiguousIDMatches++
				}
				continue
			}
		}
		if idx, dup := byKey[ev.Key]; dup && ev.Key != "" {
			// Same event stored twice: keep the first copy but union ticket
			// state, since a later sync may have attached the PSA ticket.
			kept := &events[idx]
			kept.HasTicket = kept.HasTicket || ev.HasTicket
			for _, tk := range ev.Tickets {
				kept.Tickets = saasAppendUnique(kept.Tickets, tk)
			}
			if kept.PullLag == nil {
				kept.PullLag = ev.PullLag
			}
			continue
		}
		byKey[ev.Key] = len(events)
		events = append(events, ev)
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].Time.Before(events[j].Time) })
	return events, nil
}

// saasLoadResourceDocs decodes every synced document of the given resource
// types. Rows that are not JSON objects are skipped.
func saasLoadResourceDocs(ctx context.Context, db *store.Store, resourceTypes ...string) ([]map[string]any, error) {
	if len(resourceTypes) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(resourceTypes)), ",")
	args := make([]any, len(resourceTypes))
	for i, rt := range resourceTypes {
		args[i] = rt
	}
	return saasQueryDocs(ctx, db, `SELECT data FROM resources WHERE resource_type IN (`+placeholders+`) ORDER BY resource_type, id`, args...)
}

// saasQueryDocs drains a single-column `data` query fully before decoding.
func saasQueryDocs(ctx context.Context, db *store.Store, query string, args ...any) ([]map[string]any, error) {
	raws, err := saasQueryRaw(ctx, db, query, args...)
	if err != nil {
		return nil, err
	}
	docs := make([]map[string]any, 0, len(raws))
	for _, raw := range raws {
		var doc map[string]any
		if err := json.Unmarshal([]byte(raw), &doc); err != nil || doc == nil {
			continue
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

// saasQueryRaw drains a single-column `data` query fully and returns the raw
// JSON text. A missing table (never-migrated store) yields nothing.
func saasQueryRaw(ctx context.Context, db *store.Store, query string, args ...any) ([]string, error) {
	rows, err := db.DB().QueryContext(ctx, query, args...)
	if err != nil {
		if syncHintMissingTable(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("querying local store: %w", err)
	}
	var raws []string
	for rows.Next() {
		var data *string
		if err := rows.Scan(&data); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scanning local store: %w", err)
		}
		if data != nil {
			raws = append(raws, *data)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("reading local store: %w", err)
	}
	_ = rows.Close()
	return raws, nil
}

// ---- customers and whitelists ----

type saasIPRange struct {
	prefix netip.Prefix
	from   netip.Addr
	to     netip.Addr
}

func (r saasIPRange) contains(a netip.Addr) bool {
	if r.prefix.IsValid() {
		return r.prefix.Contains(a)
	}
	if !r.from.IsValid() || r.from.Is4() != a.Is4() {
		return false
	}
	return a.Compare(r.from) >= 0 && a.Compare(r.to) <= 0
}

// saasLocalCustomer is a synced Reports customer plus its whitelists.
type saasLocalCustomer struct {
	ID        string
	Name      string
	Status    string
	IPRanges  []saasIPRange
	ASNs      map[string]bool // lowercased asnList[].name
	Countries map[string]bool // lowercased countries[] code and label
	// HasWhitelistFields is true when the document carries any of ipRanges,
	// asnList or countries. Customers synced without them cannot be excluded.
	HasWhitelistFields bool
}

func saasDecodeCustomer(doc map[string]any) saasLocalCustomer {
	c := saasLocalCustomer{
		ID:        saasStr(doc, "id"),
		Name:      saasStr(doc, "name"),
		Status:    saasStr(doc, "status"),
		ASNs:      map[string]bool{},
		Countries: map[string]bool{},
	}
	for _, key := range []string{"ipRanges", "asnList", "countries"} {
		if _, ok := doc[key]; ok {
			c.HasWhitelistFields = true
		}
	}
	for _, raw := range saasArr(doc, "ipRanges") {
		// IpRange is {from, to?}; a missing `to` means a single address. A
		// CIDR in `from` is not in the spec but is accepted defensively.
		from := strings.TrimSpace(saasStr(raw, "from"))
		to := strings.TrimSpace(saasStr(raw, "to"))
		if s, isStr := raw.(string); isStr {
			from = strings.TrimSpace(s)
		}
		if p, err := netip.ParsePrefix(from); err == nil {
			c.IPRanges = append(c.IPRanges, saasIPRange{prefix: p.Masked()})
			continue
		}
		fa, err := netip.ParseAddr(from)
		if err != nil {
			continue
		}
		ta := fa
		if to != "" {
			if parsed, err := netip.ParseAddr(to); err == nil && parsed.Is4() == fa.Is4() {
				ta = parsed
			}
		}
		if ta.Compare(fa) < 0 {
			fa, ta = ta, fa
		}
		c.IPRanges = append(c.IPRanges, saasIPRange{from: fa.Unmap(), to: ta.Unmap()})
	}
	for _, raw := range saasArr(doc, "asnList") {
		// AsnEntry is {name, country, state, city}: it has no AS number, so
		// whitelisted ASNs are matched by name (INFERRED from the schema).
		name := strings.ToLower(strings.TrimSpace(saasStr(raw, "name")))
		if s, isStr := raw.(string); isStr {
			name = strings.ToLower(strings.TrimSpace(s))
		}
		if name != "" {
			c.ASNs[name] = true
		}
	}
	for _, raw := range saasArr(doc, "countries") {
		// ISCustomer.countries is Country{code,label,phone}; the SetWhitelists
		// body sends plain strings. Accept both. Treating ISCustomer.countries
		// as the country whitelist is INFERRED from the shared field name.
		if s, isStr := raw.(string); isStr {
			if v := strings.ToLower(strings.TrimSpace(s)); v != "" {
				c.Countries[v] = true
			}
			continue
		}
		for _, k := range []string{"code", "label"} {
			if v := strings.ToLower(strings.TrimSpace(saasStr(raw, k))); v != "" {
				c.Countries[v] = true
			}
		}
	}
	return c
}

func (c saasLocalCustomer) whitelistsIP(ip string) bool {
	a, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return false
	}
	a = a.Unmap()
	for _, r := range c.IPRanges {
		if r.contains(a) {
			return true
		}
	}
	return false
}

func (c saasLocalCustomer) whitelistsASN(asn, name string) bool {
	for _, v := range []string{asn, name} {
		if v = strings.ToLower(strings.TrimSpace(v)); v != "" && c.ASNs[v] {
			return true
		}
	}
	return false
}

func (c saasLocalCustomer) whitelistsCountry(country string) bool {
	v := strings.ToLower(strings.TrimSpace(country))
	return v != "" && c.Countries[v]
}

// saasLoadLocalCustomers returns synced customers keyed by id.
func saasLoadLocalCustomers(ctx context.Context, db *store.Store) (map[string]saasLocalCustomer, []saasLocalCustomer, error) {
	docs, err := saasLoadResourceDocs(ctx, db, "customers")
	if err != nil {
		return nil, nil, err
	}
	byID := make(map[string]saasLocalCustomer, len(docs))
	list := make([]saasLocalCustomer, 0, len(docs))
	for _, doc := range docs {
		c := saasDecodeCustomer(doc)
		if c.ID == "" {
			continue
		}
		if _, dup := byID[c.ID]; dup {
			continue
		}
		byID[c.ID] = c
		list = append(list, c)
	}
	return byID, list, nil
}

// ---- command plumbing ----

// saasResolveDBPath applies the shared default.
func saasResolveDBPath(dbPath string) string {
	if strings.TrimSpace(dbPath) == "" {
		return defaultDBPath(saasLocalCLIName)
	}
	return dbPath
}

// saasLocalMirrorExists reports whether the store file is present.
func saasLocalMirrorExists(dbPath string) bool {
	_, statErr := os.Stat(dbPath)
	return statErr == nil || !errors.Is(statErr, fs.ErrNotExist)
}

// saasLocalMirrorMissing handles the missing-mirror guard. When the store
// file does not exist it prints a one-line sync hint to stderr (prefixed
// "hint:" so the MCP shell-out keeps it) and writes the command's empty
// result (machine output only). handled=true means the caller must return
// err without doing any work.
func saasLocalMirrorMissing(cmd *cobra.Command, flags *rootFlags, dbPath, resources string, empty any, keep []string) (handled bool, err error) {
	if saasLocalMirrorExists(dbPath) {
		return false, nil
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "hint: no local mirror at %s; run: %s sync --resources %s --since 7d --db %s\n", dbPath, saasLocalCLIName, resources, dbPath)
	if wantsHumanTable(cmd.OutOrStdout(), flags) {
		return true, nil
	}
	saasSelectKeepsSyncState(flags, "synced", "note", "unsynced_resources") // saas-alerts hand-fix: --select keeps the unsynced signal
	return true, printJSONFilteredKeep(cmd.OutOrStdout(), empty, flags, keep...)
}

func saasOpenLocalStore(cmd *cobra.Command, dbPath string) (*store.Store, error) {
	db, err := store.OpenReadOnlyContext(cmd.Context(), dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local database %s: %w\nrun: %s sync --db %s", dbPath, err, saasLocalCLIName, dbPath)
	}
	return db, nil
}

// saasUnsyncedResources returns the resources that sync_state has never
// recorded (the same signal hintIfUnsynced uses). A nil db means no store.
func saasUnsyncedResources(db *store.Store, resources ...string) []string {
	var missing []string
	for _, r := range resources {
		if db == nil {
			missing = append(missing, r)
			continue
		}
		if state, err := readSyncHintState(db, r); err != nil || !state.hasState {
			missing = append(missing, r)
		}
	}
	return missing
}

// saasIncompleteResources lists resources that were synced before but whose
// latest sync attempt did not finish (sync_state.last_attempt_complete = 0): a
// page-capped auto-refresh, --max-pages, or an interrupted or rejected sync
// left newer rows unfetched. Reporting such a store as synced would present a
// partial window as complete, so callers fold these into synced:false.
func saasIncompleteResources(db *store.Store, resources ...string) []string {
	var incomplete []string
	if db == nil {
		return incomplete
	}
	for _, r := range resources {
		var complete int
		err := db.DB().QueryRow(`SELECT last_attempt_complete FROM sync_state WHERE resource_type = ?`, r).Scan(&complete)
		if err == nil && complete == 0 {
			incomplete = append(incomplete, r)
		}
	}
	return incomplete
}

// saasSyncGaps returns the never-synced and the incompletely synced resources,
// the never-synced ones first, without duplicates. A resource whose sync stored
// rows but has never finished (a first sync that stopped at a page cap) counts
// as incomplete, not never-synced, so its note does not claim the store is empty.
// It also returns unsynced with those resources removed, for the note.
func saasSyncGaps(db *store.Store, unsynced []string, resources ...string) (gaps, incomplete []string) {
	gaps = append(make([]string, 0, len(unsynced)), unsynced...)
	// Only events is windowed: an unfinished events refresh leaves a hole in
	// the time window. List resources (rule triggers, rules, organizations,
	// devices, users) keep their last complete snapshot when a page-capped
	// head-of-list refresh does not finish, so that is staleness (the stderr
	// hint) rather than a hole; flagging it made investigate synced:false 15
	// minutes after every full rule-triggers sync. A list resource that stored
	// rows but never finished is still caught by saasStartedResources.
	windowed := make([]string, 0, 1)
	if slices.Contains(resources, "events") {
		windowed = append(windowed, "events")
	}
	for _, r := range saasIncompleteResources(db, windowed...) {
		if !slices.Contains(unsynced, r) {
			incomplete = append(incomplete, r)
			gaps = append(gaps, r)
		}
	}
	return gaps, incomplete
}

// saasStartedResources is unsynced minus the resources whose sync stored rows
// without finishing; those move to incomplete for the note.
func saasStartedResources(db *store.Store, unsynced, incomplete []string) (stillUnsynced, moreIncomplete []string) {
	moreIncomplete = append(make([]string, 0, len(incomplete)), incomplete...)
	for _, r := range unsynced {
		var count int
		if db != nil && db.DB().QueryRow(`SELECT total_count FROM sync_state WHERE resource_type = ? AND last_attempt_complete = 0`, r).Scan(&count) == nil && count > 0 {
			moreIncomplete = append(moreIncomplete, r)
			continue
		}
		stillUnsynced = append(stillUnsynced, r)
	}
	return stillUnsynced, moreIncomplete
}

// saasSyncGapNote explains synced:false: never-synced resources first, then
// resources whose last sync did not finish. Empty when there is no gap.
func saasSyncGapNote(db *store.Store, unsynced, incomplete []string, syncResources string) string {
	unsynced, incomplete = saasStartedResources(db, unsynced, incomplete)
	if len(unsynced) > 0 {
		return saasUnsyncedNote(unsynced, syncResources)
	}
	if len(incomplete) == 0 {
		return ""
	}
	list := strings.Join(incomplete, ",")
	return fmt.Sprintf("the last %s sync did not finish, so newer rows may be missing from these results; run: %s sync --resources %s", strings.Join(incomplete, ", "), saasLocalCLIName, list)
}

// saasEventsWindowGap returns the events coverage floor and true when the
// completed events syncs cover less than a window starting at start: a store
// synced with --since 24h cannot answer a 7d question, and incremental syncs
// only move forward, so the gap never closes by itself.
func saasEventsWindowGap(db *store.Store, start time.Time) (time.Time, bool) {
	if db == nil {
		return time.Time{}, false
	}
	floor, ok, err := db.EventsCoverage()
	if err != nil || !ok || floor.IsZero() {
		return time.Time{}, false
	}
	return floor, start.Before(floor)
}

// saasApplyWindowGap folds an events window gap into synced/note. The note is
// replaced only when nothing else already explains synced:false.
func saasApplyWindowGap(db *store.Store, now time.Time, window time.Duration, synced *bool, note *string, gaps *[]string) {
	floor, gap := saasEventsWindowGap(db, now.Add(-window))
	if !gap {
		return
	}
	if *synced {
		*note = fmt.Sprintf("the local store holds events only from %s, later than the start of this window; run: %s sync --resources events --since %s", saasFormatTime(floor), saasLocalCLIName, saasSyncSinceArg(window))
	}
	*synced = false
	if gaps != nil && !slices.Contains(*gaps, "events") {
		*gaps = append(*gaps, "events")
	}
}

// saasSelectKeepsSyncState makes --select keep the completeness fields, so a
// narrowed result can never drop the signal that it is partial or unsynced.
func saasSelectKeepsSyncState(flags *rootFlags, fields ...string) {
	if flags == nil || strings.TrimSpace(flags.selectFields) == "" {
		return
	}
	have := map[string]bool{}
	for _, f := range strings.Split(flags.selectFields, ",") {
		have[strings.TrimSpace(f)] = true
	}
	for _, f := range fields {
		if !have[f] {
			flags.selectFields += "," + f
		}
	}
}

// saasSyncSinceArg renders a window as a value `sync --since` accepts
// (^\d+[dhwm]$), rounding up, so a compound --since such as 1h30m still yields
// a runnable remedy.
func saasSyncSinceArg(d time.Duration) string {
	switch {
	case d > 0 && d%(24*time.Hour) == 0:
		return fmt.Sprintf("%dd", d/(24*time.Hour))
	case d > 0 && d%time.Hour == 0:
		return fmt.Sprintf("%dh", d/time.Hour)
	default:
		return fmt.Sprintf("%dm", (d+time.Minute-1)/time.Minute)
	}
}

// saasUnsyncedNote explains an empty result from a never-synced store so it
// does not read as all-clear. Empty when nothing is missing.
func saasUnsyncedNote(missing []string, syncResources string) string {
	if len(missing) == 0 {
		return ""
	}
	list := missing[0]
	if n := len(missing); n > 1 {
		list = strings.Join(missing[:n-1], ", ") + " or " + missing[n-1]
	}
	return fmt.Sprintf("local store has no synced %s; run: %s sync --resources %s --since 7d", list, saasLocalCLIName, syncResources)
}

// saasLocalSyncHints emits the unsynced-or-stale hint for the primary resource.
func saasLocalSyncHints(cmd *cobra.Command, db *store.Store, flags *rootFlags, resource string) {
	if !hintIfUnsynced(cmd, db, resource) {
		hintIfStale(cmd, db, resource, flags.maxAge)
	}
}

// saasParseWindow parses a loose positive duration flag (24h, 7d, 1w).
func saasParseWindow(flagName, value string) (time.Duration, error) {
	d, err := cliutil.ParseDurationLoose(value)
	if err != nil {
		return 0, usageErr(fmt.Errorf("invalid --%s %q: use a duration like 24h, 7d or 1w", flagName, value))
	}
	if d <= 0 {
		return 0, usageErr(fmt.Errorf("--%s must be greater than zero, got %q", flagName, value))
	}
	return d, nil
}

// ---- small value helpers ----

// saasGet walks nested objects; nil when any segment is missing.
func saasGet(v any, path ...string) any {
	cur := v
	for _, key := range path {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = obj[key]
	}
	return cur
}

// saasStr renders a scalar leaf as a string; objects/arrays/null become "".
func saasStr(v any, path ...string) string {
	switch t := saasGet(v, path...).(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	case json.Number:
		return t.String()
	}
	return ""
}

func saasArr(v any, path ...string) []any {
	arr, _ := saasGet(v, path...).([]any)
	return arr
}

func saasBoolPtr(v any, path ...string) *bool {
	if b, ok := saasGet(v, path...).(bool); ok {
		return &b
	}
	return nil
}

// saasParseTime accepts ISO-8601 strings (with or without zone, T or space
// separator), epoch seconds/milliseconds (number or numeric string) and
// Firestore-style {seconds,nanoseconds} objects.
func saasParseTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return time.Time{}, false
		}
		if n, err := strconv.ParseFloat(s, 64); err == nil {
			return saasEpochTime(n), true
		}
		for _, layout := range []string{
			time.RFC3339Nano,
			"2006-01-02T15:04:05.999999999Z0700",
			"2006-01-02 15:04:05.999999999Z07:00",
			"2006-01-02 15:04:05.999999999Z0700",
			"2006-01-02T15:04:05.999999999",
			"2006-01-02 15:04:05.999999999",
			"2006-01-02",
		} {
			if parsed, err := time.Parse(layout, s); err == nil {
				return parsed.UTC(), true
			}
		}
	case float64:
		if t > 0 {
			return saasEpochTime(t), true
		}
	case json.Number:
		if n, err := t.Float64(); err == nil && n > 0 {
			return saasEpochTime(n), true
		}
	case map[string]any:
		secs, ok := saasNum(t["seconds"])
		if !ok {
			secs, ok = saasNum(t["_seconds"])
		}
		if ok {
			nanos, _ := saasNum(t["nanoseconds"])
			if nanos == 0 {
				nanos, _ = saasNum(t["_nanoseconds"])
			}
			return time.Unix(int64(secs), int64(nanos)).UTC(), true
		}
	}
	return time.Time{}, false
}

// saasNum reads a JSON number decoded either as float64 or json.Number.
func saasNum(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

// saasEpochTime treats values >= 1e12 as milliseconds (Respond timestamps).
func saasEpochTime(n float64) time.Time {
	if n >= 1e12 {
		return time.UnixMilli(int64(n)).UTC()
	}
	sec, frac := math.Modf(n)
	return time.Unix(int64(sec), int64(frac*1e9)).UTC()
}

func saasFormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func saasAppendUnique(list []string, v string) []string {
	for _, existing := range list {
		if existing == v {
			return list
		}
	}
	return append(list, v)
}

// saasSortedKeys returns the map keys sorted, never nil.
func saasSortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// saasLagSeconds rounds a lag to whole seconds for output.
func saasLagSeconds(lag *float64) *int64 {
	if lag == nil {
		return nil
	}
	v := int64(math.Round(*lag))
	return &v
}

// saasFormatAge renders a duration compactly: 3d4h, 5h12m, 7m, 40s.
func saasFormatAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d >= 24*time.Hour:
		days := int64(d / (24 * time.Hour))
		hours := int64((d % (24 * time.Hour)) / time.Hour)
		if hours == 0 {
			return fmt.Sprintf("%dd", days)
		}
		return fmt.Sprintf("%dd%dh", days, hours)
	case d >= time.Hour:
		hours := int64(d / time.Hour)
		mins := int64((d % time.Hour) / time.Minute)
		if mins == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh%dm", hours, mins)
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int64(d/time.Minute))
	}
	return fmt.Sprintf("%ds", int64(d/time.Second))
}
