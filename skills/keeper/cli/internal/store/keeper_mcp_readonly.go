// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

// keeper handfix (read-only-mcp-writes-nothing): a CLI child of the MCP server cannot
// write the store. Only a command the root classifies as a writer (posture report,
// behind its confirm gate) lifts this; every other child gets a read-only handle, so
// a prune, a usage row or a migration in a read-only tool fails instead of writing,
// whichever code path it takes. A store that does not exist yet reads as an empty one
// in memory, and no file appears.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync/atomic"

	"keeper-pp-cli/internal/cliutil"
	"keeper-pp-cli/internal/credsql"
)

var mcpChildMayWrite atomic.Bool

// AllowMCPChildWrites says whether this process's command may write the store.
func AllowMCPChildWrites(may bool) { mcpChildMayWrite.Store(may) }

func readOnlyMCPChild() bool { return cliutil.IsMCPChildEnv() && !mcpChildMayWrite.Load() }

func openMCPChildStore(ctx context.Context, dbPath string) (*Store, error) {
	if _, err := os.Stat(dbPath); !errors.Is(err, fs.ErrNotExist) {
		return OpenReadOnlyContext(ctx, dbPath)
	}
	db, err := sql.Open(credsql.Driver, fmt.Sprintf("file:keeper-mcp-empty-%d?mode=memory&cache=shared", os.Getpid()))
	if err != nil {
		return nil, fmt.Errorf("opening an empty store: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("preparing an empty store: %w", err)
	}
	return s, nil
}
