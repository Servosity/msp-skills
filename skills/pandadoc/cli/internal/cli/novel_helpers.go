package cli

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"pandadoc-pp-cli/internal/store"
)

// novelDoc is the parsed, store-backed view of a PandaDoc document used by the
// hand-built transcendence commands (pipeline, stalled, aging, since,
// template-stats, value, engagement). It is derived from the JSON the sync
// command persists under resource_type "documents".
type novelDoc struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Status        string    `json:"status"`
	DateCreated   time.Time `json:"-"`
	DateModified  time.Time `json:"-"`
	DateCompleted time.Time `json:"-"`
	TemplateID    string    `json:"template_id,omitempty"`
	TemplateName  string    `json:"template_name,omitempty"`
	GrandTotal    float64   `json:"grand_total,omitempty"`
	Currency      string    `json:"currency,omitempty"`
}

// docTimeLayouts covers the ISO-8601 shapes PandaDoc returns (with and without
// fractional seconds, with Z or numeric offset).
var docTimeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.999999Z07:00",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02T15:04:05",
	"2006-01-02",
}

// parseDocTime parses a PandaDoc timestamp string, returning the zero time on
// empty/unparseable input rather than erroring — missing dates are common and
// should degrade gracefully, not abort an aggregation.
func parseDocTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range docTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// normalizeStatus maps user-friendly status shortcuts ("sent", "viewed") to the
// canonical PandaDoc status string ("document.sent"). A value that already
// contains a dot is returned unchanged so callers can pass full status strings.
func normalizeStatus(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	if strings.Contains(s, ".") {
		return s
	}
	return "document." + s
}

// shortStatus strips the "document." prefix for compact display.
func shortStatus(s string) string {
	return strings.TrimPrefix(s, "document.")
}

// parseStatusList splits a comma-separated --status value into a set of
// canonical status strings.
func parseStatusList(csv string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.Split(csv, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out[normalizeStatus(part)] = true
	}
	return out
}

// isTerminalStatus reports whether a status represents a completed/closed document.
func isTerminalStatus(status string) bool {
	switch normalizeStatus(status) {
	case "document.completed", "document.paid", "document.declined",
		"document.voided", "document.expired", "document.rejected":
		return true
	}
	return false
}

// parseNovelDoc converts a decoded document JSON object into a novelDoc.
func parseNovelDoc(m map[string]json.RawMessage) novelDoc {
	d := novelDoc{
		ID:            jsonStr(m, "id"),
		Name:          jsonStr(m, "name"),
		Status:        jsonStr(m, "status"),
		DateCreated:   parseDocTime(jsonStr(m, "date_created")),
		DateModified:  parseDocTime(jsonStr(m, "date_modified")),
		DateCompleted: parseDocTime(jsonStr(m, "date_completed")),
	}
	if tmplRaw, ok := m["template"]; ok {
		var tmpl map[string]json.RawMessage
		if json.Unmarshal(tmplRaw, &tmpl) == nil {
			d.TemplateID = jsonStr(tmpl, "id")
			d.TemplateName = jsonStr(tmpl, "name")
		}
	}
	if gtRaw, ok := m["grand_total"]; ok {
		var gt map[string]json.RawMessage
		if json.Unmarshal(gtRaw, &gt) == nil {
			d.Currency = jsonStr(gt, "currency")
			amt := jsonStr(gt, "amount")
			if amt != "" {
				if v, err := strconv.ParseFloat(amt, 64); err == nil {
					d.GrandTotal = v
				}
			}
		}
	}
	return d
}

