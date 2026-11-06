// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored shared plumbing for the ImmyBot novel commands: one reader for
// a mirrored computer's identity, whichever resource the row came from. Kept
// in its own file so `generate --force` preserves it.
//
// pp:data-source local

package cli

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"sort"
	"strings"

	"immybot-pp-cli/internal/store"
)

// immyComputerFields names the JSON fields that carry a computer's identity in
// one resource's rows. Each list is tried in order; a field the schema does not
// carry is left empty and is never read.
type immyComputerFields struct {
	id, name, tenantID, tenantName, online, excluded []string
}

func immyFields(names ...string) []string { return names }

// immyComputerShapes maps every mirrored resource whose rows name a computer to
// the fields of the response schema its sync GET returns (spec.json
// components.schemas; the GET is syncResourcePath in sync.go). The names differ
// by resource: ComputerSearch says name and tenant where ComputerListViewModel
// says computerName and tenantName, and on a user-affinity row id is the
// affinity, not the computer. Reading one resource's rows with another's names
// finds nothing, which is how a paged-only mirror reported no such computer.
var immyComputerShapes = map[string]immyComputerFields{
	// GET /api/v1/computers: ComputerSearch.
	"computers": {id: immyFields("id"), name: immyFields("name"), tenantID: immyFields("tenantId"),
		tenantName: immyFields("tenant"), online: immyFields("online"), excluded: immyFields("excludeFromMaintenance")},
	// GET /api/v1/computers/paged: ComputerPageResponse.results, ComputerListViewModel.
	"computers-paged": {id: immyFields("id"), name: immyFields("computerName"), tenantID: immyFields("tenantId"),
		tenantName: immyFields("tenantName"), online: immyFields("isOnline"), excluded: immyFields("excludeFromMaintenance")},
	// GET /api/v1/computers/dx: LoadResult.data, which the spec leaves untyped.
	// Read with the computer-list names, ComputerListViewModel's first and then
	// ComputerSearch's; the JSON type check keeps a nested object (the tenant
	// of a GetComputerResponse) from ever reading as a name.
	"computers-dx": {id: immyFields("id"), name: immyFields("computerName", "name"), tenantID: immyFields("tenantId"),
		tenantName: immyFields("tenantName", "tenant"), online: immyFields("isOnline", "online"),
		excluded: immyFields("excludeFromMaintenance")},
	// GET /api/v1/computers/agent-status: ComputerAgentStatusDto.
	"computers-agent-status": {id: immyFields("id"), name: immyFields("computerName"), tenantID: immyFields("tenantId"),
		tenantName: immyFields("tenantName")},
	// GET /api/v1/computers/onboarding: GetOnboardingComputerResponse, no tenantId.
	"computers-onboarding": {id: immyFields("id"), name: immyFields("computerName"), tenantName: immyFields("tenantName"),
		online: immyFields("isOnline")},
	// GET /api/v1/computers/inventory: DxComputerInventoryScriptResult.
	"computers-inventory": {id: immyFields("computerId"), name: immyFields("computerName"), tenantID: immyFields("tenantId"),
		tenantName: immyFields("tenantName")},
	// GET /api/v1/computers/user-affinities: ComputerUserAffinityResponse.
	"computers-user-affinities": {id: immyFields("computerId"), name: immyFields("computerName"), tenantID: immyFields("tenantId"),
		tenantName: immyFields("tenantName")},
	// GET /api/v1/computers/my: MyComputerResponse, no tenant.
	"computers-my": {id: immyFields("id"), name: immyFields("name"), online: immyFields("isOnline")},
	// GET /api/v1/tenants/software-from-inventory/dx: DetectedComputerSoftwareResponse.
	"tenants-software-from-inventory-dx": {id: immyFields("computerId"), name: immyFields("computerName"),
		tenantID: immyFields("tenantId"), tenantName: immyFields("tenantName")},
	// GET /api/v1/maintenance-actions/{dx,maintenance-item,version}: GetMaintenanceActionResponse.
	"maintenance-actions":                  immyActionComputerFields,
	"maintenance-actions-maintenance-item": immyActionComputerFields,
	"maintenance-actions-version":          immyActionComputerFields,
}

var immyActionComputerFields = immyComputerFields{id: immyFields("computerId"), name: immyFields("computerName"),
	tenantID: immyFields("tenantId"), tenantName: immyFields("tenantName")}

// immyProviderLinkComputerFields is GetProviderLinkResponse.computers[], each a
// GetComputerResponse.
var immyProviderLinkComputerFields = immyComputerFields{id: immyFields("id"), name: immyFields("computerName"),
	tenantID: immyFields("tenantId"), tenantName: immyFields("tenantName"), online: immyFields("isOnline"),
	excluded: immyFields("excludeFromMaintenance")}

