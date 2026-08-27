// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Shared local-store helpers for the hand-written Auvik transcendence commands.
// Every Auvik record lands in the generic `resources` table as a JSON:API
// resource object -- {"id":..,"type":..,"attributes":{..},"relationships":{..}} --
// keyed by the sync resource name. These helpers centralize the drain-first read
// and the attribute plucking so each novel command stays small.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"auvik-pp-cli/internal/store"
)

// Sync resource names, which are also the `resource_type` values written by
// `sync` (see defaultSyncResources/knownSyncResourceNames in sync.go). Several
// families are addressed by more than one name depending on whether the API
// exposed them flat or parent-scoped, so lookups use the candidate-list form.
var (
	rtDevices         = []string{"inventory", "inventory-device-info"}
	rtDeviceDetail    = []string{"inventory-device-detail"}
	rtDeviceLifecycle = []string{"inventory-device-lifecycle"}
	// v1 lifecycle carries only STATUS enums; the actual end-of-support and
	// end-of-sale DATES exist only on the v2 lifecycle resource.
	rtDeviceLifecycleV2 = []string{"auvik-inventory-lifecycle"}
	rtDeviceWarranty    = []string{"inventory-device-warranty"}
	rtConfigurations    = []string{"inventory-configuration"}
	rtEntityAudit       = []string{"inventory-entity-audit"}
	rtEntityNote        = []string{"inventory-entity-note"}
	rtAlerts            = []string{"alert"}
	rtTenants           = []string{"tenants", "tenants-detail"}
	rtDiscoveryStatus   = []string{"auvik-inventory-discovery-status"}
	rtBilling           = []string{"billing"}
	rtASMApps           = []string{"asm", "asm-app-info"}
	rtASMUsers          = []string{"asm-user-info"}
	rtASMLicenses       = []string{"asm-license-info"}
	rtASMClients        = []string{"asm-client-info"}
)

// auvikRow is one stored JSON:API resource object plus its primary key.
type auvikRow struct {
	ID   string
	Data map[string]any
}

// attr returns a nested value from the resource object's attributes map.
// Path segments walk nested objects: attr("deviceName") or attr("a","b").
func (r auvikRow) attr(path ...string) any {
	attrs, _ := r.Data["attributes"].(map[string]any)
	return walkAny(attrs, path...)
}

// rel returns the id of a to-one relationship, e.g. rel("tenant").
func (r auvikRow) rel(name string) string {
	rels, _ := r.Data["relationships"].(map[string]any)
	node, _ := walkAny(rels, name, "data").(map[string]any)
	if node == nil {
		return ""
	}
	return anyString(node["id"])
}

// relMany returns the ids of a to-many relationship. Several Auvik
// relationships are lists (asmAppRelationships.users,
// clientUsageRelationships.devices, asmUserRelationships.applications), where
// `data` is an array of resource identifier objects rather than one object.
func (r auvikRow) relMany(name string) []string {
	rels, _ := r.Data["relationships"].(map[string]any)
	list, _ := walkAny(rels, name, "data").([]any)
	out := make([]string, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if id := anyString(m["id"]); id != "" {
			out = append(out, id)
		}
	}
	return out
}

// relManyAttr pulls one attribute from each member of a to-many relationship.
// clientUsageRelationships.devices carries a name alongside the id, which is
// what makes billing attribution readable instead of a list of opaque ids.
func (r auvikRow) relManyAttr(name, attr string) map[string]string {
	rels, _ := r.Data["relationships"].(map[string]any)
	list, _ := walkAny(rels, name, "data").([]any)
	out := make(map[string]string, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id := anyString(m["id"])
		if id == "" {
			continue
		}
		out[id] = anyString(walkAny(m, "attributes", attr))
	}
	return out
}

func (r auvikRow) attrString(path ...string) string { return anyString(r.attr(path...)) }

func walkAny(root map[string]any, path ...string) any {
	var cur any = root
	for _, seg := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[seg]
	}
	return cur
}

