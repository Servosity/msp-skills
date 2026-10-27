// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Class report-row-identity (hand-fix; reapply/store_keys.py and reapply/row_identity.py
// wire it into the generated store.go).
//
// A /v2/queries/* report row is keyed on its entity (resourceIDFieldOverrides, with a
// documented fallback) and then on the identity its spec.yaml row schema documents:
//
//   - a per-item identity (DeviceAntivirusThreat.threatId, DeviceNetworkInterfaces.interfaceIndex,
//     DeviceRAIDController.controllerIndex, DeviceRAIDDrive.driveId, DeviceOSPatch.id,
//     DeviceSoftwarePatch.id, NodeAttributesDetailed.entityType, ScopedAttributes.scope):
//     resourceCompositeKeyColumns appends it;
//   - one row per device (DeviceComputerSystem, NodeAttributes, DeviceHealthSummary,
//     DeviceOperatingSystem, DevicePolicyOverrides): the device alone;
//   - no per-item identity at all (reportRowOrdinalReports): the row's ordinal among its
//     device's rows in one walk of the report. A display name is not an identity: two
//     DeviceProcessor rows of a dual-socket server carry the same name, and the version of a
//     DeviceApplication changes when it is upgraded.
//
// The ordinal is a position, so it exists only for a walk that starts at page one: sync
// stamps it on each row (ReportRowOrdinals), sync is the only writer of these partitions (an
// endpoint read cannot know which page or filter it saw), and only a whole walk writes them:
// sync stages the walk and ReplaceReportRows swaps the partition for it in one transaction,
// so a device that now reports fewer rows does not keep its old trailing ones and a walk that
// stopped early never leaves a mix of two walks. A stored row is replaced whole rather than
// merged with the row that held its position before.
//
// Upgraded stores hold rows under the key the shipped release computed. When a row is stored
// under its current key, the row at each of its pre-fix keys is removed in the same
// transaction, but only when the current rules would key that stored row elsewhere, so a row
// whose current key happens to equal another row's legacy key is never touched.

// ReportRowOrdinalField is stamped by sync on each row of an ordinal report: the row's
// 0-based position among its device's rows, counted in NinjaOne's response order across the
// page boundaries of one walk.
const ReportRowOrdinalField = "_ninja_row_ordinal"

// reportRowOrdinalReports are the reports whose spec.yaml row schema documents no per-item
// identity (value: the report and row schema).
var reportRowOrdinalReports = map[string]string{
	"queries":                  "DiskDriveReport / DeviceDiskDrive",
	"queries-antivirus-status": "AntivirusStatusReport / DeviceAntivirusStatus",
	"queries-logged-on-users":  "LoggedOnUsersReport / DeviceLoggedOnUser",
	"queries-processors":       "ProcessorReport / DeviceProcessor",
	"queries-software":         "SoftwareReport / DeviceApplication",
	"queries-volumes":          "VolumesReport / DeviceVolume",
	"queries-windows-services": "WindowsServiceReport / DeviceWindowsService",
}

// ReportRowsAreOrdinal reports whether resourceType keys its rows on a device ordinal.
func ReportRowsAreOrdinal(resourceType string) bool {
	_, ok := reportRowOrdinalReports[resourceType]
	return ok
}

