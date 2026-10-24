package cli

// Transcendence commands: fleet-wide rollups, cross-entity correlation, and
// local-history analytics that the read-mostly, per-org Huntress API cannot
// return in a single call. All read from the local SQLite mirror populated by
// `sync`; run `sync` first or they return empty results honestly.
//
// STORE SHAPE (verified against the generated internal/store): sync writes every
// entity into the generic `resources(id, resource_type, data, synced_at,
// updated_at)` table, where `id` is the storage id and the entity's fields live
// inside the `data` JSON blob. So every field is read via
// json_extract(data,'$.field').
//
// ONE ENTITY, SEVERAL RESOURCE TYPES: sync stores the same entity under more
// than one resource_type. A standard credential syncs the flat collections
// (/v1/agents -> "agents", /v1/incident_reports -> "incident-reports"); a
// reseller credential also fans out per account (/v1/accounts/{id}/agents ->
// "accounts_agents"). A command that reads only one of them answers "nothing"
// for the other kind of credential. entitySource() unions every resource_type
// the entity is stored under and keeps one row per entity id (the most
// recently synced representation), so nothing is missed and nothing is
// counted twice.
//
// RECONCILED, NOT UNIONED: a union alone resurrects a row from one alias that
// a later complete sync of another alias (or of its own) did not return. The
// commands read entities only through novelStore.src, which openStoreFor
// builds by reconciling the aliases against their sync snapshots
// (reconcileEntity): a row is left out only when a FULL walk of its scope
// (from the first page in one run, to a proven natural end, with no
// row-selecting filter, as walk_coverage records it) started after the row
// was written and did not return it; a row any other later walk did not
// return (filtered, resumed, unfinished, of unknown scope, or a sync the walk
// records do not describe) is kept, counted and named in a warning; and the
// account an entity belongs to is merged across every copy before anything is
// grouped by it.
//
// Timestamps from the API are ISO8601 with 'T'/'Z', which SQLite's julianday()
// will not parse raw, so timestamp fields are wrapped via jdx() to normalize
// 'T'->' ' and strip trailing 'Z' around the json_extract.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"huntress-pp-cli/internal/store"
)

const transcendenceApp = "huntress-cli"

// Entities the novel commands read. Each maps to every resource_type sync
// stores it under (entityResourceTypes).
const (
	entIncidents = "incident reports"
	entAgents    = "agents"
	entOrgs      = "organizations"
	entInvoices  = "invoices"
	entPorts     = "external ports"
	entEscal     = "escalations"
	entSignals   = "signals"
)

// entityResourceTypes lists, per entity, the flat resource_type a standard
// credential's sync writes and the reseller fan-out partition. The literals
// are the names sync passes to the store (resource_paths.go and
// dependentResourceDefs in sync.go).
var entityResourceTypes = map[string][]string{
	entIncidents: {"incident-reports", "accounts_incident_reports"},
	entAgents:    {"agents", "accounts_agents"},
	entOrgs:      {"organizations", "accounts_organizations"},
	entInvoices:  {"invoices", "accounts_invoices"},
	entPorts:     {"external-ports", "accounts_external_ports"},
	entEscal:     {"escalations"},
	entSignals:   {"signals", "accounts_signals"},
}

// entityIDSQL is one entity's identity across aliases: its API id as text,
// with a whole-number REAL (7.0) read as the integer it is, so 7, 7.0 and "7"
// are one entity. A row without an id keeps a per-alias storage identity.
const entityIDSQL = `COALESCE(CASE WHEN json_type(r.data,'$.id') = 'real' AND json_extract(r.data,'$.id') = CAST(json_extract(r.data,'$.id') AS INTEGER)
		THEN CAST(CAST(json_extract(r.data,'$.id') AS INTEGER) AS TEXT) ELSE CAST(json_extract(r.data,'$.id') AS TEXT) END,
		r.resource_type || ':' || r.id)`

// entitySource is the FROM-clause union for one entity: every resource_type
// it is stored under, one row per entity id, the most recently synced
// representation winning. It exposes the columns id, resource_type, data,
// synced_at and pp_eid (the entity id). Commands read novelStore.src, which
// is this union reconciled against the sync snapshots.
func entitySource(entity string) string {
	types := entityResourceTypes[entity]
	if len(types) == 0 {
		panic("entitySource: unknown entity " + entity)
	}
	quoted := make([]string, len(types))
	for i, t := range types {
		quoted[i] = "'" + t + "'"
	}
	return `(SELECT id, resource_type, data, synced_at, pp_eid FROM (
		SELECT r.id, r.resource_type, r.data, r.synced_at, ` + entityIDSQL + ` AS pp_eid,
			ROW_NUMBER() OVER (
				PARTITION BY ` + entityIDSQL + `
				ORDER BY r.synced_at DESC, r.rowid DESC) AS pp_rank
		FROM resources r WHERE r.resource_type IN (` + strings.Join(quoted, ",") + `)
	) WHERE pp_rank = 1)`
}

// novelStore is the local store as the novel commands read it: openStoreFor
// reconciles each entity the command reads once, and every query reads that
// reconciliation through src and orgJoin.
type novelStore struct {
	*store.Store
	views map[string]*entityView
}

// entityView is one entity's reconciliation across its aliases.
type entityView struct {
	// dropped holds the entity ids a later FULL walk of their scope (from
	// the first page in one run, complete, unfiltered) did not return:
	// removed upstream, so never counted.
	dropped []string
	// ident is the account each entity id belongs to, merged across every
	// alias copy (a flat copy carries no account; its partition copy does).
	ident map[string]accountIdentity
	// mixed is true when any copy of any row of this entity names an
	// account, so a row that names none cannot be assumed to be the
	// credential's own account.
	mixed bool
	// coverage is the walk records reconciliation used.
	coverage map[coverageKey]walkCoverage
}

// accountIdentity is the account an entity's copies name.
type accountIdentity struct {
	account  string // "" when no copy names one
	conflict bool   // the copies name different accounts
}

func (n *novelStore) view(entity string) *entityView {
	v, ok := n.views[entity]
	if !ok {
		panic("novel command reads " + entity + " without passing it to openStoreFor")
	}
	return v
}

// src is the FROM-clause source for one entity: entitySource without the
// entity ids reconcileEntity proved removed. Alias it like a table
// ("... FROM " + st.src(entAgents) + " a").
func (n *novelStore) src(entity string) string {
	v := n.view(entity)
	if len(v.dropped) == 0 {
		return entitySource(entity)
	}
	quoted := make([]string, len(v.dropped))
	for i, id := range v.dropped {
		quoted[i] = "'" + strings.ReplaceAll(id, "'", "''") + "'"
	}
	return "(SELECT * FROM " + entitySource(entity) + " WHERE pp_eid NOT IN (" + strings.Join(quoted, ",") + "))"
}

// orgJoin is the standard LEFT JOIN from an aliased incident/agent row to the
// organization (alias o), matching json organization_id to the org's id.
func (n *novelStore) orgJoin(childAlias string) string {
	return "LEFT JOIN " + n.src(entOrgs) + " o ON " + jx("o", "id") + " = " + jx(childAlias, "organization_id")
}

// aliasIsPartition reports whether a resource type is a reseller fan-out
// partition (sync walks it per account and records the walk's END time),
// rather than a flat collection (sync records the walk's START time minus
// one second as its watermark).
func aliasIsPartition(rt string) bool { return strings.HasPrefix(rt, "accounts_") }

// aliasState is one resource type's last sync attempt as sync_state records
// it: whether it finished, and its watermark (zero when unreadable).
type aliasState struct {
	complete bool
	at       time.Time
}

// readAliasStates reads the last sync attempt of each resource type. An
// error means completeness cannot be checked (a store whose sync_state
// predates last_attempt_complete, or an unreadable row); the caller reports
// the provenance as unknown rather than as complete.
func readAliasStates(ctx context.Context, db *sql.DB, types []string) (map[string]aliasState, error) {
	out := map[string]aliasState{}
	if len(types) == 0 {
		return out, nil
	}
	typesJSON, err := json.Marshal(types)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT resource_type, last_attempt_complete, CAST(last_synced_at AS TEXT) FROM sync_state
		WHERE resource_type IN (SELECT value FROM json_each(?))`, string(typesJSON))
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var rt string
		var complete sql.NullInt64
		var at sql.NullString
		if err := rows.Scan(&rt, &complete, &at); err != nil {
			return nil, fmt.Errorf("reading the sync state: %w", err)
		}
		if !complete.Valid {
			return nil, fmt.Errorf("the sync state of %s records no completion flag", rt)
		}
		s := aliasState{complete: complete.Int64 == 1}
		if t, ok := parseStoreTime(at.String); ok && t.Year() > 1 {
			s.at = t
		}
		out[rt] = s
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the sync state: %w", err)
	}
	return out, nil
}

// parseStoreTime reads a timestamp the store wrote (RFC3339 from sync, the
// space-separated form from a column default).
func parseStoreTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05Z07:00", "2006-01-02 15:04:05.999999999Z07:00", "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02 15:04:05.999999999 -0700 MST"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// canonID is an id as one comparable string: 7, 7.0 and "7" are "7".
func canonID(s string) string {
	s = strings.TrimSpace(s)
	if f, err := strconv.ParseFloat(s, 64); err == nil && f == math.Trunc(f) && math.Abs(f) < 1<<53 {
		return strconv.FormatInt(int64(f), 10)
	}
	return s
}

// reconcileSlack absorbs the one-second resolution of synced_at and the sync
// watermark: a snapshot counts as later than a row only when it is at least
// this much later.
const reconcileSlack = 2 * time.Second

// aliasCopy is one stored copy of an entity, as reconciliation reads it.
type aliasCopy struct {
	rt     string
	rowid  int64
	at     time.Time
	atOK   bool
	org    string
	parent string // the reseller partition the copy was stored under ($.parent_id)
	accts  []string
}

// walkCoverage is what the store records about the walks of one scope of a
// resource type (store.RecordWalkCoverage, written by sync): the most recent
// walk (dated from the start of its logical walk, so a walk resumed from a
// saved page cursor counts the pages the interrupted run read), and the most
// recent FULL walk, one that started from the first page in the same run,
// reached a proven natural end and carried no row-selecting filter.
// fullStarted is zero when no walk of the scope was ever full. Only a full
// walk's silence about a row proves anything.
type walkCoverage struct {
	lastStarted, lastEnded time.Time // zero when no walk of the scope finished
	lastComplete           bool
	lastFiltered           bool
	lastResumed            bool // continued from a saved page cursor: never proof
	fullStarted, fullEnded time.Time
}

// coverageKey names one walked scope: "" is the resource-level walk (a flat
// walk, or a whole reseller fan-out), a parent id one partition of the
// fan-out.
type coverageKey struct{ rt, scope string }

// readWalkCoverage reads the walk records of the given resource types. A
// store without the table (synced before walks were recorded) has none, which
// is not an error: reconcileEntity then treats every later complete sync as
// one it cannot describe. A record it cannot read is an error; the caller
// warns and reads the store as having no records.
func readWalkCoverage(ctx context.Context, db *sql.DB, types []string) (map[coverageKey]walkCoverage, error) {
	out := map[coverageKey]walkCoverage{}
	typesJSON, err := json.Marshal(types)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT resource_type, scope, last_started_at, last_ended_at, last_complete, last_filtered, last_resumed, full_started_at, full_ended_at
		FROM walk_coverage WHERE resource_type IN (SELECT value FROM json_each(?))`, string(typesJSON))
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var rt, scope string
		var complete, filtered, resumed int64
		var lastStarted, lastEnded, fullStarted, fullEnded sql.NullString
		if err := rows.Scan(&rt, &scope, &lastStarted, &lastEnded, &complete, &filtered, &resumed, &fullStarted, &fullEnded); err != nil {
			return nil, fmt.Errorf("reading the walk records: %w", err)
		}
		c := walkCoverage{lastComplete: complete == 1, lastFiltered: filtered != 0, lastResumed: resumed != 0}
		var ok1, ok2 bool
		if lastStarted.Valid || lastEnded.Valid {
			c.lastStarted, ok1 = parseStoreTime(lastStarted.String)
			c.lastEnded, ok2 = parseStoreTime(lastEnded.String)
			if !ok1 || !ok2 {
				return nil, fmt.Errorf("the walk record of %s has an unreadable time", rt)
			}
		}
		if fullStarted.Valid || fullEnded.Valid {
			c.fullStarted, ok1 = parseStoreTime(fullStarted.String)
			c.fullEnded, ok2 = parseStoreTime(fullEnded.String)
			if !ok1 || !ok2 {
				return nil, fmt.Errorf("the full-walk record of %s has an unreadable time", rt)
			}
		}
		out[coverageKey{rt, canonID(scope)}] = c
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the walk records: %w", err)
	}
	return out, nil
}

// recordWalkCoverage is how sync records what one walk enumerated; sync.go
// calls it at the end of each flat walk, each reseller partition and each
// whole fan-out. skipReason is syncPruneSkipReason for the walk: non-empty
// means a filter narrowed the rows it asked for, so its silence about a row
// proves nothing. resumed is true when the walk continued from a saved page
// cursor rather than the first page. A record that cannot be written leaves
// the previous one in place, which the novels read as a later sync they
// cannot describe: the rows it did not return are kept and flagged, never
// left out.
func recordWalkCoverage(db *store.Store, resource, scope string, started time.Time, resumed, complete bool, skipReason string, syncEvents io.Writer) {
	if db == nil {
		return
	}
	if err := db.RecordWalkCoverage(resource, canonID(scope), started, time.Now(), complete, skipReason != "", resumed); err != nil {
		warnWalkUnrecorded(resource, err, syncEvents)
	}
}

// noteWalkStart is how sync notes that a flat walk is starting, from the
// first page or resumed from a saved page cursor, so the walk that finishes
// it is dated from the start of the logical walk.
func noteWalkStart(db *store.Store, resource string, started time.Time, resumed bool, skipReason string, syncEvents io.Writer) {
	if db == nil {
		return
	}
	if err := db.NoteWalkStart(resource, "", started, skipReason != "", resumed); err != nil {
		warnWalkUnrecorded(resource, err, syncEvents)
	}
}

func warnWalkUnrecorded(resource string, err error, syncEvents io.Writer) {
	if humanFriendly {
		fmt.Fprintf(os.Stderr, "  %s: could not record what this walk enumerated (%v); local commands will flag the rows it did not return as unverified\n", resource, err)
	} else if syncEvents != nil {
		fmt.Fprintf(syncEvents, `{"event":"sync_warning","resource":%q,"reason":"walk_coverage_unrecorded","message":%q}`+"\n", resource, err.Error())
	}
}

// reconcileReport is what reconcileEntity found, per alias that decided it.
type reconcileReport struct {
	dropped    map[string]int // alias whose later full walk of the rows' scope did not return them
	unverified map[string]int // alias whose later complete walk did not return the rows, scope unknown
	filtered   map[string]int // alias whose later walk did not return the rows but was filtered
	unfinished map[string]int // alias whose later walk did not return the rows but did not finish
	resumed    map[string]int // alias whose later walk did not return the rows but resumed from a saved page
	unproven   map[string]int // alias with a later complete sync the walk records do not describe
	unwalked   map[string]int // partition whose later complete fan-out did not walk the rows' account
	untimed    int            // rows whose sync time cannot be read
	// coverageErr is set when the walk records could not be read: every
	// later sync is then one they do not describe (nothing is left out).
	coverageErr error
}

