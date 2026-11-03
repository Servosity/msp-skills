package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"

	"xero-pp-cli/internal/store"
)

// openXeroStore opens the local SQLite store at dbPath (default: the canonical
// path) READ-ONLY. The analytics are MCP read-only tools, so a read must never
// create the file, its directory or a schema: a missing store returns
// (nil, nil), which the loaders below treat as nothing synced. Callers release
// it with closeXeroStore.
func openXeroStore(ctx context.Context, dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("xero-cli")
	}
	if _, err := os.Stat(dbPath); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return store.OpenReadOnlyContext(ctx, dbPath)
}

func closeXeroStore(db *store.Store) {
	if db != nil {
		_ = db.Close()
	}
}

// loadAllRows returns every synced row of resourceType. It deliberately uses a
// direct query rather than store.List, which caps at 200 rows — the
// transcendence analytics (aging, tie-out, exposure, …) must see the whole org,
// not a truncated page. Returns an empty slice (not an error) when nothing is
// synced, so analytics on an empty store return empty results and exit 0.
func loadAllRows(db *store.Store, resourceType string) ([]json.RawMessage, error) {
	if db == nil {
		return nil, nil
	}
	rows, err := db.Query(`SELECT data FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []json.RawMessage
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, json.RawMessage(d))
	}
	return out, rows.Err()
}

// rowWithID pairs a synced row with its store primary key.
type rowWithID struct {
	ID   string
	Data json.RawMessage
}

// loadRowsWithID returns every synced row of resourceType paired with its store
// id. Used by `since`, which reports which records changed, not just how many.
func loadRowsWithID(db *store.Store, resourceType string) ([]rowWithID, error) {
	if db == nil {
		return nil, nil
	}
	rows, err := db.Query(`SELECT id, data FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []rowWithID
	for rows.Next() {
		var id, d string
		if err := rows.Scan(&id, &d); err != nil {
			return nil, err
		}
		out = append(out, rowWithID{ID: id, Data: json.RawMessage(d)})
	}
	return out, rows.Err()
}
