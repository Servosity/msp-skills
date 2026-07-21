// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "github.com/spf13/cobra"

// init registers the hand-authored Granola novel commands through the
// generator's durable novelCommandHooks extension point, so root.go stays
// generated. The v7.4x rewire routes all data through the public REST API
// (GRANOLA_API_KEY) and Granola's official MCP; the encrypted desktop store
// is sealed on v7.4x+ and is only a soft-failing shim.
func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		// Reclaim the top-level "sync" name for the cache→REST+enrich
		// wrapper. The generator-emitted public-API sync is re-exposed as
		// "sync-api" (newSyncApiCmd wraps newSyncCmd internally).
		for _, c := range root.Commands() {
			if c.Name() == "sync" {
				root.RemoveCommand(c)
			}
		}
		root.AddCommand(newSyncCacheCmd(flags)) // "sync"
		root.AddCommand(newSyncApiCmd(flags))   // "sync-api"
		root.AddCommand(newEnrichCmd(flags))
		root.AddCommand(newMCPAuthCmd(flags))

		// Meeting / transcript / notes surface (REST-fed store + MCP human notes).
		root.AddCommand(newMeetingsCmd(flags))
		root.AddCommand(newTranscriptCmd(flags))
		root.AddCommand(newPanelCmd(flags))
		root.AddCommand(newNotesShowCmd(flags))
		root.AddCommand(newExtractCmd(flags))
		root.AddCommand(newExportAllCmd(flags))
		root.AddCommand(newPreflightCmd(flags))
		root.AddCommand(newWarmCmd(flags))
		root.AddCommand(newShowCmd(flags))
		root.AddCommand(newStatsCmd(flags))
		root.AddCommand(newCollectCmd(flags))
		root.AddCommand(newRecipesCmd(flags))
		root.AddCommand(newWorkspacesCmd(flags))
		root.AddCommand(newMemoCmd(flags))
		root.AddCommand(newAttendeeCmd(flags))
		root.AddCommand(newFolderCmd(flags))
		root.AddCommand(newTalktimeCmd(flags))
		root.AddCommand(newChatCmd(flags))
		root.AddCommand(newDuplicatesCmd(flags))
		root.AddCommand(newTiptapCmd(flags))
		root.AddCommand(newCalendarCmd(flags))
	})
}