// immyComputerRosterTypes are the computer-list resources: their rows are the
// fleet roster, so a count of local computers reads these.
var immyComputerRosterTypes = []string{"computers", "computers-paged", "computers-dx"}

// immyComputerIdentityTypes adds the per-computer resources that name a
// computer and its tenant without being the roster. Looking up one computer
// reads all of them, roster first, so a field one source lacks (computers-my
// carries no tenant) is filled from another.
var immyComputerIdentityTypes = append(append([]string{}, immyComputerRosterTypes...),
	"computers-agent-status", "computers-onboarding", "computers-inventory", "computers-user-affinities", "computers-my")

func immyComputerShape(resourceType string) immyComputerFields {
	shape, ok := immyComputerShapes[resourceType]
	if !ok {
		panic(fmt.Sprintf("immybot: no computer shape declared for resource %q", resourceType))
	}
	return shape
}

// immyJSONPath is the json_extract path of one top-level field.
func immyJSONPath(field string) string { return "'$." + field + "'" }

// immySQLFirst reads, as text, the first of the fields whose JSON type is one
// of types: a number as its digits, a boolean ("true"/"false" in types) as true
// or false. An empty string counts as absent, so the next field is tried.
func immySQLFirst(fields []string, types ...string) string {
	if len(fields) == 0 {
		return "NULL"
	}
	boolean := len(types) > 0 && (types[0] == "true" || types[0] == "false")
	quoted := make([]string, len(types))
	for i, t := range types {
		quoted[i] = "'" + t + "'"
	}
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		p := immyJSONPath(f)
		if boolean {
			parts = append(parts, fmt.Sprintf("CASE json_type(data,%s) WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' END", p))
			continue
		}
		parts = append(parts, fmt.Sprintf("CASE WHEN json_type(data,%s) IN (%s) THEN NULLIF(CAST(json_extract(data,%s) AS TEXT), '') END",
			p, strings.Join(quoted, ","), p))
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return "COALESCE(" + strings.Join(parts, ", ") + ")"
}

// immyComputerSelect is the SELECT list (computer id, name, tenant id, tenant
// name, online, excluded from maintenance) that reads one resource's rows with
// that resource's own field names. An id is read only when it is a JSON integer
// or text, a name only when it is text, a flag only when it is a boolean, so a
// nested object never reads as a value; a field the schema lacks reads NULL.
// Scan it into an immyComputerScan.
func immyComputerSelect(resourceType string) string {
	shape := immyComputerShape(resourceType)
	return strings.Join([]string{
		immySQLFirst(shape.id, "integer", "text"),
		immySQLFirst(shape.name, "text"),
		immySQLFirst(shape.tenantID, "integer", "text"),
		immySQLFirst(shape.tenantName, "text"),
		immySQLFirst(shape.online, "true", "false"),
		immySQLFirst(shape.excluded, "true", "false"),
	}, ", ")
}

// immyComputerRef is one computer's identity as a row names it. Online and
// Excluded are "true", "false", or "" when no row says.
type immyComputerRef struct {
	ID, Name, TenantID, TenantName string
	Online, Excluded               string
}

// immyComputerScan receives the columns immyComputerSelect produces.
type immyComputerScan struct {
	id, name, tenantID, tenantName, online, excluded sql.NullString
}

func (s *immyComputerScan) dest() []any {
	return []any{&s.id, &s.name, &s.tenantID, &s.tenantName, &s.online, &s.excluded}
}

func (s *immyComputerScan) ref() immyComputerRef {
	return immyComputerRef{
		ID: strings.TrimSpace(nullStr(s.id)), Name: nullStr(s.name),
		TenantID: strings.TrimSpace(nullStr(s.tenantID)), TenantName: nullStr(s.tenantName),
		Online: nullStr(s.online), Excluded: nullStr(s.excluded),
	}
}

// immyComputerFromObject reads a decoded object (a provider link's computer)
// with the given field names and the same type rules as immyComputerSelect.
func immyComputerFromObject(fields immyComputerFields, obj map[string]any) immyComputerRef {
	first := func(names []string, accept func(any) (string, bool)) string {
		for _, n := range names {
			if v, ok := accept(obj[n]); ok {
				return v
			}
		}
		return ""
	}
	id := func(v any) (string, bool) {
		switch t := v.(type) {
		case string:
			return strings.TrimSpace(t), strings.TrimSpace(t) != ""
		case float64:
			if t == float64(int64(t)) {
				return anyToString(t), true
			}
		case interface{ String() string }:
			return t.String(), true
		}
		return "", false
	}
	text := func(v any) (string, bool) {
		s, ok := v.(string)
		return s, ok && s != ""
	}
	flag := func(v any) (string, bool) {
		b, ok := v.(bool)
		return fmt.Sprintf("%t", b), ok
	}
	return immyComputerRef{
		ID: first(fields.id, id), Name: first(fields.name, text),
		TenantID: first(fields.tenantID, id), TenantName: first(fields.tenantName, text),
		Online: first(fields.online, flag), Excluded: first(fields.excluded, flag),
	}
}

