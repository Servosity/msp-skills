// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Shared local-store helpers for the footprint, offboard, e911 audit,
// numbers inventory and devices health novel commands (slice prefix "local").
//
// Every loader reads the generic resources table that sync always writes:
// resources.resource_type is the sync resource name and resources.data is the
// stored row for one list item, decoded here with the field names of the spec
// response schema. The stored row is whatever the store's upsert kept for
// that item (it is not guaranteed to be the raw response body), so loaders
// tolerate missing fields. Typed tables are a union of several response
// shapes and are not used here. Whether a resource counts as synced comes
// from sync_state (see localSyncedSet), never from row presence.

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"zoom-phone-pp-cli/internal/store"
)

const localCLIName = "zoom-phone-cli"

// localFlexBool decodes true/false, "true"/"false" and 1/0.
type localFlexBool bool

func (f *localFlexBool) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.ToLower(strings.TrimSpace(string(b))), `"`)
	*f = localFlexBool(s == "true" || s == "1")
	return nil
}

type localRef struct {
	ID   zoomFlexString `json:"id"`
	Name zoomFlexString `json:"name"`
}

type localPhoneNumberRef struct {
	ID     zoomFlexString `json:"id"`
	Number zoomFlexString `json:"number"`
	Source zoomFlexString `json:"source"`
	Status zoomFlexString `json:"status"`
}

type localCallingPlan struct {
	Type             zoomFlexString `json:"type"`
	Name             zoomFlexString `json:"name"`
	BillingAccountID zoomFlexString `json:"billing_account_id"`
}

// localUser is one item of GET /phone/users (.users[]).
type localUser struct {
	ID               zoomFlexString        `json:"id"`
	Email            zoomFlexString        `json:"email"`
	Name             zoomFlexString        `json:"name"`
	ExtensionID      zoomFlexString        `json:"extension_id"`
	ExtensionNumber  zoomFlexString        `json:"extension_number"`
	PhoneUserID      zoomFlexString        `json:"phone_user_id"`
	Status           zoomFlexString        `json:"status"`
	ActivationStatus zoomFlexString        `json:"activation_status"`
	Department       zoomFlexString        `json:"department"`
	CostCenter       zoomFlexString        `json:"cost_center"`
	Site             localRef              `json:"site"`
	PhoneNumbers     []localPhoneNumberRef `json:"phone_numbers"`
	CallingPlans     []localCallingPlan    `json:"calling_plans"`
}

// localUserInactiveReason applies the shared inactive rule
// (zoomUserInactiveReason) to a synced phone user row.
func localUserInactiveReason(u localUser) string {
	return zoomUserInactiveReason(u.Status.String(), u.ActivationStatus.String())
}

// localGroup covers the list items of call_queues, shared_line_groups,
// group_call_pickup and monitoring_groups.
type localGroup struct {
	ID                    zoomFlexString  `json:"id"`
	Name                  zoomFlexString  `json:"name"`
	DisplayName           zoomFlexString  `json:"display_name"`
	ExtensionID           zoomFlexString  `json:"extension_id"`
	ExtensionNumber       zoomFlexString  `json:"extension_number"`
	Status                zoomFlexString  `json:"status"`
	Type                  zoomFlexString  `json:"type"`
	Site                  localRef        `json:"site"`
	MemberCount           *zoomFlexString `json:"member_count"`
	MonitorMembersCount   *zoomFlexString `json:"monitor_members_count"`
	MonitoredMembersCount *zoomFlexString `json:"monitored_members_count"`
}

func (g localGroup) label() string {
	if s := g.Name.String(); s != "" {
		return s
	}
	return g.DisplayName.String()
}

type localDeviceAssignee struct {
	ID              zoomFlexString `json:"id"`
	Name            zoomFlexString `json:"name"`
	ExtensionNumber zoomFlexString `json:"extension_number"`
	ExtensionType   zoomFlexString `json:"extension_type"`
	ExtensionID     zoomFlexString `json:"extension_id"`
}

// localDevice is one item of GET /phone/devices (.devices[]).
type localDevice struct {
	ID                  zoomFlexString        `json:"id"`
	DisplayName         zoomFlexString        `json:"display_name"`
	DeviceType          zoomFlexString        `json:"device_type"`
	MacAddress          zoomFlexString        `json:"mac_address"`
	Status              zoomFlexString        `json:"status"`
	FirmwareVersion     zoomFlexString        `json:"firmware_version"`
	ProvisionTemplateID *zoomFlexString       `json:"provision_template_id"`
	Site                localRef              `json:"site"`
	Assignee            *localDeviceAssignee  `json:"assignee"`
	Assignees           []localDeviceAssignee `json:"assignees"`

	// ambiguousSources is set when rows that disagree on assignment (e.g.
	// the assigned and unassigned partitions) were written at the same
	// second, so neither can be trusted over the other.
	ambiguousSources []string

	// localSource is the sync resource the row came from (assigned,
	// unassigned, devices) or "common_areas" for a desk phone known only
	// from a common area's desk_phones[].
	localSource string
}

