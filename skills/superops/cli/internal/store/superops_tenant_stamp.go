// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): a SuperOps store records the tenant that
// claimed it, and every open refuses a store of another tenant and a store
// that records no tenant (superops-tenant-scope, tenant stamp).

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"superops-pp-cli/internal/config"
)

// TenantStampTable records the one tenant a store belongs to. It holds the
// tenant identity string (config.SuperOpsTenant: the subdomain and the base
// URL's scheme, host and path), never a credential or a hash of one.
const TenantStampTable = "superops_store_tenant"

// tenantStampVersion is the version a record is written at. Version 1 (a
// table without the version column) is the earlier identity, which kept the
// whole base URL, so a password in its userinfo or a query string was stored
// and readable through the MCP sql tool. Version 2 keeps only scheme, host
// and path (superops-tenant-identity-no-credential). A read-write open of its
// own tenant rewrites a version 1 record, and no open reads one that still
// holds such text (retireLegacyTenantStamp).
const tenantStampVersion = 2

// ErrStoreTenant is the error class of every tenant refusal; match it with
// errors.Is.
var ErrStoreTenant = errors.New("local store belongs to another SuperOps tenant")

// TenantRefusal is the refusal of one store for the running tenant. Its text
// names the store path and the remedy, and never a tenant or a credential.
type TenantRefusal struct {
	Path string
	// Unproven is a store that records no tenant and holds a row in some
	// table, so nothing proves whose data it is.
	Unproven bool
	// Unclaimed is a store that records no tenant and holds no row: nothing
	// has been synced into it for this tenant. Only a read-write open can
	// claim it, so a read-only open is refused rather than reading a file
	// another tenant could claim and fill before the read.
	Unclaimed bool
	// UnknownTenant is an open whose running tenant could not be determined
	// (the configuration did not load), which proves no store is its own.
	UnknownTenant bool
	// Unscrubbed is a read-only open of this tenant's store whose record is
	// still in the version 1 form, which can hold a credential from the base
	// URL; only a read-write open can rewrite it.
	Unscrubbed bool
}

func (e *TenantRefusal) Error() string {
	switch {
	case e.Unscrubbed:
		return fmt.Sprintf("local store %s records its SuperOps tenant in an earlier form that can hold a password or query string "+
			"from SUPEROPS_BASE_URL, so it is not opened read-only. Run any superops-cli command that writes the store as this "+
			"tenant (for example 'superops-cli sync', with the same --db if one was given) to rewrite the record, or delete the file",
			e.Path)
	case e.UnknownTenant:
		return fmt.Sprintf("local store %s is not read or written because the running SuperOps tenant could not be determined "+
			"(the configuration did not load). Run 'superops-cli doctor' to see why, fix the configuration, then run "+
			"'superops-cli sync' (with the same --db if one was given); the file is left in place",
			e.Path)
	case e.Unclaimed:
		return fmt.Sprintf("local store %s records no SuperOps tenant: no data has been synced for this tenant into that file, "+
			"so it is not read. Run 'superops-cli sync' for this tenant first (with the same --db if one was given); "+
			"the sync records the tenant and fills the file",
			e.Path)
	}
	why := "belongs to a different SuperOps tenant (another subdomain or API host)"
	if e.Unproven {
		why = "holds data but does not record which SuperOps tenant it belongs to"
	}
	return fmt.Sprintf("local store %s %s, so it is not read or written as this tenant. "+
		"Run 'superops-cli sync' for this tenant into a fresh --db path, or without --db "+
		"into the default store (kept per token, subdomain and host); the file is left in place",
		e.Path, why)
}

func (e *TenantRefusal) Is(target error) bool { return target == ErrStoreTenant }

var storeTenantSource struct {
	sync.RWMutex
	fn func() (tenant string, ok bool)
}

// SetTenantSource installs the function every open asks for the running
// tenant. The cli package installs it in init, so both binaries carry it (the
// MCP server imports cli for MCPStorePath); only code that never imports cli,
// such as this package's own tests, opens stores unchecked. ok false means
// the tenant cannot be determined, and then no store that holds or records a
// tenant is opened.
func SetTenantSource(fn func() (tenant string, ok bool)) {
	storeTenantSource.Lock()
	storeTenantSource.fn = fn
	storeTenantSource.Unlock()
}

func currentTenantSource() func() (string, bool) {
	storeTenantSource.RLock()
	defer storeTenantSource.RUnlock()
	return storeTenantSource.fn
}

