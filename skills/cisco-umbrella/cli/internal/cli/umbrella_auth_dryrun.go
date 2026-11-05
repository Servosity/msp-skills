// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// umbrella handfix (dry-run-no-token-mint): 'auth login', 'auth set-token' and
// 'auth logout' honour --dry-run. Under it they describe what they would do and
// neither send the key and secret to the token endpoint nor read, save or delete
// a credential.

import "github.com/spf13/cobra"

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		auth, _, err := root.Find([]string{"auth"})
		if err != nil || auth == root {
			return
		}
		for _, sub := range auth.Commands() {
			switch sub.Name() {
			case "login", "set-token", "logout":
			default:
				continue
			}
			next, name := sub.RunE, "auth "+sub.Name()
			if next == nil {
				continue
			}
			sub.RunE = func(cmd *cobra.Command, args []string) error {
				if dryRunOK(flags) {
					return writeDryRun(cmd.OutOrStdout(), flags, name)
				}
				return next(cmd, args)
			}
		}
	})
}