func anyString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		// Ids arrive as JSON numbers on some endpoints.
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// openLocalMirror resolves the db path, refuses politely when no mirror exists,
// and returns a read-only store. handled==true means the caller should return
// immediately -- the "no mirror yet" response has already been written.
func openLocalMirror(cmd *cobra.Command, flags *rootFlags, dbPath string, empty any) (db *store.Store, handled bool, err error) {
	if dbPath == "" {
		dbPath = defaultDBPath("auvik-cli")
	}
	if _, statErr := os.Stat(dbPath); os.IsNotExist(statErr) {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"no local mirror at %s\nrun: auvik-cli sync --resources tenants,inventory --db %s\n",
			dbPath, dbPath)
		if !wantsHumanTable(cmd.OutOrStdout(), flags) {
			return nil, true, printJSONFiltered(cmd.OutOrStdout(), empty, flags)
		}
		return nil, true, nil
	}
	st, err := store.OpenReadOnlyContext(cmd.Context(), dbPath)
	if err != nil {
		return nil, false, fmt.Errorf("opening database: %w", err)
	}
	return st, false, nil
}

// loadResources drains every row for the first candidate resource_type that has
// data. Drain-first is mandatory: SQLite uses a single connection, so no
// follow-up query may run while this parent *sql.Rows is open.
func loadResources(ctx context.Context, db *store.Store, candidates []string) ([]auvikRow, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(candidates))
	args := make([]any, len(candidates))
	for i, c := range candidates {
		placeholders[i] = "?"
		args[i] = c
	}
	q := fmt.Sprintf(
		`SELECT id, data FROM resources WHERE resource_type IN (%s)`,
		strings.Join(placeholders, ","))

	rows, err := db.DB().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query %v: %w", candidates, err)
	}
	type rawRow struct {
		id   string
		data []byte
	}
	raw := make([]rawRow, 0)
	for rows.Next() {
		var rr rawRow
		if err := rows.Scan(&rr.id, &rr.data); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan %v: %w", candidates, err)
		}
		raw = append(raw, rr)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate %v: %w", candidates, err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close %v: %w", candidates, err)
	}

	out := make([]auvikRow, 0, len(raw))
	for _, rr := range raw {
		var obj map[string]any
		if err := json.Unmarshal(rr.data, &obj); err != nil {
			// A single malformed row must not sink the whole report.
			continue
		}
		out = append(out, auvikRow{ID: rr.id, Data: obj})
	}
	return out, nil
}

// tenantNames maps tenant id -> human label, so reports never print bare ids.
// Falls back to the id itself when tenants have not been synced.
func tenantNames(ctx context.Context, db *store.Store) map[string]string {
	names := map[string]string{}
	rows, err := loadResources(ctx, db, rtTenants)
	if err != nil {
		return names
	}
	for _, r := range rows {
		for _, key := range []string{"domainPrefix", "name", "tenantName"} {
			if v := r.attrString(key); v != "" {
				names[r.ID] = v
				break
			}
		}
	}
	return names
}

func tenantLabel(names map[string]string, id string) string {
	if id == "" {
		return "(unknown)"
	}
	if n, ok := names[id]; ok && n != "" {
		return n
	}
	return id
}

// parseAuvikTime accepts the ISO-8601 shapes Auvik emits, plus bare dates used
// by lifecycle/warranty fields.
func parseAuvikTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// emitLocalReport routes machine formats through the shared filter and leaves
// human rendering to the caller. Returns true when output is done.
//
// csvRows is the flat row slice inside the report envelope. Every command here
// returns an ENVELOPE object (counts + rows), and the shared CSV renderer only
// projects tables and arrays -- handed an object it passes JSON straight
// through, so `--csv` would silently emit JSON. When CSV is requested we emit
// the row slice instead, which is what a spreadsheet actually wants. Pass nil
// when the report has no single natural row set.
func emitLocalReport(cmd *cobra.Command, flags *rootFlags, v any, csvRows any) (bool, error) {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		payload := v
		if flags != nil && flags.csv && csvRows != nil {
			payload = csvRows
		}
		return true, printJSONFiltered(cmd.OutOrStdout(), payload, flags)
	}
	return false, nil
}