type tenantSourceKey struct{}

// WithTenantSource returns ctx carrying the tenant source of one run: one
// command execution or one MCP tool call. Every open made with that context
// asks fn, not the process-wide source, so the cli package can check a store
// against the tenant of the one configuration the run's client was built
// from, with no second read of the configuration in between.
func WithTenantSource(ctx context.Context, fn func() (tenant string, ok bool)) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, tenantSourceKey{}, fn)
}

// tenantSourceOf is the run's source when ctx carries one, else the
// process-wide source.
func tenantSourceOf(ctx context.Context) func() (string, bool) {
	if ctx != nil {
		if fn, ok := ctx.Value(tenantSourceKey{}).(func() (string, bool)); ok && fn != nil {
			return fn
		}
	}
	return currentTenantSource()
}

// enforceStoreTenant is the one tenant check of every SuperOps store open:
// OpenReadOnlyContext calls it with writable false, OpenWithContext with
// writable true before it migrates anything. The running tenant comes from
// the source in ctx (WithTenantSource), else the process-wide one.
//
// A store that records a tenant opens only for that tenant. A store that
// records no tenant is never read: a read-only open refuses it (it cannot
// record a tenant, and another tenant could claim and fill the file before
// the read), an open whose tenant is unknown refuses it, and a read-write
// open claims it (claimTenant) only when no table in it holds a row, inside
// the same write-locked transaction that records the tenant, before it
// migrates or writes anything. Every other open is refused.
func (s *Store) enforceStoreTenant(ctx context.Context, writable bool) error {
	source := tenantSourceOf(ctx)
	if source == nil {
		return nil
	}
	tenant, known := source()
	deadline := time.Now().Add(migrationLockTimeout)
	var stamp string
	var stamped bool
	if err := retryOnBusy(ctx, deadline, "reading the recorded tenant", func() error {
		var err error
		stamp, stamped, err = readTenantStamp(ctx, s.db)
		return err
	}); err != nil {
		return fmt.Errorf("local store %s: %w", s.path, err)
	}
	if stamped {
		// superops-tenant-identity-no-credential: a version 1 record is
		// rewritten or refused before it is compared, and never printed.
		var err error
		if stamp, err = s.retireLegacyTenantStamp(ctx, deadline, stamp, tenant, known, writable); err != nil {
			return err
		}
		if !known {
			return &TenantRefusal{Path: s.path, UnknownTenant: true}
		}
		if stamp != tenant {
			return &TenantRefusal{Path: s.path}
		}
		return nil
	}
	if !writable || !known {
		// Refused either way; the check only picks the remedy to name.
		var holds bool
		if err := retryOnBusy(ctx, deadline, "checking the store for data", func() error {
			var err error
			holds, err = storeHoldsRows(ctx, s.db)
			return err
		}); err != nil {
			return fmt.Errorf("local store %s: %w", s.path, err)
		}
		switch {
		case holds:
			return &TenantRefusal{Path: s.path, Unproven: true}
		case !known:
			return &TenantRefusal{Path: s.path, UnknownTenant: true}
		default:
			return &TenantRefusal{Path: s.path, Unclaimed: true}
		}
	}
	var refusal *TenantRefusal
	err := retryOnBusy(ctx, deadline, "recording the tenant of local store "+s.path, func() error {
		err := s.claimTenant(ctx, tenant)
		if errors.As(err, &refusal) {
			return nil
		}
		return err
	})
	if refusal != nil {
		return refusal
	}
	return err
}

// claimTenant records tenant in a store that records none. It runs on one
// connection inside BEGIN IMMEDIATE, which takes the write lock before the
// first read, so the check that no table holds a row and the record it
// allows are one step: no other process or connection can add a row or a
// record in between. Of two claims of one file, the second waits for the
// first and then finds its record, so it is refused (or, for the same
// tenant, finds its own).
func (s *Store) claimTenant(ctx context.Context, tenant string) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), `ROLLBACK`)
		}
	}()
	stamp, stamped, err := readTenantStamp(ctx, conn)
	if err != nil {
		return err
	}
	if stamped {
		if stamp != tenant {
			return &TenantRefusal{Path: s.path}
		}
		return nil
	}
	holds, err := storeHoldsRows(ctx, conn)
	if err != nil {
		return err
	}
	if holds {
		return &TenantRefusal{Path: s.path, Unproven: true}
	}
	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS `+TenantStampTable+` (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		tenant TEXT NOT NULL,
		stamped_at TEXT NOT NULL,
		version INTEGER NOT NULL DEFAULT 1
	)`); err != nil {
		return err
	}
	if err := ensureTenantStampVersionColumn(ctx, conn); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO `+TenantStampTable+` (id, tenant, stamped_at, version) VALUES (1, ?, ?, ?)`,
		tenant, time.Now().UTC().Format(time.RFC3339), tenantStampVersion); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return err
	}
	committed = true
	return nil
}

