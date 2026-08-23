// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelKbGetCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "get <answer_id>",
		Short:       "Resolve a KB answer id from the init bundle to its full translated body.",
		Example:     "  zammad-cli kb get 42 --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.ErrOrStderr(), "would fetch KB")
				return nil
			}
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				return usageErr(fmt.Errorf("answer_id is required\nUsage: %s <answer_id>", cmd.CommandPath()))
			}
			bundle, err := fetchZammadKBBundle(cmd, flags)
			if err != nil {
				return err
			}
			answerID := strings.TrimSpace(args[0])
			answer, ok := bundle.Answers[answerID]
			if !ok {
				return notFoundErr(fmt.Errorf("KB answer %q not found", answerID))
			}
			return printJSONFiltered(cmd.OutOrStdout(), answer, flags)
		},
	}
	return cmd
}