// localDeviceAssigneeList merges assignee and assignees[] (deduped by id).
func localDeviceAssigneeList(d localDevice) []localDeviceAssignee {
	out := make([]localDeviceAssignee, 0, len(d.Assignees)+1)
	seen := map[string]bool{}
	add := func(a localDeviceAssignee) {
		key := a.ID.String() + "|" + a.ExtensionID.String()
		if a.ID.String() == "" && a.ExtensionID.String() == "" {
			return
		}
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, a)
	}
	for _, a := range d.Assignees {
		add(a)
	}
	if d.Assignee != nil {
		dup := false
		for _, a := range out {
			if d.Assignee.ID.String() != "" && a.ID == d.Assignee.ID {
				dup = true
			}
		}
		if !dup {
			add(*d.Assignee)
		}
	}
	return out
}

type localNumberAssignee struct {
	ID              zoomFlexString `json:"id"`
	Name            zoomFlexString `json:"name"`
	Type            zoomFlexString `json:"type"`
	ExtensionNumber zoomFlexString `json:"extension_number"`
}

// localNumber is one item of GET /phone/numbers (.phone_numbers[]).
type localNumber struct {
	ID                     zoomFlexString       `json:"id"`
	Number                 zoomFlexString       `json:"number"`
	DisplayName            zoomFlexString       `json:"display_name"`
	Location               zoomFlexString       `json:"location"`
	NumberType             zoomFlexString       `json:"number_type"`
	Source                 zoomFlexString       `json:"source"`
	Status                 zoomFlexString       `json:"status"`
	Assignee               *localNumberAssignee `json:"assignee"`
	Site                   localRef             `json:"site"`
	Carrier                *localRef            `json:"carrier"`
	SipGroup               *localRef            `json:"sip_group"`
	EmergencyAddressStatus *zoomFlexString      `json:"emergency_address_status"`

	storageID string
}

// localNumberSourceClass returns internal | external | byoc | unknown.
// BYOC numbers are the ones carrying carrier / sip_group details, which the
// spec documents as present when the list is requested with type=byoc.
func localNumberSourceClass(n localNumber) string {
	if n.Carrier != nil || n.SipGroup != nil || n.EmergencyAddressStatus != nil {
		return "byoc"
	}
	if s := strings.ToLower(n.Source.String()); s != "" {
		return s
	}
	return "unknown"
}

type localSite struct {
	ID       zoomFlexString `json:"id"`
	Name     zoomFlexString `json:"name"`
	SiteCode zoomFlexString `json:"site_code"`
}

type localAddressOwner struct {
	ID              zoomFlexString `json:"id"`
	Name            zoomFlexString `json:"name"`
	ExtensionNumber zoomFlexString `json:"extension_number"`
}

// localEmergencyAddress is one item of GET /phone/emergency_addresses.
type localEmergencyAddress struct {
	ID           zoomFlexString     `json:"id"`
	AddressLine1 zoomFlexString     `json:"address_line1"`
	AddressLine2 zoomFlexString     `json:"address_line2"`
	City         zoomFlexString     `json:"city"`
	StateCode    zoomFlexString     `json:"state_code"`
	Zip          zoomFlexString     `json:"zip"`
	Country      zoomFlexString     `json:"country"`
	BusinessName zoomFlexString     `json:"business_name"`
	IsDefault    localFlexBool      `json:"is_default"`
	Level        zoomFlexString     `json:"level"`
	Status       zoomFlexString     `json:"status"`
	Owner        *localAddressOwner `json:"owner"`
	Site         localRef           `json:"site"`
}

func (a localEmergencyAddress) oneLine() string {
	parts := []string{}
	for _, p := range []zoomFlexString{a.AddressLine1, a.AddressLine2, a.City, a.StateCode, a.Zip, a.Country} {
		if s := p.String(); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ", ")
}

type localDeskPhone struct {
	ID          zoomFlexString `json:"id"`
	DisplayName zoomFlexString `json:"display_name"`
	DeviceType  zoomFlexString `json:"device_type"`
	Status      zoomFlexString `json:"status"`
}