// retireLegacyTenantStamp brings a version 1 record to version 2 and returns
// the record to compare with the running tenant. A record already at version
// 2, a record of another tenant, and any record when the running tenant is
// unknown come back untouched (the caller refuses the last two, printing
// nothing of the record). A version 1 record of this tenant is rewritten, with
// every checkpoint scope key, by a read-write open. A read-only open cannot
// rewrite it, so it refuses the store when the record or a scope key still
// holds text the identity now drops (TenantRefusal.Unscrubbed).
func (s *Store) retireLegacyTenantStamp(ctx context.Context, deadline time.Time, stamp, tenant string, known, writable bool) (string, error) {
	var version int
	if err := retryOnBusy(ctx, deadline, "reading the recorded tenant version", func() error {
		var err error
		version, err = readTenantStampVersion(ctx, s.db)
		return err
	}); err != nil {
		return "", fmt.Errorf("local store %s: %w", s.path, err)
	}
	if version >= tenantStampVersion || !known || config.SuperOpsTenantFromRecord(stamp) != tenant {
		return stamp, nil
	}
	if !writable {
		stale := stamp != tenant
		if !stale {
			if err := retryOnBusy(ctx, deadline, "reading the checkpoint scopes", func() error {
				scopes, err := legacyScopeKeys(ctx, s.db)
				stale = len(scopes) > 0
				return err
			}); err != nil {
				return "", fmt.Errorf("local store %s: %w", s.path, err)
			}
		}
		if stale {
			return "", &TenantRefusal{Path: s.path, Unscrubbed: true}
		}
		return stamp, nil
	}
	var recorded string
	err := retryOnBusy(ctx, deadline, "rewriting the recorded tenant of local store "+s.path, func() error {
		var err error
		recorded, err = s.scrubLegacyTenantStamp(ctx, tenant)
		return err
	})
	return recorded, err
}