// walkFlag is the category of a row a later walk did not return, by what
// that walk was.
func (rep reconcileReport) walkFlag(c walkCoverage) map[string]int {
	switch {
	case !c.lastComplete:
		return rep.unfinished
	case c.lastFiltered:
		return rep.filtered
	case c.lastResumed:
		return rep.resumed
	}
	return rep.unverified
}

// reconcileEntity reconciles one entity's alias copies against the sync
// snapshots. For each entity id it takes the most recently synced copy and
// asks every alias whether a walk that started at least reconcileSlack after
// that copy was written (so did not return it) can say anything:
//
//   - A row is LEFT OUT only on proof: a FULL walk (complete, unfiltered,
//     recorded in walk_coverage) of the row's scope started after the row was
//     written. For a flat alias the full walk must also have returned other
//     rows of the row's organization (or, with none, its account), or its
//     scope is unknown. For a reseller partition the full walk is the one of
//     the partition (parent account) the newest copy was stored under.
//   - Every other later walk that did not return the row keeps it, counted,
//     and names it in a warning: a filtered walk, an unfinished one, a full
//     flat walk whose scope is unknown, a fan-out that did not walk the row's
//     account, and a complete sync the walk records do not describe (a store
//     synced before they were kept, whose single sync_state row cannot show
//     what the walk enumerated or whether it was filtered).
//   - A partition alias holding no copy flags the entity when it later walked
//     the entity's account, or when the entity's own walk cannot vouch for it
//     and the partition could cover its account.
//
// Identity is merged across copies first: an entity belongs to the one
// account its copies name (parent_id on a partition copy, account_id, or the
// nested account id); copies naming two accounts conflict.
//
// reconcileEntity reads the walk records itself (readWalkCoverage); one it
// cannot read is reported in the report and treated as no record, so nothing
// is left out on its say-so. The view keeps the records it used.
func reconcileEntity(ctx context.Context, db *sql.DB, entity string, states map[string]aliasState, parents map[string]bool) (*entityView, reconcileReport, error) {
	cov, covErr := readWalkCoverage(ctx, db, entityResourceTypes[entity])
	if covErr != nil {
		cov = map[coverageKey]walkCoverage{}
	}
	view, rep, err := reconcileAliases(ctx, db, entity, states, cov, parents)
	rep.coverageErr = covErr
	if view != nil {
		view.coverage = cov
	}
	return view, rep, err
}

// reconcileAliases is reconcileEntity over walk records already read.
func reconcileAliases(ctx context.Context, db *sql.DB, entity string, states map[string]aliasState, cov map[coverageKey]walkCoverage, parents map[string]bool) (*entityView, reconcileReport, error) {
	types := entityResourceTypes[entity]
	rep := reconcileReport{dropped: map[string]int{}, unverified: map[string]int{}, filtered: map[string]int{},
		unfinished: map[string]int{}, resumed: map[string]int{}, unproven: map[string]int{}, unwalked: map[string]int{}}
	view := &entityView{ident: map[string]accountIdentity{}}
	typesJSON, err := json.Marshal(types)
	if err != nil {
		return nil, rep, err
	}
	// The resource types and the nested JSON paths are bound, not spliced.
	rows, err := db.QueryContext(ctx, `SELECT r.rowid, r.resource_type, `+entityIDSQL+`, CAST(r.synced_at AS TEXT),
		CAST(COALESCE(json_extract(r.data,'$.organization_id'), json_extract(r.data, ?)) AS TEXT),
		CAST(json_extract(r.data,'$.parent_id') AS TEXT), CAST(json_extract(r.data,'$.account_id') AS TEXT),
		CAST(json_extract(r.data, ?) AS TEXT)
		FROM resources r WHERE r.resource_type IN (SELECT value FROM json_each(?))`,
		"$."+nested("organization", "id"), "$."+nested("account", "id"), string(typesJSON))
	if err != nil {
		return nil, rep, err
	}
	defer rows.Close()
	copies := map[string][]aliasCopy{}
	order := []string{}
	for rows.Next() {
		var c aliasCopy
		var eid string
		var at, org, parent, acct, nestedAcct sql.NullString
		if err := rows.Scan(&c.rowid, &c.rt, &eid, &at, &org, &parent, &acct, &nestedAcct); err != nil {
			return nil, rep, err
		}
		c.at, c.atOK = parseStoreTime(at.String)
		c.org = canonID(org.String)
		c.parent = canonID(parent.String)
		for _, a := range []sql.NullString{parent, acct, nestedAcct} {
			if v := canonID(a.String); v != "" {
				c.accts = append(c.accts, v)
			}
		}
		if _, seen := copies[eid]; !seen {
			order = append(order, eid)
		}
		copies[eid] = append(copies[eid], c)
	}
	if err := rows.Err(); err != nil {
		return nil, rep, err
	}

	// Merge identity across copies.
	orgOf := map[string]string{}
	for eid, cs := range copies {
		accts, orgs := map[string]bool{}, map[string]bool{}
		for _, c := range cs {
			for _, a := range c.accts {
				accts[a] = true
			}
			if c.org != "" {
				orgs[c.org] = true
			}
		}
		id := accountIdentity{conflict: len(accts) > 1}
		for a := range accts {
			if len(accts) == 1 {
				id.account = a
			}
			view.mixed = true
		}
		view.ident[eid] = id
		if len(orgs) == 1 {
			for o := range orgs {
				orgOf[eid] = o
			}
		}
	}

	// Evidence: every sync time per alias for each organization and account
	// (by the entity's merged identity), the newest of them, and which
	// accounts each alias holds.
	type scopeKey struct{ rt, kind, key string }
	latest := map[scopeKey]time.Time{}
	seen := map[scopeKey][]time.Time{}
	holds := map[scopeKey]bool{}
	aliasHasRows := map[string]bool{}
	note := func(k scopeKey, t time.Time) {
		if t.After(latest[k]) {
			latest[k] = t
		}
		seen[k] = append(seen[k], t)
	}
	for eid, cs := range copies {
		id := view.ident[eid]
		for _, c := range cs {
			aliasHasRows[c.rt] = true
			if !c.atOK {
				continue
			}
			if o := orgOf[eid]; o != "" {
				note(scopeKey{c.rt, "org", o}, c.at)
			}
			if id.account != "" {
				note(scopeKey{c.rt, "acct", id.account}, c.at)
				holds[scopeKey{c.rt, "acct", id.account}] = true
			}
		}
	}
	for k := range seen {
		sort.Slice(seen[k], func(i, j int) bool { return seen[k][i].Before(seen[k][j]) })
	}
	// scopeKeyOf is the entity's organization (or, with none, its account)
	// in alias rt; preferAccount picks the account first.
	scopeKeyOf := func(rt, eid string, preferAccount bool) (scopeKey, bool) {
		org, id := orgOf[eid], view.ident[eid]
		acct := ""
		if !id.conflict {
			acct = id.account
		}
		switch {
		case preferAccount && acct != "":
			return scopeKey{rt, "acct", acct}, true
		case org != "":
			return scopeKey{rt, "org", org}, true
		case acct != "":
			return scopeKey{rt, "acct", acct}, true
		}
		return scopeKey{}, false
	}
	// scopeSeenAfter: alias rt holds a row of the entity's organization (or,
	// with none, its account) synced after t.
	scopeSeenAfter := func(rt, eid string, preferAccount bool, t time.Time) bool {
		k, ok := scopeKeyOf(rt, eid, preferAccount)
		return ok && latest[k].After(t)
	}
	// scopeHeld: alias rt could hold the entity's scope: it holds no rows at
	// all (nothing shows what its walks cover), the entity has no known
	// organization or account, or rt holds a row of that organization (or,
	// with none, that account), at any time.
	scopeHeld := func(rt, eid string) bool {
		if !aliasHasRows[rt] {
			return true
		}
		k, ok := scopeKeyOf(rt, eid, false)
		return !ok || len(seen[k]) > 0
	}
	// scopeSeenWithin: alias rt holds a row of the entity's organization (or,
	// with none, its account) that the walk from..to wrote, i.e. the walk
	// covered the entity's scope.
	scopeSeenWithin := func(rt, eid string, from, to time.Time) bool {
		k, ok := scopeKeyOf(rt, eid, false)
		if !ok {
			return false
		}
		ts := seen[k]
		from = from.Truncate(time.Second)
		i := sort.Search(len(ts), func(i int) bool { return !ts[i].Before(from) })
		return i < len(ts) && !ts[i].After(to)
	}

	for _, eid := range order {
		cs := copies[eid]
		win := cs[0]
		for _, c := range cs[1:] {
			if c.atOK && (!win.atOK || c.at.After(win.at) || (c.at.Equal(win.at) && c.rowid > win.rowid)) {
				win = c
			}
		}
		if !win.atOK {
			rep.untimed++
			continue
		}
		// later: a walk that started at t did not return the newest copy.
		later := func(t time.Time) bool { return !t.IsZero() && !t.Before(win.at.Add(reconcileSlack)) }
		own := states[win.rt]
		ownConfirmed := !aliasIsPartition(win.rt) && own.complete && !own.at.IsZero() && win.at.After(own.at)
		// newestIn is the newest copy alias rt holds.
		newestIn := func(rt string) (aliasCopy, bool) {
			var best aliasCopy
			found := false
			for _, c := range cs {
				if c.rt == rt && c.atOK && (!found || c.at.After(best.at) || (c.at.Equal(best.at) && c.rowid > best.rowid)) {
					best, found = c, true
				}
			}
			return best, found
		}
		id := view.ident[eid]
		acct := ""
		if !id.conflict {
			acct = id.account
		}
		dropBy, flagBy := "", ""
		var flagIn map[string]int
		flag := func(m map[string]int, y string) {
			if flagIn == nil {
				flagIn, flagBy = m, y
			}
		}
		for _, y := range types {
			sy, hasState := states[y]
			laterComplete := hasState && sy.complete && later(sy.at)
			rc, hasRes := cov[coverageKey{y, ""}]
			// A later complete sync of y the walk records do not describe:
			// nothing shows what it enumerated or whether it was filtered.
			unrecorded := laterComplete && (!hasRes || sy.at.After(rc.lastEnded.Add(reconcileSlack)))
			if !aliasIsPartition(y) {
				switch {
				case hasRes && later(rc.fullStarted) && scopeSeenWithin(y, eid, rc.fullStarted, rc.fullEnded.Add(reconcileSlack)):
					if dropBy == "" {
						dropBy = y
					}
				case !scopeHeld(y, eid):
					// The flat list holds rows, none of them in the entity's
					// organization or account (a reseller's own list never
					// holds a sub-account's agents): no walk of it could have
					// returned the entity, so its silence says nothing.
				case unrecorded:
					flag(rep.unproven, y)
				case hasRes && later(rc.lastStarted):
					flag(rep.walkFlag(rc), y)
				}
				continue
			}
			if yc, ok := newestIn(y); ok {
				// The partition copy's own account was walked (or not)
				// after the newest copy was written.
				c, has := cov[coverageKey{y, yc.parent}]
				has = has && yc.parent != ""
				switch {
				case has && later(c.fullStarted):
					if yc.rowid == win.rowid && yc.rt == win.rt {
						if dropBy == "" {
							dropBy = y
						}
					} else {
						// The newest copy is another alias's: this walk shows
						// only that the entity left this account.
						flag(rep.unverified, y)
					}
				case unrecorded:
					flag(rep.unproven, y)
				case has && later(c.lastStarted):
					flag(rep.walkFlag(c), y)
				case hasRes && later(rc.lastStarted):
					if rc.lastComplete {
						flag(rep.unwalked, y)
					} else {
						flag(rep.unfinished, y)
					}
				}
				continue
			}
			if acct != "" {
				if c, has := cov[coverageKey{y, acct}]; has && later(c.lastStarted) {
					flag(rep.walkFlag(c), y)
					continue
				}
			}
			if !laterComplete {
				continue
			}
			// Could the partition's walk have covered the entity's account?
			// parents is nil when the synced account list could not be read:
			// then any account may be one it walked. An entity with no
			// (or a conflicting) account could be in any account the
			// partition walked, so only a partition that walked nothing is
			// ruled out.
			var couldCover bool
			switch {
			case parents == nil:
				couldCover = true
			case id.account == "" || id.conflict:
				couldCover = len(parents) > 0 || aliasHasRows[y]
			default:
				couldCover = parents[id.account] || holds[scopeKey{y, "acct", id.account}]
			}
			if scopeSeenAfter(y, eid, true, win.at.Add(reconcileSlack)) || (!ownConfirmed && couldCover) {
				flag(rep.unverified, y)
			}
		}
		switch {
		case dropBy != "":
			rep.dropped[dropBy]++
			view.dropped = append(view.dropped, eid)
		case flagIn != nil:
			flagIn[flagBy]++
		}
	}
	sort.Strings(view.dropped)
	return view, rep, nil
}

// print writes the reconciliation to stderr as warning lines (the MCP server
// forwards only hint and warning lines): rows left out, and rows kept whose
// removal cannot be established, by why.
func (rep reconcileReport) print(w io.Writer, entity string) {
	if rep.coverageErr != nil {
		fmt.Fprintf(w, "warning: could not read what the last syncs of local %s data enumerated (%v); no row is left out as removed upstream, and rows a later sync did not return are flagged as unverified.\n", entity, rep.coverageErr)
	}
	for _, y := range sortedKeys(rep.dropped) {
		fmt.Fprintf(w, "warning: local %s data: left out %d row(s) that a later complete sync of %s did not return although it returned their organization's or account's other rows (removed upstream; '%s sync --full' prunes them).\n", entity, rep.dropped[y], y, transcendenceApp)
	}
	for _, y := range sortedKeys(rep.unverified) {
		fmt.Fprintf(w, "warning: local %s data: %d row(s) were not returned by the later complete sync of %s, and whether that sync covers them cannot be established; they are still counted. Run '%s sync --full' to reconcile before acting on them.\n", entity, rep.unverified[y], y, transcendenceApp)
	}
	for _, y := range sortedKeys(rep.filtered) {
		fmt.Fprintf(w, "warning: local %s data: %d row(s) were not returned by the later sync of %s, which was filtered (a --param, --global-param or --resource-param narrowed it), so it cannot show they were removed; they are still counted. Run '%s sync' without filters to reconcile before acting on them.\n", entity, rep.filtered[y], y, transcendenceApp)
	}
	for _, y := range sortedKeys(rep.unfinished) {
		fmt.Fprintf(w, "warning: local %s data: %d row(s) were not returned by the later sync of %s, which did not finish, so whether they still exist upstream is unknown; they are still counted. Run '%s sync' to completion before acting on them.\n", entity, rep.unfinished[y], y, transcendenceApp)
	}
	for _, y := range sortedKeys(rep.resumed) {
		fmt.Fprintf(w, "warning: local %s data: %d row(s) were not returned by the later sync of %s, which resumed from a page an interrupted sync had saved, so it did not read the whole list in one walk and cannot show they were removed; they are still counted. Run '%s sync --full' to reconcile before acting on them.\n", entity, rep.resumed[y], y, transcendenceApp)
	}
	for _, y := range sortedKeys(rep.unwalked) {
		fmt.Fprintf(w, "warning: local %s data: %d row(s) belong to an account the later complete sync of %s did not walk (it is no longer in the synced account list), so whether they still exist upstream cannot be established; they are still counted. Run '%s sync --full' to reconcile before acting on them.\n", entity, rep.unwalked[y], y, transcendenceApp)
	}
	for _, y := range sortedKeys(rep.unproven) {
		fmt.Fprintf(w, "warning: local %s data: %d row(s) are older than the later complete sync of %s, and the store does not record what that sync enumerated or whether it was filtered (it predates walk records, or another build ran it), so whether it returned them cannot be established; they are still counted. Run '%s sync' with this build to reconcile before acting on them.\n", entity, rep.unproven[y], y, transcendenceApp)
	}
	if rep.untimed > 0 {
		fmt.Fprintf(w, "warning: local %s data: %d row(s) have an unreadable sync time, so whether a later sync removed them cannot be checked.\n", entity, rep.untimed)
	}
}