// localCommonArea is one item of GET /phone/common_areas.
type localCommonArea struct {
	ID              zoomFlexString        `json:"id"`
	DisplayName     zoomFlexString        `json:"display_name"`
	ExtensionNumber zoomFlexString        `json:"extension_number"`
	Status          zoomFlexString        `json:"status"`
	Site            localRef              `json:"site"`
	PhoneNumbers    []localPhoneNumberRef `json:"phone_numbers"`
	CallingPlans    []localCallingPlan    `json:"calling_plans"`
	DeskPhones      []localDeskPhone      `json:"desk_phones"`
}

// localLocation is one item of GET /phone/locations (emergency service
// locations).
type localLocation struct {
	ID   zoomFlexString `json:"id"`
	Name zoomFlexString `json:"name"`
	Site localRef       `json:"site"`

	// storageID is the resources.id of the row, which walk row sets use.
	storageID string
}

// localLoadLocations loads emergency service locations with their storage
// ids, which per-site walks use to keep only the rows that walk returned.
func localLoadLocations(q localQuerier) ([]localLocation, error) {
	raws, err := localScanRaw(q, "locations")
	if err != nil {
		return nil, err
	}
	out := make([]localLocation, 0, len(raws))
	for _, raw := range raws {
		var l localLocation
		if json.Unmarshal(raw.Data, &l) == nil {
			l.storageID = raw.ID
			out = append(out, l)
		}
	}
	return out, nil
}

// localLoadNumbers loads numbers with their storage ids (the BYOC walk row
// set is keyed by them).
func localLoadNumbers(q localQuerier) ([]localNumber, error) {
	raws, err := localScanRaw(q, "numbers")
	if err != nil {
		return nil, err
	}
	out := make([]localNumber, 0, len(raws))
	for _, raw := range raws {
		var n localNumber
		if json.Unmarshal(raw.Data, &n) == nil {
			n.storageID = raw.ID
			out = append(out, n)
		}
	}
	return out, nil
}

// localLocationCoverage says which sites' emergency service locations are
// known completely.
type localLocationCoverage struct {
	// all: an unfiltered full locations sync covers every site.
	all bool
	// sites maps a covered site id to the row ids its per-site walk wrote;
	// only those rows count for that site (others were not returned).
	sites map[string]map[string]bool
	// uncovered lists synced site ids with no usable per-site sync.
	uncovered []string
	// pending lists the uncovered sites whose latest walk attempt did not
	// complete (failed or interrupted).
	pending []string
}

// complete requires affirmative coverage of every site: an empty uncovered
// list alone is not proof (coverage returns early, with nothing listed, when
// the sites sync itself is not usable).
func (lc localLocationCoverage) complete(sites []localSite) bool {
	if lc.all {
		return true
	}
	if len(sites) == 0 || len(lc.uncovered) != 0 {
		return false
	}
	for _, s := range sites {
		if _, ok := lc.sites[s.ID.String()]; !ok {
			return false
		}
	}
	return true
}

// localLocationCoverageFor decides location coverage. GET /phone/locations
// requires site_id when Multiple Sites is enabled, so an account-wide walk
// is impossible there: locations are covered either by an unfiltered full
// locations sync (single-site accounts), or site by site, when every site in
// the fully synced sites table has a completed (not page-capped) locations
// sync filtered exactly by its site_id whose checkpoint is not older than
// the sites sync. Per-site syncs are additive: press 4.34.2 never prunes a
// filtered sync, so each site's record stands beside the others.
func localLocationCoverageFor(q localQuerier, cov localCoverage, sites []localSite) localLocationCoverage {
	lc := localLocationCoverage{sites: map[string]map[string]bool{}, uncovered: make([]string, 0), pending: make([]string, 0)}
	if cov.synced["locations"] {
		lc.all = true
		return lc
	}
	if !cov.synced["sites"] {
		return lc
	}
	// A sites checkpoint that cannot be read accepts no walk: an empty
	// sitesAt would otherwise compare older walks as current.
	sitesScope, sitesRecorded := localReadSyncScope(q)["sites"]
	sitesAt := sitesScope.syncedAt
	walks := localReadWalks(q, "locations")
	for _, s := range sites {
		id := s.ID.String()
		w, ok := walks["site_id="+id]
		if ok && w.usable() && sitesRecorded && w.syncedAt >= sitesAt {
			lc.sites[id] = w.rows
			continue
		}
		if ok && w.pending {
			lc.pending = append(lc.pending, id)
		}
		lc.uncovered = append(lc.uncovered, id)
	}
	sort.Strings(lc.uncovered)
	sort.Strings(lc.pending)
	return lc
}

