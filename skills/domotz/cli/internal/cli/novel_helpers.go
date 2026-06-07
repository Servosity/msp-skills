// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel-feature support: the shared data-source contract for
// the cross-fleet (transcendence) commands. Centralizes the per-command
// retrofit the dogfood gate expects from every hand-written novel command:
// validate the --data-source strategy, open the local mirror, and emit
// sync-freshness hints to stderr before returning local results.

package cli

import (
	"domotz-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

// novelOpenStoreChecked is the one-call data-source contract for store-backed
// (strategy "local") novel commands. It rejects --data-source live with a
// clear "no live equivalent" error, opens the local SQLite mirror at dbPath
// (or the default path when empty), and emits unsynced/stale hints on stderr
// for the command's primary resource. Use "" for commands that scan all
// local resources.
func novelOpenStoreChecked(cmd *cobra.Command, flags *rootFlags, dbPath, primaryResource string) (*store.Store, error) {
	if err := validateDataSourceStrategy(flags, "local"); err != nil {
		return nil, err
	}
	db, err := openFleetStore(cmd.Context(), dbPath)
	if err != nil {
		return nil, err
	}
	maybeEmitSyncHints(cmd, db, primaryResource, flags.maxAge)
	return db, nil
}

// novelRequireLive is the data-source contract for live fan-out (strategy
// "live") novel commands. It rejects --data-source local with a clear "no
// local data source" error before the command dials the API.
func novelRequireLive(flags *rootFlags) error {
	return validateDataSourceStrategy(flags, "live")
}
