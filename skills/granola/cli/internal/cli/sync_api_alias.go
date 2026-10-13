// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

// newSyncApiCmd exposes the generator-emitted public-API sync command
// under its own name ("sync-api") so we can claim the top-level "sync"
// for the cache-hydration command. Both surfaces stay available.
func newSyncApiCmd(flags *rootFlags) *cobra.Command {
	cmd := newSyncCmd(flags)
	cmd.Use = "sync-api"
	cmd.Short = "Sync the Granola PUBLIC API (~3 endpoints) into the local store"
	cmd.Long = `Calls Granola's public REST API (using GRANOLA_API_KEY) to sync the
narrow set of resources the public spec exposes — notes list/get and
folders. For transcripts, panels, and attendees, use the top-level
'sync' command, which runs this public sync and then enriches the rich
tables from the REST detail endpoint (/v1/notes/{id}).`
	cmd.Example = strings.Trim(`
  granola-cli sync-api
  granola-cli sync-api --full
  granola-cli sync-api --since 7d`, "\n")
	return cmd
}
