// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Shared helpers for the hand-written fleet/trace commands. Rows come from the
// generic resources table that `sync` fills. Outputs are built field by field,
// so secret-bearing fields (SMTP passwords, SIP and provisioning passwords,
// voicemail PINs, API keys) never reach a fleet or trace result.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"netsapiens-pp-cli/internal/store"
)

// Resource types written by sync (flat names, and the per-domain walker names).
const (
	rtDomains        = "domains"
	rtUsers          = "users"
	rtDevices        = "devices"
	rtCallqueues     = "callqueues"
	rtAutoattendants = "autoattendants"
	rtBilling        = "billing"
	rtAddresses      = "domains_addresses"
	rtPhonenumbers   = "phonenumbers"
	rtSmsnumbers     = "smsnumbers"
)

// Every NDP (MAC provisioning) table: reseller-wide /phones and per-domain /domains/{domain}/phones.
var rtPhones = []string{"phones", "domains_phones"}

// fleetSyncHint is the sync line printed when the local mirror lacks fleet data.
const fleetSyncHint = "netsapiens-cli sync --resources domains,users,devices,callqueues,autoattendants,billing,domains_addresses,phonenumbers,phones"

type fleetRow map[string]any

func fleetDBPath(p string) string {
	if p != "" {
		return p
	}
	return defaultDBPath("netsapiens-cli")
}

// openFleetStore opens the mirror read-only. When it does not exist it prints a
// sync hint plus the empty result (whose completeness is false) and returns a
// nil store; row-only output refuses instead.
func openFleetStore(ctx context.Context, cmd *cobra.Command, flags *rootFlags, dbPath string, empty any) (*store.Store, error) {
	path := fleetDBPath(dbPath)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Fprintf(cmd.ErrOrStderr(), "hint: no local mirror at %s, so this result is empty, not clean\nhint: run: %s --db %s\n", path, fleetSyncHint, path)
		switch {
		case fleetRowOutput(flags) || flags.selectFields != "":
			return nil, incompleteAuditErr("no local mirror at "+path, "")
		case !wantsHumanTable(cmd.OutOrStdout(), flags):
			return nil, printJSONFiltered(cmd.OutOrStdout(), empty, flags)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Incomplete: no local mirror at %s, so nothing was checked. Run: %s --db %s\n", path, fleetSyncHint, path)
		return nil, nil
	}
	db, err := store.OpenReadOnlyContext(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("opening local mirror %s: %w", path, err)
	}
	return db, nil
}