// scrubLegacyTenantStamp rewrites this tenant's version 1 record and its
// checkpoint scope keys in one immediate transaction, on one connection with
// secure_delete on so the old text is zeroed in its page rather than left in
// free space, then checkpoints the WAL so neither the main file nor the log
// keeps an earlier page. It returns the record as it stands afterwards.
func (s *Store) scrubLegacyTenantStamp(ctx context.Context, tenant string) (string, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	var secureDelete int
	if err := conn.QueryRowContext(ctx, `PRAGMA secure_delete`).Scan(&secureDelete); err != nil {
		return "", err
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA secure_delete = ON`); err != nil {
		return "", err
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), fmt.Sprintf(`PRAGMA secure_delete = %d`, secureDelete))
	}()
	recorded, scrubbed, err := scrubLegacyTenantStampTx(ctx, conn, tenant)
	if err != nil {
		return "", err
	}
	if scrubbed {
		// Best effort: a reader holding an older snapshot leaves the rest
		// to the next checkpoint, which the last close of the store runs.
		_, _ = conn.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	}
	return recorded, nil
}

func scrubLegacyTenantStampTx(ctx context.Context, conn *sql.Conn, tenant string) (string, bool, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback()
	stamp, stamped, err := readTenantStamp(ctx, tx)
	if err != nil || !stamped {
		return "", false, err
	}
	version, err := readTenantStampVersion(ctx, tx)
	if err != nil {
		return "", false, err
	}
	if version >= tenantStampVersion || config.SuperOpsTenantFromRecord(stamp) != tenant {
		return stamp, false, nil
	}
	if err := ensureTenantStampVersionColumn(ctx, tx); err != nil {
		return "", false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE `+TenantStampTable+` SET tenant = ?, version = ? WHERE id = 1`, tenant, tenantStampVersion); err != nil {
		return "", false, err
	}
	scopes, err := legacyScopeKeys(ctx, tx)
	if err != nil {
		return "", false, err
	}
	for resource, scope := range scopes {
		if _, err := tx.ExecContext(ctx, `UPDATE sync_state SET scope_key = ? WHERE resource_type = ?`, scope, resource); err != nil {
			return "", false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", false, err
	}
	return tenant, true, nil
}

// legacyScopeKeys returns, per resource, the rewritten form of every
// checkpoint scope key whose tenant line the identity rule now rewrites. Each
// key keeps the tenant it named (re-read through config.SuperOpsTenantFromRecord),
// so a checkpoint of this tenant resumes and any other is reset by the next
// sync as before.
func legacyScopeKeys(ctx context.Context, q tenantRowsQuerier) (map[string]string, error) {
	exists, err := tenantTableExists(ctx, q, "sync_state")
	if err != nil || !exists {
		return nil, err
	}
	hasScope, err := tableHasColumn(ctx, q, "sync_state", "scope_key")
	if err != nil || !hasScope {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, `SELECT resource_type, scope_key FROM sync_state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	scopes := map[string]string{}
	for rows.Next() {
		var resource string
		var scope sql.NullString
		if err := rows.Scan(&resource, &scope); err != nil {
			return nil, err
		}
		if clean := scrubTenantScopeKey(scope.String); clean != scope.String {
			scopes[resource] = clean
		}
	}
	return scopes, rows.Err()
}

// scrubTenantScopeKey rewrites the tenant line of one checkpoint scope key
// (the generated syncCheckpointScope form: sorted kind:name=value lines).
func scrubTenantScopeKey(key string) string {
	prefix := "template:" + config.SuperOpsTenantScopeVar + "="
	lines := strings.Split(key, "\n")
	changed := false
	for i, line := range lines {
		record, ok := strings.CutPrefix(line, prefix)
		if !ok {
			continue
		}
		if clean := prefix + config.SuperOpsTenantFromRecord(record); clean != line {
			lines[i] = clean
			changed = true
		}
	}
	if !changed {
		return key
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// readTenantStampVersion is 1 for a table without the version column.
func readTenantStampVersion(ctx context.Context, q tenantQuerier) (int, error) {
	hasVersion, err := tableHasColumn(ctx, q, TenantStampTable, "version")
	if err != nil || !hasVersion {
		return 1, err
	}
	var version int
	err = q.QueryRowContext(ctx, `SELECT version FROM `+TenantStampTable+` WHERE id = 1`).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return 1, nil
	}
	return version, err
}

func ensureTenantStampVersionColumn(ctx context.Context, q tenantExecer) error {
	hasVersion, err := tableHasColumn(ctx, q, TenantStampTable, "version")
	if err != nil || hasVersion {
		return err
	}
	_, err = q.ExecContext(ctx, `ALTER TABLE `+TenantStampTable+` ADD COLUMN version INTEGER NOT NULL DEFAULT 1`)
	return err
}

func tableHasColumn(ctx context.Context, q tenantQuerier, table, column string) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n)
	return n > 0, err
}

type tenantQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type tenantRowsQuerier interface {
	tenantQuerier
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

type tenantExecer interface {
	tenantQuerier
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func tenantTableExists(ctx context.Context, q tenantQuerier, name string) (bool, error) {
	var found string
	err := q.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func readTenantStamp(ctx context.Context, q tenantQuerier) (string, bool, error) {
	exists, err := tenantTableExists(ctx, q, TenantStampTable)
	if err != nil || !exists {
		return "", false, err
	}
	var tenant string
	err = q.QueryRowContext(ctx, `SELECT tenant FROM `+TenantStampTable+` WHERE id = 1`).Scan(&tenant)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return tenant, true, nil
}

// storeHoldsRows reports whether any table of the store holds a row: every
// table in sqlite_master except SQLite's own and the tenant record, so the
// synced tables, the sync checkpoints, the FTS tables and their shadow
// tables, the learn tables and any table a later build adds all count. A
// table this build cannot read is an error, never "empty".
func storeHoldsRows(ctx context.Context, q tenantRowsQuerier) (bool, error) {
	rows, err := q.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite\_%' ESCAPE '\' AND name <> ?`, TenantStampTable)
	if err != nil {
		return false, err
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return false, err
		}
		tables = append(tables, name)
	}
	if err := rows.Close(); err != nil {
		return false, err
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	for _, table := range tables {
		var one int
		err := q.QueryRowContext(ctx, `SELECT 1 FROM "`+strings.ReplaceAll(table, `"`, `""`)+`" LIMIT 1`).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return false, fmt.Errorf("checking table %q for rows: %w", table, err)
		}
		return true, nil
	}
	return false, nil
}
