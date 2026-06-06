// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelConditionCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "condition",
		Short: "Build or explain a ConnectWise conditions filter expression.",
		Long: strings.Trim(`
The ConnectWise conditions DSL is strict — string values are double-quoted,
dates are bracketed ([2024-01-01T00:00:00Z]), the default join is AND, and an
OR set must be parenthesized. Getting it wrong yields silent empty results or a
400. These subcommands construct a correct expression for you ('build') or
break an existing one into its clauses ('explain').`, "\n"),
	}
	cmd.AddCommand(newConditionBuildCmd(flags))
	cmd.AddCommand(newConditionExplainCmd(flags))
	return cmd
}

func newConditionBuildCmd(flags *rootFlags) *cobra.Command {
	var fields, ops, values []string
	var join string

	cmd := &cobra.Command{
		Use:   "build",
		Short: "Assemble a validated conditions string from repeated --field/--op/--value clauses.",
		Example: strings.Trim(`
  connectwise-manage-cli condition build --field board/name --op = --value "Help Desk"
  connectwise-manage-cli condition build --field board/id --op in --value 2,3 --field status/name --op = --value New
  connectwise-manage-cli condition build --field closedFlag --op = --value false --field summary --op like --value "%vpn%" --join or`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if len(fields) == 0 {
				return cmd.Help()
			}
			if len(fields) != len(ops) || len(fields) != len(values) {
				return fmt.Errorf("--field, --op and --value must be repeated the same number of times (got %d field, %d op, %d value)", len(fields), len(ops), len(values))
			}
			clauses := make([]condClause, len(fields))
			for i := range fields {
				clauses[i] = condClause{Field: fields[i], Op: ops[i], Value: values[i]}
			}
			expr, err := buildConditions(clauses, join)
			if err != nil {
				return err
			}
			if flags.asJSON || flags.compact || flags.csv || flags.quiet || (!isTerminal(cmd.OutOrStdout()) && !humanFriendly) {
				return flags.printJSON(cmd, map[string]string{"conditions": expr})
			}
			fmt.Fprintln(cmd.OutOrStdout(), expr)
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&fields, "field", nil, "Field path, e.g. board/name or status/id (repeatable)")
	cmd.Flags().StringArrayVar(&ops, "op", nil, `Operator: = != < <= > >= contains like in "not in" (repeatable)`)
	cmd.Flags().StringArrayVar(&values, "value", nil, "Value; comma-separate the list for in/not in (list elements cannot contain commas) (repeatable)")
	cmd.Flags().StringVar(&join, "join", "and", "Join the clauses with and|or")
	return cmd
}

func newConditionExplainCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "explain [conditions]",
		Short: "Break a conditions expression into its top-level clauses.",
		Example: strings.Trim(`
  connectwise-manage-cli condition explain 'board/name="Help Desk" AND closedFlag=false'
  connectwise-manage-cli condition explain '(status/id in (1,2) OR priority/name="High")' --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			expr := strings.Join(args, " ")
			clauses, jn := explainConditions(expr)
			if clauses == nil {
				clauses = []string{}
			}
			result := map[string]any{"conditions": expr, "join": jn, "clauses": clauses, "clause_count": len(clauses)}
			if flags.asJSON || flags.compact || flags.csv || flags.quiet || (!isTerminal(cmd.OutOrStdout()) && !humanFriendly) {
				return flags.printJSON(cmd, result)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "conditions: %s\n", expr)
			if jn != "" {
				fmt.Fprintf(w, "join: %s\n", jn)
			}
			for i, c := range clauses {
				fmt.Fprintf(w, "  %d. %s\n", i+1, c)
			}
			return nil
		},
	}
	return cmd
}