// warnFilteredFreshness applies the provenance rule to the --max-age check,
// which reads only the sync time sync_state records. A filtered sync
// advances that time, so the store reads as fresh although the rows its
// filter did not select were last refreshed by the last full walk; a sync
// resumed from a saved page cursor advances it too, although the pages before
// the cursor were read when the interrupted walk started. Say so when either
// is older than maxAge.
func warnFilteredFreshness(w io.Writer, entity string, types []string, cov map[coverageKey]walkCoverage, maxAge time.Duration) {
	if maxAge <= 0 {
		return
	}
	for _, rt := range types {
		c, ok := cov[coverageKey{rt, ""}]
		if !ok {
			continue
		}
		if c.lastResumed && !c.lastStarted.IsZero() {
			if age := time.Since(c.lastStarted); age > maxAge {
				fmt.Fprintf(w, "warning: local %s data: the last sync of %s resumed a walk an interrupted sync started %s ago, older than --max-age=%s, so the rows that walk read first may be stale. Run '%s sync --full'.\n", entity, rt, syncHintRoundAge(age), maxAge, transcendenceApp)
			}
		}
		if !c.lastFiltered {
			continue
		}
		if c.fullStarted.IsZero() {
			fmt.Fprintf(w, "warning: local %s data: the last sync of %s was filtered, so it refreshed only the rows its filter selected, and no unfiltered complete sync of %s is recorded; rows outside the filter may be stale. Run '%s sync' without filters.\n", entity, rt, rt, transcendenceApp)
			continue
		}
		if age := time.Since(c.fullStarted); age > maxAge {
			fmt.Fprintf(w, "warning: local %s data: the last sync of %s was filtered, so it refreshed only the rows its filter selected; the last unfiltered complete sync of %s was %s ago, older than --max-age=%s, so rows outside the filter may be stale. Run '%s sync' without filters.\n", entity, rt, rt, syncHintRoundAge(age), maxAge, transcendenceApp)
		}
	}
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// readParentAccounts is the set of account ids the reseller fan-out walks
// (the synced accounts list).
func readParentAccounts(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+entityIDSQL+` FROM resources r WHERE r.resource_type = 'accounts'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[canonID(id)] = true
	}
	return out, rows.Err()
}

// jx returns json_extract(<alias>.data,'$.<field>') for reading a blob field.
func jx(alias, field string) string {
	return "json_extract(" + alias + ".data,'$." + field + "')"
}

// jdx wraps a blob timestamp field in julianday() after normalizing the API's
// ISO8601 'T'/'Z' format so SQLite can parse it.
func jdx(alias, field string) string {
	return "julianday(replace(replace(json_extract(" + alias + ".data,'$." + field + "'),'T',' '),'Z',''))"
}

// ---- collections: the one envelope parser ------------------------------------
//
// Every list a novel reads (an API page, an envelope embedded in a stored
// row, a JSON array stored as text, a snapshot file) goes through
// collectionAt / decodeJSONList, and an API page through parseListPage. A
// missing, null or non-array list, a non-object item where objects are
// required, or pagination metadata that cannot be read is an error: the
// caller reports a failure or an unknown value, never an empty, complete list.

// decodeJSONValue decodes one JSON document with numbers kept exact.
func decodeJSONValue(raw []byte) (interface{}, error) {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, fmt.Errorf("trailing data after the JSON value")
	}
	return v, nil
}

// collectionAt returns the list stored under key in a decoded JSON object.
func collectionAt(obj map[string]interface{}, key string) ([]interface{}, error) {
	v, ok := obj[key]
	switch {
	case !ok:
		return nil, fmt.Errorf("the response has no %q list", key)
	case v == nil:
		return nil, fmt.Errorf("the %q list is null", key)
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil, fmt.Errorf("%q is not a list", key)
	}
	return arr, nil
}

// decodeJSONList reads a JSON array (a stored field, a snapshot file).
func decodeJSONList(raw []byte) ([]interface{}, error) {
	v, err := decodeJSONValue(raw)
	if err != nil {
		return nil, err
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil, fmt.Errorf("not a JSON list")
	}
	return arr, nil
}

// objectItems requires every list item to be a JSON object.
func objectItems(arr []interface{}, what string) ([]map[string]interface{}, error) {
	out := make([]map[string]interface{}, 0, len(arr))
	for i, it := range arr {
		m, ok := it.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%s item %d is not an object", what, i)
		}
		out = append(out, m)
	}
	return out, nil
}

// parseListPage reads one page of a page_token-paginated API list: the
// objects under key and the next page token ("" at the end). The pagination
// object is required: without it, whether the list is complete is unknown.
func parseListPage(raw []byte, key string) ([]map[string]interface{}, string, error) {
	v, err := decodeJSONValue(raw)
	if err != nil {
		return nil, "", fmt.Errorf("decoding the response: %w", err)
	}
	env, ok := v.(map[string]interface{})
	if !ok {
		return nil, "", fmt.Errorf("the response is not a JSON object")
	}
	arr, err := collectionAt(env, key)
	if err != nil {
		return nil, "", err
	}
	items, err := objectItems(arr, key)
	if err != nil {
		return nil, "", err
	}
	pg, ok := env["pagination"]
	if !ok {
		return nil, "", fmt.Errorf("the response has no pagination object, so whether the %s list is complete is unknown", key)
	}
	pm, ok := pg.(map[string]interface{})
	if !ok {
		return nil, "", fmt.Errorf("the pagination value is not an object")
	}
	switch tok := pm["next_page_token"].(type) {
	case nil:
		return items, "", nil
	case string:
		return items, tok, nil
	default:
		return nil, "", fmt.Errorf("next_page_token is not a string")
	}
}

// jsonCount reads a non-negative whole-number count from decoded JSON.
func jsonCount(v interface{}) (int, bool) {
	switch n := v.(type) {
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil && i >= 0
	case float64:
		return int(n), n >= 0 && n == math.Trunc(n) && n < 1<<53
	}
	return 0, false
}

// remediationCountSQL is the number of remediations an incident row carries,
// or NULL when the field cannot be read. The API embeds them as a
// {total_count, has_more, items} envelope; an older or alternate shape is a
// bare array. Mirrors remediationsOf.
func remediationCountSQL(alias string) string {
	d := alias + ".data"
	items := "'$." + nested("remediations", "items") + "'"
	tc := "'$." + nested("remediations", "total_count") + "'"
	hm := "'$." + nested("remediations", "has_more") + "'"
	return "(CASE json_type(" + d + ",'$.remediations')" +
		" WHEN 'array' THEN json_array_length(" + d + ",'$.remediations')" +
		" WHEN 'object' THEN CASE WHEN json_type(" + d + "," + items + ") = 'array'" +
		" AND COALESCE(json_type(" + d + "," + tc + "), 'null') IN ('null','integer')" +
		" AND COALESCE(json_extract(" + d + "," + tc + "), 0) >= 0" +
		" AND COALESCE(json_type(" + d + "," + hm + "), 'null') IN ('null','true','false')" +
		" THEN MAX(json_array_length(" + d + "," + items + "), COALESCE(json_extract(" + d + "," + tc + "), 0))" +
		" ELSE NULL END" +
		" ELSE NULL END)"
}

// remediationsOf decodes an incident's remediations in either shape. total is
// the API's total_count when it gives one (it embeds only the first items);
// complete is false when the embedded list is known to be shorter than total.
// known is false when the field is missing or cannot be read: the count is
// unknown, not zero.
func remediationsOf(v interface{}) (items []interface{}, total int, complete, known bool) {
	switch r := v.(type) {
	case []interface{}:
		return r, len(r), true, true
	case map[string]interface{}:
		items, err := collectionAt(r, "items")
		if err != nil {
			return nil, 0, false, false
		}
		total = len(items)
		if tc, present := r["total_count"]; present && tc != nil {
			n, ok := jsonCount(tc)
			if !ok {
				return nil, 0, false, false
			}
			if n > total {
				total = n
			}
		}
		hasMore := false
		switch hm := r["has_more"].(type) {
		case nil:
		case bool:
			hasMore = hm
		default:
			return nil, 0, false, false
		}
		return items, total, !hasMore && len(items) >= total, true
	}
	return nil, 0, false, false
}

// resolveDB returns the dbPath override or the default store location.
func resolveDB(dbPath string) string {
	if dbPath == "" {
		return defaultDBPath(transcendenceApp)
	}
	return dbPath
}

// openStore opens the local store at the resolved path.
func openStore(ctx context.Context, dbPath string) (*store.Store, error) {
	path := resolveDB(dbPath)
	// Prefer a read-only handle when the schema already exists: read-only takes
	// no write lock, so parallel transcendence reads don't collide with
	// SQLITE_BUSY. Only fall back to a read-write open (which migrates) when the
	// DB is absent or not yet migrated. A short retry resolves the first-run
	// race where one command is mid-migration and another sees the half-built
	// file (read-only would otherwise hit "no such table: resources").
	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		if st, ok := tryOpenReadOnlyMigrated(path); ok {
			return st, nil
		}
		st, err := store.OpenWithContext(ctx, path)
		if err == nil {
			return st, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		time.Sleep(time.Duration(20*(attempt+1)) * time.Millisecond)
	}
	return nil, fmt.Errorf("opening local store: %w (run `sync` first)", lastErr)
}

// tryOpenReadOnlyMigrated opens the store read-only only when the file exists
// AND the resources table is present (i.e. a prior read-write open finished
// migrating). Returns ok=false otherwise so the caller migrates via a
// read-write open.
func tryOpenReadOnlyMigrated(path string) (*store.Store, bool) {
	if _, err := os.Stat(path); err != nil {
		return nil, false
	}
	st, err := store.OpenReadOnly(path)
	if err != nil {
		return nil, false
	}
	var one int
	if err := st.DB().QueryRow(`SELECT 1 FROM sqlite_master WHERE type='table' AND name='resources' LIMIT 1`).Scan(&one); err != nil {
		_ = st.Close()
		return nil, false
	}
	return st, true
}

// openStoreFor opens the local store for a transcendence command: rejects
// --data-source live (these commands have no live equivalent), then reports
// the provenance of every entity the command reads: a never-synced hint, a
// stale hint, a warning when the last sync walk of any resource type the
// entity is stored under did not finish (its rows are a partial answer), a
// warning when that cannot be checked at all, a warning when the last sync
// was filtered or resumed and older data stands behind it
// (warnFilteredFreshness), and the alias reconciliation (reconcileEntity). Warnings go to stderr with a "warning:" prefix, which
// the MCP server forwards with the tool result. Every entity the command
// reads must be named here; the command reads them through the returned
// novelStore.
func openStoreFor(cmd *cobra.Command, flags *rootFlags, dbPath string, entities ...string) (*novelStore, error) {
	if flags.dataSource == "live" {
		return nil, fmt.Errorf("this command reads only synced local data and has no live equivalent; use --data-source auto or local (run `sync` first)")
	}
	st, err := openStore(cmd.Context(), dbPath)
	if err != nil {
		return nil, err
	}
	ns := &novelStore{Store: st, views: map[string]*entityView{}}
	w := cmd.ErrOrStderr()
	var parents map[string]bool
	parentsRead := false
	for _, e := range entities {
		types := entityResourceTypes[e]
		var synced []string
		for _, rt := range types {
			state, err := readSyncHintState(st, rt)
			if err != nil {
				fmt.Fprintf(w, "warning: could not read when %s was last synced (%v); treat local %s results as unverified.\n", rt, err, e)
				continue
			}
			if state.hasState {
				synced = append(synced, rt)
			}
		}
		if len(synced) == 0 {
			hintIfUnsynced(cmd, st, types[0])
		}
		for _, rt := range synced {
			hintIfStale(cmd, st, rt, flags.maxAge)
		}
		states, serr := readAliasStates(cmd.Context(), st.DB(), types)
		if serr != nil {
			fmt.Fprintf(w, "warning: could not check whether the last sync of %s finished (%v); treat local %s results as unverified, and run '%s sync' to refresh the sync state.\n", strings.Join(types, ", "), serr, e, transcendenceApp)
		}
		sortedTypes := append([]string{}, types...)
		sort.Strings(sortedTypes)
		for _, rt := range sortedTypes {
			if s, ok := states[rt]; ok && !s.complete {
				fmt.Fprintf(w, "warning: local %s data is partial: the last sync of %s did not finish, so results built from it can be short. Run '%s sync' to completion before acting on them.\n", e, rt, transcendenceApp)
			}
		}
		if !parentsRead && len(types) > 1 {
			parentsRead = true
			if parents, err = readParentAccounts(cmd.Context(), st.DB()); err != nil {
				fmt.Fprintf(w, "warning: could not read the synced account list (%v); reseller partitions are treated as possibly covering every account.\n", err)
				parents = nil
			}
		}
		view, rep, err := reconcileEntity(cmd.Context(), st.DB(), e, states, parents)
		if err != nil {
			_ = st.Close()
			return nil, fmt.Errorf("reading local %s: %w", e, err)
		}
		warnFilteredFreshness(w, e, sortedTypes, view.coverage, flags.maxAge)
		rep.print(w, e)
		ns.views[e] = view
	}
	return ns, nil
}

// queryMaps runs a SELECT and returns each row as a map for JSON emission.
func queryMaps(ctx context.Context, db *sql.DB, query string, args ...interface{}) ([]map[string]interface{}, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []map[string]interface{}{}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]interface{}, len(cols))
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				m[c] = string(b)
			} else {
				m[c] = vals[i]
			}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func csvList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// inClause builds "<expr> IN (?,?,...)" plus args from a value slice.
func inClause(expr string, vals []string) (string, []interface{}) {
	if len(vals) == 0 {
		return "", nil
	}
	ph := make([]string, len(vals))
	args := make([]interface{}, len(vals))
	for i, v := range vals {
		ph[i] = "?"
		args[i] = v
	}
	return fmt.Sprintf("%s IN (%s)", expr, strings.Join(ph, ",")), args
}

// jsonArrayHasSQL is a predicate: the JSON array at alias.data.field holds an
// element equal (case-insensitively) to one of the bound values. An exact
// element match, not a substring of the array's text.
func jsonArrayHasSQL(alias, field string, n int) string {
	ph := strings.TrimSuffix(strings.Repeat("?,", n), ",")
	return "EXISTS (SELECT 1 FROM json_each(" + alias + ".data,'$." + field + "') je WHERE lower(CAST(je.value AS TEXT)) IN (" + ph + "))"
}

// likeLiteral is a LIKE pattern matching s literally anywhere in a value: the
// LIKE metacharacters % and _ (and the escape character) in s are escaped, so
// they never act as wildcards. Use it with ` ESCAPE '\'`.
func likeLiteral(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(s) + "%"
}

// parseWindowDays reads a look-back window: a positive, finite number with an
// h, d or w unit (a bare number is days). "all" (or "0") means no window and
// returns 0. Anything else is refused, so a typo never widens or replaces the
// window the operator asked for.
func parseWindowDays(flag, s string) (float64, error) {
	t := strings.TrimSpace(strings.ToLower(s))
	if t == "all" || t == "0" {
		return 0, nil
	}
	bad := fmt.Errorf("--%s %q: want a duration like 12h, 7d or 2w, or \"all\"", flag, s)
	mult := 1.0
	switch {
	case strings.HasSuffix(t, "h"):
		mult = 1.0 / 24.0
		t = strings.TrimSuffix(t, "h")
	case strings.HasSuffix(t, "d"):
		t = strings.TrimSuffix(t, "d")
	case strings.HasSuffix(t, "w"):
		mult = 7.0
		t = strings.TrimSuffix(t, "w")
	}
	f, err := strconv.ParseFloat(t, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f <= 0 {
		return 0, bad
	}
	d := f * mult
	if math.IsInf(d, 0) || d <= 0 {
		return 0, bad
	}
	return d, nil
}

// parsePositiveID reads an organization, incident or other API id: a whole
// positive number and nothing else ("12x" is refused, not read as 12).
func parsePositiveID(name, s string) (int64, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s %q: want a positive whole-number id", name, s)
	}
	return n, nil
}

// requireNonNegative refuses a negative count or threshold flag, which would
// otherwise turn "older than N days" into "everything".
func requireNonNegative(flag string, n int) error {
	if n < 0 {
		return usageErr(fmt.Errorf("--%s %d: must be zero or more", flag, n))
	}
	return nil
}

// entityKey is the one identity for an entity id, whatever type carried it:
// a SQLite integer, a JSON float (1e+06 would otherwise differ from 1000000),
// a json.Number, or a string.
func entityKey(v interface{}) string {
	switch n := v.(type) {
	case nil:
		return ""
	case int64:
		return strconv.FormatInt(n, 10)
	case int:
		return strconv.Itoa(n)
	case float64:
		if n == math.Trunc(n) && math.Abs(n) < 1<<53 {
			return strconv.FormatInt(int64(n), 10)
		}
		return strconv.FormatFloat(n, 'f', -1, 64)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return strconv.FormatInt(i, 10)
		}
		return n.String()
	case string:
		return n
	}
	return fmt.Sprintf("%v", v)
}

// ---- indicators: address tokens, not substrings --------------------------------

type indicatorKind int

const (
	indicatorText indicatorKind = iota
	indicatorIP
	indicatorMAC
	indicatorDomain
)

// How a row matched an indicator, strongest first. "" is no match.
const (
	matchExact     = "exact"
	matchSubdomain = "subdomain"
	matchFragment  = "fragment"
)

func matchRank(m string) int {
	switch m {
	case matchExact:
		return 3
	case matchSubdomain:
		return 2
	case matchFragment:
		return 1
	}
	return 0
}

// strongestMatch is the strongest of several field matches.
func strongestMatch(ms ...string) string {
	best := ""
	for _, m := range ms {
		if matchRank(m) > matchRank(best) {
			best = m
		}
	}
	return best
}

// indicatorMatcher matches one blast-radius indicator. The indicator and
// every address token of the stored text are read by ONE parser,
// parseAddressToken, so both sides normalize identically: an IP (IPv4, or
// IPv6 bare or in brackets, with or without a port, a zone or the
// IPv4-mapped prefix) matches the same address in any spelling; a MAC
// matches with any separator; a domain or FQDN matches that name, or a
// subdomain of it, ignoring case, a port and a trailing dot. A label such as
// ip:, host: or mac: before an address is not part of it. None matches a
// longer address that merely contains it. Any other indicator (a hash, a
// word, a single-label hostname) matches as a case-insensitive fragment,
// marked "fragment" when it is found only inside a longer word or inside an
// address.
type indicatorMatcher struct {
	raw    string
	lower  string
	kind   indicatorKind
	ip     net.IP
	mac    net.HardwareAddr
	domain string
}

// newIndicatorMatcher reads the indicator with parseAddressToken, keeping
// only what the whole indicator spells (an address merely held inside it, as
// in a range, does not make it that address). It is an address indicator
// only when it spells exactly one address of the strongest kind found (IP,
// then MAC, then domain); an ambiguous indicator is matched as text.
func newIndicatorMatcher(s string) indicatorMatcher {
	t := strings.TrimSpace(s)
	m := indicatorMatcher{raw: t, lower: strings.ToLower(t)}
	var ips []net.IP
	var macs []net.HardwareAddr
	var domains []string
	for _, f := range parseAddressToken(t) {
		if f.held {
			continue
		}
		switch f.kind {
		case indicatorIP:
			if !slices.ContainsFunc(ips, f.ip.Equal) {
				ips = append(ips, f.ip)
			}
		case indicatorMAC:
			if !slices.ContainsFunc(macs, func(h net.HardwareAddr) bool { return string(h) == string(f.mac) }) {
				macs = append(macs, f.mac)
			}
		case indicatorDomain:
			if !slices.Contains(domains, f.domain) {
				domains = append(domains, f.domain)
			}
		}
	}
	switch {
	case len(ips) > 0:
		if len(ips) == 1 {
			m.kind, m.ip = indicatorIP, ips[0]
		}
	case len(macs) > 0:
		if len(macs) == 1 {
			m.kind, m.mac = indicatorMAC, macs[0]
		}
	case len(domains) == 1:
		m.kind, m.domain = indicatorDomain, domains[0]
	}
	return m
}

// addrForm is one address a token spells or, held, contains as a piece of a
// compound token (a range, a list), with its byte span [start, end) in the
// token.
type addrForm struct {
	kind       indicatorKind
	ip         net.IP
	mac        net.HardwareAddr
	domain     string
	start, end int
	held       bool
}

const (
	allowIP = 1 << iota
	allowMAC
	allowDomain
	allowAnyAddress = allowIP | allowMAC | allowDomain
)

// parseAddressToken is the one address parser blast-radius uses, on the
// --indicator argument and on every address token of stored text. It
// returns every address the token spells, read through a label (ip:, host:,
// mac:, mac-), brackets with or without a port ("[2001:db8::5]:443"), a port
// after an IPv4 address or a name ("203.0.113.5:443"; an IPv6 address carries
// a port only in brackets), an IPv6 zone, and dots at either end; net.ParseIP
// reads the IPv4-mapped forms (::ffff:203.0.113.5, ::ffff:cb00:7105) as the
// IPv4 address. It also returns, marked held, the addresses a compound token
// contains: each IPv4 address between ':', '-', '_' or '%'
// ("10.0.0.1-10.0.0.9") and each name between ':'.
func parseAddressToken(tok string) []addrForm {
	var out []addrForm
	spellAddress(tok, 0, len(tok), allowAnyAddress, 0, &out)
	from := 0
	for i := 0; i <= len(tok); i++ {
		if i < len(tok) && strings.IndexByte(":-_%", tok[i]) < 0 {
			continue
		}
		if s, e := trimSpan(tok, from, i, "."); s < e {
			if ip := net.ParseIP(tok[s:e]); ip != nil && ip.To4() != nil {
				out = append(out, addrForm{kind: indicatorIP, ip: ip, start: s, end: e, held: true})
			}
		}
		from = i + 1
	}
	from = 0
	for i := 0; i <= len(tok); i++ {
		if i < len(tok) && tok[i] != ':' {
			continue
		}
		if s, e := trimSpan(tok, from, i, ".-_"); s < e {
			if d := strings.ToLower(tok[s:e]); isDomainName(d) {
				out = append(out, addrForm{kind: indicatorDomain, domain: d, start: s, end: e, held: true})
			}
		}
		from = i + 1
	}
	return out
}

// spellAddress appends what tok[start:end] spells as a whole, for the kinds
// allow permits, unwrapping one label, brackets or port at a time.
func spellAddress(tok string, start, end, allow, depth int, out *[]addrForm) {
	if start >= end || depth > 4 {
		return
	}
	s := tok[start:end]
	if s[0] == '[' {
		if j := strings.IndexByte(s, ']'); j > 1 && (j == len(s)-1 || isPortSuffix(s[j+1:])) {
			spellAddress(tok, start+1, start+j, allow&^allowMAC, depth+1, out)
		}
		return
	}
	if i := strings.IndexAny(s, ":-"); i > 0 && isAddressLabel(s[:i]) {
		next := allow
		if s[i] == '-' {
			next &= allowMAC // "mac-00-11-...": a hyphenated name keeps its first label
		}
		spellAddress(tok, start+i+1, end, next, depth+1, out)
	}
	if allow&allowIP != 0 {
		as, ae := trimSpan(tok, start, end, ".")
		ip := net.ParseIP(tok[as:ae])
		if z := strings.IndexByte(tok[as:ae], '%'); ip == nil && z > 0 {
			ae = as + z // an IPv6 zone
			ip = net.ParseIP(tok[as:ae])
		}
		if ip != nil {
			*out = append(*out, addrForm{kind: indicatorIP, ip: ip, start: as, end: ae})
		}
	}
	if allow&allowMAC != 0 {
		ms, me := trimSpan(tok, start, end, ".:-_")
		if hw, err := net.ParseMAC(tok[ms:me]); ms < me && err == nil {
			*out = append(*out, addrForm{kind: indicatorMAC, mac: hw, start: ms, end: me})
		}
	}
	if allow&allowDomain != 0 {
		ds, de := trimSpan(tok, start, end, ".-_")
		if d := strings.ToLower(tok[ds:de]); ds < de && isDomainName(d) {
			*out = append(*out, addrForm{kind: indicatorDomain, domain: d, start: ds, end: de})
		}
	}
	if i := strings.LastIndexByte(s, ':'); i > 0 && isPortSuffix(s[i:]) && !strings.Contains(s[:i], ":") {
		spellAddress(tok, start, start+i, allow&(allowIP|allowDomain), depth+1, out)
	}
}

// isPortSuffix reports whether p is ":" followed by a port number.
func isPortSuffix(p string) bool {
	if len(p) < 2 || len(p) > 6 || p[0] != ':' {
		return false
	}
	for i := 1; i < len(p); i++ {
		if p[i] < '0' || p[i] > '9' {
			return false
		}
	}
	return true
}

// isAddressLabel reports whether l can be a label before an address ("ip",
// "host", "mac"): a word that starts with a letter and is not hex, so it is
// never the first group of an IPv6 address or a MAC.
func isAddressLabel(l string) bool {
	if l == "" || len(l) > 32 || isHexLabel(l) {
		return false
	}
	for i, r := range l {
		letter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if (i == 0 && !letter) || !(letter || (r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}
	return true
}

// trimSpan narrows [start, end) of tok past any of cut at either end.
func trimSpan(tok string, start, end int, cut string) (int, int) {
	for start < end && strings.IndexByte(cut, tok[start]) >= 0 {
		start++
	}
	for end > start && strings.IndexByte(cut, tok[end-1]) >= 0 {
		end--
	}
	return start, end
}

// isDomainName: two or more dot-separated labels of letters, digits, '-' and
// '_', the last of which holds a letter (so a version number is not a name).
func isDomainName(d string) bool {
	labels := strings.Split(d, ".")
	if len(labels) < 2 {
		return false
	}
	for _, l := range labels {
		if l == "" || len(l) > 63 || strings.HasPrefix(l, "-") || strings.HasSuffix(l, "-") {
			return false
		}
		for _, r := range l {
			if !(r < 0x80 && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_')) {
				return false
			}
		}
	}
	return strings.IndexFunc(labels[len(labels)-1], unicode.IsLetter) >= 0
}

func isHexLabel(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}

// addressTokens splits text at every character that cannot be part of an
// address or name: brackets, parentheses, slashes, quotes, commas, '@',
// '=' and whitespace all end a token. extra lists further token characters.
func addressTokens(text, extra string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return !(r < 0x80 && (unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(".:-_"+extra, r)))
	})
}

func (m indicatorMatcher) ipInToken(tok string) bool {
	for _, f := range parseAddressToken(tok) {
		if f.kind == indicatorIP && f.ip.Equal(m.ip) {
			return true
		}
	}
	return false
}

func (m indicatorMatcher) macInToken(tok string) bool {
	for _, f := range parseAddressToken(tok) {
		if f.kind == indicatorMAC && string(f.mac) == string(m.mac) {
			return true
		}
	}
	return false
}

func (m indicatorMatcher) domainInToken(tok string) string {
	best := ""
	for _, f := range parseAddressToken(tok) {
		if f.kind != indicatorDomain {
			continue
		}
		switch {
		case f.domain == m.domain:
			return matchExact
		case strings.HasSuffix(f.domain, "."+m.domain):
			best = matchSubdomain
		}
	}
	return best
}

// fragmentIn matches a free-text indicator: exact when an occurrence stands
// as a word of its own ('.' and punctuation end a word; letters, digits, '-'
// and '_' continue it), fragment when every occurrence is inside a longer word.
func (m indicatorMatcher) fragmentIn(text string) string {
	if m.lower == "" {
		return ""
	}
	isWord := func(b byte) bool {
		return b >= 0x80 || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '-' || b == '_'
	}
	needBefore, needAfter := isWord(m.lower[0]), isWord(m.lower[len(m.lower)-1])
	lt := strings.ToLower(text)
	best := ""
	for from := 0; from <= len(lt); {
		i := strings.Index(lt[from:], m.lower)
		if i < 0 {
			break
		}
		start, end := from+i, from+i+len(m.lower)
		before := !needBefore || start == 0 || !isWord(lt[start-1])
		after := !needAfter || end == len(lt) || !isWord(lt[end])
		if before && after && !insideLongerAddress(lt, start, end) {
			return matchExact
		}
		best = matchFragment
		from = start + 1
	}
	return best
}

// insideLongerAddress reports whether lt[start:end] covers part of an IP or
// MAC address without covering all of it: "113.22" in "203.0.113.22" or in
// "ip:203.0.113.22", "113.22:443" in "203.0.113.22:443", or "44:55" in a MAC.
// The addresses are the ones parseAddressToken finds in the surrounding
// token, so a label, a port or a zone is never mistaken for part of one. '.'
// and ':' end a word for names ("host1" is a label of "host1.corp.local"),
// but a part of an address is never the address, so such a match is a
// fragment.
func insideLongerAddress(lt string, start, end int) bool {
	isTok := func(b byte) bool {
		return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '.' || b == ':' || b == '-' || b == '_' || b == '%'
	}
	ts, te := start, end
	for ts > 0 && isTok(lt[ts-1]) {
		ts--
	}
	for te < len(lt) && isTok(lt[te]) {
		te++
	}
	for _, f := range parseAddressToken(lt[ts:te]) {
		if f.kind != indicatorIP && f.kind != indicatorMAC {
			continue
		}
		as, ae := ts+f.start, ts+f.end
		if start < ae && end > as && (start > as || end < ae) {
			return true
		}
	}
	return false
}

// matchText matches the indicator against free text (a summary, a hostname).
func (m indicatorMatcher) matchText(text string) string {
	switch m.kind {
	case indicatorIP:
		for _, tok := range addressTokens(text, "%") {
			if m.ipInToken(tok) {
				return matchExact
			}
		}
		return ""
	case indicatorMAC:
		for _, tok := range addressTokens(text, "") {
			if m.macInToken(tok) {
				return matchExact
			}
		}
		return ""
	case indicatorDomain:
		best := ""
		for _, tok := range addressTokens(text, "") {
			best = strongestMatch(best, m.domainInToken(tok))
		}
		return best
	}
	return m.fragmentIn(text)
}

// matchValue matches a structured single-value field (an IP or MAC field):
// an address indicator by its parsed form; any other is exact only when it is
// the whole value, and found inside a longer value it is a fragment, as it
// is in free text, so a partial address is reported the same way (match:
// fragment) on every row type and is never labelled exact.
func (m indicatorMatcher) matchValue(v string) string {
	if m.kind == indicatorText {
		if strings.EqualFold(strings.TrimSpace(v), m.raw) {
			return matchExact
		}
		if m.fragmentIn(v) != "" {
			return matchFragment
		}
		return ""
	}
	return m.matchText(v)
}

// likePatterns is the SQL LIKE pre-filter: a SUPERSET of the rows the
// matcher can accept, so the prefilter never decides a match. Every spelling
// an address matcher accepts contains one of these (SQLite's LIKE folds ASCII
// case only):
//   - an IPv4 address (or an IPv4-mapped IPv6 one) is accepted written
//     dotted, or as any IPv4-mapped IPv6 spelling, which always holds the
//     group ffff (::ffff:cb00:7107, 0:0:0:0:0:FFFF:CB00:7107);
//   - any other IPv6 address always holds a ':';
//   - a MAC is accepted with ':', '-' or '.' separators, in hex;
//   - a domain is matched in ASCII address tokens, so it appears as written.
//
// Free text is matched after Go's Unicode lowercasing (and EqualFold for a
// structured value), which LIKE cannot reproduce, so its pattern is
// foldSafeLike: every character a non-ASCII one can lowercase or fold to is
// a single-character wildcard.
func (m indicatorMatcher) likePatterns() []string {
	switch m.kind {
	case indicatorIP:
		if v4 := m.ip.To4(); v4 != nil {
			return []string{likeLiteral(v4.String()), "%ffff%"}
		}
		return []string{"%:%"}
	case indicatorMAC:
		colon := m.mac.String()
		hexs := strings.ReplaceAll(colon, ":", "")
		var dotted []string
		for i := 0; i < len(hexs); i += 4 {
			dotted = append(dotted, hexs[i:i+4])
		}
		return []string{likeLiteral(colon), likeLiteral(strings.ReplaceAll(colon, ":", "-")), likeLiteral(strings.Join(dotted, "."))}
	case indicatorDomain:
		return []string{likeLiteral(m.domain)}
	}
	if p, ok := foldSafeLike(m.lower); ok {
		return []string{"%" + p + "%"}
	}
	return []string{"%"}
}

// foldAmbiguousASCII is every ASCII letter some non-ASCII rune lowercases or
// case-folds to (the Kelvin sign to k, the dotted capital I to i, the long s
// to s): a LIKE literal for one of them would miss the non-ASCII spelling the
// Go matcher accepts. Computed once, on first use, from the Unicode tables.
var foldAmbiguousASCII = sync.OnceValue(func() map[rune]bool {
	out := map[rune]bool{}
	for r := rune(0x80); r <= unicode.MaxRune; r++ {
		if l := unicode.ToLower(r); l < 0x80 {
			out[unicode.ToLower(l)] = true
		}
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			if f < 0x80 {
				out[unicode.ToLower(f)] = true
			}
		}
	}
	return out
})