// jsonStr extracts a string field from a decoded JSON object, tolerating
// missing keys and non-string values (returns "").
func jsonStr(m map[string]json.RawMessage, key string) string {
	raw, ok := m[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}

// lastActivity returns the most recent of date_modified/date_created for aging
// math, falling back to created when modified is absent.
func (d novelDoc) lastActivity() time.Time {
	if !d.DateModified.IsZero() {
		return d.DateModified
	}
	return d.DateCreated
}

// openNovelStore opens the local store for a hand-built transcendence command
// and emits the generated sync hints for resourceType before returning the
// handle. Callers own Close().
func openNovelStore(cmd *cobra.Command, flags *rootFlags, dbPath, resourceType string) (*store.Store, string, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("pandadoc-cli")
	}
	db, err := pdOpenStoreSchemaAware(cmd.Context(), dbPath)
	if err != nil {
		return nil, dbPath, err
	}
	if !hintIfUnsynced(cmd, db, resourceType) {
		hintIfStale(cmd, db, resourceType, flags.maxAge)
	}
	return db, dbPath, nil
}

// loadDocumentsHinted opens the store with sync hints for "documents", loads
// every synced document, and closes the handle. The store-backed novel
// commands that only need the document corpus use this instead of
// loadDocuments so unsynced/stale stores produce an actionable stderr hint.
func loadDocumentsHinted(cmd *cobra.Command, flags *rootFlags, dbPath string) ([]novelDoc, error) {
	db, _, err := openNovelStore(cmd, flags, dbPath, "documents")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return loadDocumentsFromStore(db)
}

// loadDocumentsFromStore parses every synced document from an already-open store.
func loadDocumentsFromStore(db *store.Store) ([]novelDoc, error) {
	raws, err := db.List("documents", 1000000)
	if err != nil {
		return nil, err
	}
	docs := make([]novelDoc, 0, len(raws))
	for _, raw := range raws {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		docs = append(docs, parseNovelDoc(m))
	}
	return docs, nil
}

// ageDays returns whole days elapsed since t, treating the zero time as
// "ancient" (a very large age) so undated documents sort into the coldest
// bucket instead of silently looking fresh.
func ageDays(t time.Time) int {
	if t.IsZero() {
		return 1 << 20
	}
	return int(time.Since(t).Hours() / 24)
}

// recipientRef is the (email, name) pair extracted from a document's embedded
// recipients array, shared by the recipient-centric rollups (engagement,
// cold-clients, followup).
type recipientRef struct {
	Email string
	Name  string
	Raw   map[string]json.RawMessage
}

// parseRecipients extracts the embedded recipients from a decoded document
// JSON object, normalizing emails (lowercase, trimmed) and assembling display
// names. Recipients without an email are skipped. Returns nil when the
// document carries no recipient data.
func parseRecipients(m map[string]json.RawMessage) []recipientRef {
	recRaw, ok := m["recipients"]
	if !ok {
		return nil
	}
	var recs []map[string]json.RawMessage
	if json.Unmarshal(recRaw, &recs) != nil {
		return nil
	}
	out := make([]recipientRef, 0, len(recs))
	for _, r := range recs {
		email := strings.ToLower(strings.TrimSpace(jsonStr(r, "email")))
		if email == "" {
			continue
		}
		out = append(out, recipientRef{
			Email: email,
			Name:  strings.TrimSpace(jsonStr(r, "first_name") + " " + jsonStr(r, "last_name")),
			Raw:   r,
		})
	}
	return out
}

// pdOpenStoreSchemaAware opens the local mirror read-only when the schema
// already exists (no write lock -> no SQLITE_BUSY under parallel novel reads),
// else read-write-migrate, with a short retry to ride out the first-run
// create/migrate race.
func pdOpenStoreSchemaAware(ctx context.Context, dbPath string) (*store.Store, error) {
	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		if st, ok := pdTryReadOnlyMigrated(dbPath); ok {
			return st, nil
		}
		st, err := store.OpenWithContext(ctx, dbPath)
		if err == nil {
			return st, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		time.Sleep(time.Duration(20*(attempt+1)) * time.Millisecond)
	}
	return nil, lastErr
}

func pdTryReadOnlyMigrated(path string) (*store.Store, bool) {
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
