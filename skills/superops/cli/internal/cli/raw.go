// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// newRawCmd is the GraphQL escape hatch: run any SuperOps query or mutation
// with full control over the document and variables. It matches and beats the
// custom_query/custom_mutation tools other SuperOps integrations expose, adding
// --dry-run, file/stdin input, --vars from inline JSON or @file, and structured
// output.
func newRawCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "raw",
		Short: "Run an arbitrary SuperOps GraphQL query or mutation",
		Long: `Send a raw GraphQL document to the SuperOps API. Use 'raw query' for reads and
'raw mutation' for writes. The document may be passed as an argument, as @path to
read from a file, or as - to read from stdin. Variables are supplied with --vars
as inline JSON or @path.

This is the supported path for any operation the typed commands do not wrap yet
(e.g. createTicket, updateTicket, resolveAlerts, reference-data lookups). Pair
with --dry-run to print the exact request without sending it.`,
		Annotations: map[string]string{"mcp:read-only": "false"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newRawOpCmd(flags, "query", false))
	cmd.AddCommand(newRawOpCmd(flags, "mutation", true))
	return cmd
}

func newRawOpCmd(flags *rootFlags, name string, isMutation bool) *cobra.Command {
	var varsArg string
	readOnly := "true"
	verb := "query"
	if isMutation {
		readOnly = "false"
		verb = "mutation"
	}
	cmd := &cobra.Command{
		Use:   name + " <graphql|@file|->",
		Short: fmt.Sprintf("Execute a raw GraphQL %s", verb),
		Long: fmt.Sprintf(`Execute a raw GraphQL %s. The document is the first argument, @path to read
from a file, or - to read from stdin.`, verb),
		Example: strings.Trim(fmt.Sprintf(`
  superops-cli raw %s 'query{ getTicketList(input:{pageSize:5}){ tickets{ ticketId subject } } }'
  superops-cli raw %s @op.graphql --vars '{"input":{"pageSize":5}}'
  echo '{ getStatusList { statusId name } }' | superops-cli raw query - --json
`, verb, verb), "\n"),
		Annotations: map[string]string{"mcp:read-only": readOnly},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			doc, err := resolveDocArg(cmd, args[0])
			if err != nil {
				return err
			}
			if strings.TrimSpace(doc) == "" {
				return fmt.Errorf("empty GraphQL document")
			}
			vars := map[string]any{}
			if strings.TrimSpace(varsArg) != "" {
				raw, err := resolveDocArg(cmd, varsArg)
				if err != nil {
					return fmt.Errorf("reading --vars: %w", err)
				}
				if err := json.Unmarshal([]byte(raw), &vars); err != nil {
					return fmt.Errorf("--vars is not valid JSON: %w", err)
				}
			}
			if dryRunOK(flags) {
				req := map[string]any{"query": doc, "variables": vars}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(req)
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			var data json.RawMessage
			if isMutation {
				data, err = c.Mutate(cmd.Context(), doc, vars)
			} else {
				data, err = c.Query(cmd.Context(), doc, vars)
			}
			if err != nil {
				return err
			}
			return printJSONFiltered(cmd.OutOrStdout(), data, flags)
		},
	}
	cmd.Flags().StringVar(&varsArg, "vars", "", "GraphQL variables as inline JSON or @path")
	return cmd
}

// resolveDocArg resolves a CLI argument that may be a literal string, @path to a
// file, or - for stdin.
func resolveDocArg(cmd *cobra.Command, arg string) (string, error) {
	switch {
	case arg == "-":
		b, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return "", fmt.Errorf("reading stdin: %w", err)
		}
		return string(b), nil
	case strings.HasPrefix(arg, "@"):
		b, err := os.ReadFile(arg[1:])
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", arg[1:], err)
		}
		return string(b), nil
	default:
		return arg, nil
	}
}
