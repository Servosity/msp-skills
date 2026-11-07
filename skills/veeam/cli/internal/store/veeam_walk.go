// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

// Persistence for the walk membership the Veeam local-mirror reports read
// (table veeam_walk, created in migrateExtras). The rules for when a walk
// starts, resumes, finishes or breaks live in internal/cli/veeam_walk.go; this
// file only reads and writes the record, under the store write lock so a walk
// update serializes with the batch upserts of sync workers on other partitions.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// Walk states. Only a finished walk proves which records a partition holds.
const (
	VeeamWalkWalking  = "walking"
	VeeamWalkFinished = "finished"
	VeeamWalkBroken   = "broken"
)

// VeeamWalk is one partition's record: the state of its latest unrestricted
// sync walk, where a walk capped by --max-pages resumes (NextCursor), the
// record IDs the in-progress walk has returned so far (Pending), and the IDs
// the last finished walk returned (Members).
type VeeamWalk struct {
	State      string
	NextCursor string
	Pending    []string
	Members    []string
}

type veeamWalkQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// ReadVeeamWalk returns resourceType's walk record. ok is false when there is
// none, including a store created before the table existed (a read-only handle
// never migrates). An unreadable ID list proves nothing, so it reads as broken.
func (s *Store) ReadVeeamWalk(ctx context.Context, resourceType string) (VeeamWalk, bool, error) {
	return readVeeamWalk(ctx, s.db, resourceType)
}

func readVeeamWalk(ctx context.Context, q veeamWalkQuerier, resourceType string) (VeeamWalk, bool, error) {
	var tables int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'veeam_walk'`).Scan(&tables); err != nil {
		return VeeamWalk{}, false, err
	}
	if tables == 0 {
		return VeeamWalk{}, false, nil
	}
	var w VeeamWalk
	var pending, members string
	err := q.QueryRowContext(ctx, `SELECT state, next_cursor, pending, members FROM veeam_walk WHERE resource_type = ?`, resourceType).
		Scan(&w.State, &w.NextCursor, &pending, &members)
	if errors.Is(err, sql.ErrNoRows) {
		return VeeamWalk{}, false, nil
	}
	if err != nil {
		return VeeamWalk{}, false, err
	}
	if json.Unmarshal([]byte(pending), &w.Pending) != nil || json.Unmarshal([]byte(members), &w.Members) != nil {
		return VeeamWalk{State: VeeamWalkBroken}, true, nil
	}
	return w, true, nil
}

// UpdateVeeamWalk applies f to resourceType's walk record (ok is false when
// there is none yet) and stores the result, in one write transaction.
func (s *Store) UpdateVeeamWalk(ctx context.Context, resourceType string, f func(w VeeamWalk, ok bool) VeeamWalk) error {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting the %s walk update: %w", resourceType, err)
	}
	defer tx.Rollback() //nolint:errcheck
	w, ok, err := readVeeamWalk(ctx, tx, resourceType)
	if err != nil {
		return err
	}
	w = f(w, ok)
	if w.Pending == nil {
		w.Pending = []string{}
	}
	if w.Members == nil {
		w.Members = []string{}
	}
	pending, err := json.Marshal(w.Pending)
	if err != nil {
		return err
	}
	members, err := json.Marshal(w.Members)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO veeam_walk (resource_type, state, next_cursor, pending, members) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(resource_type) DO UPDATE SET state = excluded.state, next_cursor = excluded.next_cursor,
		pending = excluded.pending, members = excluded.members`,
		resourceType, w.State, w.NextCursor, string(pending), string(members)); err != nil {
		return err
	}
	return tx.Commit()
}
