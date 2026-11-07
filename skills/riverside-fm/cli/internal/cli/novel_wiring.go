// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "github.com/spf13/cobra"

// The generated root.go and parent groups register novel commands through
// newNovel*Cmd constructors. The implementations predate that naming (they
// shipped on press 4.4.0), so each constructor below hands the generated call
// site the real command. Parents (bulk, media, transcripts) attach their own
// subcommands, so the generated stub subcommands are not needed.

func newNovelBulkCmd(flags *rootFlags) *cobra.Command        { return newBulkCmd(flags) }
func newNovelGrabCmd(flags *rootFlags) *cobra.Command        { return newGrabCmd(flags) }
func newNovelMediaCmd(flags *rootFlags) *cobra.Command       { return newMediaCmd(flags) }
func newNovelReadyCmd(flags *rootFlags) *cobra.Command       { return newReadyCmd(flags) }
func newNovelSearchCmd(flags *rootFlags) *cobra.Command      { return newSearchCmd(flags) }
func newNovelTranscriptsCmd(flags *rootFlags) *cobra.Command { return newTranscriptsCmd(flags) }
func newNovelWaitCmd(flags *rootFlags) *cobra.Command        { return newWaitCmd(flags) }
func newNovelStaleCmd(flags *rootFlags) *cobra.Command       { return newStaleCmd(flags) }
func newNovelClipsHarvestCmd(flags *rootFlags) *cobra.Command {
	return newClipsHarvestCmd(flags)
}
func newNovelClipsListCmd(flags *rootFlags) *cobra.Command { return newClipsListCmd(flags) }
