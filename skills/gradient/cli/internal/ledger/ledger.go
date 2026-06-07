// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored package; survives regeneration as a whole unit.

// Package ledger persists the only history of Synthesize interactions that
// can exist anywhere: the vendor API has no read-back for pushed unit counts
// and no list route for dispatched alerts, so this CLI records both locally.
//
// Storage is append-only JSONL under the CLI config directory (no database
// dependency; the data is small and scan-shaped). Alert records are updated
// in place via read-modify-rewrite, which is safe at this volume.
package ledger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Dir returns the ledger directory, creating it if needed.
// GRADIENT_LEDGER_DIR overrides the default (used by tests and CI).
func Dir() (string, error) {
	if v := os.Getenv("GRADIENT_LEDGER_DIR"); v != "" {
		if err := os.MkdirAll(v, 0o700); err != nil { // #nosec G703 -- operator-set ledger dir; user controls their own machine
			return "", fmt.Errorf("creating ledger dir: %w", err)
		}
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	dir := filepath.Join(home, ".config", "gradient-cli", "ledger")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("creating ledger dir: %w", err)
	}
	return dir, nil
}

// PushRecord is one unit-count POST as sent to
// POST /vendor-api/service/{serviceId}/count.
type PushRecord struct {
	RunID     string    `json:"run_id"`
	At        time.Time `json:"at"`
	ServiceID string    `json:"service_id"`
	AccountID string    `json:"account_id"`
	UnitCount float64   `json:"unit_count"`
	NoBuild   bool      `json:"no_build"`
	Status    string    `json:"status"` // "sent" | "failed"
	Error     string    `json:"error,omitempty"`
}

// AlertRecord is one alert dispatch plus its last-known ticket state.
type AlertRecord struct {
	At           time.Time `json:"at"`
	AccountID    string    `json:"account_id"`
	AlertID      string    `json:"alert_id"`
	MessageID    string    `json:"message_id"`
	Title        string    `json:"title"`
	TicketID     string    `json:"ticket_id,omitempty"`
	TicketStatus string    `json:"ticket_status"` // "pending" | "created" | "timeout" | "failed"
	CheckedAt    time.Time `json:"checked_at,omitempty"`
}

func pushesPath(dir string) string { return filepath.Join(dir, "pushes.jsonl") }
func alertsPath(dir string) string { return filepath.Join(dir, "alerts.jsonl") }

// AppendPushes appends push records to the ledger.
func AppendPushes(dir string, records []PushRecord) error {
	f, err := os.OpenFile(pushesPath(dir), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("opening push ledger: %w", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, r := range records {
		if err := enc.Encode(r); err != nil {
			return fmt.Errorf("writing push ledger: %w", err)
		}
	}
	return nil
}

// ReadPushes returns every push record in file order (oldest first).
// A missing ledger file yields an empty slice, not an error.
func ReadPushes(dir string) ([]PushRecord, error) {
	return readJSONL[PushRecord](pushesPath(dir))
}

// AppendAlert appends one alert dispatch record.
func AppendAlert(dir string, rec AlertRecord) error {
	f, err := os.OpenFile(alertsPath(dir), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("opening alert ledger: %w", err)
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(rec); err != nil {
		return fmt.Errorf("writing alert ledger: %w", err)
	}
	return nil
}

// ReadAlerts returns every alert record in file order (oldest first).
// A missing ledger file yields an empty slice, not an error.
func ReadAlerts(dir string) ([]AlertRecord, error) {
	return readJSONL[AlertRecord](alertsPath(dir))
}

// UpdateAlert rewrites the record whose MessageID matches via a temp-file
// swap. Returns false when no record matched.
func UpdateAlert(dir string, updated AlertRecord) (bool, error) {
	alerts, err := ReadAlerts(dir)
	if err != nil {
		return false, err
	}
	matched := false
	for i := range alerts {
		if alerts[i].MessageID == updated.MessageID {
			alerts[i] = updated
			matched = true
		}
	}
	if !matched {
		return false, nil
	}
	tmp, err := os.CreateTemp(dir, "alerts-*.jsonl")
	if err != nil {
		return false, fmt.Errorf("creating temp alert ledger: %w", err)
	}
	enc := json.NewEncoder(tmp)
	for _, a := range alerts {
		if err := enc.Encode(a); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
			return false, fmt.Errorf("rewriting alert ledger: %w", err)
		}
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return false, fmt.Errorf("closing temp alert ledger: %w", err)
	}
	if err := os.Rename(tmp.Name(), alertsPath(dir)); err != nil {
		_ = os.Remove(tmp.Name())
		return false, fmt.Errorf("swapping alert ledger: %w", err)
	}
	return true, nil
}

func readJSONL[T any](path string) ([]T, error) {
	f, err := os.Open(path) // #nosec G304 -- CLI-owned ledger path under the config dir, not user-supplied
	if err != nil {
		if os.IsNotExist(err) {
			return []T{}, nil
		}
		return nil, fmt.Errorf("opening ledger %s: %w", filepath.Base(path), err)
	}
	defer f.Close()
	out := []T{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	line := 0
	for sc.Scan() {
		line++
		if len(sc.Bytes()) == 0 {
			continue
		}
		var rec T
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			return nil, fmt.Errorf("parsing ledger %s line %d: %w", filepath.Base(path), line, err)
		}
		out = append(out, rec)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading ledger %s: %w", filepath.Base(path), err)
	}
	return out, nil
}