// immyComputerJoinFor matches one resource's rows to a computer by the
// computer id that resource's schema documents (cast, so the integer the vendor
// sends equals the text id), and by nothing else. A row with no id
// (GetMaintenanceActionResponse.computerId is nullable) joins no computer:
// names repeat across tenants and change on rename, so a name never stands for
// a computer. immyComputerNoID counts those rows instead. It returns the
// clause and its bind.
func immyComputerJoinFor(resourceType, computerID string) (string, []any) {
	return "CAST(json_extract(data," + immyJSONPath(immyComputerShape(resourceType).id[0]) + ") AS TEXT) = ?", []any{computerID}
}

// immyComputerNoID matches one resource's rows that carry no usable computer
// id (absent, null, or not an integer or text), the rows no computer can claim.
func immyComputerNoID(resourceType string) string {
	return immySQLFirst(immyComputerShape(resourceType).id, "integer", "text") + " IS NULL"
}

// immyComputerRoster is the mirrored computers of a set of resources, merged by
// computer id. For each field the most recently synced row that carries a
// value wins, so a later sync of any one resource replaces what an older sync
// of another left; a row with no value (a null isOnline) never hides one, and
// rows synced in the same second keep the order read. Every name a computer
// carries anywhere still finds it.
type immyComputerRoster struct {
	byID  map[string]*immyComputerRef
	names map[string]map[string]struct{} // computer id -> lowercased names
	order []string
	// seen is every row read for a computer id, in the order read.
	seen map[string][]immyComputerSeen
	// Unidentified counts rows that carry no computer id: they name no
	// computer the roster can join on, so they are left out of it.
	Unidentified int
}

// immyComputerSeen is one resource row's reading of a computer.
type immyComputerSeen struct {
	source string
	ref    immyComputerRef
	at     float64 // julianday(synced_at); 0 when the row records none
}

// immyLoadComputers reads the given resources' rows into one roster. A mirror
// without the resources table yields an empty roster.
func immyLoadComputers(ctx context.Context, db *store.Store, resourceTypes []string) (immyComputerRoster, error) {
	r := immyComputerRoster{byID: map[string]*immyComputerRef{}, names: map[string]map[string]struct{}{},
		seen: map[string][]immyComputerSeen{}}
	for _, rt := range resourceTypes {
		rows, err := db.DB().QueryContext(ctx,
			`SELECT `+immyComputerSelect(rt)+`, COALESCE(julianday(synced_at), 0) FROM resources WHERE resource_type = ? ORDER BY id`, rt)
		if err != nil {
			if immyNoSuchTableErr(err) {
				return r, nil
			}
			return r, fmt.Errorf("querying %s: %w", rt, err)
		}
		for rows.Next() {
			var s immyComputerScan
			var at float64
			if err := rows.Scan(append(s.dest(), &at)...); err != nil {
				_ = rows.Close()
				return r, fmt.Errorf("scanning %s: %w", rt, err)
			}
			r.add(immyComputerSeen{source: rt, ref: s.ref(), at: at})
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return r, fmt.Errorf("iterating %s: %w", rt, err)
		}
		if err := rows.Close(); err != nil {
			return r, fmt.Errorf("closing %s: %w", rt, err)
		}
	}
	r.merge()
	return r, nil
}

func (r *immyComputerRoster) add(s immyComputerSeen) {
	c := s.ref
	if c.ID == "" {
		r.Unidentified++
		return
	}
	if _, ok := r.byID[c.ID]; !ok {
		r.byID[c.ID] = &immyComputerRef{ID: c.ID}
		r.names[c.ID] = map[string]struct{}{}
		r.order = append(r.order, c.ID)
	}
	r.seen[c.ID] = append(r.seen[c.ID], s)
	if n := strings.ToLower(strings.TrimSpace(c.Name)); n != "" {
		r.names[c.ID][n] = struct{}{}
	}
}

