// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
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
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gradient-pp-cli/internal/cliutil"
	"gradient-pp-cli/internal/credkit"
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
	Error        string    `json:"error,omitempty"` // why a "failed" alert did not become a ticket
}

func pushesPath(dir string) string { return filepath.Join(dir, "pushes.jsonl") }
func alertsPath(dir string) string { return filepath.Join(dir, "alerts.jsonl") }

// AppendPushes appends push records to the ledger.
// Every ledger write goes through the shared credential kit: records carry
// API error text and operator-typed titles (gradient handfix:
// ledger-files-withheld, handfixes.json credkit-wiring). The records of one
// call are one unit, appended with one credkit.AppendFile.
func AppendPushes(dir string, records []PushRecord) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, r := range records {
		if err := enc.Encode(r); err != nil {
			return fmt.Errorf("writing push ledger: %w", err)
		}
	}
	if buf.Len() == 0 {
		return nil
	}
	if err := credkit.AppendFile(pushesPath(dir), buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("writing push ledger: %w", err)
	}
	return nil
}

// ReadPushes returns every push record in file order (oldest first).
// A missing ledger file yields an empty slice, not an error.
func ReadPushes(dir string) ([]PushRecord, error) {
	return readJSONL[PushRecord](pushesPath(dir))
}

// AppendAlert appends one alert dispatch record, through the credential kit
// (handfixes.json credkit-wiring).
func AppendAlert(dir string, rec AlertRecord) error {
	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("writing alert ledger: %w", err)
	}
	if err := credkit.AppendFile(alertsPath(dir), append(line, '\n'), 0o600); err != nil {
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
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, a := range alerts {
		if err := enc.Encode(a); err != nil {
			return false, fmt.Errorf("rewriting alert ledger: %w", err)
		}
	}
	// A temp-file swap whose bytes go through the credential kit
	// (cliutil.AtomicWritePrivateFile writes the temp with credkit.WriteFile).
	if err := cliutil.AtomicWritePrivateFile(alertsPath(dir), buf.Bytes(), 0o600, 0o700); err != nil {
		return false, fmt.Errorf("rewriting alert ledger: %w", err)
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
		// Hand-fix (handfixes.json credkit-wiring): a record the credential kit
		// withheld is its notice line in the file; it is no record, so the
		// readers (usage drift, alert trace) pass over it instead of failing.
		if string(bytes.TrimSpace(sc.Bytes())) == credkit.Notice(credkit.SinkFile) {
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