// foldSafeLike is a LIKE pattern (without the surrounding %) that matches
// every text whose Go lowercasing contains lower, and every value EqualFold
// to a string that lowercases to lower: ASCII characters no other rune folds
// to stay literal (LIKE matches their ASCII case variants), everything else
// is '_' (exactly one character), and LIKE's metacharacters are escaped for
// ESCAPE '\'. ok is false when lower holds U+FFFD (an invalid byte), whose
// character count SQLite and Go read differently; the caller then compares
// every row.
func foldSafeLike(lower string) (string, bool) {
	amb := foldAmbiguousASCII()
	var b strings.Builder
	for _, r := range lower {
		switch {
		case r == utf8.RuneError:
			return "", false
		case r >= 0x80 || amb[unicode.ToLower(r)]:
			b.WriteByte('_')
		case r == '%' || r == '_' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String(), true
}

// likeAnySQL is "(col LIKE ? ESCAPE '\' OR ...)" over every column and
// pattern, with its arguments.
func likeAnySQL(cols, patterns []string) (string, []interface{}) {
	var parts []string
	var args []interface{}
	for _, c := range cols {
		for _, p := range patterns {
			parts = append(parts, c+` LIKE ? ESCAPE '\'`)
			args = append(args, p)
		}
	}
	return "(" + strings.Join(parts, " OR ") + ")", args
}

// jsonArrayCandidateSQL is a prefilter predicate for a JSON array the Go side
// reads element by element: an element that is not JSON text (the Go side
// prints a number, boolean or nested value its own way, which SQLite's CAST
// does not reproduce) or a text element matching one of the LIKE patterns.
func jsonArrayCandidateSQL(alias, field string, patterns []string) (string, []interface{}) {
	cond, args := likeAnySQL([]string{"CAST(je.value AS TEXT)"}, patterns)
	return "EXISTS (SELECT 1 FROM json_each(" + alias + ".data,'$." + field + "') je WHERE je.type != 'text' OR " + cond + ")", args
}

// idGreater orders two API ids, numerically when both are numbers.
func idGreater(a, b interface{}) bool {
	fa, oka := toFloat(a)
	fb, okb := toFloat(b)
	if oka && okb {
		return fa > fb
	}
	return fmt.Sprint(a) > fmt.Sprint(b)
}

// seatQuantity reads an invoice's seat quantity: a finite, non-negative whole
// number. Anything else is unreadable, never 0.
func seatQuantity(v interface{}) (int, bool) {
	if v == nil {
		return 0, false
	}
	f, ok := toFloat(v)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 || f != math.Trunc(f) || f >= 1<<53 {
		return 0, false
	}
	return int(f), true
}

func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case []byte:
		f, err := strconv.ParseFloat(strings.TrimSpace(string(n)), 64)
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	}
	return 0, false
}