// localPerSiteLocationsHint is the per-site sync form for multi-site accounts.
func localPerSiteLocationsHint(siteIDs []string) string {
	ex := "<site id>"
	if len(siteIDs) > 0 {
		ex = siteIDs[0]
	}
	return fmt.Sprintf("%s sync --resources locations --resource-param locations:site_id=%s (once per site; a filtered sync never prunes)", localCLIName, ex)
}

// localQuerier is what every local reader needs; *sql.DB and *sql.Tx both
// satisfy it. Commands pass the *sql.Tx of localReadSnapshot so rows,
// sync_state and the coverage tables are all read from one snapshot.
type localQuerier interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// localSnapshotHook lets tests interleave a writer between reads; nil in
// production. It fires after each resource scan.
var localSnapshotHook func(resourceType string)

// localReadSnapshot runs fn with every query inside ONE read transaction, so
// a sync committing meanwhile can never be seen half-applied (old numbers
// joined against new users). The store uses a rollback journal, so a
// concurrent sync's commit simply waits for this short read to finish.
func localReadSnapshot(ctx context.Context, db *store.Store, fn func(q localQuerier) error) error {
	tx, err := db.DB().BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("starting a read snapshot of the local store: %w", err)
	}
	defer tx.Rollback()
	return fn(tx)
}

type localRawRow struct {
	ResourceType string
	ID           string
	UpdatedAt    string
	Data         []byte
}

func localMissingTable(err error) bool {
	for err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return true
		}
		err = errors.Unwrap(err)
	}
	return false
}

// localScanRaw drains every resources row of one resource_type before
// returning, so callers can run follow-up queries on the same handle.
func localScanRaw(q localQuerier, resourceType string) ([]localRawRow, error) {
	out := make([]localRawRow, 0)
	if q == nil {
		return out, nil
	}
	rows, err := q.Query(`SELECT id, COALESCE(CAST(updated_at AS TEXT), CAST(synced_at AS TEXT), ''), data FROM resources WHERE resource_type = ? ORDER BY id`, resourceType)
	if err != nil {
		if localMissingTable(err) {
			return out, nil
		}
		return nil, fmt.Errorf("reading %s from local store: %w", resourceType, err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, updated, data sql.NullString
		if err := rows.Scan(&id, &updated, &data); err != nil {
			return nil, fmt.Errorf("scanning %s row: %w", resourceType, err)
		}
		if !data.Valid {
			continue
		}
		out = append(out, localRawRow{ResourceType: resourceType, ID: id.String, UpdatedAt: updated.String, Data: []byte(data.String)})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading %s rows: %w", resourceType, err)
	}
	if localSnapshotHook != nil {
		_ = rows.Close() // test hook only: release the cursor before the hook writes
		localSnapshotHook(resourceType)
	}
	return out, nil
}

// localLoad decodes every row of the given resource types into T. Rows that
// do not decode are skipped.
func localLoad[T any](q localQuerier, resourceTypes ...string) ([]T, error) {
	out := make([]T, 0)
	for _, rt := range resourceTypes {
		raws, err := localScanRaw(q, rt)
		if err != nil {
			return nil, err
		}
		for _, raw := range raws {
			var v T
			if json.Unmarshal(raw.Data, &v) == nil {
				out = append(out, v)
			}
		}
	}
	return out, nil
}

func localLoadUsers(q localQuerier) ([]localUser, error) {
	users, err := localLoad[localUser](q, "users")
	if err != nil {
		return nil, err
	}
	out := users[:0]
	for _, u := range users {
		if u.ID.String() != "" {
			out = append(out, u)
		}
	}
	return out, nil
}

func (d localDevice) ambiguous() bool { return len(d.ambiguousSources) > 0 }

// localDeviceRowAssigned reports whether a device row says the device is
// assigned: rows from the assigned partition are, rows from the unassigned
// partition are not, and a "devices" row is judged by its assignees.
func localDeviceRowAssigned(d localDevice) bool {
	switch d.localSource {
	case "assigned":
		return true
	case "unassigned":
		return false
	}
	return len(localDeviceAssigneeList(d)) > 0
}

// localLoadDevices builds the device set from the two reconciled device
// partitions synced by default: /phone/devices?type=assigned ("assigned")
// and ?type=unassigned ("unassigned"). Those partitions are the coverage
// basis, so they alone decide which devices exist: a row stored under the
// generic "devices" resource (e.g. `sync --resources devices --param
// type=assigned`) is never reconciled with them, so it can only fill empty
// fields of a device a partition holds and is otherwise ignored.
//
// When one device id is in both partitions the strictly newer row wins.
// Rows that disagree on assignment with the same timestamp (sync writes at
// second resolution) are not resolved by partition order: the device is kept
// with ambiguousSources set, so callers can flag it instead of silently
// treating it as assigned or unassigned.
func localLoadDevices(q localQuerier) ([]localDevice, error) {
	type pick struct {
		updated string
		dev     localDevice
	}
	byID := map[string]pick{}
	order := []string{}
	for _, rt := range []string{"assigned", "unassigned"} {
		raws, err := localScanRaw(q, rt)
		if err != nil {
			return nil, err
		}
		for _, raw := range raws {
			var d localDevice
			if json.Unmarshal(raw.Data, &d) != nil || d.ID.String() == "" {
				continue
			}
			d.localSource = rt
			id := d.ID.String()
			prev, ok := byID[id]
			if !ok {
				order = append(order, id)
				byID[id] = pick{updated: raw.UpdatedAt, dev: d}
				continue
			}
			switch {
			case raw.UpdatedAt > prev.updated:
				byID[id] = pick{updated: raw.UpdatedAt, dev: d}
			case raw.UpdatedAt == prev.updated && localDeviceRowAssigned(d) != localDeviceRowAssigned(prev.dev):
				// Keep the row that names an assignee so callers can see
				// whose device is in doubt, and record every source.
				rep := prev.dev
				if localDeviceRowAssigned(d) {
					rep = d
				}
				sources := append([]string{}, prev.dev.ambiguousSources...)
				if len(sources) == 0 {
					sources = append(sources, prev.dev.localSource)
				}
				sources = append(sources, d.localSource)
				sort.Strings(sources)
				rep.ambiguousSources = sources
				byID[id] = pick{updated: raw.UpdatedAt, dev: rep}
			}
		}
	}
	generic, err := localScanRaw(q, "devices")
	if err != nil {
		return nil, err
	}
	for _, raw := range generic {
		var g localDevice
		if json.Unmarshal(raw.Data, &g) != nil {
			continue
		}
		p, ok := byID[g.ID.String()]
		if !ok {
			continue // not in a reconciled partition: not a device we can vouch for
		}
		p.dev = localEnrichDevice(p.dev, g)
		byID[g.ID.String()] = p
	}
	out := make([]localDevice, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id].dev)
	}
	return out, nil
}

