// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored transcendence support (Printing Press Phase 3). Shared by the
// novel-feature commands (related, war-room, mttr, coverage-gaps, etc.). These
// read ONLY the local SQLite mirror — no live API call, no credential needed to
// run. `sync` populates the mirror; everything here is an offline join over it.

package cli

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"rootly-pp-cli/internal/store"
)

// novelTimeLayouts is the ordered set of layouts the on-disk Rootly timestamps
// are parsed against. Rootly emits RFC3339 (with and without fractional
// seconds, Z or numeric offset); the date-only fallback covers due_date-style
// fields.
var novelTimeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.000Z07:00",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02",
}

// record is one synced JSON:API resource unwrapped for ergonomic access.
// Attrs is the `attributes` object; Rels is `relationships`; Raw is the whole
// `{id,type,attributes,relationships}` envelope.
type record struct {
	ID    string
	Attrs map[string]any
	Rels  map[string]any
	Raw   map[string]any
}

// novelOpenStore opens the local store read-only with a friendly "run sync
// first" error. The novel commands never write.
func novelOpenStore(cmd *cobra.Command, dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("rootly-cli")
	}
	db, err := store.OpenWithContext(cmd.Context(), dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local database: %w\nRun 'rootly-cli sync' first to populate the offline mirror.", err)
	}
	return db, nil
}

// novelOpenStoreChecked is the standard entry point for local-only novel
// commands (// pp:data-source local). It rejects --data-source live via
// validateDataSourceStrategy, opens the local store read-only, and emits the
// generated sync hints for the command's primary resource ("" scans all
// resources) so agents see unsynced/stale warnings on stderr without
// disturbing stdout/JSON output.
func novelOpenStoreChecked(cmd *cobra.Command, flags *rootFlags, dbPath, primaryResource string) (*store.Store, error) {
	if err := validateDataSourceStrategy(flags, "local"); err != nil {
		return nil, err
	}
	db, err := novelOpenStore(cmd, dbPath)
	if err != nil {
		return nil, err
	}
	if !hintIfUnsynced(cmd, db, primaryResource) {
		hintIfStale(cmd, db, primaryResource, flags.maxAge)
	}
	return db, nil
}