// Wiring. cli-printing-press 4.32 scaffolds every research.json novel as a
// separate internal/cli/<novel>.go file whose constructor is named
// newNovel<Feature>Cmd and registers it from the generated root.go via
// addNovelCommandIfAbsent. The eleven commands below are implemented in THIS
// file instead of eleven scaffold files (they share the SQLite helpers above),
// so each newNovel* constructor the generated root expects is a one-line shim
// onto the real constructor. That keeps root.go byte-identical to what the
// press emits: a reprint regenerates root.go untouched, deletes the eleven
// scaffold files, and this file supplies the symbols.
func newNovelFleetIncidentsCmd(flags *rootFlags) *cobra.Command { return newFleetIncidentsCmd(flags) }
func newNovelCoverageGapsCmd(flags *rootFlags) *cobra.Command   { return newCoverageGapsCmd(flags) }
func newNovelBlastRadiusCmd(flags *rootFlags) *cobra.Command    { return newBlastRadiusCmd(flags) }
func newNovelTriageAgeCmd(flags *rootFlags) *cobra.Command      { return newTriageAgeCmd(flags) }
func newNovelBillingReconcileCmd(flags *rootFlags) *cobra.Command {
	return newBillingReconcileCmd(flags)
}
func newNovelDriftCmd(flags *rootFlags) *cobra.Command        { return newDriftCmd(flags) }
func newNovelMttrCmd(flags *rootFlags) *cobra.Command         { return newMttrCmd(flags) }
func newNovelCanaryWatchCmd(flags *rootFlags) *cobra.Command  { return newCanaryWatchCmd(flags) }
func newNovelOrgScorecardCmd(flags *rootFlags) *cobra.Command { return newOrgScorecardCmd(flags) }
func newNovelStaleAgentsCmd(flags *rootFlags) *cobra.Command  { return newStaleAgentsCmd(flags) }
func newNovelHandoffCmd(flags *rootFlags) *cobra.Command      { return newHandoffCmd(flags) }

// countRows runs a SELECT COUNT(*) and returns the count.
func countRows(ctx context.Context, db *sql.DB, query string, args ...interface{}) (int, error) {
	var n int
	if err := db.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// warnUnreadable prints a warning naming how many rows had a field the
// command could not read and what that did to the answer.
func warnUnreadable(cmd *cobra.Command, n int, what, consequence string) {
	if n > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d %s; %s\n", n, what, consequence)
	}
}

// unreadableTimeSQL is true for a row whose timestamp field is present but
// cannot be read, or (required) is absent: its age or window is unknown.
func unreadableTimeSQL(alias, field string, required bool) string {
	if required {
		return jdx(alias, field) + " IS NULL"
	}
	return "(COALESCE(" + jx(alias, field) + ", '') != '' AND " + jdx(alias, field) + " IS NULL)"
}

// ---- fleet-incidents --------------------------------------------------------

// pp:data-source local
func newFleetIncidentsCmd(flags *rootFlags) *cobra.Command {
	var severity, status, indicatorType, sortBy, dbPath string
	var org, limit int
	cmd := &cobra.Command{
		Use:         "fleet-incidents",
		Short:       "Unified incident queue across every organization, age-sorted",
		Long:        "Fans incident reports out across all synced organizations, joins organization names,\nand returns one globally sorted queue — the cross-tenant view the per-org API can't return.\nDo NOT use it to enrich one incident with remediations/agent; use 'incident-detail' instead.\nDo NOT use it for the SLA-aging breakdown; use 'triage-age' instead.\nReads the local store; run `sync` first.",
		Example:     "  huntress-cli fleet-incidents --severity critical --status sent --sort age --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("org") && org <= 0 {
				return usageErr(fmt.Errorf("--org %d: want a positive organization id", org))
			}
			if err := requireNonNegative("limit", limit); err != nil {
				return err
			}
			if sortBy != "age" && sortBy != "severity" {
				return usageErr(fmt.Errorf("--sort %q: want age or severity", sortBy))
			}
			if dryRunOK(flags) {
				return nil
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, entIncidents, entOrgs)
			if err != nil {
				return err
			}
			defer st.Close()

			where := []string{"1=1"}
			qargs := []interface{}{}
			if c, a := inClause(jx("i", "severity"), csvList(severity)); c != "" {
				where = append(where, c)
				qargs = append(qargs, a...)
			}
			if c, a := inClause(jx("i", "status"), csvList(status)); c != "" {
				where = append(where, c)
				qargs = append(qargs, a...)
			}
			if its := lowerAll(csvList(indicatorType)); len(its) > 0 {
				where = append(where, jsonArrayHasSQL("i", "indicator_types", len(its)))
				for _, it := range its {
					qargs = append(qargs, it)
				}
			}
			if org > 0 {
				where = append(where, jx("i", "organization_id")+" = ?")
				qargs = append(qargs, org)
			}
			order := jdx("i", "sent_at") + " ASC"
			if sortBy == "severity" {
				order = "CASE " + jx("i", "severity") + " WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'low' THEN 2 ELSE 3 END ASC, " + jdx("i", "sent_at") + " ASC"
			}
			if limit <= 0 {
				limit = 100
			}
			q := fmt.Sprintf(`SELECT %s AS id, %s AS severity, %s AS status, %s AS indicator_types,
				%s AS organization_id, %s AS organization_name, %s AS agent_id, %s AS sent_at,
				CAST((julianday('now') - %s) * 24 AS INTEGER) AS hours_open,
				%s AS remediations_count,
				%s AS subject, %s AS summary
			FROM %s i
			%s
			WHERE %s
			ORDER BY %s
			LIMIT %d`,
				jx("i", "id"), jx("i", "severity"), jx("i", "status"), jx("i", "indicator_types"),
				jx("i", "organization_id"), jx("o", "name"), jx("i", "agent_id"), jx("i", "sent_at"),
				jdx("i", "sent_at"), remediationCountSQL("i"), jx("i", "subject"), jx("i", "summary"),
				st.src(entIncidents), st.orgJoin("i"), strings.Join(where, " AND "), order, limit)

			res, err := queryMaps(ctx, st.DB(), q, qargs...)
			if err != nil {
				return err
			}
			var total int
			if err := st.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM "+st.src(entIncidents)+" i WHERE "+strings.Join(where, " AND "), qargs...).Scan(&total); err != nil {
				return fmt.Errorf("counting incidents: %w", err)
			}
			if total > len(res) {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: showing %d of %d matching incidents; raise --limit to see the rest.\n", len(res), total)
			}
			unknownRem := 0
			for _, r := range res {
				if r["remediations_count"] == nil {
					unknownRem++
				}
			}
			warnUnreadable(cmd, unknownRem, "incident(s) shown have a missing or unreadable remediations list", "their remediations_count is null (unknown), not 0.")
			return flags.printJSON(cmd, res)
		},
	}
	cmd.Flags().StringVar(&severity, "severity", "", "Filter by severity CSV (low,high,critical)")
	cmd.Flags().StringVar(&status, "status", "sent", "Filter by status CSV (sent,closed,dismissed,...); empty for all")
	cmd.Flags().StringVar(&indicatorType, "indicator-type", "", "Filter by indicator type CSV (footholds,ransomware_canaries,...)")
	cmd.Flags().IntVar(&org, "org", 0, "Filter by organization ID")
	cmd.Flags().StringVar(&sortBy, "sort", "age", "Sort order: age (oldest first) or severity")
	cmd.Flags().IntVar(&limit, "limit", 100, "Max rows to return")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}

// ---- coverage-gaps ----------------------------------------------------------

