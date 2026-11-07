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

// auvikFamily is one Auvik record family as the local store holds it.
//
// partitions are the `resource_type` values `sync` writes it under: the CURRENT
// partition first (what this build's syncer writes; see syncResourcePath in
// sync.go), then names an older print wrote, so a mirror synced before an
// upgrade is still read until a writable open moves those rows into the
// current partition (see loadResources and auvikLegacyMoves). The press renames partitions between versions: 4.35.1
// moved "inventory" from devices to components, "asm" from apps to clients and
// "auvik-inventory" from v2 devices to v2 discovery status.
//
// types are the JSON:API `type` values Auvik documents for the family's
// records (auvik-v1/v2 OpenAPI). A row is read only when its type is listed,
// so a partition that holds another family under the same name can never leak
// into a report, whatever any print called it.
//
// path is the list endpoint the current partition must sync from;
// TestAuvikFamiliesPinTheirEndpoint fails the build when a reprint points the
// current name somewhere else.
type auvikFamily struct {
	partitions []string
	types      []string
	path       string
}

// current is the partition this build's `sync` writes the family to: the name
// every hint and every sync instruction must use.
func (f auvikFamily) current() string { return f.partitions[0] }

var (
	rtDevices         = auvikFamily{[]string{"inventory-device-info", "inventory"}, []string{"device"}, "/v1/inventory/device/info"}
	rtDeviceDetail    = auvikFamily{[]string{"inventory-device-detail"}, []string{"deviceDetail"}, "/v1/inventory/device/detail"}
	rtDeviceLifecycle = auvikFamily{[]string{"inventory-device-lifecycle"}, []string{"deviceLifecycle"}, "/v1/inventory/device/lifecycle"}
	// v1 lifecycle carries only STATUS enums; the actual end-of-support and
	// end-of-sale DATES exist only on the v2 lifecycle resource.
	rtDeviceLifecycleV2 = auvikFamily{[]string{"auvik-inventory-lifecycle"}, []string{"deviceLifecycle"}, "/v2/api/inventory/device/lifecycle"}
	rtDeviceWarranty    = auvikFamily{[]string{"inventory-device-warranty"}, []string{"deviceWarranty"}, "/v1/inventory/device/warranty"}
	rtConfigurations    = auvikFamily{[]string{"inventory-configuration"}, []string{"configuration"}, "/v1/inventory/configuration"}
	rtEntityAudit       = auvikFamily{[]string{"inventory-entity-audit"}, []string{"entityAudit"}, "/v1/inventory/entity/audit"}
	rtEntityNote        = auvikFamily{[]string{"inventory-entity-note"}, []string{"entityNote"}, "/v1/inventory/entity/note"}
	rtAlerts            = auvikFamily{[]string{"alert"}, []string{"alert"}, "/v1/alert/history/info"}
	rtTenants           = auvikFamily{[]string{"tenants", "tenants-detail"}, []string{"tenant", "tenants"}, "/v1/tenants"}
	rtDiscoveryStatus   = auvikFamily{[]string{"auvik-inventory", "auvik-inventory-discovery-status"}, []string{"deviceDiscoveryStatus"}, "/v2/api/inventory/device/discoveryStatus"}
	rtBilling           = auvikFamily{[]string{"billing"}, []string{"clientUsage"}, "/v1/billing/usage/client"}
	rtASMApps           = auvikFamily{[]string{"asm-app-info", "asm"}, []string{"asmApp"}, "/v1/asm/app/info"}
	rtASMUsers          = auvikFamily{[]string{"asm-user-info"}, []string{"asmUser"}, "/v1/asm/user/info"}
	rtASMLicenses       = auvikFamily{[]string{"asm-license-info"}, []string{"asmLicense"}, "/v1/asm/license/info"}
	rtASMClients        = auvikFamily{[]string{"asm", "asm-client-info"}, []string{"asmClient"}, "/v1/asm/client/info"}
)

// auvikLegacyMoves are the partitions the 4.30 print wrote a family under that
// this build writes another family to (or nothing): their rows of the family's
// types move into the current partition at every writable store open. A listed
// partition that is NOT here is one this build still syncs (tenants-detail).
var auvikLegacyMoves = []store.PartitionMove{
	{From: "inventory", To: rtDevices.current(), Types: rtDevices.types},
	{From: "asm", To: rtASMApps.current(), Types: rtASMApps.types},
	{From: "auvik-inventory-discovery-status", To: rtDiscoveryStatus.current(), Types: rtDiscoveryStatus.types},
	{From: "asm-client-info", To: rtASMClients.current(), Types: rtASMClients.types},
}

func init() { store.LegacyPartitionMoves = auvikLegacyMoves }

// auvikFamilies is every family a novel reads; the partition tests walk it.
var auvikFamilies = map[string]auvikFamily{
	"devices": rtDevices, "device-detail": rtDeviceDetail, "device-lifecycle": rtDeviceLifecycle,
	"device-lifecycle-v2": rtDeviceLifecycleV2, "device-warranty": rtDeviceWarranty,
	"configurations": rtConfigurations, "entity-audit": rtEntityAudit, "entity-note": rtEntityNote,
	"alerts": rtAlerts, "tenants": rtTenants, "discovery-status": rtDiscoveryStatus, "billing": rtBilling,
	"asm-apps": rtASMApps, "asm-users": rtASMUsers, "asm-licenses": rtASMLicenses, "asm-clients": rtASMClients,
}

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
			"no local mirror at %s\nrun: auvik-cli sync --resources tenants,inventory-device-info --db %s\n",
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

// loadResources drains every row of a family, keeping only rows whose JSON:API
// type the family allows, one row per id (the most recently synced copy wins).
//
// It reads every partition the family lists. Rows an older print left under a
// partition this build no longer writes the family to are moved into the
// current partition at the first writable store open (auvikLegacyMoves,
// store.moveLegacyPartitions), which every `sync` performs before it walks, so
// the current partition's reconcile governs them: an unfiltered full sync
// prunes a record Auvik stopped returning, a filtered one prunes nothing. A
// mirror read before any post-upgrade sync still answers from the legacy rows.
//
// Drain-first is mandatory: SQLite uses a single connection, so no follow-up
// query may run while this parent *sql.Rows is open.
func loadResources(ctx context.Context, db *store.Store, family auvikFamily) ([]auvikRow, error) {
	candidates := family.partitions
	if len(candidates) == 0 || len(family.types) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(candidates))
	args := make([]any, 0, len(candidates)+len(family.types))
	for i, c := range candidates {
		placeholders[i] = "?"
		args = append(args, c)
	}
	typePlaceholders := make([]string, len(family.types))
	for i, t := range family.types {
		typePlaceholders[i] = "?"
		args = append(args, t)
	}
	// Allowlist by the record's own documented type; a row with no type, or
	// another family's type stored under a shared partition name, is not read.
	// Newest first, so the first copy of an id kept below is the freshest.
	q := fmt.Sprintf(
		`SELECT id, data FROM resources WHERE resource_type IN (%s) AND json_valid(data) AND json_extract(data, '$.type') IN (%s)
		 ORDER BY synced_at DESC, updated_at DESC`,
		strings.Join(placeholders, ","), strings.Join(typePlaceholders, ","))

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
	seen := make(map[string]bool, len(raw))
	for _, rr := range raw {
		if seen[rr.id] {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(rr.data, &obj); err != nil {
			// A single malformed row must not sink the whole report.
			continue
		}
		seen[rr.id] = true
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
