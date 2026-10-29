// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (handfixes.json warranty-store): the warranty mirror and the
// vendor_events table the novel commands read and write. Created from
// migrateExtras (extras.go).
//
// vendor_events follows the zammad connector's shape so it is ready for MCP
// Events (SEP-3415, draft) without implementing it: seq is the cursor,
// event_id the stable id (UNIQUE, so a repeated lookup never repeats an
// event), type the event name, data the minimal before/after payload. The
// client column holds the device's customer.

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"warranty-pp-cli/internal/warranty"
)

var warrantyMigrations = []string{
	`CREATE TABLE IF NOT EXISTS warranty_devices (
		serial TEXT PRIMARY KEY,
		vendor TEXT NOT NULL DEFAULT '',
		vendor_source TEXT NOT NULL DEFAULT '',
		customer TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		hostname TEXT NOT NULL DEFAULT '',
		ship_date TEXT NOT NULL DEFAULT '',
		purchase_date TEXT NOT NULL DEFAULT '',
		coverage_end TEXT NOT NULL DEFAULT '',
		coverage_open INTEGER NOT NULL DEFAULT 0,
		fingerprint TEXT NOT NULL DEFAULT '',
		last_checked TEXT NOT NULL DEFAULT '',
		last_result TEXT NOT NULL DEFAULT '',
		last_message TEXT NOT NULL DEFAULT '',
		added_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_warranty_devices_customer ON warranty_devices(customer)`,
	`CREATE TABLE IF NOT EXISTS warranty_coverage (
		serial TEXT NOT NULL,
		line_id TEXT NOT NULL,
		vendor TEXT NOT NULL,
		kind TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		service_level TEXT NOT NULL DEFAULT '',
		start_date TEXT NOT NULL DEFAULT '',
		end_date TEXT NOT NULL DEFAULT '',
		hardware INTEGER NOT NULL DEFAULT 1,
		active INTEGER NOT NULL DEFAULT 1,
		renewable INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (serial, line_id)
	)`,
	`CREATE TABLE IF NOT EXISTS vendor_events (
		seq INTEGER PRIMARY KEY AUTOINCREMENT,
		event_id TEXT NOT NULL UNIQUE,
		type TEXT NOT NULL,
		source TEXT NOT NULL,
		occurred_at TEXT NOT NULL,
		received_at TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		client TEXT NOT NULL DEFAULT '',
		data TEXT NOT NULL DEFAULT '{}'
	)`,
	`CREATE INDEX IF NOT EXISTS idx_vendor_events_type_seq ON vendor_events(type, seq)`,
}

// Device sources.
const (
	DeviceSourceImport   = "import"
	DeviceSourceLookup   = "lookup"
	DeviceSourceAppleOrg = "apple-org"
)

// Device is one warranty_devices row.
type Device struct {
	Serial       string `json:"serial"`
	Vendor       string `json:"vendor"`
	VendorSource string `json:"vendor_source"`
	Customer     string `json:"customer"`
	Source       string `json:"source"`
	Model        string `json:"model"`
	Hostname     string `json:"hostname"`
	ShipDate     string `json:"ship_date"`
	PurchaseDate string `json:"purchase_date"`
	CoverageEnd  string `json:"coverage_end"`
	CoverageOpen bool   `json:"coverage_open"`
	Fingerprint  string `json:"-"`
	LastChecked  string `json:"last_checked"`
	LastResult   string `json:"last_result"`
	LastMessage  string `json:"last_message,omitempty"`
}

func warrantyNow() string { return time.Now().UTC().Format(time.RFC3339) }

func isMissingTable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table")
}

const deviceColumns = `serial, vendor, vendor_source, customer, source, model, hostname, ship_date,
	purchase_date, coverage_end, coverage_open, fingerprint, last_checked, last_result, last_message`

func scanDevice(sc interface{ Scan(...any) error }) (Device, error) {
	var d Device
	var open int
	err := sc.Scan(&d.Serial, &d.Vendor, &d.VendorSource, &d.Customer, &d.Source, &d.Model, &d.Hostname,
		&d.ShipDate, &d.PurchaseDate, &d.CoverageEnd, &open, &d.Fingerprint, &d.LastChecked, &d.LastResult, &d.LastMessage)
	d.CoverageOpen = open != 0
	return d, err
}