// novelResolveType returns the first candidate resource_type that actually has
// rows in the store, falling back to the first candidate when none are present
// (so the caller still gets a deterministic, queryable name). Sync stores
// top-level resources under their hyphenated names (e.g. "action-items"), but
// being tolerant of underscore variants keeps the commands robust if that ever
// changes.
func novelResolveType(db *store.Store, candidates ...string) string {
	status, err := db.Status()
	if err == nil {
		for _, c := range candidates {
			if status[c] > 0 {
				return c
			}
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return ""
}

// novelLoad loads every synced record of resourceType. Returns an empty slice
// (not nil-error) when the type has no rows, so callers report an honest empty
// result rather than crashing.
func novelLoad(db *store.Store, resourceType string) ([]record, error) {
	rows, err := db.Query(`SELECT id, data FROM resources WHERE resource_type = ? ORDER BY id`, resourceType)
	if err != nil {
		return nil, fmt.Errorf("querying %s: %w", resourceType, err)
	}
	defer rows.Close()

	var out []record
	for rows.Next() {
		var id, data string
		if err := rows.Scan(&id, &data); err != nil {
			continue
		}
		var raw map[string]any
		if json.Unmarshal([]byte(data), &raw) != nil {
			continue
		}
		rec := record{ID: id, Raw: raw}
		if a, ok := raw["attributes"].(map[string]any); ok {
			rec.Attrs = a
		} else {
			// Some stores may persist a flattened object; treat the whole
			// envelope as attributes so accessors still find fields.
			rec.Attrs = raw
		}
		if r, ok := raw["relationships"].(map[string]any); ok {
			rec.Rels = r
		}
		out = append(out, rec)
	}
	if out == nil {
		out = []record{}
	}
	return out, rows.Err()
}

// recStr returns the first non-empty string-valued attribute among keys.
func recStr(attrs map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := attrs[k]; ok && v != nil {
			if s, ok := v.(string); ok {
				if s != "" {
					return s
				}
				continue
			}
			s := fmt.Sprintf("%v", v)
			if s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}

// recTime parses the first attribute among keys into a time.Time.
func recTime(attrs map[string]any, keys ...string) (time.Time, bool) {
	raw := recStr(attrs, keys...)
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range novelTimeLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// recName extracts a human name from a value that may be a bare string, a
// {name|title} object, or a JSON:API envelope ({data:{attributes:{name}}}).
func recName(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case map[string]any:
		for _, k := range []string{"name", "title", "full_name", "label", "email"} {
			if s, ok := x[k].(string); ok && s != "" {
				return s
			}
		}
		if d, ok := x["data"].(map[string]any); ok {
			return recName(d)
		}
		if a, ok := x["attributes"].(map[string]any); ok {
			return recName(a)
		}
		if id, ok := x["id"].(string); ok {
			return id
		}
	}
	return ""
}

// recNames flattens an array attribute (services, groups, ...) into names.
func recNames(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		if n := recName(v); n != "" {
			return []string{n}
		}
		return nil
	}
	var out []string
	for _, item := range arr {
		if n := recName(item); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// recIDs flattens an array attribute into the ids it references.
func recIDs(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			if d, ok := m["data"].(map[string]any); ok {
				m = d
			}
			if id, ok := m["id"].(string); ok && id != "" {
				out = append(out, id)
			}
		}
	}
	return out
}

// incidentTitle returns a display title for an incident record.
func incidentTitle(r record) string {
	t := recStr(r.Attrs, "title", "public_title", "summary")
	if t == "" {
		t = "(untitled incident)"
	}
	return t
}

// incidentSeverity returns the incident's severity name ("" if none).
func incidentSeverity(r record) string {
	if v, ok := r.Attrs["severity"]; ok {
		return recName(v)
	}
	return ""
}

// incidentServiceNames returns the names of services attached to an incident.
func incidentServiceNames(r record) []string {
	if v, ok := r.Attrs["services"]; ok {
		return recNames(v)
	}
	return nil
}

// incidentTeamNames returns the names of teams (groups) attached to an incident.
func incidentTeamNames(r record) []string {
	if v, ok := r.Attrs["groups"]; ok {
		return recNames(v)
	}
	return nil
}

// incidentStart is the best "incident began" timestamp.
func incidentStart(r record) (time.Time, bool) {
	return recTime(r.Attrs, "started_at", "created_at", "detected_at")
}

// incidentResolved is the best "incident resolved" timestamp.
func incidentResolved(r record) (time.Time, bool) {
	return recTime(r.Attrs, "resolved_at", "mitigated_at", "closed_at")
}

// incidentOpen reports whether an incident has no resolution/closure timestamp
// and is not cancelled — i.e. it is still active.
func incidentOpen(r record) bool {
	if _, ok := recTime(r.Attrs, "resolved_at"); ok {
		return false
	}
	if _, ok := recTime(r.Attrs, "closed_at"); ok {
		return false
	}
	if _, ok := recTime(r.Attrs, "cancelled_at"); ok {
		return false
	}
	status := strings.ToLower(recStr(r.Attrs, "status"))
	switch status {
	case "resolved", "closed", "cancelled", "completed":
		return false
	}
	return true
}

// humanDuration renders a duration as a compact "2d 3h 12m" style string.
func humanDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	parts := []string{}
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if mins > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%dm", mins))
	}
	return strings.Join(parts, " ")
}

// roundMinutes returns whole minutes for a duration (for JSON fields).
func roundMinutes(d time.Duration) int {
	return int(math.Round(d.Minutes()))
}

// --- lightweight TF-IDF over an in-memory corpus (used by `related`) ---

var novelTokenStop = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true, "of": true,
	"to": true, "in": true, "on": true, "for": true, "is": true, "are": true,
	"was": true, "were": true, "with": true, "at": true, "by": true, "from": true,
	"this": true, "that": true, "it": true, "as": true, "be": true, "incident": true,
}

// novelTokenize lowercases, splits on non-alphanumerics, drops stopwords and
// 1-char tokens.
func novelTokenize(s string) []string {
	s = strings.ToLower(s)
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if len(f) < 2 || novelTokenStop[f] {
			continue
		}
		out = append(out, f)
	}
	return out
}

// tfidfCorpus holds document term-frequency vectors plus the corpus IDF.
type tfidfCorpus struct {
	docs map[string]map[string]float64 // id -> term -> tf
	idf  map[string]float64
}