// localEnrichDevice fills empty descriptive fields of a partition row from a
// generic "devices" row. Membership, assignment, status and the assignees
// always come from the partition row.
func localEnrichDevice(d, g localDevice) localDevice {
	if d.DisplayName == "" {
		d.DisplayName = g.DisplayName
	}
	if d.DeviceType == "" {
		d.DeviceType = g.DeviceType
	}
	if d.MacAddress == "" {
		d.MacAddress = g.MacAddress
	}
	if d.FirmwareVersion == "" {
		d.FirmwareVersion = g.FirmwareVersion
	}
	if d.ProvisionTemplateID == nil && g.ProvisionTemplateID != nil {
		v := *g.ProvisionTemplateID
		d.ProvisionTemplateID = &v
	}
	return d
}

// localSyncedSet reports, per resource type, whether the account is covered
// by a completed, unfiltered sync of it (see localCoverageFor).
func localSyncedSet(q localQuerier, resourceTypes ...string) map[string]bool {
	return localCoverageFor(q, nil, resourceTypes...).synced
}

// localCoverage says which resources cover the whole account, and why each
// other one does not.
type localCoverage struct {
	synced  map[string]bool
	reasons map[string]string
}

// localCoverageFor reports, per resource type, whether the local rows cover
// the account. That needs a completed sync: its sync_state row has
// last_attempt_complete = 1 and a non-zero last_synced_at. Rows alone do
// not count. An interrupted sync leaves partial rows with
// last_attempt_complete = 0, and a `sync --full` first writes a zero-time
// (0001-01-01) checkpoint, so neither reads as synced. A synced resource
// with zero rows is still synced. The latest completed sync must also have
// a zoom_sync_scope record for that exact checkpoint (written by the sync
// hook in zoom_local_sync_scope.go) saying it was a full reconciled walk
// (`sync --full`, which prunes rows the API no longer returns) and
// unfiltered (no --param, --resource-param or --global-param), unless
// allow[resource] accepts that exact filter. A resource with no record for
// its current checkpoint (a sync before this feature, or a record left by an
// older sync) does not cover the account.
func localCoverageFor(q localQuerier, allow map[string]func(params string) bool, resourceTypes ...string) localCoverage {
	cov := localCoverage{synced: make(map[string]bool, len(resourceTypes)), reasons: map[string]string{}}
	if q == nil {
		for _, rt := range resourceTypes {
			cov.reasons[rt] = "no local store"
		}
		return cov
	}
	scope := localReadSyncScope(q)
	for _, rt := range resourceTypes {
		var ts sql.NullString
		var complete sql.NullInt64
		err := q.QueryRow(`SELECT CAST(last_synced_at AS TEXT), last_attempt_complete FROM sync_state WHERE resource_type = ?`, rt).Scan(&ts, &complete)
		if err != nil && strings.Contains(err.Error(), "no such column") {
			// Store written before last_attempt_complete existed: only a
			// non-zero watermark is available.
			complete = sql.NullInt64{Int64: 1, Valid: true}
			err = q.QueryRow(`SELECT CAST(last_synced_at AS TEXT) FROM sync_state WHERE resource_type = ?`, rt).Scan(&ts)
		}
		switch {
		case err != nil:
			cov.reasons[rt] = "never synced"
			continue
		case !complete.Valid || complete.Int64 != 1 || !localNonZeroTimestamp(ts):
			cov.reasons[rt] = "the last sync did not complete"
			continue
		}
		sc, recorded := scope[rt]
		if !recorded || sc.syncedAt != ts.String {
			cov.reasons[rt] = "no full reconciled sync is recorded for the latest checkpoint (a plain sync does not delete rows the API no longer returns); run: " + localSyncCommand([]string{rt})
			continue
		}
		if sc.filtered {
			if accept := allow[rt]; accept == nil || !accept(sc.params) {
				cov.reasons[rt] = fmt.Sprintf("the latest completed sync was filtered (%s), so it does not cover the account; run an unfiltered sync: %s",
					sc.params, localSyncCommand([]string{rt}))
				continue
			}
		}
		if !sc.full {
			cov.reasons[rt] = "the last sync was not a full reconciled walk (it ran without --full, or with --no-prune, --latest-only, --max-pages or --since), so rows the API no longer returns may remain; run: " + localSyncCommand([]string{rt})
			continue
		}
		cov.synced[rt] = true
	}
	return cov
}