// pp:data-source local
func newCoverageGapsCmd(flags *rootFlags) *cobra.Command {
	var staleDays int
	var dbPath string
	cmd := &cobra.Command{
		Use:         "coverage-gaps",
		Short:       "Per-org posture exposure: stale callbacks and unhealthy agents",
		Long:        "Rolls up agent health across every organization: total agents and how many have a\nstale callback, or Defender or firewall not reported healthy, ordered worst-first.\nDo NOT use it for the one-line fleet glance; use 'fleet-summary' instead.\nReads the local store; run `sync` first.",
		Example:     "  huntress-cli coverage-gaps --stale-days 7 --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNonNegative("stale-days", staleDays); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, entOrgs, entAgents)
			if err != nil {
				return err
			}
			defer st.Close()

			healthy := "('enabled','active','running','healthy','on')"
			// stale_unknown counts agents whose last_callback_at is missing or
			// unreadable: whether they are stale is unknown, not "no".
			q := fmt.Sprintf(`SELECT %s AS organization_id, %s AS organization_name,
				COUNT(a.id) AS agent_count,
				SUM(CASE WHEN %s IS NOT NULL AND %s != ''
					AND julianday('now') - %s > ? THEN 1 ELSE 0 END) AS stale_agents,
				SUM(CASE WHEN a.id IS NOT NULL AND %s THEN 1 ELSE 0 END) AS stale_unknown,
				SUM(CASE WHEN %s IS NOT NULL AND %s != ''
					AND lower(%s) NOT IN %s THEN 1 ELSE 0 END) AS defender_flagged,
				SUM(CASE WHEN %s IS NOT NULL AND %s != ''
					AND lower(%s) NOT IN %s THEN 1 ELSE 0 END) AS firewall_flagged
			FROM %s o
			LEFT JOIN %s a ON %s = %s
			GROUP BY %s, %s
			HAVING stale_agents > 0 OR stale_unknown > 0 OR defender_flagged > 0 OR firewall_flagged > 0
			ORDER BY (stale_agents + stale_unknown + defender_flagged + firewall_flagged) DESC, agent_count DESC`,
				jx("o", "id"), jx("o", "name"),
				jx("a", "last_callback_at"), jx("a", "last_callback_at"), jdx("a", "last_callback_at"),
				unreadableTimeSQL("a", "last_callback_at", true),
				jx("a", "defender_status"), jx("a", "defender_status"), jx("a", "defender_status"), healthy,
				jx("a", "firewall_status"), jx("a", "firewall_status"), jx("a", "firewall_status"), healthy,
				st.src(entOrgs), st.src(entAgents), jx("a", "organization_id"), jx("o", "id"),
				jx("o", "id"), jx("o", "name"))

			res, err := queryMaps(ctx, st.DB(), q, staleDays)
			if err != nil {
				return err
			}
			unknown := 0
			for _, r := range res {
				if n, ok := toFloat(r["stale_unknown"]); ok {
					unknown += int(n)
				}
			}
			warnUnreadable(cmd, unknown, "agent(s) have a missing or unreadable last_callback_at", "whether they are stale is unknown; they are counted under stale_unknown, not stale_agents.")
			return flags.printJSON(cmd, res)
		},
	}
	cmd.Flags().IntVar(&staleDays, "stale-days", 7, "Agents with no callback in this many days count as stale")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}

// ---- blast-radius -----------------------------------------------------------

// blastRadiusCap is how many rows of each collection blast-radius prints; the
// totals always count every match.
const blastRadiusCap = 200

// pp:data-source local
func newBlastRadiusCmd(flags *rootFlags) *cobra.Command {
	var indicator, dbPath string
	cmd := &cobra.Command{
		Use:   "blast-radius",
		Short: "Correlate an indicator (IP, hash, host) across the whole fleet",
		Long: "Given an indicator, searches incident reports, agents, and external ports across every\n" +
			"organization for matches: the cross-entity correlation the API offers no single query for.\n" +
			"An address is matched as an address: an IP matches that address in any spelling, with or\n" +
			"without a port or brackets; a MAC matches with any separator; a domain or FQDN matches that\n" +
			"name or its subdomains. None matches a longer address that merely contains it. Any other\n" +
			"text matches hostnames, services, addresses and incident text; found inside a longer word or\n" +
			"value, or as part of an address, it is a fragment, never exact. Every row says how it\n" +
			"matched (match: exact, subdomain or fragment); fragment_matches counts the rows where the\n" +
			"indicator was found only inside a longer word.\n" +
			"Reads the local store; run `sync` first.",
		Example:     "  huntress-cli blast-radius --indicator 203.0.113.7 --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if indicator == "" && len(args) > 0 {
				indicator = args[0]
			}
			indicator = strings.TrimSpace(indicator)
			if indicator == "" {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, entIncidents, entAgents, entPorts, entOrgs)
			if err != nil {
				return err
			}
			defer st.Close()
			m := newIndicatorMatcher(indicator)
			pats := m.likePatterns()
			exact := strings.ToLower(indicator)

			incLike, incArgs := likeAnySQL([]string{jx("i", "summary"), jx("i", "body"), jx("i", "subject")}, pats)
			// indicator_types is matched in Go by EqualFold on each element,
			// so its prefilter is the fold-safe whole-value pattern.
			typePats := []string{"%"}
			if p, ok := foldSafeLike(exact); ok {
				typePats = []string{p}
			}
			typeCand, typeArgs := jsonArrayCandidateSQL("i", "indicator_types", typePats)
			incArgs = append(incArgs, typeArgs...)
			incQ := fmt.Sprintf(`SELECT %s AS id, %s AS organization_id, %s AS organization_name,
				%s AS severity, %s AS status, %s AS indicator_types, %s AS sent_at, %s AS subject, %s AS summary,
				%s AS pp_body
			FROM %s i %s
			WHERE (%s OR %s)
			ORDER BY %s DESC`,
				jx("i", "id"), jx("i", "organization_id"), jx("o", "name"),
				jx("i", "severity"), jx("i", "status"), jx("i", "indicator_types"), jx("i", "sent_at"), jx("i", "subject"), jx("i", "summary"),
				jx("i", "body"),
				st.src(entIncidents), st.orgJoin("i"),
				incLike, typeCand,
				jdx("i", "sent_at"))
			incidents, err := queryMaps(ctx, st.DB(), incQ, incArgs...)
			if err != nil {
				return err
			}
			incidents, incFrag := keepMatching(incidents, func(r map[string]interface{}) string {
				typ := ""
				if jsonArrayHas(r["indicator_types"], exact) {
					typ = matchExact
				}
				return strongestMatch(typ, textField(m, r, "summary"), textField(m, r, "pp_body"), textField(m, r, "subject"))
			})
			for _, r := range incidents {
				delete(r, "pp_body")
			}

			agLike, agArgs := likeAnySQL([]string{jx("a", "hostname"), jx("a", "external_ip"), jx("a", "ipv4_address")}, pats)
			macLike, macArgs := jsonArrayCandidateSQL("a", "mac_addresses", pats)
			agQ := fmt.Sprintf(`SELECT %s AS id, %s AS hostname, %s AS organization_id, %s AS organization_name,
				%s AS platform, %s AS external_ip, %s AS ipv4_address, %s AS last_callback_at, %s AS mac_addresses
			FROM %s a %s
			WHERE (%s OR %s)
			ORDER BY %s DESC`,
				jx("a", "id"), jx("a", "hostname"), jx("a", "organization_id"), jx("o", "name"),
				jx("a", "platform"), jx("a", "external_ip"), jx("a", "ipv4_address"), jx("a", "last_callback_at"), jx("a", "mac_addresses"),
				st.src(entAgents), st.orgJoin("a"),
				agLike, macLike,
				jdx("a", "last_callback_at"))
			agents, err := queryMaps(ctx, st.DB(), agQ, append(agArgs, macArgs...)...)
			if err != nil {
				return err
			}
			agents, agFrag := keepMatching(agents, func(r map[string]interface{}) string {
				mac := ""
				if s, ok := r["mac_addresses"].(string); ok {
					if arr, err := decodeJSONList([]byte(s)); err == nil {
						for _, v := range arr {
							mac = strongestMatch(mac, m.matchValue(fmt.Sprint(v)))
						}
					}
				}
				return strongestMatch(textField(m, r, "hostname"), valueField(m, r, "external_ip"), valueField(m, r, "ipv4_address"), mac)
			})
			for _, r := range agents {
				delete(r, "mac_addresses")
			}

			portLike, portArgs := likeAnySQL([]string{jx("p", "ip_address"), jx("p", "service")}, pats)
			portQ := fmt.Sprintf(`SELECT %s AS id, %s AS ip_address, %s AS port, %s AS protocol,
				%s AS service, %s AS risky_service, %s AS last_external_scan_at, %s AS organization_ids
			FROM %s p
			WHERE %s
			ORDER BY %s DESC`,
				jx("p", "id"), jx("p", "ip_address"), jx("p", "port"), jx("p", "protocol"),
				jx("p", "service"), jx("p", "risky_service"), jx("p", "last_external_scan_at"), jx("p", "organization_ids"),
				st.src(entPorts), portLike,
				jx("p", "last_external_scan_at"))
			ports, err := queryMaps(ctx, st.DB(), portQ, portArgs...)
			if err != nil {
				return err
			}
			ports, portFrag := keepMatching(ports, func(r map[string]interface{}) string {
				return strongestMatch(valueField(m, r, "ip_address"), textField(m, r, "service"))
			})

			totals := map[string]int{"incident_reports": len(incidents), "agents": len(agents), "external_ports": len(ports)}
			capRows := func(rows []map[string]interface{}) []map[string]interface{} {
				if len(rows) > blastRadiusCap {
					return rows[:blastRadiusCap]
				}
				return rows
			}
			truncated := false
			for name, n := range totals {
				if n > blastRadiusCap {
					truncated = true
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: blast-radius matched %d %s; showing the first %d (the totals count all of them).\n", n, strings.ReplaceAll(name, "_", " "), blastRadiusCap)
				}
			}
			fragments := incFrag + agFrag + portFrag
			if fragments > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: blast-radius: %d match(es) contain %q only inside a longer word or name (match: fragment); check them before acting, or pass a full address or name.\n", fragments, indicator)
			}
			result := map[string]interface{}{
				"indicator":              indicator,
				"incident_reports":       capRows(incidents),
				"agents":                 capRows(agents),
				"external_ports":         capRows(ports),
				"incident_reports_total": totals["incident_reports"],
				"agents_total":           totals["agents"],
				"external_ports_total":   totals["external_ports"],
				"match_count":            totals["incident_reports"] + totals["agents"] + totals["external_ports"],
				"fragment_matches":       fragments,
				"truncated":              truncated,
			}
			return printJSONFilteredKeep(cmd.OutOrStdout(), result, flags, "match_count", "fragment_matches", "truncated", "incident_reports_total", "agents_total", "external_ports_total")
		},
	}
	cmd.Flags().StringVar(&indicator, "indicator", "", "Indicator to correlate: IP, MAC, domain or hostname, hash, or text fragment")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}

// textField matches the indicator against a free-text column of a row.
func textField(m indicatorMatcher, r map[string]interface{}, col string) string {
	if s, ok := r[col].(string); ok {
		return m.matchText(s)
	}
	return ""
}

// valueField matches the indicator against a structured single-value column.
func valueField(m indicatorMatcher, r map[string]interface{}, col string) string {
	if s, ok := r[col].(string); ok {
		return m.matchValue(s)
	}
	return ""
}

// keepMatching keeps the rows the matcher accepts, records how each matched
// under "match", and counts the rows that matched only as a fragment.
func keepMatching(rows []map[string]interface{}, match func(map[string]interface{}) string) ([]map[string]interface{}, int) {
	out := make([]map[string]interface{}, 0, len(rows))
	fragments := 0
	for _, r := range rows {
		how := match(r)
		if how == "" {
			continue
		}
		if how == matchFragment {
			fragments++
		}
		r["match"] = how
		out = append(out, r)
	}
	return out, fragments
}

// jsonArrayHas reports whether a JSON array (as stored text) holds an element
// equal to want, case-insensitively. A value that is not a readable list
// holds nothing.
func jsonArrayHas(raw interface{}, want string) bool {
	s, ok := raw.(string)
	if !ok {
		return false
	}
	arr, err := decodeJSONList([]byte(s))
	if err != nil {
		return false
	}
	for _, v := range arr {
		if strings.EqualFold(fmt.Sprint(v), want) {
			return true
		}
	}
	return false
}

// ---- triage-age -------------------------------------------------------------