// loadFleetRows reads every row of the given resource types. Drain-first: the
// rows cursor is closed before the caller runs any follow-up query.
func loadFleetRows(ctx context.Context, db *store.Store, types ...string) ([]fleetRow, error) {
	if len(types) == 0 {
		return nil, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(types)), ",")
	args := make([]any, len(types))
	for i, t := range types {
		args[i] = t
	}
	rows, err := db.DB().QueryContext(ctx,
		`SELECT data, COALESCE(CAST(synced_at AS TEXT), ''), resource_type FROM resources WHERE resource_type IN (`+ph+`) ORDER BY resource_type, id`, args...)
	if err != nil {
		return nil, fmt.Errorf("querying %s: %w", strings.Join(types, ","), err)
	}
	type raw struct{ data, synced, rt string }
	var raws []raw
	for rows.Next() {
		var r raw
		if err := rows.Scan(&r.data, &r.synced, &r.rt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scanning %s: %w", strings.Join(types, ","), err)
		}
		raws = append(raws, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	out := make([]fleetRow, 0, len(raws))
	for _, r := range raws {
		dec := json.NewDecoder(strings.NewReader(r.data))
		dec.UseNumber()
		var m map[string]any
		if err := dec.Decode(&m); err != nil || m == nil {
			continue // a non-object row cannot carry fleet fields
		}
		m["_synced_at"] = r.synced
		m["_rt"] = r.rt
		out = append(out, m)
	}
	return out, nil
}

// fstr returns the first non-empty value among keys, as a trimmed string.
// Several keys exist because v45 platforms return kebab-case v2 fields while
// some endpoints still answer with the legacy snake_case shape.
func (r fleetRow) fstr(keys ...string) string {
	for _, k := range keys {
		v, ok := r[k]
		if !ok || v == nil {
			continue
		}
		var s string
		switch t := v.(type) {
		case string:
			s = t
		case json.Number:
			s = t.String()
		case bool:
			s = strconv.FormatBool(t)
		default:
			continue // objects and arrays are not scalar identifiers
		}
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return ""
}

// fnum returns the first numeric value among keys.
func (r fleetRow) fnum(keys ...string) (float64, bool) {
	for _, k := range keys {
		s := r.fstr(k)
		if s == "" {
			continue
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// fyes reports a NetSapiens yes/no (or boolean) flag.
func (r fleetRow) fyes(keys ...string) bool {
	switch strings.ToLower(r.fstr(keys...)) {
	case "yes", "true", "1", "y", "on", "enabled":
		return true
	}
	return false
}

// domainOf names the tenant a row belongs to; walker rows carry the injected parent key.
func (r fleetRow) domainOf() string {
	return r.fstr("domain", "domains_id", "parent_id")
}

func fullName(r fleetRow) string {
	return strings.TrimSpace(r.fstr("name-first-name") + " " + r.fstr("name-last-name"))
}

// normDigits keeps the digits of a phone number, its country code included:
// the country code is part of an E.164 number's identity (+64 9 555 0142 is
// not +1 649 555 0142). Compare numbers with phoneIdentity/phoneCandidates.
func normDigits(s string) string {
	var b strings.Builder
	for _, c := range s {
		if c >= '0' && c <= '9' {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// netsapiens hand-fix (netsapiens-trace-number-needs-phone-syntax): identifier-disambiguation.

// phoneIdentity is the E.164 number a stored value names (Phonenumber.phonenumber
// and caller IDs are documented E.164 with the country code and no '+').
func phoneIdentity(stored string) string {
	if d := normDigits(stored); d != "" {
		return "+" + d
	}
	return ""
}

// phoneCandidates lists every E.164 number an operator's input can mean. An
// international marker (a '+' anywhere before the first digit, so "(+64) 9",
// "tel:+64" and "+64" alike, or a leading 00 or 011 international prefix) is
// exactly one number, the digits after the marker: never a NANP reading.
// Otherwise the digits as written (E.164 without '+') and, for 10 digits, the
// NANP national number under +1. More than one stored match is ambiguous.
func phoneCandidates(q string) map[string]bool {
	d := normDigits(q)
	if d == "" {
		return nil
	}
	if i := strings.IndexAny(q, "0123456789"); strings.Contains(q[:i], "+") {
		return map[string]bool{"+" + d: true}
	}
	for _, prefix := range []string{"011", "00"} {
		if rest, ok := strings.CutPrefix(d, prefix); ok && rest != "" {
			return map[string]bool{"+" + rest: true}
		}
	}
	out := map[string]bool{"+" + d: true}
	if len(d) == 10 {
		out["+1"+d] = true
	}
	return out
}

// normMAC lowercases a MAC and strips separators.
func normMAC(s string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(s) {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// isMAC accepts 12 hex digits with optional separators. An all-digit query
// without ':' or '.' is a phone number first (442071234567 is a UK DID).
func isMAC(s string) bool {
	l := strings.ToLower(s)
	if len(normMAC(s)) != 12 || len(strings.Trim(l, "0123456789abcdef:-.")) != 0 {
		return false
	}
	return strings.ContainsAny(l, "abcdef:.")
}

// parseNSTime reads the datetime shapes NetSapiens returns.
func parseNSTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "["); i > 0 {
		s = s[:i] // "2023-11-27T13:00:00Z[US/Pacific]"
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// key joins a domain-scoped identity; extensions and queue numbers repeat across tenants.
// NetSapiens domain names are case-sensitive, so neither part is case-folded.
func key(domain, id string) string { return domain + "\x00" + id }

// indexBy maps rows by domain-scoped key on the first non-empty id field.
func indexBy(rows []fleetRow, idKeys ...string) map[string]fleetRow {
	m := make(map[string]fleetRow, len(rows))
	for _, r := range rows {
		if id := r.fstr(idKeys...); id != "" {
			m[key(r.domainOf(), id)] = r
		}
	}
	return m
}

// regInfo tallies a user's devices by registration state; devices whose record
// omits the state are unknown, so "zero registered" needs unknown == 0.
type regInfo struct{ registered, unknown int }

// knownZero reports a confirmed "no registered phone".
func (r regInfo) knownZero() bool { return r.registered == 0 && r.unknown == 0 }

// regMap tallies registration per domain-scoped user. Devices whose record
// omits the owner are tallied under key(domain, "") and could belong to any
// user of that domain, so they make every user's zero unverifiable.
type regMap map[string]regInfo

// forUser returns the user's tally with the domain's unowned devices folded in
// as unknown.
func (m regMap) forUser(domain, user string) regInfo {
	if domain == "" || user == "" {
		return regInfo{unknown: 1} // an absent half of the key cannot be joined
	}
	ri := m[key(domain, user)]
	for _, k := range []string{key(domain, ""), key("", "")} { // unowned in this domain, or with no domain at all
		un := m[k]
		ri.unknown += un.registered + un.unknown
	}
	return ri
}

// registeredDevices tallies registration per domain-scoped user.
func registeredDevices(devices []fleetRow) regMap {
	m := regMap{}
	for _, d := range devices {
		owner := ""
		if has(d, "user") {
			owner = d.fstr("user")
		}
		if d.domainOf() == "" {
			owner = "" // no tenant: it could be any domain's device
		}
		k := key(d.domainOf(), owner)
		ri := m[k]
		switch {
		case !has(d, "device-sip-registration-state"):
			ri.unknown++
		case strings.EqualFold(d.fstr("device-sip-registration-state"), "registered"):
			ri.registered++
		}
		m[k] = ri
	}
	return m
}

// filterDomain keeps rows of one domain when scope is set.
func filterDomain(rows []fleetRow, scope string) []fleetRow {
	if scope == "" {
		return rows
	}
	out := rows[:0:0]
	for _, r := range rows {
		if r.domainOf() == scope { // domains are case-sensitive
			out = append(out, r)
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// fleetSynced reports whether any of the resource types is coverage (see
// fleetCoverageOf). Row presence cannot tell a synced empty collection (a
// tenant whose last address was deleted) from one never fetched, cut short or
// filtered, so every absence-based claim (missing, zero, gap, removed) is gated on this.
func fleetSynced(db *store.Store, types ...string) bool {
	for _, rt := range types {
		if fleetCoverageOf(db, rt).ok {
			return true
		}
	}
	return false
}

// fleetCoverage is one resource's standing as evidence for an absence-based claim.
type fleetCoverage struct {
	ok     bool      // the latest completed walk was recorded, unfiltered and uncapped
	since  time.Time // when that walk started: rows older than it were not returned by it
	behind string    // a dependent whose parent was walked again after it
	why    string    // why it is not coverage
}

// fleetCoverageOf judges one resource from sync_state plus the provenance the
// sync command records (netsapiens_sync_provenance.go). A dependent (users,
// devices, ... walked per domain) is coverage only over a covered parent; when
// the parent was walked again after it, it may miss rows under new parents and
// is reported as behind (stale).
func fleetCoverageOf(db *store.Store, rt string) fleetCoverage {
	var at string
	if err := db.DB().QueryRow(`SELECT CAST(last_synced_at AS TEXT) FROM sync_state WHERE resource_type = ? AND last_synced_at IS NOT NULL AND last_attempt_complete = 1`, rt).Scan(&at); err != nil {
		return fleetCoverage{why: "its latest sync did not complete (or never ran)"}
	}
	p, ok := readFleetProvenance(db, rt)
	switch {
	case !ok || p.stateSyncedAt != at:
		return fleetCoverage{why: "its latest completed sync was not recorded by this CLI's sync command"}
	case !p.authoritative:
		return fleetCoverage{why: "its latest sync was narrowed (" + p.detail + "), so it covers only part of the table"}
	}
	c := fleetCoverage{ok: true, since: p.runStarted}
	if parent := fleetDependentParent[rt]; parent != "" && parent != rt {
		pc := fleetCoverageOf(db, parent)
		switch {
		case !pc.ok:
			return fleetCoverage{why: "it was walked over " + parent + ", and " + parent + ": " + pc.why}
		case pc.since.After(c.since):
			c.behind = parent
		case pc.behind != "":
			c.behind = pc.behind
		}
	}
	return c
}

// fleetStale names the covered resources older than --max-age, or walked
// before their parent's latest walk, with one stderr hint each. A dependent's
// parent is judged with it: rows under a domain added since the parent's walk
// were never walked, however fresh the dependent is.
func fleetStale(cmd *cobra.Command, flags *rootFlags, db *store.Store, types ...string) []string {
	var out []string
	seen := map[string]bool{}
	types = append([]string(nil), types...)
	for i := 0; i < len(types); i++ {
		rt := types[i]
		if seen[rt] {
			continue
		}
		seen[rt] = true
		// netsapiens hand-fix (netsapiens-audits-judge-every-dependency): mirror-freshness, parent age.
		if parent := fleetDependentParent[rt]; parent != "" && parent != rt {
			types = append(types, parent)
		}
		c := fleetCoverageOf(db, rt)
		switch {
		case !c.ok:
			continue
		case c.behind != "":
			fmt.Fprintf(cmd.ErrOrStderr(), "hint: %s was synced before %s was synced again, so it may miss rows under new %s. Run: netsapiens-cli sync --resources %s\n", rt, c.behind, c.behind, rt)
		case flags != nil && flags.maxAge > 0 && time.Since(c.since) > flags.maxAge:
			fmt.Fprintf(cmd.ErrOrStderr(), "hint: %s was last fully synced %s ago, older than --max-age=%s. Run: netsapiens-cli sync --resources %s\n", rt, syncHintRoundAge(time.Since(c.since)), flags.maxAge, rt)
		default:
			continue
		}
		out = append(out, rt)
	}
	return out
}

// auditCoverage is the completeness every audit result carries: an absence in
// the result is a clean answer only when Complete is true.
type auditCoverage struct {
	Complete bool     `json:"complete"`
	Missing  []string `json:"unsynced_resources,omitempty"`
	Stale    []string `json:"stale_resources,omitempty"`
	remedy   string   // how to complete it, when not by a sync
}

// noMirrorCoverage is the result of an audit with no local mirror at all.
func noMirrorCoverage() auditCoverage { return auditCoverage{Missing: []string{"local mirror"}} }

// fleetAuditCoverage judges an audit's required sources: each group needs one
// covered member (alternatives such as the two NDP tables share a group), and
// every member, plus every other table the audit read into cov (a dependency
// such as the users a route target is checked against), is judged for age.
func fleetAuditCoverage(cmd *cobra.Command, flags *rootFlags, db *store.Store, cov fleetCov, groups ...[]string) auditCoverage {
	var ac auditCoverage
	var all []string
	for _, g := range groups {
		if !cov.anyCovered(g...) {
			ac.Missing = append(ac.Missing, strings.Join(g, "|"))
		}
		all = append(all, g...)
	}
	// netsapiens hand-fix (netsapiens-audits-judge-every-dependency): mirror-freshness.
	read := cov.tables()
	ac.Stale = fleetStale(cmd, flags, db, append(all, read...)...)
	ac.Complete = len(ac.Missing) == 0 && len(ac.Stale) == 0
	return ac
}

// fleetRowOutput reports output shapes that print bare rows and so cannot
// carry an audit's completeness.
func fleetRowOutput(flags *rootFlags) bool { return flags.csv || flags.plain || flags.quiet }

// incompleteAuditErr refuses a row-only or --select output of an incomplete
// audit: a partial list there reads exactly like a clean one.
func incompleteAuditErr(what, remedy string) error {
	if remedy == "" {
		remedy = "run: " + fleetSyncHint
	}
	return partialFailureErr(fmt.Errorf("incomplete audit (%s); --csv, --plain, --quiet and --select cannot mark a partial result, so nothing was printed. To complete it, %s; or use --json to see the partial result with complete:false", what, remedy))
}

// printFleetAudit prints an audit: JSON shapes get the envelope with its
// completeness, row-only shapes get the rows, and an audit missing a required
// source refuses row-only and --select output.
func printFleetAudit(cmd *cobra.Command, flags *rootFlags, ac auditCoverage, envelope, rows any, keep ...string) error {
	// netsapiens hand-fix (netsapiens-select-keeps-incompleteness): incomplete-audit-output.
	// Any incompleteness (missing or stale) refuses the shapes that cannot mark it.
	if !ac.Complete && (fleetRowOutput(flags) || flags.selectFields != "") {
		var parts []string
		if len(ac.Missing) > 0 {
			if ac.remedy != "" {
				parts = append(parts, strings.Join(ac.Missing, ", "))
			} else {
				parts = append(parts, "not covered by a completed, unfiltered sync: "+strings.Join(ac.Missing, ", "))
			}
		}
		if len(ac.Stale) > 0 {
			parts = append(parts, "stale: "+strings.Join(ac.Stale, ", "))
		}
		if len(parts) == 0 {
			parts = append(parts, "not complete")
		}
		return incompleteAuditErr(strings.Join(parts, "; "), ac.remedy)
	}
	if fleetRowOutput(flags) {
		return printJSONFilteredKeep(cmd.OutOrStdout(), rows, flags, keep...)
	}
	return printJSONFilteredKeep(cmd.OutOrStdout(), envelope, flags, append(keep, "complete", "unsynced_resources", "stale_resources")...)
}

// printIncomplete writes the human-mode incompleteness line; it replaces any all-clear.
func printIncomplete(cmd *cobra.Command, ac auditCoverage) {
	if ac.Complete {
		return
	}
	var parts []string
	if len(ac.Missing) > 0 {
		parts = append(parts, "not covered by a completed, unfiltered sync: "+strings.Join(ac.Missing, ", "))
	}
	if len(ac.Stale) > 0 {
		parts = append(parts, "stale: "+strings.Join(ac.Stale, ", "))
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Incomplete (%s). An empty or short list here is not a clean result. Run: %s\n", strings.Join(parts, "; "), fleetSyncHint)
}

// realValue blanks NetSapiens inheritance markers: "[*]" (the schema default
// for device emergency caller ID) and "*" mean "use the next level up", not a value.
func realValue(s string) string {
	switch t := strings.TrimSpace(s); t {
	case "[*]", "*", "[]":
		return ""
	default:
		return t
	}
}

// hintCoverage writes one stderr hint per resource whose latest sync did not
// complete, so an empty or short list is never read as a clean result. The
// hint: prefix is what the MCP shell-out forwards to agents.
func hintCoverage(cmd *cobra.Command, db *store.Store, types ...string) {
	for _, rt := range types {
		if c := fleetCoverageOf(db, rt); !c.ok {
			fmt.Fprintf(cmd.ErrOrStderr(), "hint: %s is not covered: %s; results may be missing rows. Run: netsapiens-cli sync --resources %s\n", rt, c.why, rt)
		}
	}
}

// fnumPtr returns the first numeric value among keys, or nil when no key is
// present: an absent billing field is unknown, not zero.
func (r fleetRow) fnumPtr(keys ...string) *float64 {
	if v, ok := r.fnum(keys...); ok {
		return &v
	}
	return nil
}

// optYes is fyes for a single key that may be absent (nil = unknown).
func optYes(r fleetRow, k string) *bool {
	if r.fstr(k) == "" {
		return nil
	}
	v := r.fyes(k)
	return &v
}

// has reports that the record carries the field with a non-null value. Absent
// fields are unknown: no claim of "empty", "zero" or "unset" may rest on them.
func has(r fleetRow, k string) bool {
	v, ok := r[k]
	return ok && v != nil
}

// fleetCov records, per resource type, whether its latest sync completed.
type fleetCov map[string]bool

// loadCovered returns rows only from resource types that are coverage (see
// fleetCoverageOf) and records coverage in cov. Rows older than the covering
// walk's start were not returned by it (deleted or moved upstream; only --full
// prunes them), so they are not read. Every fleet table is tenant-scoped: a
// table holding any row without a tenant (no domain) cannot be joined safely,
// so it is treated as incomplete too.
func loadCovered(ctx context.Context, db *store.Store, cov fleetCov, types ...string) ([]fleetRow, error) {
	var out []fleetRow
	for _, rt := range types {
		cov[rt] = false
		c := fleetCoverageOf(db, rt)
		if !c.ok {
			continue
		}
		rows, err := loadFleetRows(ctx, db, rt)
		if err != nil {
			return nil, err
		}
		kept := rows[:0]
		tenantless := 0
		for _, r := range rows {
			// Second resolution is enough: a walk starts on a whole second after any
			// earlier walk's last write (netsapiens_sync_provenance.go).
			at, ok := parseNSTime(fmt.Sprint(r["_synced_at"]))
			if !ok || at.Before(c.since) {
				continue
			}
			if r.domainOf() == "" {
				tenantless++
			}
			kept = append(kept, r)
		}
		if tenantless > 0 {
			fmt.Fprintf(os.Stderr, "hint: %d %s row(s) carry no domain, so %s is treated as incomplete; re-sync it\n", tenantless, rt, rt)
			continue
		}
		cov[rt] = true
		out = append(out, kept...)
	}
	return out, nil
}

// tables lists every table read into c (covered or not), sorted: the
// dependencies an answer joined, each of which fleetStale must judge.
func (c fleetCov) tables() []string {
	return sortedKeys(c)
}

// anyCovered reports whether any of the types completed a sync.
func (c fleetCov) anyCovered(types ...string) bool {
	for _, t := range types {
		if c[t] {
			return true
		}
	}
	return false
}

// uncovered lists the types that did not complete a sync.
func (c fleetCov) uncovered(types ...string) []string {
	var out []string
	for _, t := range types {
		if !c[t] {
			out = append(out, t)
		}
	}
	return out
}

// optBool returns &v when known, else nil (unknown).
func optBool(known, v bool) *bool {
	if !known {
		return nil
	}
	return &v
}