// localMissingReasons keeps the reasons for the resources in missing.
func localMissingReasons(cov localCoverage, missing []string) map[string]string {
	out := make(map[string]string, len(missing))
	for _, m := range missing {
		if r, ok := cov.reasons[m]; ok {
			out[m] = r
		} else {
			out[m] = "not synced"
		}
	}
	return out
}

func localNonZeroTimestamp(ts sql.NullString) bool {
	v := strings.TrimSpace(ts.String)
	return ts.Valid && v != "" && v != "0" && !strings.HasPrefix(v, "0001-01-01")
}

// localMissing returns the resources in want that the store has never
// synced, sorted, never nil.
func localMissing(synced map[string]bool, want ...string) []string {
	out := make([]string, 0)
	for _, rt := range want {
		if !synced[rt] {
			out = append(out, rt)
		}
	}
	sort.Strings(out)
	return out
}

// localSyncCommand is the sync that makes resources cover the account: a
// full reconciled walk (--full prunes rows the API no longer returns).
func localSyncCommand(resources []string) string {
	return localCLIName + " sync --full --resources " + strings.Join(resources, ",")
}

// localOpenStore opens the local mirror read-only. A missing file at the
// default path is not an error: it prints the sync hint to stderr and returns
// (nil, nil) so the caller emits an unassessed result and exits 0. A missing
// file named explicitly with --db is a usage error (exit 2).
func localOpenStore(cmd *cobra.Command, dbPath string, syncResources []string) (*store.Store, error) {
	explicit := cmd.Flags().Changed("db")
	if dbPath == "" {
		dbPath = defaultDBPath(localCLIName)
	}
	if _, err := os.Stat(dbPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if explicit {
				return nil, usageErr(fmt.Errorf("--db %s does not exist; pass an existing store or run: %s --db %s", dbPath, localSyncCommand(syncResources), dbPath))
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "no local mirror at %s\nrun: %s\n", dbPath, localSyncCommand(syncResources))
			return nil, nil
		}
		return nil, fmt.Errorf("checking local store %s: %w", dbPath, err)
	}
	db, err := store.OpenReadOnlyContext(cmd.Context(), dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local store %s: %w", dbPath, err)
	}
	return db, nil
}