// parseBuckets reads the --buckets thresholds: positive, finite hours, sorted
// ascending. An entry it cannot read is refused rather than dropped.
func parseBuckets(s string) ([]float64, error) {
	var out []float64
	seen := map[float64]bool{}
	for _, b := range csvList(s) {
		f, err := strconv.ParseFloat(b, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f <= 0 {
			return nil, fmt.Errorf("--buckets %q: %q is not a positive number of hours", s, b)
		}
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--buckets %q: want hour thresholds like 4,24,72", s)
	}
	sort.Float64s(out)
	return out, nil
}

// pp:data-source local
func newTriageAgeCmd(flags *rootFlags) *cobra.Command {
	var buckets, status, dbPath string
	cmd := &cobra.Command{
		Use:         "triage-age",
		Short:       "SLA aging of open incidents, bucketed by hours-open",
		Long:        "Buckets open incidents across all organizations by how many hours they've been open and\nbreaks them out by severity, surfacing SLA breaches. The API has no aging concept.\nReads the local store; run `sync` first.",
		Example:     "  huntress-cli triage-age --buckets 4,24,72 --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			thresholds, err := parseBuckets(buckets)
			if err != nil {
				return usageErr(err)
			}
			if dryRunOK(flags) {
				return nil
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, entIncidents)
			if err != nil {
				return err
			}
			defer st.Close()

			// An incident whose sent_at is missing or unreadable has an
			// unknown age: it goes to an "unknown age" bucket, never to the
			// youngest one (a NULL age read as 0 hours).
			where := []string{"1=1"}
			qargs := []interface{}{}
			if c, a := inClause(jx("i", "status"), csvList(status)); c != "" {
				where = append(where, c)
				qargs = append(qargs, a...)
			}
			q := fmt.Sprintf(`SELECT %s AS id, %s AS severity,
				(julianday('now') - %s) * 24 AS hours_open
			FROM %s i WHERE %s`, jx("i", "id"), jx("i", "severity"), jdx("i", "sent_at"), st.src(entIncidents), strings.Join(where, " AND "))
			rows, err := queryMaps(ctx, st.DB(), q, qargs...)
			if err != nil {
				return err
			}
			type bucket struct {
				Label    string         `json:"bucket"`
				Total    int            `json:"total"`
				BySev    map[string]int `json:"by_severity"`
				Breached interface{}    `json:"sla_breached"` // bool, or null when unknown
			}
			labels := make([]string, 0, len(thresholds)+1)
			for i := range thresholds {
				if i == 0 {
					labels = append(labels, fmt.Sprintf("<%gh", thresholds[0]))
				} else {
					labels = append(labels, fmt.Sprintf("%g-%gh", thresholds[i-1], thresholds[i]))
				}
			}
			labels = append(labels, fmt.Sprintf(">%gh", thresholds[len(thresholds)-1]))
			out := make([]*bucket, len(labels))
			for i := range out {
				out[i] = &bucket{Label: labels[i], BySev: map[string]int{}, Breached: false}
			}
			unknownAge := &bucket{Label: "unknown age", BySev: map[string]int{}, Breached: nil}
			for _, r := range rows {
				target := unknownAge
				if h, ok := toFloat(r["hours_open"]); ok && r["hours_open"] != nil {
					idx := len(thresholds)
					for i, t := range thresholds {
						if h < t {
							idx = i
							break
						}
					}
					target = out[idx]
				}
				target.Total++
				sev, _ := r["severity"].(string)
				if sev == "" {
					sev = "unknown"
				}
				target.BySev[sev]++
			}
			// SLA breach is a fact about incidents, not bucket scaffolding:
			// only flag the oldest bucket when something actually aged into it,
			// and say "unknown" (null) when an incident of unknown age might have.
			oldest := out[len(out)-1]
			switch {
			case oldest.Total > 0:
				oldest.Breached = true
			case unknownAge.Total > 0:
				oldest.Breached = nil
			}
			if unknownAge.Total > 0 {
				out = append(out, unknownAge)
				warnUnreadable(cmd, unknownAge.Total, "incident(s) have a missing or unreadable sent_at", "they are counted in the \"unknown age\" bucket, and sla_breached is null unless an incident of known age breached.")
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().StringVar(&buckets, "buckets", "4,24,72", "Hour thresholds CSV defining aging buckets")
	cmd.Flags().StringVar(&status, "status", "sent", "Incident status to age (default open=sent); empty for all")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}

// ---- billing-reconcile ------------------------------------------------------

// pp:data-source local
func newBillingReconcileCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "billing-reconcile",
		Short: "Invoiced seats vs deployed agents for the account",
		Long: "Compares the seat quantity on the latest invoice of each account (one billing period, not the\n" +
			"invoice history) against the count of agents actually present in the local mirror, flagging\n" +
			"the delta. Joins billing and fleet data the API never correlates. The invoices it used are\n" +
			"listed under billing_period_invoices. An invoice whose account cannot be established is\n" +
			"listed under unattributed_invoices, and invoiced_seats and delta are then null, as they are\n" +
			"when an account's latest invoice cannot be told (an unreadable created_at among several\n" +
			"invoices) or its quantity cannot be read.\n" +
			"Do NOT use it for a reseller's per-account roll-up; use 'reseller-rollup' instead. Run `sync` first.",
		Example:     "  huntress-cli billing-reconcile --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, entInvoices, entAgents)
			if err != nil {
				return err
			}
			defer st.Close()

			// The latest invoice per account is the current billing period.
			// An invoice's account is the identity merged across every alias
			// copy BEFORE any grouping: a flat /v1/invoices copy names no
			// account while its reseller partition copy carries parent_id, and
			// grouping by the freshest copy alone split one account in two and
			// counted its seats twice. In a store where no invoice names an
			// account (a standard credential), every invoice is the
			// credential's own account. Otherwise an invoice no copy attributes,
			// or whose copies disagree, is reported as unattributed and the
			// totals are withheld: it is never merged into a guessed account.
			invs, err := queryMaps(ctx, st.DB(), fmt.Sprintf(`SELECT v.pp_eid AS pp_eid, %s AS invoice_id, %s AS created_at, %s AS pp_created, %s AS quantity, %s AS status FROM %s v`,
				jx("v", "id"), jx("v", "created_at"), jdx("v", "created_at"), jx("v", "quantity"), jx("v", "status"), st.src(entInvoices)))
			if err != nil {
				return err
			}
			view := st.view(entInvoices)
			groups := map[string][]map[string]interface{}{}
			unattributed := []map[string]interface{}{}
			for _, r := range invs {
				id := view.ident[fmt.Sprint(r["pp_eid"])]
				reason := ""
				switch {
				case id.conflict:
					reason = "its copies name different accounts"
				case id.account == "" && view.mixed:
					reason = "no copy names its account"
				}
				if reason != "" {
					unattributed = append(unattributed, map[string]interface{}{
						"invoice_id": r["invoice_id"], "created_at": r["created_at"], "quantity": r["quantity"], "status": r["status"], "reason": reason,
					})
					continue
				}
				groups[id.account] = append(groups[id.account], r)
			}
			accounts := make([]string, 0, len(groups))
			for a := range groups {
				accounts = append(accounts, a)
			}
			sort.Strings(accounts)
			var unreadable []string
			latest := []map[string]interface{}{}
			invoiced, totalKnown := 0, len(unattributed) == 0
			for _, a := range accounts {
				var pick map[string]interface{}
				periodKnown := true
				if len(groups[a]) == 1 {
					// One invoice is the latest whatever its created_at says.
					pick = groups[a][0]
				}
				for _, r := range groups[a] {
					if len(groups[a]) == 1 {
						break
					}
					jd, ok := toFloat(r["pp_created"])
					if !ok {
						periodKnown = false
						unreadable = append(unreadable, fmt.Sprintf("invoice %v has no readable created_at, so which invoice is the latest billing period of its account is unknown", r["invoice_id"]))
						continue
					}
					if pick == nil {
						pick = r
						continue
					}
					pj, _ := toFloat(pick["pp_created"])
					if jd > pj || (jd == pj && idGreater(r["invoice_id"], pick["invoice_id"])) {
						pick = r
					}
				}
				if !periodKnown || pick == nil {
					totalKnown = false
					continue
				}
				if q, ok := seatQuantity(pick["quantity"]); ok {
					invoiced += q
				} else {
					totalKnown = false
					unreadable = append(unreadable, fmt.Sprintf("invoice %v has no readable quantity", pick["invoice_id"]))
				}
				out := map[string]interface{}{"invoice_id": pick["invoice_id"], "created_at": pick["created_at"], "quantity": pick["quantity"], "status": pick["status"]}
				if a != "" {
					out["account"] = a
				}
				latest = append(latest, out)
			}
			var deployed int
			if err := st.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM "+st.src(entAgents)+" a").Scan(&deployed); err != nil {
				return fmt.Errorf("counting agents: %w", err)
			}
			row := map[string]interface{}{
				"invoiced_seats":          nil,
				"deployed_agents":         deployed,
				"delta":                   nil,
				"billing_period_invoices": latest,
				"unattributed_invoices":   unattributed,
			}
			if totalKnown {
				row["invoiced_seats"] = invoiced
				row["delta"] = invoiced - deployed
			}
			if len(unattributed) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d invoice(s) are unattributed (no account can be established for them); invoiced_seats and delta are withheld rather than totalled under a guessed account. They are listed under unattributed_invoices.\n", len(unattributed))
			}
			for _, u := range unreadable {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s; invoiced_seats and delta are withheld.\n", u)
			}
			if len(invs) == 0 {
				row["note"] = "no invoices in the local store; run `sync` first"
			}
			return printJSONFilteredKeep(cmd.OutOrStdout(), row, flags, "invoiced_seats", "deployed_agents", "delta", "billing_period_invoices", "unattributed_invoices")
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}

// ---- drift (file-snapshot history) ------------------------------------------

// driftSnapshotPath is the snapshot file for one database: the snapshot is
// named after the database's absolute path, so two stores in one directory
// (or the per-credential default stores) never share a baseline.
func driftSnapshotPath(dbPath, entity string) (string, error) {
	abs, err := filepath.Abs(resolveDB(dbPath))
	if err != nil {
		return "", fmt.Errorf("resolving the database path: %w", err)
	}
	sum := sha256.Sum256([]byte(abs))
	return filepath.Join(filepath.Dir(abs), "snapshots", "drift-"+entity+"-"+hex.EncodeToString(sum[:8])+".json"), nil
}

// pp:data-source local
func newDriftCmd(flags *rootFlags) *cobra.Command {
	var entity, dbPath string
	cmd := &cobra.Command{
		Use:         "drift",
		Short:       "Diff the current fleet against the prior drift snapshot",
		Long:        "Captures a snapshot of agents (id, org, last callback) each run and diffs it against the\nprevious one: added, removed, and went-quiet agents. Real history the point-in-time API lacks.\nEach database keeps its own snapshot. Reads the local store; run `sync` first.",
		Example:     "  huntress-cli drift --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if entity != "agents" {
				return usageErr(fmt.Errorf("unsupported --entity %q (only 'agents' is supported)", entity))
			}
			if dryRunOK(flags) {
				return nil
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, entAgents)
			if err != nil {
				return err
			}
			defer st.Close()

			q := fmt.Sprintf(`SELECT %s AS id, %s AS organization_id, %s AS hostname, %s AS last_callback_at
				FROM %s a`,
				jx("a", "id"), jx("a", "organization_id"), jx("a", "hostname"), jx("a", "last_callback_at"), st.src(entAgents))
			cur, err := queryMaps(ctx, st.DB(), q)
			if err != nil {
				return err
			}
			curByID := map[string]map[string]interface{}{}
			for _, r := range cur {
				curByID[entityKey(r["id"])] = r
			}

			snapPath, err := driftSnapshotPath(dbPath, entity)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(snapPath), 0o700); err != nil {
				return fmt.Errorf("creating snapshot dir: %w", err)
			}

			prevByID := map[string]map[string]interface{}{}
			// snapPath is derived from the resolved database path plus a fixed
			// name; --db is refused over MCP, so it is not tool-argument tainted.
			// A snapshot that exists but cannot be read is an error, not "no
			// prior snapshot": diffing against nothing would report every
			// agent as added, and overwriting it would lose the baseline.
			b, e := os.ReadFile(snapPath) // #nosec G304 -- path is app-internal, not user-tainted
			hadPrior := e == nil
			switch {
			case e == nil:
				list, err := decodeJSONList(b)
				var prev []map[string]interface{}
				if err == nil {
					prev, err = objectItems(list, "snapshot")
				}
				if err != nil {
					return fmt.Errorf("reading the prior drift snapshot %s: %w; move it aside to start a new baseline", snapPath, err)
				}
				for _, r := range prev {
					prevByID[entityKey(r["id"])] = r
				}
			case !os.IsNotExist(e):
				return fmt.Errorf("reading the prior drift snapshot %s: %w", snapPath, e)
			}

			added := []map[string]interface{}{}
			removed := []map[string]interface{}{}
			wentQuiet := []map[string]interface{}{}
			for id, r := range curByID {
				if _, ok := prevByID[id]; !ok {
					added = append(added, r)
				}
			}
			for id, pr := range prevByID {
				cr, ok := curByID[id]
				if !ok {
					removed = append(removed, pr)
					continue
				}
				pc, _ := pr["last_callback_at"].(string)
				cc, _ := cr["last_callback_at"].(string)
				if pc != "" && cc == pc {
					wentQuiet = append(wentQuiet, cr)
				}
			}

			if b, e := json.Marshal(cur); e == nil {
				if werr := os.WriteFile(snapPath, b, 0o600); werr != nil {
					return fmt.Errorf("writing drift snapshot: %w", werr)
				}
			}

			result := map[string]interface{}{
				"entity":           entity,
				"had_prior":        hadPrior,
				"current_count":    len(curByID),
				"added":            added,
				"removed":          removed,
				"went_quiet":       wentQuiet,
				"added_count":      len(added),
				"removed_count":    len(removed),
				"went_quiet_count": len(wentQuiet),
			}
			return flags.printJSON(cmd, result)
		},
	}
	cmd.Flags().StringVar(&entity, "entity", "agents", "Entity to track (only 'agents' supported)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}

// ---- mttr -------------------------------------------------------------------

// pp:data-source local
func newMttrCmd(flags *rootFlags) *cobra.Command {
	var groupBy, since, dbPath string
	cmd := &cobra.Command{
		Use:         "mttr",
		Short:       "Mean time-to-resolve for closed incidents",
		Long:        "Computes mean hours from sent to closed for resolved incidents, grouped by org or severity.\nThe API exposes the timestamps but never the metric. Reads the local store; run `sync` first.",
		Example:     "  huntress-cli mttr --group-by org --since 30d --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if groupBy != "org" && groupBy != "severity" {
				return usageErr(fmt.Errorf("--group-by %q: want org or severity", groupBy))
			}
			var days float64
			if strings.TrimSpace(since) != "" {
				d, err := parseWindowDays("since", since)
				if err != nil {
					return usageErr(err)
				}
				days = d
			}
			if dryRunOK(flags) {
				return nil
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, entIncidents, entOrgs)
			if err != nil {
				return err
			}
			defer st.Close()

			var groupExpr, groupName, joinOrg, groupKey string
			if groupBy == "severity" {
				groupExpr = jx("i", "severity")
				groupName = ""
				joinOrg = ""
				groupKey = "severity"
			} else {
				groupExpr = jx("i", "organization_id")
				groupName = ", MAX(" + jx("o", "name") + ") AS organization_name"
				joinOrg = st.orgJoin("i")
				groupKey = "organization_id"
			}
			// A resolved incident is one with a closed_at. Its duration needs a
			// readable sent_at and closed_at; when either cannot be read (or a
			// window cannot place it), the group's mean would silently cover
			// fewer incidents than it claims, so mttr_hours is withheld (null)
			// and the incident is counted under unreadable_timestamps.
			unreadable := "(" + jdx("i", "closed_at") + " IS NULL OR " + jdx("i", "sent_at") + " IS NULL)"
			where := []string{jx("i", "closed_at") + " IS NOT NULL", jx("i", "closed_at") + " != ''"}
			qargs := []interface{}{}
			if days > 0 {
				where = append(where, "("+unreadable+" OR julianday('now') - "+jdx("i", "closed_at")+" <= ?)")
				qargs = append(qargs, days)
			}
			q := fmt.Sprintf(`SELECT %[1]s AS %[2]s%[3]s,
				SUM(CASE WHEN %[4]s THEN 0 ELSE 1 END) AS resolved_count,
				SUM(CASE WHEN %[4]s THEN 1 ELSE 0 END) AS unreadable_timestamps,
				CASE WHEN SUM(CASE WHEN %[4]s THEN 1 ELSE 0 END) > 0 THEN NULL
					ELSE ROUND(AVG((%[5]s - %[6]s) * 24), 2) END AS mttr_hours
			FROM %[7]s i %[8]s
			WHERE %[9]s
			GROUP BY %[1]s
			ORDER BY mttr_hours DESC`,
				groupExpr, groupKey, groupName, unreadable,
				jdx("i", "closed_at"), jdx("i", "sent_at"),
				st.src(entIncidents), joinOrg, strings.Join(where, " AND "))

			res, err := queryMaps(ctx, st.DB(), q, qargs...)
			if err != nil {
				return err
			}
			total := 0
			for _, r := range res {
				if n, ok := toFloat(r["unreadable_timestamps"]); ok {
					total += int(n)
				}
			}
			warnUnreadable(cmd, total, "resolved incident(s) have an unreadable sent_at or closed_at", "mttr_hours is withheld (null) for their group; they are counted under unreadable_timestamps.")
			return flags.printJSON(cmd, res)
		},
	}
	cmd.Flags().StringVar(&groupBy, "group-by", "org", "Group by 'org' or 'severity'")
	cmd.Flags().StringVar(&since, "since", "", "Only incidents closed within this window, e.g. 30d, 12h (default: all time)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}

// ---- canary-watch -----------------------------------------------------------

// pp:data-source local
func newCanaryWatchCmd(flags *rootFlags) *cobra.Command {
	var window, dbPath string
	cmd := &cobra.Command{
		Use:         "canary-watch",
		Short:       "Ransomware-canary and foothold incidents in a time window",
		Long:        "Surfaces only the highest-signal early-ransomware indicators (ransomware canaries and\nfootholds) across the fleet within a window — a curated view the API can't assemble in one call.\nReads the local store; run `sync` first.",
		Example:     "  huntress-cli canary-watch --window 24h --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			days, err := parseWindowDays("window", window)
			if err != nil {
				return usageErr(err)
			}
			if dryRunOK(flags) {
				return nil
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, entIncidents, entOrgs)
			if err != nil {
				return err
			}
			defer st.Close()

			where := []string{jsonArrayHasSQL("i", "indicator_types", 2)}
			qargs := []interface{}{"ransomware_canaries", "footholds"}
			if days > 0 {
				// A canary whose sent_at cannot be read cannot be placed in the
				// window: it is not listed, and the warning below says so.
				unreadable, err := countRows(ctx, st.DB(), "SELECT COUNT(*) FROM "+st.src(entIncidents)+" i WHERE "+where[0]+" AND "+unreadableTimeSQL("i", "sent_at", true), qargs...)
				if err != nil {
					return fmt.Errorf("checking incident timestamps: %w", err)
				}
				warnUnreadable(cmd, unreadable, "ransomware-canary or foothold incident(s) have a missing or unreadable sent_at", "whether they fall in the window is unknown, so they are not listed; run with --window all to see them.")
				where = append(where, jx("i", "sent_at")+" IS NOT NULL AND julianday('now') - "+jdx("i", "sent_at")+" <= ?")
				qargs = append(qargs, days)
			}
			q := fmt.Sprintf(`SELECT %s AS id, %s AS organization_id, %s AS organization_name,
				%s AS severity, %s AS status, %s AS indicator_types, %s AS sent_at, %s AS subject, %s AS summary
			FROM %s i %s
			WHERE %s
			ORDER BY %s DESC`,
				jx("i", "id"), jx("i", "organization_id"), jx("o", "name"),
				jx("i", "severity"), jx("i", "status"), jx("i", "indicator_types"), jx("i", "sent_at"), jx("i", "subject"), jx("i", "summary"),
				st.src(entIncidents), st.orgJoin("i"), strings.Join(where, " AND "), jdx("i", "sent_at"))

			res, err := queryMaps(ctx, st.DB(), q, qargs...)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, res)
		},
	}
	cmd.Flags().StringVar(&window, "window", "24h", "Look-back window, e.g. 24h, 7d, or all")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}

