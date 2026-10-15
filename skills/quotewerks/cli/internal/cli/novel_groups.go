// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "github.com/spf13/cobra"

// The docs, items and products groups are re-attached with plain AddCommand.
// Behavior is identical to the generated addNovelCommandIfAbsent wiring, but
// press 4.33.5 dogfood maps registered command paths only through AddCommand
// calls, so without this it cannot see "docs search" and flags it as the
// unrelated "tables search" (same for get, create and list).
func init() {
	registerNovelCommand(func(rootCmd *cobra.Command, flags *rootFlags) {
		for _, name := range []string{"docs", "items", "products"} {
			if old, _, err := rootCmd.Find([]string{name}); err == nil && old != rootCmd {
				rootCmd.RemoveCommand(old)
			}
		}
		rootCmd.AddCommand(newNovelDocsCmd(flags))
		rootCmd.AddCommand(newNovelItemsCmd(flags))
		rootCmd.AddCommand(newNovelProductsCmd(flags))
	})
}
