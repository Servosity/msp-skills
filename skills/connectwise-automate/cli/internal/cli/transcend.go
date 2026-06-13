// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"connectwise-automate-pp-cli/internal/store"
)

// transcendStoreLimit is effectively "all rows" — the fleet roll-ups operate
// over the whole synced mirror, not a page.
const transcendStoreLimit = 1000000

// openTranscendStore opens the local SQLite mirror the transcendence commands
// read from, creating the data directory (and an empty DB) if it does not yet
// exist. An empty/just-created store yields empty roll-ups rather than an error,
// so the commands stay safe to run before the first `sync`.
func openTranscendStore(cmd *cobra.Command) (*store.Store, error) {
	dbPath := defaultDBPath("connectwise-automate-cli")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("creating data dir %s: %w", filepath.Dir(dbPath), err)
	}
	return store.OpenWithContext(cmd.Context(), dbPath)
}

// loadResource returns every synced object of one resource type as decoded
// maps (json.Number-preserving), ready for the fleet aggregation functions.
func loadResource(s *store.Store, resourceType string) ([]map[string]any, error) {
	raws, err := s.List(resourceType, transcendStoreLimit)
	if err != nil {
		return nil, fmt.Errorf("reading %s from local store: %w", resourceType, err)
	}
	out := make([]map[string]any, 0, len(raws))
	for _, r := range raws {
		obj, err := store.DecodeJSONObject(r)
		if err != nil {
			continue
		}
		out = append(out, obj)
	}
	return out, nil
}

// emitResult prints a Go-typed result through the shared output pipeline so the
// transcendence commands pick up --json, --select, --compact, --csv, and
// --quiet for free.
func emitResult(cmd *cobra.Command, flags *rootFlags, v any) error {
	return printJSONFiltered(cmd.OutOrStdout(), v, flags)
}