// ---- org-scorecard ----------------------------------------------------------

// idFromFlagOrArg resolves a required id given as --<flag> or the first
// positional argument. given is false when neither was supplied.
func idFromFlagOrArg(cmd *cobra.Command, flag string, flagVal int, args []string) (id int64, given bool, err error) {
	if cmd.Flags().Changed(flag) {
		if flagVal <= 0 {
			return 0, true, fmt.Errorf("--%s %d: want a positive whole-number id", flag, flagVal)
		}
		return int64(flagVal), true, nil
	}
	if len(args) > 0 {
		id, err := parsePositiveID(flag, args[0])
		return id, true, err
	}
	return 0, false, nil
}

// pp:data-source local
func newOrgScorecardCmd(flags *rootFlags) *cobra.Command {
	var org int
	var dbPath string
	cmd := &cobra.Command{
		Use:         "org-scorecard",
		Short:       "Per-client QBR rollup: agents, incidents, MTTR",
		Long:        "Assembles one organization's security story — agent count, open/closed incident counts,\nand mean time-to-resolve — into a single rollup the API returns nowhere pre-aggregated.\nDo NOT use it for the whole-fleet top-line; use 'fleet-summary' instead.\nReads the local store; run `sync` first.",
		Example:     "  huntress-cli org-scorecard --org 4821 --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			orgID, given, err := idFromFlagOrArg(cmd, "org", org, args)
			if err != nil {
				return usageErr(err)
			}
			if !given {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, entOrgs, entAgents, entIncidents)
			if err != nil {
				return err
			}
			defer st.Close()

			card := map[string]interface{}{"organization_id": orgID}
			rows, err := queryMaps(ctx, st.DB(),
				fmt.Sprintf(`SELECT %s AS name FROM %s o WHERE %s = ?`, jx("o", "name"), st.src(entOrgs), jx("o", "id")), orgID)
			if err != nil {
				return fmt.Errorf("reading the organization: %w", err)
			}
			if len(rows) > 0 {
				card["organization_name"] = rows[0]["name"]
			} else {
				// No organization row: whether the organization exists at all
				// is unknown from the organizations data. If no agent or
				// incident names it either, the store holds nothing about it,
				// and its counts are unknown, not 0.
				named, err := countRows(ctx, st.DB(), fmt.Sprintf(`SELECT (SELECT COUNT(*) FROM %s a WHERE %s = ?) + (SELECT COUNT(*) FROM %s i WHERE %s = ?)`,
					st.src(entAgents), jx("a", "organization_id"), st.src(entIncidents), jx("i", "organization_id")), orgID, orgID)
				if err != nil {
					return fmt.Errorf("looking up the organization: %w", err)
				}
				if named == 0 {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: organization %d is not in the local store: no organization, agent or incident row names it, so its counts are withheld (null), not 0. Check the id, or run '%s sync'.\n", orgID, transcendenceApp)
					card["organization_found"] = false
					for _, k := range []string{"agent_count", "open_incidents", "closed_incidents", "total_incidents", "mttr_hours", "unreadable_timestamps"} {
						card[k] = nil
					}
					return flags.printJSON(cmd, card)
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: organization %d is not in the local organizations data (its name is unknown); its agent and incident rows are counted.\n", orgID)
			}
			card["organization_found"] = true
			rows, err = queryMaps(ctx, st.DB(),
				fmt.Sprintf(`SELECT COUNT(*) AS n FROM %s a WHERE %s = ?`, st.src(entAgents), jx("a", "organization_id")), orgID)
			if err != nil {
				return fmt.Errorf("counting agents: %w", err)
			}
			card["agent_count"] = rows[0]["n"]
			rows, err = queryMaps(ctx, st.DB(), fmt.Sprintf(`SELECT
				COALESCE(SUM(CASE WHEN %s='sent' THEN 1 ELSE 0 END), 0) AS open_incidents,
				COALESCE(SUM(CASE WHEN %s='closed' THEN 1 ELSE 0 END), 0) AS closed_incidents,
				COUNT(*) AS total_incidents
			FROM %s i WHERE %s = ?`,
				jx("i", "status"), jx("i", "status"), st.src(entIncidents), jx("i", "organization_id")), orgID)
			if err != nil {
				return fmt.Errorf("counting incidents: %w", err)
			}
			card["open_incidents"] = rows[0]["open_incidents"]
			card["closed_incidents"] = rows[0]["closed_incidents"]
			card["total_incidents"] = rows[0]["total_incidents"]
			// As in mttr: a resolved incident whose sent_at or closed_at cannot
			// be read withholds the mean rather than leaving it out of it.
			unreadable := "(" + jdx("i", "closed_at") + " IS NULL OR " + jdx("i", "sent_at") + " IS NULL)"
			rows, err = queryMaps(ctx, st.DB(), fmt.Sprintf(`SELECT
				SUM(CASE WHEN %[1]s THEN 1 ELSE 0 END) AS unreadable,
				CASE WHEN SUM(CASE WHEN %[1]s THEN 1 ELSE 0 END) > 0 THEN NULL ELSE ROUND(AVG((%[2]s-%[3]s)*24),2) END AS mttr_hours
			FROM %[4]s i WHERE %[5]s = ? AND %[6]s IS NOT NULL AND %[6]s != ''`,
				unreadable, jdx("i", "closed_at"), jdx("i", "sent_at"), st.src(entIncidents), jx("i", "organization_id"), jx("i", "closed_at")), orgID)
			if err != nil {
				return fmt.Errorf("computing mttr: %w", err)
			}
			card["mttr_hours"] = rows[0]["mttr_hours"]
			n, _ := toFloat(rows[0]["unreadable"])
			card["unreadable_timestamps"] = int(n)
			warnUnreadable(cmd, int(n), "resolved incident(s) have an unreadable sent_at or closed_at", "mttr_hours is withheld (null).")
			return flags.printJSON(cmd, card)
		},
	}
	cmd.Flags().IntVar(&org, "org", 0, "Organization ID to score")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}

// ---- stale-agents -----------------------------------------------------------

// pp:data-source local
func newStaleAgentsCmd(flags *rootFlags) *cobra.Command {
	var days int
	var platform, dbPath string
	cmd := &cobra.Command{
		Use:         "stale-agents",
		Short:       "Agents whose last callback exceeds a threshold",
		Long:        "Lists agents that haven't called back in --days days, across all organizations and optionally\nfiltered by platform — decommissioned-but-billed machines and broken installs.\nReads the local store; run `sync` first.",
		Example:     "  huntress-cli stale-agents --days 14 --platform windows --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNonNegative("days", days); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, entAgents, entOrgs)
			if err != nil {
				return err
			}
			defer st.Close()

			where := []string{
				jx("a", "last_callback_at") + " IS NOT NULL", jx("a", "last_callback_at") + " != ''",
				"julianday('now') - " + jdx("a", "last_callback_at") + " > ?"}
			qargs := []interface{}{days}
			if platform != "" {
				where = append(where, jx("a", "platform")+" = ?")
				qargs = append(qargs, platform)
			}
			q := fmt.Sprintf(`SELECT %s AS id, %s AS hostname, %s AS platform, %s AS organization_id, %s AS organization_name,
				%s AS last_callback_at,
				CAST(julianday('now') - %s AS INTEGER) AS days_since_callback
			FROM %s a %s
			WHERE %s
			ORDER BY %s ASC`,
				jx("a", "id"), jx("a", "hostname"), jx("a", "platform"), jx("a", "organization_id"), jx("o", "name"),
				jx("a", "last_callback_at"), jdx("a", "last_callback_at"),
				st.src(entAgents), st.orgJoin("a"), strings.Join(where, " AND "), jdx("a", "last_callback_at"))

			res, err := queryMaps(ctx, st.DB(), q, qargs...)
			if err != nil {
				return err
			}
			uq := "SELECT COUNT(*) FROM " + st.src(entAgents) + " a WHERE " + unreadableTimeSQL("a", "last_callback_at", true)
			uargs := []interface{}{}
			if platform != "" {
				uq += " AND " + jx("a", "platform") + " = ?"
				uargs = append(uargs, platform)
			}
			unreadable, err := countRows(ctx, st.DB(), uq, uargs...)
			if err != nil {
				return fmt.Errorf("checking agent callbacks: %w", err)
			}
			warnUnreadable(cmd, unreadable, "agent(s) have a missing or unreadable last_callback_at", "whether they are stale is unknown, so they are not listed.")
			return flags.printJSON(cmd, res)
		},
	}
	cmd.Flags().IntVar(&days, "days", 7, "Flag agents with no callback in this many days")
	cmd.Flags().StringVar(&platform, "platform", "", "Filter by platform (windows, darwin, linux)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}

// ---- handoff ----------------------------------------------------------------

// pp:data-source local
func newHandoffCmd(flags *rootFlags) *cobra.Command {
	var since, dbPath string
	cmd := &cobra.Command{
		Use:         "handoff",
		Short:       "What changed in a look-back window (--since 8h): shift-change report",
		Long:        "Summarizes fleet activity since --since: new incidents, resolved incidents, and new\nescalations across all organizations — ready to paste into a shift handoff note.\nReads the local store; run `sync` first.",
		Example:     "  huntress-cli handoff --since 8h --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := parseWindowDays("since", since)
			if err == nil && d <= 0 {
				err = fmt.Errorf("--since %q: a handoff needs a window like 8h or 1d", since)
			}
			if err != nil {
				return usageErr(err)
			}
			if dryRunOK(flags) {
				return nil
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, entIncidents, entEscal, entOrgs)
			if err != nil {
				return err
			}
			defer st.Close()

			newInc, err := queryMaps(ctx, st.DB(), fmt.Sprintf(`SELECT %s AS id, %s AS organization_id, %s AS organization_name,
				%s AS severity, %s AS indicator_types, %s AS sent_at
			FROM %s i %s
			WHERE %s IS NOT NULL AND %s != '' AND julianday('now') - %s <= ?
			ORDER BY %s DESC`,
				jx("i", "id"), jx("i", "organization_id"), jx("o", "name"),
				jx("i", "severity"), jx("i", "indicator_types"), jx("i", "sent_at"),
				st.src(entIncidents), st.orgJoin("i"), jx("i", "sent_at"), jx("i", "sent_at"), jdx("i", "sent_at"), jdx("i", "sent_at")), d)
			if err != nil {
				return err
			}
			resolved, err := queryMaps(ctx, st.DB(), fmt.Sprintf(`SELECT %s AS id, %s AS organization_id, %s AS organization_name,
				%s AS severity, %s AS closed_at
			FROM %s i %s
			WHERE %s IS NOT NULL AND %s != '' AND julianday('now') - %s <= ?
			ORDER BY %s DESC`,
				jx("i", "id"), jx("i", "organization_id"), jx("o", "name"),
				jx("i", "severity"), jx("i", "closed_at"),
				st.src(entIncidents), st.orgJoin("i"), jx("i", "closed_at"), jx("i", "closed_at"), jdx("i", "closed_at"), jdx("i", "closed_at")), d)
			if err != nil {
				return err
			}
			// Escalations use created_at (no sent_at).
			newEsc, err := queryMaps(ctx, st.DB(), fmt.Sprintf(`SELECT %s AS id, %s AS severity, %s AS status, %s AS subject, %s AS created_at
			FROM %s e
			WHERE %s IS NOT NULL AND %s != '' AND julianday('now') - %s <= ?
			ORDER BY %s DESC`,
				jx("e", "id"), jx("e", "severity"), jx("e", "status"), jx("e", "subject"), jx("e", "created_at"),
				st.src(entEscal), jx("e", "created_at"), jx("e", "created_at"), jdx("e", "created_at"), jdx("e", "created_at")), d)
			if err != nil {
				return err
			}
			// A row whose timestamp cannot be read cannot be placed in the
			// window; say how many were left out instead of dropping them
			// silently.
			for _, u := range []struct{ entity, alias, field, what string }{
				{entIncidents, "i", "sent_at", "incident(s) have a missing or unreadable sent_at"},
				{entIncidents, "i", "closed_at", "incident(s) have an unreadable closed_at"},
				{entEscal, "e", "created_at", "escalation(s) have a missing or unreadable created_at"},
			} {
				n, err := countRows(ctx, st.DB(), "SELECT COUNT(*) FROM "+st.src(u.entity)+" "+u.alias+" WHERE "+unreadableTimeSQL(u.alias, u.field, u.field != "closed_at"))
				if err != nil {
					return fmt.Errorf("checking %s timestamps: %w", u.entity, err)
				}
				warnUnreadable(cmd, n, u.what, "whether they fall in the window is unknown, so they are not counted.")
			}
			result := map[string]interface{}{
				"window_days":      d,
				"new_incidents":    newInc,
				"resolved":         resolved,
				"new_escalations":  newEsc,
				"new_incident_n":   len(newInc),
				"resolved_n":       len(resolved),
				"new_escalation_n": len(newEsc),
			}
			return flags.printJSON(cmd, result)
		},
	}
	cmd.Flags().StringVar(&since, "since", "12h", "Look-back window, e.g. 8h, 1d")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}