// GetDevice returns one device by serial.
func (s *Store) GetDevice(ctx context.Context, serial string) (Device, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+deviceColumns+` FROM warranty_devices WHERE serial = ?`, serial)
	d, err := scanDevice(row)
	if err == sql.ErrNoRows || isMissingTable(err) {
		return Device{}, false, nil
	}
	if err != nil {
		return Device{}, false, err
	}
	return d, true, nil
}

// ListDevices returns every device, or one customer's (case-insensitive),
// ordered by customer then serial.
func (s *Store) ListDevices(ctx context.Context, customer string) ([]Device, error) {
	q := `SELECT ` + deviceColumns + ` FROM warranty_devices`
	args := []any{}
	if customer != "" {
		q += ` WHERE customer = ? COLLATE NOCASE`
		args = append(args, customer)
	}
	q += ` ORDER BY customer COLLATE NOCASE, serial`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if isMissingTable(err) {
		return []Device{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Lines returns a device's coverage lines, latest end first.
func (s *Store) Lines(ctx context.Context, serial string) ([]warranty.Line, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT line_id, kind, description, service_level, start_date, end_date,
		hardware, active, renewable, status FROM warranty_coverage WHERE serial = ? ORDER BY end_date DESC, line_id`, serial)
	if isMissingTable(err) {
		return []warranty.Line{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []warranty.Line{}
	for rows.Next() {
		var l warranty.Line
		var hw, active, renew int
		if err := rows.Scan(&l.LineID, &l.Kind, &l.Description, &l.ServiceLevel, &l.Start, &l.End, &hw, &active, &renew, &l.Status); err != nil {
			return nil, err
		}
		l.Hardware, l.Active, l.Renewable = hw != 0, active != 0, renew != 0
		out = append(out, l)
	}
	return out, rows.Err()
}

// DeviceUpsert is what import and apple-pull know about a device.
type DeviceUpsert struct {
	Serial       string
	Vendor       string
	VendorSource string
	Customer     string
	Source       string
	Model        string
	Hostname     string
	PurchaseDate string
}

// UpsertResult says what an upsert did.
type UpsertResult struct {
	Created      bool   `json:"created"`
	PrevCustomer string `json:"previous_customer,omitempty"`
	// PrevVendor is set when the upsert changed an existing device's vendor:
	// the coverage the mirror held belonged to that vendor and was cleared.
	PrevVendor string `json:"previous_vendor,omitempty"`
}

// ClearedFingerprint marks coverage the mirror cleared after a definitive
// vendor answer or a vendor change, so a later answer is still a change.
const ClearedFingerprint = "cleared"

// UpsertDevice inserts a device or refreshes the fields the caller knows.
// Lookup results are never touched. A declared vendor replaces a detected
// one; a detected vendor never replaces a declared one. An empty field keeps
// the stored value.
func (s *Store) UpsertDevice(ctx context.Context, u DeviceUpsert) (UpsertResult, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UpsertResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := upsertDeviceTx(ctx, tx, u)
	if err != nil {
		return UpsertResult{}, err
	}
	return res, tx.Commit()
}

func upsertDeviceTx(ctx context.Context, tx *sql.Tx, u DeviceUpsert) (UpsertResult, error) {
	now := warrantyNow()
	row := tx.QueryRowContext(ctx, `SELECT `+deviceColumns+` FROM warranty_devices WHERE serial = ?`, u.Serial)
	cur, err := scanDevice(row)
	if err == sql.ErrNoRows {
		_, err = tx.ExecContext(ctx, `INSERT INTO warranty_devices (serial, vendor, vendor_source, customer, source,
			model, hostname, purchase_date, added_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			u.Serial, u.Vendor, u.VendorSource, u.Customer, u.Source, u.Model, u.Hostname, u.PurchaseDate, now, now)
		return UpsertResult{Created: true}, err
	}
	if err != nil {
		return UpsertResult{}, err
	}
	res := UpsertResult{}
	if u.Customer != "" && !strings.EqualFold(u.Customer, cur.Customer) && cur.Customer != "" {
		res.PrevCustomer = cur.Customer
	}
	vendor, vsrc := cur.Vendor, cur.VendorSource
	if u.Vendor != "" && (cur.Vendor == "" || u.VendorSource == warranty.SourceDeclared || cur.VendorSource != warranty.SourceDeclared) {
		vendor, vsrc = u.Vendor, u.VendorSource
	}
	if cur.Vendor != "" && vendor != cur.Vendor {
		// Coverage belongs to the vendor that answered: under another vendor
		// it is wrong, so it is cleared and the verdict starts over.
		res.PrevVendor = cur.Vendor
		if _, err := tx.ExecContext(ctx, `DELETE FROM warranty_coverage WHERE serial = ?`, u.Serial); err != nil {
			return res, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE warranty_devices SET coverage_end = '', coverage_open = 0,
			fingerprint = CASE WHEN fingerprint = '' THEN '' ELSE ? END, last_result = '', last_message = '', last_checked = ''
			WHERE serial = ?`, ClearedFingerprint, u.Serial); err != nil {
			return res, err
		}
	}
	// An import marks the device as present in the RMM export whatever
	// first created it (apple-pull compares against this).
	source := cur.Source
	if u.Source == DeviceSourceImport {
		source = DeviceSourceImport
	}
	_, err = tx.ExecContext(ctx, `UPDATE warranty_devices SET vendor = ?, vendor_source = ?, source = ?,
		customer = CASE WHEN ? <> '' THEN ? ELSE customer END,
		model = CASE WHEN model = '' THEN ? ELSE model END,
		hostname = CASE WHEN ? <> '' THEN ? ELSE hostname END,
		purchase_date = CASE WHEN purchase_date = '' THEN ? ELSE purchase_date END,
		updated_at = ? WHERE serial = ?`,
		vendor, vsrc, source, u.Customer, u.Customer, u.Model, u.Hostname, u.Hostname, u.PurchaseDate, now, u.Serial)
	return res, err
}

// LookupWrite is one lookup outcome to record.
type LookupWrite struct {
	Record       warranty.Record
	VendorSource string
	Customer     string // set only when the caller names one
	CheckedAt    string // RFC 3339; empty when no vendor call was made
	AsOf         string // YYYY-MM-DD, for expiring/expired events
	WindowDays   int
}

// ApplyLookup records one lookup outcome in one transaction: the device row,
// its coverage lines and the coverage events the change raised. It returns
// the events inserted.
//
//   - A vendor answer with coverage replaces the lines.
//   - A definitive negative answer (not found, invalid, not in the Apple
//     organization, ambiguous) clears what the mirror held.
//   - A failed call records the error and keeps the lines; report and
//     lifecycle then treat the device as unverified until a good answer.
//   - No vendor call (not configured, not detected, over budget) changes
//     nothing the mirror knew, except on a device never looked up.
//   - A change of vendor clears the previous vendor's coverage.
func (s *Store) ApplyLookup(ctx context.Context, w LookupWrite) ([]VendorEvent, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	r := w.Record
	now := warrantyNow()
	before, existed, err := deviceTx(ctx, tx, r.Serial)
	if err != nil {
		return nil, err
	}
	up, err := upsertDeviceTx(ctx, tx, DeviceUpsert{Serial: r.Serial, Vendor: r.Vendor, VendorSource: w.VendorSource,
		Customer: w.Customer, Source: DeviceSourceLookup, Model: r.Model, PurchaseDate: r.PurchaseDate})
	if err != nil {
		return nil, err
	}
	cur, _, err := deviceTx(ctx, tx, r.Serial)
	if err != nil {
		return nil, err
	}
	inserted := []VendorEvent{}
	emit := func(e warranty.Event) error {
		ve, err := insertEvent(ctx, tx, e, r.Vendor, cur.Customer, firstNonBlank(w.CheckedAt, now), now)
		if err == nil && ve != nil {
			inserted = append(inserted, *ve)
		}
		return err
	}
	if existed && up.PrevVendor != "" && realFingerprint(before.Fingerprint) {
		if err := emit(clearedEvent(r.Serial, up.PrevVendor, before, "vendor changed to "+cur.Vendor)); err != nil {
			return nil, err
		}
	}
	prior := warranty.Prior{Fingerprint: cur.Fingerprint, End: cur.CoverageEnd, Open: cur.CoverageOpen,
		Checked: cur.Fingerprint != "", CheckedAt: cur.LastChecked}
	answered := r.Result == warranty.ResultOK || r.Result == warranty.ResultDuplicated
	switch {
	case answered:
		// The saved row and its dated events describe today. A lookup judged on
		// another --as-of raises only coverage.changed: an expiring or expired
		// alert for that date would be false today and would spend its id.
		day := time.Now().UTC().Format("2006-01-02")
		dated := w.AsOf == "" || w.AsOf == day
		sum := warranty.Summarize(r.Lines, day)
		if _, err := tx.ExecContext(ctx, `DELETE FROM warranty_coverage WHERE serial = ?`, r.Serial); err != nil {
			return nil, err
		}
		for _, l := range r.Lines {
			if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO warranty_coverage (serial, line_id, vendor, kind, description,
				service_level, start_date, end_date, hardware, active, renewable, status) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				r.Serial, l.LineID, r.Vendor, l.Kind, l.Description, l.ServiceLevel, l.Start, l.End,
				boolInt(l.Hardware), boolInt(l.Active), boolInt(l.Renewable), l.Status); err != nil {
				return nil, err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE warranty_devices SET ship_date = CASE WHEN ? <> '' THEN ? ELSE ship_date END,
			model = CASE WHEN ? <> '' THEN ? ELSE model END,
			coverage_end = ?, coverage_open = ?, fingerprint = ?, last_checked = ?, last_result = ?, last_message = ?,
			updated_at = ? WHERE serial = ?`,
			r.ShipDate, r.ShipDate, r.Model, r.Model, sum.End, boolInt(sum.Open), sum.Fingerprint, w.CheckedAt, r.Result, r.Message, now, r.Serial); err != nil {
			return nil, err
		}
		for _, e := range warranty.Events(r.Serial, r.Vendor, prior, sum, day, w.WindowDays) {
			if !dated && e.Type != warranty.EventChanged {
				continue
			}
			if err := emit(e); err != nil {
				return nil, err
			}
		}
	case w.CheckedAt != "" && definitiveNegative(r.Result):
		if _, err := tx.ExecContext(ctx, `DELETE FROM warranty_coverage WHERE serial = ?`, r.Serial); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE warranty_devices SET coverage_end = '', coverage_open = 0, fingerprint = ?,
			last_result = ?, last_message = ?, last_checked = ?, updated_at = ? WHERE serial = ?`,
			ClearedFingerprint, r.Result, r.Message, w.CheckedAt, now, r.Serial); err != nil {
			return nil, err
		}
		if realFingerprint(cur.Fingerprint) {
			if err := emit(clearedEvent(r.Serial, r.Vendor, cur, r.Result)); err != nil {
				return nil, err
			}
		}
	case w.CheckedAt == "" && r.Result != warranty.ResultError && cur.LastResult != "":
		// No vendor call: nothing new is known; the last verdict stands.
	default:
		if _, err := tx.ExecContext(ctx, `UPDATE warranty_devices SET last_result = ?, last_message = ?,
			last_checked = CASE WHEN ? <> '' THEN ? ELSE last_checked END, updated_at = ? WHERE serial = ?`,
			r.Result, r.Message, w.CheckedAt, w.CheckedAt, now, r.Serial); err != nil {
			return nil, err
		}
	}
	return inserted, tx.Commit()
}

func deviceTx(ctx context.Context, tx *sql.Tx, serial string) (Device, bool, error) {
	d, err := scanDevice(tx.QueryRowContext(ctx, `SELECT `+deviceColumns+` FROM warranty_devices WHERE serial = ?`, serial))
	if err == sql.ErrNoRows {
		return Device{}, false, nil
	}
	return d, err == nil, err
}

func realFingerprint(fp string) bool { return fp != "" && fp != ClearedFingerprint }

// clearedEvent is the coverage.changed event for coverage the mirror cleared.
func clearedEvent(serial, vendor string, before Device, why string) warranty.Event {
	return warranty.Event{
		EventID: strings.Join([]string{warranty.EventChanged, serial, before.Fingerprint, before.LastChecked, ClearedFingerprint}, ":"),
		Type:    warranty.EventChanged, Serial: serial,
		Data: map[string]any{"vendor": vendor, "before_coverage_end": before.CoverageEnd, "after_coverage_end": "",
			"before_coverage_open": before.CoverageOpen, "after_coverage_open": false, "reason": why},
	}
}

func firstNonBlank(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// definitiveNegative is a vendor answer that the serial has no coverage
// record the mirror can keep: not found, invalid, not in the Apple
// organization, or ambiguous (Lenovo 101: the bare serial names no one machine).
func definitiveNegative(result string) bool {
	switch result {
	case warranty.ResultNotFound, warranty.ResultInvalid, warranty.ResultNotInOrg, warranty.ResultAmbiguous:
		return true
	}
	return false
}

// insertEvent writes one event once (event_id is UNIQUE) and returns it, or
// nil when an earlier lookup already raised it.
func insertEvent(ctx context.Context, tx *sql.Tx, e warranty.Event, source, customer, occurred, received string) (*VendorEvent, error) {
	data, _ := json.Marshal(e.Data)
	ve := VendorEvent{EventID: e.EventID, Type: e.Type, Source: source, OccurredAt: occurred, ReceivedAt: received,
		ResourceType: "device", ResourceID: e.Serial, Client: customer, Data: data}
	res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO vendor_events
		(event_id, type, source, occurred_at, received_at, resource_type, resource_id, client, data)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ve.EventID, ve.Type, ve.Source, ve.OccurredAt, ve.ReceivedAt, ve.ResourceType, ve.ResourceID, ve.Client, string(data))
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil, nil
	}
	ve.Seq, _ = res.LastInsertId()
	return &ve, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// VendorEvent is one vendor_events row.
type VendorEvent struct {
	Seq          int64           `json:"seq"`
	EventID      string          `json:"event_id"`
	Type         string          `json:"type"`
	Source       string          `json:"source"`
	OccurredAt   string          `json:"occurred_at"`
	ReceivedAt   string          `json:"received_at"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	Client       string          `json:"client"`
	Data         json.RawMessage `json:"data"`
}

// VendorEventsSince returns up to limit events after the cursor, oldest
// first, optionally of one type, and whether more remain.
func (s *Store) VendorEventsSince(ctx context.Context, since int64, eventType string, limit int) ([]VendorEvent, bool, error) {
	q := `SELECT seq, event_id, type, source, occurred_at, received_at, resource_type, resource_id, client, data
		FROM vendor_events WHERE seq > ?`
	args := []any{since}
	if eventType != "" {
		q += ` AND type = ?`
		args = append(args, eventType)
	}
	q += ` ORDER BY seq LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if isMissingTable(err) {
		return []VendorEvent{}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := []VendorEvent{}
	for rows.Next() {
		var e VendorEvent
		var data string
		if err := rows.Scan(&e.Seq, &e.EventID, &e.Type, &e.Source, &e.OccurredAt, &e.ReceivedAt,
			&e.ResourceType, &e.ResourceID, &e.Client, &data); err != nil {
			return nil, false, err
		}
		e.Data = json.RawMessage(data)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}

// SerialEventCount reports how many events mention a serial (tests, report).
func (s *Store) SerialEventCount(ctx context.Context, serial string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM vendor_events WHERE resource_id = ?`, serial).Scan(&n)
	if isMissingTable(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("counting events: %w", err)
	}
	return n, nil
}
