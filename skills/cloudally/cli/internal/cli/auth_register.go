// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "github.com/spf13/cobra"

// Attach the CloudAlly login commands to the generated auth group from a
// hook, so regenerating auth.go never drops them.
func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		for _, child := range root.Commands() {
			if child.Name() != "auth" {
				continue
			}
			addNovelCommandIfAbsent(child, newAuthLoginCmd(flags))
			addNovelCommandIfAbsent(child, newAuthUseAccountCmd(flags))
			return
		}
	})
}
