// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

// Hand-written (NOT generated; handfixes.json read-only-tools-change-nothing):
// under cliutil.RefuseWrites, OpenWithContext opens read-only: nothing is
// created or migrated, and a delete or insert fails in SQLite.

import (
	"context"
	"errors"
	"os"
)

func openWritesRefused(ctx context.Context, dbPath string) (*Store, error) {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, errors.New("no local data yet: run sync first")
	}
	return openReadOnly(ctx, dbPath, 0)
}