// newTFIDF builds a corpus from id->text.
func newTFIDF(textByID map[string]string) *tfidfCorpus {
	c := &tfidfCorpus{docs: map[string]map[string]float64{}, idf: map[string]float64{}}
	df := map[string]int{}
	for id, text := range textByID {
		toks := novelTokenize(text)
		tf := map[string]float64{}
		for _, t := range toks {
			tf[t]++
		}
		// normalize tf by document length
		if n := float64(len(toks)); n > 0 {
			for t := range tf {
				tf[t] /= n
			}
		}
		c.docs[id] = tf
		for t := range tf {
			df[t]++
		}
	}
	n := float64(len(textByID))
	for t, d := range df {
		c.idf[t] = math.Log((n+1)/(float64(d)+1)) + 1
	}
	return c
}

// vec returns the tf-idf vector for a document id.
func (c *tfidfCorpus) vec(id string) map[string]float64 {
	tf := c.docs[id]
	v := make(map[string]float64, len(tf))
	for t, f := range tf {
		v[t] = f * c.idf[t]
	}
	return v
}

// cosine similarity of two sparse vectors.
func cosine(a, b map[string]float64) float64 {
	var dot, na, nb float64
	for t, av := range a {
		na += av * av
		if bv, ok := b[t]; ok {
			dot += av * bv
		}
	}
	for _, bv := range b {
		nb += bv * bv
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// scored pairs an id with a similarity/score for ranking.
type scored struct {
	id    string
	score float64
}

func sortScoredDesc(s []scored) {
	sort.Slice(s, func(i, j int) bool {
		if s[i].score == s[j].score {
			return s[i].id < s[j].id
		}
		return s[i].score > s[j].score
	})
}

// novelLoadChildTable loads rows from a generated sub-resource table
// (e.g. incidents_events) filtered by its foreign-key column. It tolerates a
// missing table (returns empty) so callers degrade gracefully when a dependent
// resource was never synced.
func novelLoadChildTable(db *store.Store, table, fkCol, fkVal string) []record {
	// table/fkCol are internal constants (never user input); fkVal is bound.
	q := fmt.Sprintf(`SELECT id, data FROM "%s" WHERE "%s" = ? ORDER BY id`, table, fkCol)
	rows, err := db.Query(q, fkVal)
	if err != nil {
		return nil // missing table or column -> no children
	}
	defer rows.Close()
	var out []record
	for rows.Next() {
		var id, data string
		if rows.Scan(&id, &data) != nil {
			continue
		}
		var raw map[string]any
		if json.Unmarshal([]byte(data), &raw) != nil {
			continue
		}
		rec := record{ID: id, Raw: raw}
		if a, ok := raw["attributes"].(map[string]any); ok {
			rec.Attrs = a
		} else {
			rec.Attrs = raw
		}
		if r, ok := raw["relationships"].(map[string]any); ok {
			rec.Rels = r
		}
		out = append(out, rec)
	}
	return out
}

// recRefersTo reports whether any of the record's relationships points at the
// given resource id (the JSON:API `relationships.<rel>.data.id` shape). Used to
// link a top-level action-item back to its incident without assuming a
// particular relationship name.
func recRefersTo(r record, id string) bool {
	if id == "" || r.Rels == nil {
		return false
	}
	for _, rel := range r.Rels {
		m, ok := rel.(map[string]any)
		if !ok {
			continue
		}
		switch d := m["data"].(type) {
		case map[string]any:
			if did, ok := d["id"].(string); ok && did == id {
				return true
			}
		case []any:
			for _, item := range d {
				if im, ok := item.(map[string]any); ok {
					if did, ok := im["id"].(string); ok && did == id {
						return true
					}
				}
			}
		}
	}
	return false
}

// actionItemOpen reports whether an action item is still open/in-progress.
func actionItemOpen(r record) bool {
	switch strings.ToLower(recStr(r.Attrs, "status")) {
	case "open", "in_progress", "":
		return true
	}
	return false
}

// parseWindowDuration accepts "30d", "12h", "90m", or any Go duration string and
// returns the duration. Empty string -> 0, ok=false.
func parseWindowDuration(s string) (time.Duration, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, false
	}
	if strings.HasSuffix(s, "d") {
		var n float64
		if _, err := fmt.Sscanf(s, "%fd", &n); err == nil {
			return time.Duration(n * 24 * float64(time.Hour)), true
		}
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d, true
	}
	return 0, false
}

// relID returns the id referenced by a named JSON:API relationship
// (relationships.<rel>.data.id), or "".
func relID(r record, rel string) string {
	if r.Rels == nil {
		return ""
	}
	m, ok := r.Rels[rel].(map[string]any)
	if !ok {
		return ""
	}
	if d, ok := m["data"].(map[string]any); ok {
		if id, ok := d["id"].(string); ok {
			return id
		}
	}
	return ""
}

// oncallEntry is one current on-call assignment from the synced `oncalls`
// snapshot.
type oncallEntry struct {
	User               string `json:"user"`
	Schedule           string `json:"schedule,omitempty"`
	EscalationPolicy   string `json:"escalation_policy,omitempty"`
	EscalationPolicyID string `json:"escalation_policy_id,omitempty"`
}

// currentOncallEntries reads the synced `oncalls` resource into a normalized
// list. Tolerant of attribute-vs-relationship encodings of user/schedule/policy.
func currentOncallEntries(db *store.Store) []oncallEntry {
	rows, err := novelLoad(db, novelResolveType(db, "oncalls"))
	if err != nil {
		return nil
	}
	var out []oncallEntry
	for _, r := range rows {
		e := oncallEntry{
			User:               firstNonEmpty(recName(r.Attrs["user"]), recName(r.Rels["user"])),
			Schedule:           firstNonEmpty(recName(r.Attrs["schedule"]), recName(r.Rels["schedule"]), recStr(r.Attrs, "schedule_name")),
			EscalationPolicy:   firstNonEmpty(recName(r.Attrs["escalation_policy"]), recName(r.Rels["escalation_policy"])),
			EscalationPolicyID: firstNonEmpty(relID(r, "escalation_policy"), recStr(r.Attrs, "escalation_policy_id")),
		}
		if e.User == "" && e.Schedule == "" {
			continue
		}
		out = append(out, e)
	}
	return out
}

// oncallForServices returns "user (schedule)" labels for the on-call people
// covering the named services. It links service -> escalation_policy_id ->
// on-call entries; when no policy linkage is discoverable it falls back to the
// full current on-call roster so the caller still gets an honest answer.
func oncallForServices(db *store.Store, serviceNames []string) []string {
	entries := currentOncallEntries(db)
	if len(entries) == 0 {
		return nil
	}
	wanted := map[string]bool{}
	if len(serviceNames) > 0 {
		svc, _ := novelLoad(db, novelResolveType(db, "services"))
		nameSet := map[string]bool{}
		for _, n := range serviceNames {
			nameSet[strings.ToLower(n)] = true
		}
		for _, s := range svc {
			if nameSet[strings.ToLower(recStr(s.Attrs, "name"))] {
				if pid := recStr(s.Attrs, "escalation_policy_id"); pid != "" {
					wanted[pid] = true
				}
			}
		}
	}
	collect := func(filtered bool) []string {
		var out []string
		seen := map[string]bool{}
		for _, e := range entries {
			if filtered && len(wanted) > 0 && e.EscalationPolicyID != "" && !wanted[e.EscalationPolicyID] {
				continue
			}
			label := e.User
			if e.Schedule != "" {
				label = fmt.Sprintf("%s (%s)", e.User, e.Schedule)
			}
			if label != "" && !seen[label] {
				seen[label] = true
				out = append(out, label)
			}
		}
		return out
	}
	out := collect(true)
	// If service filtering eliminated everyone but we do have a roster, the
	// linkage was absent — return the full roster rather than a false empty.
	if len(out) == 0 && len(wanted) > 0 {
		out = collect(false)
	}
	return out
}

// collectActionItems returns summaries of action items linked to an incident,
// unioning the synced sub-resource table with any top-level action-items that
// reference the incident. Deduped by summary. When openOnly is true, only
// open/in-progress items are included.
func collectActionItems(db *store.Store, incidentID string, openOnly bool) []string {
	seen := map[string]bool{}
	var out []string
	add := func(r record) {
		if openOnly && !actionItemOpen(r) {
			return
		}
		s := strings.TrimSpace(recStr(r.Attrs, "summary", "description"))
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, r := range novelLoadChildTable(db, "incidents_action_items", "incidents_id", incidentID) {
		add(r)
	}
	if topLevel, err := novelLoad(db, novelResolveType(db, "action-items", "action_items")); err == nil {
		for _, r := range topLevel {
			if recRefersTo(r, incidentID) {
				add(r)
			}
		}
	}
	return out
}

// collectOpenActionItems returns summaries of open action items linked to an
// incident.
func collectOpenActionItems(db *store.Store, incidentID string) []string {
	return collectActionItems(db, incidentID, true)
}

// collectAllActionItems returns summaries of ALL action items (any status)
// linked to an incident.
func collectAllActionItems(db *store.Store, incidentID string) []string {
	return collectActionItems(db, incidentID, false)
}

// firstNonEmpty returns the first non-blank string.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// emit writes v as JSON when --json/agent/piped, otherwise calls the human
// renderer. Centralizes the output decision so every novel command behaves
// identically for agents.
func novelEmit(cmd *cobra.Command, flags *rootFlags, v any, human func()) error {
	if flags.asJSON || flags.agent || (!isTerminal(cmd.OutOrStdout()) && !flags.csv && !flags.quiet && !flags.plain) {
		return flags.printJSON(cmd, v)
	}
	human()
	return nil
}

// flushHuman flushes a buffered human-output writer (e.g. a tabwriter) from
// inside a novelEmit human closure, which has no error return. A flush failure
// means a downstream write to stdout failed; we cannot recover the rendered
// table, but we surface the error to stderr rather than silently dropping it.
func flushHuman(cmd *cobra.Command, f interface{ Flush() error }) {
	if err := f.Flush(); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to flush output: %v\n", err)
	}
}

// childRecord is a sub-resource row plus the parent foreign-key value it was
// stored under (e.g. an incidents_action_items row plus its incidents_id).
type childRecord struct {
	record
	FK string
}

// novelLoadChildTableAll loads every row of a generated sub-resource table
// along with the parent foreign-key value. Tolerates a missing table (returns
// empty) so callers degrade gracefully when a dependent resource was never
// synced.
func novelLoadChildTableAll(db *store.Store, table, fkCol string) []childRecord {
	// table/fkCol are internal constants (never user input).
	q := fmt.Sprintf(`SELECT id, data, "%s" FROM "%s" ORDER BY id`, fkCol, table)
	rows, err := db.Query(q)
	if err != nil {
		return nil // missing table -> no rows
	}
	defer rows.Close()
	var out []childRecord
	for rows.Next() {
		var id, data string
		var fk sql.NullString
		if rows.Scan(&id, &data, &fk) != nil {
			continue
		}
		var raw map[string]any
		if json.Unmarshal([]byte(data), &raw) != nil {
			continue
		}
		rec := childRecord{record: record{ID: id, Raw: raw}, FK: fk.String}
		if a, ok := raw["attributes"].(map[string]any); ok {
			rec.Attrs = a
		} else {
			rec.Attrs = raw
		}
		if r, ok := raw["relationships"].(map[string]any); ok {
			rec.Rels = r
		}
		out = append(out, rec)
	}
	return out
}

// novelConfigHash returns a stable content hash for a config object's
// attributes. json.Marshal sorts map keys, so the hash is deterministic for
// equal content regardless of source ordering.
func novelConfigHash(attrs map[string]any) string {
	b, err := json.Marshal(attrs)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

// slaTargetDuration extracts a target duration from an SLA record's
// attributes. Rootly SLA shapes vary; this tries duration-string attributes
// first ("2h", "90m", "7d"), then bare-numeric attributes interpreted as
// minutes. Returns ok=false when nothing parseable is present.
func slaTargetDuration(attrs map[string]any) (time.Duration, bool) {
	for _, k := range []string{"target", "duration", "sla", "threshold", "time", "minutes", "target_minutes", "resolution_time"} {
		v, present := attrs[k]
		if !present || v == nil {
			continue
		}
		switch x := v.(type) {
		case string:
			if d, ok := parseWindowDuration(x); ok && d > 0 {
				return d, true
			}
		case float64:
			if x > 0 {
				if k == "time" || k == "duration" || k == "target" || k == "threshold" || k == "sla" || k == "resolution_time" {
					// Heuristic: large bare numbers are seconds, small are minutes.
					if x >= 3600 {
						return time.Duration(x) * time.Second, true
					}
				}
				return time.Duration(x) * time.Minute, true
			}
		}
	}
	return 0, false
}

// alertNoiseKey normalizes an alert title/summary for repeat-fire grouping:
// lowercased, whitespace-collapsed, digits stripped so "disk 91% full" and
// "disk 97% full" group together.
func alertNoiseKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastSpace := false
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			continue
		case r == ' ' || r == '\t' || r == '\n':
			if !lastSpace {
				b.WriteRune(' ')
				lastSpace = true
			}
		default:
			b.WriteRune(r)
			lastSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}