// localHintMissing prints one stderr hint naming unsynced resources.
func localHintMissing(cmd *cobra.Command, missing []string, reasons ...map[string]string) {
	if len(missing) == 0 {
		return
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "hint: local store does not cover the account for %s\nrun: %s\n", strings.Join(missing, ", "), localSyncCommand(missing))
	for _, rs := range reasons {
		for _, m := range missing {
			if r := rs[m]; strings.HasPrefix(r, "the latest completed sync was filtered") {
				fmt.Fprintf(cmd.ErrOrStderr(), "  %s: %s\n", m, r)
			}
		}
	}
}

// localNotAssessedNote is the note an unassessed local audit carries, in the
// JSON note field and first on stdout in the human table, so zero counts
// never read as a clean result. existing keeps any other note.
func localNotAssessedNote(missing []string, existing string) string {
	na := "not assessed: the local store does not cover the account for " + strings.Join(missing, ", ") + "; run: " + localSyncCommand(missing)
	if existing == "" {
		return na
	}
	return na + "; " + existing
}

// localWriteNotAssessed prints the NOT ASSESSED line that leads an unassessed
// human table.
func localWriteNotAssessed(w io.Writer, assessed bool, note string) {
	if !assessed {
		fmt.Fprintf(w, "NOT ASSESSED: %s\n\n", strings.TrimPrefix(note, "not assessed: "))
	}
}

func localSiteLabel(r localRef) string {
	if s := r.Name.String(); s != "" {
		return s
	}
	return r.ID.String()
}

// localSiteMatches reports whether a --site filter (name or id,
// case-insensitive) selects the given site reference.
func localSiteMatches(filter string, r localRef) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}
	return strings.EqualFold(filter, r.Name.String()) || strings.EqualFold(filter, r.ID.String())
}

func localSameNonEmpty(a, b string) bool {
	return a != "" && b != "" && a == b
}

func localAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// localIsPhoneQuery reports a query made only of digits and phone
// punctuation with at least 7 digits, e.g. "+1 415 555 0123" or
// "(415) 555-0123".
func localIsPhoneQuery(q string) bool {
	for _, r := range q {
		if (r < '0' || r > '9') && !strings.ContainsRune("+-(). ", r) {
			return false
		}
	}
	return len(zoomDigits(q)) >= 7
}

// How a query resolved to a user, strongest first. Only id, email and
// extension are exact enough for `offboard --apply`.
const (
	localMatchID        = "id"
	localMatchEmail     = "email"
	localMatchExtension = "extension"
	localMatchPhone     = "phone_number"
	localMatchName      = "name"
)

func localMatchIsExact(kind string) bool {
	return kind == localMatchID || kind == localMatchEmail || kind == localMatchExtension
}

// localMatchUsers resolves a footprint/offboard query, in order: a user id,
// an email (exact, case-insensitive), an extension number (exact), a phone
// number (compared with zoomPhoneKey), an exact name, then a case-insensitive
// name/email substring. It returns the matches and how they matched.
func localMatchUsers(users []localUser, query string) ([]localUser, string) {
	q := strings.TrimSpace(query)
	out := make([]localUser, 0)
	if q == "" {
		return out, ""
	}
	for _, u := range users {
		if u.ID.String() == q || (u.PhoneUserID.String() != "" && u.PhoneUserID.String() == q) {
			return append(out, u), localMatchID
		}
	}
	if strings.Contains(q, "@") {
		for _, u := range users {
			if strings.EqualFold(u.Email.String(), q) {
				out = append(out, u)
			}
		}
		if len(out) > 0 {
			return out, localMatchEmail
		}
	}
	if localAllDigits(q) {
		for _, u := range users {
			if u.ExtensionNumber.String() == q {
				out = append(out, u)
			}
		}
		if len(out) > 0 {
			return out, localMatchExtension
		}
	}
	if localIsPhoneQuery(q) {
		key := zoomPhoneKey(q)
		for _, u := range users {
			for _, pn := range u.PhoneNumbers {
				if k := zoomPhoneKey(pn.Number.String()); k != "" && k == key {
					out = append(out, u)
					break
				}
			}
		}
		if len(out) > 0 {
			return out, localMatchPhone
		}
	}
	for _, u := range users {
		if strings.EqualFold(u.Name.String(), q) {
			out = append(out, u)
		}
	}
	if len(out) > 0 {
		return out, localMatchName
	}
	lq := strings.ToLower(q)
	for _, u := range users {
		if strings.Contains(strings.ToLower(u.Name.String()), lq) || strings.Contains(strings.ToLower(u.Email.String()), lq) {
			out = append(out, u)
		}
	}
	if len(out) > 0 {
		return out, localMatchName
	}
	return out, ""
}

