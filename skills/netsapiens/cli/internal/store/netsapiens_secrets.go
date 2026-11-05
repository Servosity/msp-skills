// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// NetSapiens responses carry credentials in ordinary fields: domain and
// billing SMTP passwords (email-send-smtp-password, smtp_pwd), device SIP and
// provisioning passwords, voicemail and conference PINs, SIP auth keys, OAuth
// tokens. The local store is readable through the MCP sql and search tools,
// so no credential is ever written to it: every write (the generic resources
// blob, its FTS index and the typed-table columns, which all derive from the
// merged document) passes through RedactStoredSecrets first, with the shared
// detector (cliutil.IsSecretField). The client already redacted the answer
// (cliutil.RedactCredentials); this is the store's own check.
//
// A store this redaction did not write is never migrated: every open checks
// it once (requireCredentialFree). One that holds a credential is refused with
// how to start over; one that holds none is marked and used as it is. Nothing
// is rewritten, so nothing can be left half-scrubbed. netsapiens never shipped
// a build that stored credentials, so only pre-release test stores are refused.

package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"netsapiens-pp-cli/internal/cliutil"
)

// RedactedValue replaces a credential field's value in the store.
const RedactedValue = cliutil.RedactedValue

// IsSecretField reports whether a JSON field name holds a credential.
func IsSecretField(name string) bool { return cliutil.IsSecretField(name) }

// RedactStoredSecrets returns data with every credential redacted. A document
// that does not decode as JSON cannot be inspected by key, so it is replaced
// by an empty object when it mentions a credential word at all.
func RedactStoredSecrets(data json.RawMessage) json.RawMessage {
	if len(data) == 0 || !bytes.ContainsAny(data, `"`) {
		return data
	}
	var probe any
	if json.Unmarshal(data, &probe) != nil {
		if mentionsSecretWord(data) {
			return json.RawMessage(`{}`)
		}
		return data
	}
	out, _ := cliutil.RedactCredentials(data)
	return out
}

func mentionsSecretWord(data []byte) bool {
	lower := bytes.ToLower(data)
	for _, w := range []string{"password", "passwd", "pwd", "secret", "passcode", "passphrase", "pin", "token", "pass", "otp", "credential", "key"} {
		if bytes.Contains(lower, []byte(w)) {
			return true
		}
	}
	return false
}

// syncReplaceDepth > 0 makes writes replace the stored document instead of
// the keep-richer merge. The sync command holds it for the whole run: a list
// walk is the current state, so a field the API now returns empty, null or
// absent must not keep its old value. Detail write-through outside sync keeps
// merging. A counter, so concurrent workers do not switch it off for each other.
var syncReplaceDepth atomic.Int32

// BeginSyncReplace switches replace-on-write on until the returned func runs.
func BeginSyncReplace() func() {
	syncReplaceDepth.Add(1)
	return func() { syncReplaceDepth.Add(-1) }
}

// SyncReplaceActive reports whether a sync run currently holds replace-on-write.
func SyncReplaceActive() bool { return syncReplaceDepth.Load() > 0 }

// neverWalked is the synced_at of a row only a read outside sync has seen:
// older than any walk, so no audit reads it as returned by one.
const neverWalked = "1970-01-01T00:00:00Z"

// walkObservedAt is the synced_at a write records: now inside a sync walk,
// neverWalked outside one.
func walkObservedAt() string {
	if SyncReplaceActive() {
		return time.Now().UTC().Format(time.RFC3339)
	}
	return neverWalked
}

// redactingBuildMarker records that every row in this store was written by a
// build that redacts credentials (stores the earlier, scrubbing build marked
// are refused too: its scrub could rewrite the CLI's own keys).
const redactingBuildMarker = "credential_free_v2"

// requireCredentialFree is every open's gate. A store with the marker, or one
// whose rows hold no credential (marked now, when writable), is used; any other
// is refused with the way out. readOnly opens never write. Typed tables and the
// FTS index derive from the resources document, so it is the one checked.
func (s *Store) requireCredentialFree(readOnly bool) error {
	var marked int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'netsapiens_store_meta'`).Scan(&marked); err != nil {
		return fmt.Errorf("checking the local store for credentials: %w", err)
	}
	if marked > 0 {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM netsapiens_store_meta WHERE key = ?`, redactingBuildMarker).Scan(&n); err != nil {
			return fmt.Errorf("checking the local store for credentials: %w", err)
		}
		if n > 0 {
			return nil
		}
	}
	held, err := s.resourcesHoldCredential()
	if err != nil {
		return fmt.Errorf("checking the local store for credentials: %w", err)
	}
	if held {
		return fmt.Errorf("refusing the local store at %s: a build that kept vendor credentials in it wrote it, and it is not migrated. Delete it with `netsapiens-cli local-data purge --only store --yes` (or remove the file) and run sync again", s.path)
	}
	if readOnly {
		return nil
	}
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS netsapiens_store_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("marking the local store credential-free: %w", err)
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO netsapiens_store_meta (key, value) VALUES (?, '1')`, redactingBuildMarker); err != nil {
		return fmt.Errorf("marking the local store credential-free: %w", err)
	}
	return nil
}

// resourcesHoldCredential reports whether any stored document carries a
// credential the store's redaction would have replaced.
func (s *Store) resourcesHoldCredential() (bool, error) {
	var tables int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('resources') WHERE name = 'data'`).Scan(&tables); err != nil || tables == 0 {
		return false, err
	}
	rows, err := s.db.Query(`SELECT data FROM resources`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return false, err
		}
		var probe any
		if json.Unmarshal(data, &probe) != nil {
			if mentionsSecretWord(data) {
				return true, nil
			}
			continue
		}
		if _, changed := cliutil.RedactCredentials(data); changed {
			return true, nil
		}
	}
	return false, rows.Err()
}
