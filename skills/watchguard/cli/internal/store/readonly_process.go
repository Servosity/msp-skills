// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: dry-run-side-effect-free).

package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
)

var readOnlyProcess atomic.Bool

// ErrDryRunNoLocalStore is returned when a --dry-run process needs the local
// store and none exists yet: a dry run never creates one.
var ErrDryRunNoLocalStore = errors.New("--dry-run never creates or changes the local store, and none exists yet; run 'watchguard-cli sync --full' first")

// SetReadOnlyProcess makes every later store open in this process read-only
// (the root command sets it for --dry-run), so no command can create, migrate
// or write the local database while previewing.
func SetReadOnlyProcess(on bool) { readOnlyProcess.Store(on) }

// ReadOnlyProcess reports whether store opens are forced read-only.
func ReadOnlyProcess() bool { return readOnlyProcess.Load() }

func openForcedReadOnly(ctx context.Context, dbPath string) (*Store, error) {
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("%w (%s)", ErrDryRunNoLocalStore, dbPath)
	}
	return OpenReadOnlyContext(ctx, dbPath)
}