// merge fills each computer's fields from its rows, newest sync first.
func (r *immyComputerRoster) merge() {
	for id, seen := range r.seen {
		fresh := slices.Clone(seen)
		sort.SliceStable(fresh, func(i, j int) bool { return fresh[i].at > fresh[j].at })
		cur := r.byID[id]
		fill := func(dst *string, v string) {
			if strings.TrimSpace(*dst) == "" {
				*dst = v
			}
		}
		for _, s := range fresh {
			fill(&cur.Name, s.ref.Name)
			fill(&cur.TenantID, s.ref.TenantID)
			fill(&cur.TenantName, s.ref.TenantName)
			fill(&cur.Online, s.ref.Online)
			fill(&cur.Excluded, s.ref.Excluded)
		}
	}
}

// sources is every row read for one computer, each with its resource type.
func (r immyComputerRoster) sources(id string) []immyComputerSeen { return r.seen[id] }

// list returns every computer in the order first read.
func (r immyComputerRoster) list() []immyComputerRef {
	out := make([]immyComputerRef, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, *r.byID[id])
	}
	return out
}

// find resolves an operator's computer argument: an exact computer id, else a
// name, ignoring case. A name that two computers carry resolves to neither; it
// returns their ids so the caller can ask for one instead of guessing.
func (r immyComputerRoster) find(needle string) (immyComputerRef, []string, bool) {
	needle = strings.TrimSpace(needle)
	if c, ok := r.byID[needle]; ok {
		return *c, nil, true
	}
	want := strings.ToLower(needle)
	matches := make([]string, 0, 1)
	for _, id := range r.order {
		if _, ok := r.names[id][want]; ok {
			matches = append(matches, id)
		}
	}
	if len(matches) == 1 {
		return *r.byID[matches[0]], nil, true
	}
	sort.Strings(matches)
	return immyComputerRef{}, matches, false
}

// immyComputerNotFound is the note for an argument the roster cannot resolve.
func immyComputerNotFound(needle string, ambiguous []string) string {
	if len(ambiguous) > 1 {
		return fmt.Sprintf("%q names %d computers in the local mirror (ids %s); pass the computer id",
			needle, len(ambiguous), strings.Join(ambiguous, ", "))
	}
	return fmt.Sprintf("no computer with id or name %q in the local mirror; run 'immybot-cli sync --resources computers'", needle)
}

// immyComputerTenantName names a computer's tenant: the name its rows carry,
// else the mirrored tenant's own name (GetTenantResponse.name) for its id.
func immyComputerTenantName(ctx context.Context, db *store.Store, c immyComputerRef) (string, error) {
	if strings.TrimSpace(c.TenantName) != "" || c.TenantID == "" {
		return c.TenantName, nil
	}
	names, err := immyTenantNames(ctx, db)
	if err != nil {
		return "", err
	}
	return names[c.TenantID], nil
}

// immyTenantIndex groups rows by tenant. A tenant is its tenantId with its
// tenantName: two tenants that share a name stay two. tenantName is nullable
// (DetectedComputerSoftwareResponse), so a row with a null name joins the one
// named tenant its id carries, and stays "tenant <id>" only when the id carries
// no name or more than one. Record every row with add before reading any key.
type immyTenantIndex struct {
	names map[string]map[string]struct{} // tenant id -> non-empty names seen
}

func newImmyTenantIndex() *immyTenantIndex {
	return &immyTenantIndex{names: map[string]map[string]struct{}{}}
}

// add records one row's tenant.
func (x *immyTenantIndex) add(tenantID, tenantName string) {
	tenantID, tenantName = strings.TrimSpace(tenantID), strings.TrimSpace(tenantName)
	if tenantID == "" || tenantName == "" {
		return
	}
	if x.names[tenantID] == nil {
		x.names[tenantID] = map[string]struct{}{}
	}
	x.names[tenantID][tenantName] = struct{}{}
}

// key is the group of one row's tenant; "" when the row names no tenant.
func (x *immyTenantIndex) key(tenantID, tenantName string) string {
	tenantID, tenantName = strings.TrimSpace(tenantID), strings.TrimSpace(tenantName)
	if tenantID == "" && tenantName == "" {
		return ""
	}
	if tenantName == "" && len(x.names[tenantID]) == 1 {
		for n := range x.names[tenantID] {
			tenantName = n
		}
	}
	return tenantID + "\x00" + tenantName
}

// label names a tenant key: its name, else "tenant <id>".
func (x *immyTenantIndex) label(key string) string {
	id, name, _ := strings.Cut(key, "\x00")
	return immyTenantLabel(name, id)
}

// id is a tenant key's tenant id, "" for a row that carried only a name.
func (x *immyTenantIndex) id(key string) string {
	id, _, _ := strings.Cut(key, "\x00")
	return id
}
