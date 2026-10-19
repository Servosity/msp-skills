// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "github.com/spf13/cobra"

// Attach the portal-backed snapshot reader to the backup group from a hook,
// so regenerating the command tree never drops it.
func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		for _, child := range root.Commands() {
			if child.Name() != "backup" {
				continue
			}
			addNovelCommandIfAbsent(child, newBackupSnapshotsCmd(flags))
			return
		}
	})
}