// reportRowOrdinal returns the ordinal stamped on obj in canonical form, "" when it has none.
func reportRowOrdinal(obj map[string]any) string {
	v, ok := obj[ReportRowOrdinalField]
	if !ok {
		return ""
	}
	n, err := strconv.Atoi(ResourceIDString(v))
	if err != nil || n < 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// reportRowStorageID is consulted first by resourceStorageID: a stamped row of an ordinal
// report keys on its device and its ordinal, nothing else.
func reportRowStorageID(resourceType, id string, obj map[string]any) (string, bool) {
	if !ReportRowsAreOrdinal(resourceType) {
		return "", false
	}
	ord := reportRowOrdinal(obj)
	if ord == "" {
		return "", false
	}
	return id + "\x00#" + ord, true
}

// ReportRowOrdinals numbers the rows of one walk of a report: one value sees every page of
// one walk, in order, starting at page one.
type ReportRowOrdinals struct {
	resourceType string
	next         map[string]int
}

// NewReportRowOrdinals returns the counter for one walk of resourceType, nil when the report
// keys its rows on a documented identity.
func NewReportRowOrdinals(resourceType string) *ReportRowOrdinals {
	if !ReportRowsAreOrdinal(resourceType) {
		return nil
	}
	return &ReportRowOrdinals{resourceType: resourceType, next: map[string]int{}}
}

// Stamp returns the page with each row's ordinal stamped, in response order. A row that is
// not a JSON object or carries no entity id is returned unchanged (UpsertBatch refuses it as
// it always has). The other fields are kept byte for byte.
func (o *ReportRowOrdinals) Stamp(items []json.RawMessage) []json.RawMessage {
	if o == nil {
		return items
	}
	out := make([]json.RawMessage, len(items))
	for i, item := range items {
		out[i] = item
		obj, err := DecodeJSONObject(item)
		if err != nil {
			continue
		}
		id := ExtractResourceID(o.resourceType, obj)
		if id == "" {
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(item, &fields); err != nil || fields == nil {
			continue
		}
		fields[ReportRowOrdinalField] = json.RawMessage(strconv.Itoa(o.next[id]))
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(fields); err != nil {
			continue
		}
		o.next[id]++
		out[i] = json.RawMessage(bytes.TrimSpace(buf.Bytes()))
	}
	return out
}

// resourceIDFallbackOverrides names a documented response identity consulted when the
// resourceIDFieldOverrides field is absent from a row. spec.yaml marks
// NodeAttributesDetailed.deviceId writeOnly: /v2/queries/custom-fields-detailed responses carry
// entityId, so the deviceId override alone dropped every documented row.
var resourceIDFallbackOverrides = map[string]string{
	"queries-custom-fields-detailed": "entityId",
}

// resourceIDFallbackOverride is consulted by ExtractResourceID right after the override.
func resourceIDFallbackOverride(resourceType string, obj map[string]any) string {
	if field := resourceIDFallbackOverrides[resourceType]; field != "" {
		return canonicalIDFromOverride(obj, field)
	}
	return ""
}

// resourceLegacyKeyColumns rebuilds a key the shipped release stored a row under, joined
// with NUL, where that key is not the row's unstamped key: /v2/queries/disks rows (partition
// "queries") fell back to the bare drive name; custom-fields-detailed rows keyed on deviceId
// alone (before the entityType discriminator). A stamped ordinal row's other shipped key
// (deviceId plus its display name) is its own key without the stamp.
var resourceLegacyKeyColumns = map[string][]string{
	"queries":                        {"name"},
	"queries-custom-fields-detailed": {"deviceId"},
}

func identityFieldString(obj map[string]any, field string) string {
	v := ResourceIDString(lookupFieldValue(obj, field))
	if v == "<nil>" {
		return ""
	}
	return v
}

func legacyColumnsStorageID(resourceType string, obj map[string]any) string {
	cols := resourceLegacyKeyColumns[resourceType]
	if len(cols) == 0 {
		return ""
	}
	parts := make([]string, 0, len(cols))
	for _, col := range cols {
		v := identityFieldString(obj, col)
		if v == "" {
			return ""
		}
		parts = append(parts, v)
	}
	return strings.Join(parts, "\x00")
}

// legacyResourceStorageIDs are the keys an earlier release may have stored obj under, other
// than storageID.
func legacyResourceStorageIDs(resourceType, storageID string, obj map[string]any) []string {
	var out []string
	add := func(key string) {
		if key == "" || key == storageID {
			return
		}
		for _, k := range out {
			if k == key {
				return
			}
		}
		out = append(out, key)
	}
	if ReportRowsAreOrdinal(resourceType) && reportRowOrdinal(obj) != "" {
		unstamped := make(map[string]any, len(obj))
		for k, v := range obj {
			if k != ReportRowOrdinalField {
				unstamped[k] = v
			}
		}
		if id := ExtractResourceID(resourceType, unstamped); id != "" {
			add(resourceStorageID(resourceType, id, unstamped))
		}
	}
	add(legacyColumnsStorageID(resourceType, obj))
	return out
}

// currentStorageID is the key the current rules give a stored row, "" when sync would never
// store it as it is: a row of an ordinal report without its stamp predates the ordinal.
func currentStorageID(resourceType string, obj map[string]any) string {
	if ReportRowsAreOrdinal(resourceType) && reportRowOrdinal(obj) == "" {
		return ""
	}
	id := ExtractResourceID(resourceType, obj)
	if id == "" {
		return ""
	}
	return resourceStorageID(resourceType, id, obj)
}

// dropLegacyResourceRowTx removes the rows this item was stored under before its key gained
// device scope, a discriminator or an ordinal. A no-op for a legacy key equal to the current
// one, when no row sits there, or when the stored row is current-keyed (its own data keys it
// there).
func (s *Store) dropLegacyResourceRowTx(tx *sql.Tx, resourceType, storageID string, obj map[string]any) error {
	for _, legacy := range legacyResourceStorageIDs(resourceType, storageID, obj) {
		if err := s.dropLegacyKeyTx(tx, resourceType, legacy); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) dropLegacyKeyTx(tx *sql.Tx, resourceType, legacy string) error {
	var data string
	err := tx.QueryRow(`SELECT data FROM resources WHERE resource_type = ? AND id = ?`, resourceType, legacy).Scan(&data)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading legacy %s row: %w", resourceType, err)
	}
	stored, derr := DecodeJSONObject(json.RawMessage(data))
	if derr != nil {
		return nil // an unreadable row is never a victim
	}
	if currentStorageID(resourceType, stored) == legacy {
		return nil // the stored row is keyed there by the current rules
	}
	var typed string
	if qerr := tx.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, resourceType).Scan(&typed); qerr == nil && typed != "" {
		if _, err := tx.Exec(fmt.Sprintf(`DELETE FROM "%s" WHERE id = ?`, strings.ReplaceAll(typed, `"`, `""`)), legacy); err != nil {
			return fmt.Errorf("dropping legacy %s typed row: %w", resourceType, err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM resources_fts WHERE rowid = ?`, ftsRowID(resourceType, legacy)); err != nil {
		return fmt.Errorf("dropping legacy %s search row: %w", resourceType, err)
	}
	if _, err := tx.Exec(`DELETE FROM resources WHERE resource_type = ? AND id = ?`, resourceType, legacy); err != nil {
		return fmt.Errorf("dropping legacy %s row: %w", resourceType, err)
	}
	return nil
}

// StorageIDFor is the id UpsertBatch stores obj under in resourceType ("" when it stores
// nothing): the same extraction, storage fallback, composite key and stamped ordinal.
func StorageIDFor(resourceType string, obj map[string]any) string {
	id := ExtractResourceID(resourceType, obj)
	if id == "" {
		id = ResolveStorageID(resourceType, obj)
	}
	if id == "" {
		return ""
	}
	return resourceStorageID(resourceType, id, obj)
}

// ReconcileSnapshot removes every row of resourceType whose full storage id is not in keep:
// the rows a complete walk of a report snapshot no longer returned. Ids are compared whole
// (the generated reconcile compares the bare entity id, which keeps every stale row of a
// device the walk saw). The typed table, the search index and the generic row all go.
func (s *Store) ReconcileSnapshot(resourceType string, keep []string, typedTable string) (int, error) {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	kept := make(map[string]struct{}, len(keep))
	for _, id := range keep {
		kept[id] = struct{}{}
	}
	rows, err := tx.Query(`SELECT id FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		return 0, fmt.Errorf("snapshot %s: select: %w", resourceType, err)
	}
	var victims []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return 0, err
		}
		if _, ok := kept[id]; !ok {
			victims = append(victims, id)
		}
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, id := range victims {
		if typedTable != "" {
			if _, err := tx.Exec(fmt.Sprintf(`DELETE FROM "%s" WHERE id = ?`, strings.ReplaceAll(typedTable, `"`, `""`)), id); err != nil {
				return 0, fmt.Errorf("snapshot %s: typed delete: %w", resourceType, err)
			}
		}
		if _, err := tx.Exec(`DELETE FROM resources_fts WHERE rowid = ?`, ftsRowID(resourceType, id)); err != nil {
			return 0, fmt.Errorf("snapshot %s: search delete: %w", resourceType, err)
		}
		if _, err := tx.Exec(`DELETE FROM resources WHERE resource_type = ? AND id = ?`, resourceType, id); err != nil {
			return 0, fmt.Errorf("snapshot %s: delete: %w", resourceType, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(victims), nil
}

// ReplaceReportRows replaces every stored row of an ordinal report with the rows of one whole
// walk, in ONE transaction: each row next yields (batch by batch, until ok is false) is stored
// under its stamped key, then every row of the partition the walk did not store is removed,
// rows under a shipped key included. Any error rolls the whole replace back, so the partition
// holds either the previous walk's rows or this walk's, never a mix. A row without a stamp or
// without an entity id fails the replace (sync stages only stamped rows). typedTable is the
// typed table resourceType projects into ("" for none); a typed projection
// upsertReportTypedTx does not write fails the replace rather than leave the table stale.
// Returns the rows stored and the rows removed.
func (s *Store) ReplaceReportRows(resourceType, typedTable string, next func() (rows []json.RawMessage, ok bool, err error)) (int, int, error) {
	if !ReportRowsAreOrdinal(resourceType) {
		return 0, 0, fmt.Errorf("replace %s: not a report keyed by row ordinal", resourceType)
	}
	s.lockForWrite()
	defer s.unlockAfterWrite()
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	kept := map[string]struct{}{}
	for {
		rows, ok, err := next()
		if err != nil {
			return 0, 0, err
		}
		if !ok {
			break
		}
		for _, item := range rows {
			obj, err := DecodeJSONObject(item)
			if err != nil {
				return 0, 0, fmt.Errorf("replace %s: %w", resourceType, err)
			}
			storageID := currentStorageID(resourceType, obj)
			if storageID == "" {
				return 0, 0, fmt.Errorf("replace %s: a row carries no device id or no ordinal", resourceType)
			}
			if err := s.upsertGenericResourceTx(tx, resourceType, storageID, item); err != nil {
				return 0, 0, fmt.Errorf("replace %s/%s: %w", resourceType, storageID, err)
			}
			deriveScopeColumns(obj)
			if err := s.upsertReportTypedTx(tx, resourceType, typedTable, storageID, obj, item); err != nil {
				return 0, 0, fmt.Errorf("replace %s/%s: %w", resourceType, storageID, err)
			}
			kept[storageID] = struct{}{}
		}
	}
	victims, err := partitionVictimsTx(tx, resourceType, kept)
	if err != nil {
		return 0, 0, err
	}
	for _, id := range victims {
		if err := deleteResourceRowTx(tx, resourceType, typedTable, id); err != nil {
			return 0, 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return len(kept), len(victims), nil
}

// upsertReportTypedTx is UpsertBatchDetailed's typed-table dispatch for the ordinal reports:
// only /v2/queries/disks ("queries") projects into a typed table today.
func (s *Store) upsertReportTypedTx(tx *sql.Tx, resourceType, typedTable, storageID string, obj map[string]any, item json.RawMessage) error {
	switch resourceType {
	case "queries":
		return s.upsertQueriesTx(tx, storageID, obj, item)
	}
	if typedTable != "" {
		return fmt.Errorf("no typed projection into %q for %s", typedTable, resourceType)
	}
	return nil
}

func partitionVictimsTx(tx *sql.Tx, resourceType string, kept map[string]struct{}) ([]string, error) {
	rows, err := tx.Query(`SELECT id FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		return nil, fmt.Errorf("replace %s: select: %w", resourceType, err)
	}
	defer rows.Close()
	var victims []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if _, ok := kept[id]; !ok {
			victims = append(victims, id)
		}
	}
	return victims, rows.Err()
}

func deleteResourceRowTx(tx *sql.Tx, resourceType, typedTable, id string) error {
	if typedTable != "" {
		if _, err := tx.Exec(fmt.Sprintf(`DELETE FROM "%s" WHERE id = ?`, strings.ReplaceAll(typedTable, `"`, `""`)), id); err != nil {
			return fmt.Errorf("replace %s: typed delete: %w", resourceType, err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM resources_fts WHERE rowid = ?`, ftsRowID(resourceType, id)); err != nil {
		return fmt.Errorf("replace %s: search delete: %w", resourceType, err)
	}
	if _, err := tx.Exec(`DELETE FROM resources WHERE resource_type = ? AND id = ?`, resourceType, id); err != nil {
		return fmt.Errorf("replace %s: delete: %w", resourceType, err)
	}
	return nil
}