// localAmbiguousErr names up to 10 candidates so the caller can retry with
// an email or extension.
func localAmbiguousErr(query string, matches []localUser) error {
	names := make([]string, 0, len(matches))
	for i, u := range matches {
		if i == 10 {
			names = append(names, fmt.Sprintf("... and %d more", len(matches)-10))
			break
		}
		names = append(names, fmt.Sprintf("%s <%s> ext %s", u.Name.String(), u.Email.String(), u.ExtensionNumber.String()))
	}
	return usageErr(fmt.Errorf("%q matches %d phone users; pass an email or extension number instead:\n  %s", query, len(matches), strings.Join(names, "\n  ")))
}

// Emergency address verification status, from the GET
// /phone/emergency_addresses schema: 1 verification not required,
// 2 unverified, 3 verification requested, 4 verified, 5 rejected,
// 6 verification failed.
var localAddressStatusLabels = map[string]string{
	"1": "verification_not_required",
	"2": "unverified",
	"3": "verification_requested",
	"4": "verified",
	"5": "rejected",
	"6": "verification_failed",
}

func localAddressStatusLabel(code string) string {
	if l, ok := localAddressStatusLabels[strings.TrimSpace(code)]; ok {
		return l
	}
	return "unknown"
}

// localAddressStatusOK is true for "verification not required" and
// "verified".
func localAddressStatusOK(code string) bool {
	c := strings.TrimSpace(code)
	return c == "1" || c == "4"
}

// localEffectiveAddress is the emergency address a user or common area would
// present on a 911 call.
type localEffectiveAddress struct {
	Source     string `json:"source"`
	ID         string `json:"id,omitempty"`
	Address    string `json:"address,omitempty"`
	SiteID     string `json:"site_id,omitempty"`
	StatusCode string `json:"status_code,omitempty"`
	Status     string `json:"status,omitempty"`
	OK         bool   `json:"ok"`
}

// localResolveAddress applies the fallback chain: the owner's personal
// (level 1) address, preferring is_default (an address's owner.id, when
// present, must equal ownerID; owner.extension_number is consulted only for
// an address with no owner id); else the default company
// (level 0) address of the owner's site; else an account-wide level 0
// default with no site. Source is "not_synced" when emergency_addresses has
// never been synced, "none" when nothing in the chain exists.
func localResolveAddress(addrs []localEmergencyAddress, synced bool, ownerID, ownerExt, siteID string) localEffectiveAddress {
	if !synced {
		return localEffectiveAddress{Source: "not_synced"}
	}
	var personal []localEmergencyAddress
	for _, a := range addrs {
		if a.Level.String() != "1" || a.Owner == nil {
			continue
		}
		// A present owner id is authoritative: an address owned by a former
		// holder of this extension number is not this owner's. The number
		// is only a fallback for an address that carries no owner id.
		if oid := a.Owner.ID.String(); oid != "" {
			if ownerID != "" && oid == ownerID {
				personal = append(personal, a)
			}
		} else if ownerExt != "" && a.Owner.ExtensionNumber.String() == ownerExt {
			personal = append(personal, a)
		}
	}
	pickDefault := func(list []localEmergencyAddress) localEmergencyAddress {
		for _, a := range list {
			if bool(a.IsDefault) {
				return a
			}
		}
		return list[0]
	}
	build := func(source string, a localEmergencyAddress) localEffectiveAddress {
		return localEffectiveAddress{
			Source:     source,
			ID:         a.ID.String(),
			Address:    a.oneLine(),
			SiteID:     a.Site.ID.String(),
			StatusCode: a.Status.String(),
			Status:     localAddressStatusLabel(a.Status.String()),
			OK:         localAddressStatusOK(a.Status.String()),
		}
	}
	if len(personal) > 0 {
		return build("personal", pickDefault(personal))
	}
	if siteID != "" {
		for _, a := range addrs {
			if a.Level.String() == "0" && bool(a.IsDefault) && a.Site.ID.String() == siteID {
				return build("site_default", a)
			}
		}
	}
	for _, a := range addrs {
		if a.Level.String() == "0" && bool(a.IsDefault) && a.Site.ID.String() == "" {
			return build("account_default", a)
		}
	}
	return localEffectiveAddress{Source: "none"}
}
