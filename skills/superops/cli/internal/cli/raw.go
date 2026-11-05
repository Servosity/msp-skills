// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"superops-pp-cli/internal/cliutil"
	"superops-pp-cli/internal/credkit"
)

// raw is not a manifest novel, so it registers through the novel hook instead
// of an edit to the generated root.go (a reprint keeps both).
func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		root.AddCommand(newRawCmd(flags))
	})
}

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

'raw query' sends only query operations: a document that contains a mutation or
subscription is refused. 'raw mutation' sends nothing without --yes; without it,
it prints the request it would send.

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
	example := `
  superops-cli raw query 'query{ getTicketList(input:{pageSize:5}){ tickets{ ticketId subject } } }'
  superops-cli raw query @op.graphql --vars '{"input":{"pageSize":5}}'
  echo '{ getStatusList { statusId name } }' | superops-cli raw query - --json`
	if isMutation {
		example = `
  superops-cli raw mutation @update.graphql --vars '{"input":{"ticketId":"6028538188152340480"}}'
  superops-cli raw mutation @update.graphql --vars '{"input":{"ticketId":"6028538188152340480"}}' --yes`
	}
	cmd := &cobra.Command{
		Use:   name + " <graphql|@file|->",
		Short: fmt.Sprintf("Execute a raw GraphQL %s", verb),
		Long: fmt.Sprintf(`Execute a raw GraphQL %s. The document is the first argument, @path to read
from a file, or - to read from stdin.`, verb),
		Example:     strings.Trim(example, "\n"),
		Annotations: map[string]string{"mcp:read-only": readOnly, "mcp:read-flags": "vars", "mcp:read-positionals": "0"},
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
			if !isMutation {
				if err := requireQueryOnlyDocument(doc); err != nil {
					return usageErr(err)
				}
			}
			vars := map[string]any{}
			if strings.TrimSpace(varsArg) != "" {
				raw, err := resolveDocArg(cmd, varsArg)
				// Hand-fix (handfixes.json credkit-wiring): the variables' credential fields are held before any preview prints them.
				credkit.HoldBody([]byte(raw))
				if err != nil {
					return fmt.Errorf("reading --vars: %w", err)
				}
				if err := json.Unmarshal([]byte(raw), &vars); err != nil {
					return fmt.Errorf("--vars is not valid JSON: %w", err)
				}
			}
			req := map[string]any{"query": doc, "variables": vars}
			if dryRunOK(flags) {
				return printRawRequest(cmd.OutOrStdout(), req)
			}
			if isMutation && !flags.yes {
				if err := printRawRequest(cmd.OutOrStdout(), req); err != nil {
					return err
				}
				return usageErr(fmt.Errorf("raw mutation writes to your SuperOps tenant; nothing was sent. Re-run with --yes to send the request above"))
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

func printRawRequest(w io.Writer, req map[string]any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(req)
}

// resolveDocArg resolves a CLI argument that may be a literal string, @path to a
// file, or - for stdin. A process the MCP server spawned never reads @path: the
// file would be on the machine running the server, not the caller's.
func resolveDocArg(cmd *cobra.Command, arg string) (string, error) {
	switch {
	case arg == "-":
		b, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return "", fmt.Errorf("reading stdin: %w", err)
		}
		return string(b), nil
	case strings.HasPrefix(arg, "@"):
		if cliutil.IsMCPChildEnv() {
			return "", usageErr(fmt.Errorf("%q names a local file on the machine running the MCP server; over MCP pass the value inline, not an @path", arg))
		}
		b, err := os.ReadFile(arg[1:])
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", arg[1:], err)
		}
		return string(b), nil
	default:
		return arg, nil
	}
}

// requireQueryOnlyDocument refuses a document unless every top-level
// definition is a query operation (named or shorthand) or a fragment. A
// document it cannot classify is refused too, so 'raw query' can never carry a
// write.
func requireQueryOnlyDocument(doc string) error {
	kinds, ok := graphQLOperationKinds(doc)
	if !ok {
		return fmt.Errorf("raw query could not confirm this document holds only query operations; nothing was sent (use 'raw mutation' for writes)")
	}
	if len(kinds) == 0 {
		return fmt.Errorf("raw query found no operation in this document; nothing was sent")
	}
	for _, k := range kinds {
		if k != "query" {
			return fmt.Errorf("raw query sends only query operations, and this document contains a %s; nothing was sent (use 'raw mutation' for writes)", k)
		}
	}
	return nil
}

// graphQLOperationKinds returns the operation type of every top-level
// operation in doc ("query" for the {...} shorthand). Fragments are skipped.
// ok is false for anything else at the top level, an unterminated string or
// block, or unbalanced brackets.
func graphQLOperationKinds(doc string) ([]string, bool) {
	toks, ok := graphQLTokens(doc)
	if !ok {
		return nil, false
	}
	var kinds []string
	for i := 0; i < len(toks); {
		t := toks[i]
		switch t {
		case "{":
			end, ok := graphQLSkipBlock(toks, i)
			if !ok {
				return nil, false
			}
			kinds = append(kinds, "query")
			i = end
		case "query", "mutation", "subscription", "fragment":
			// The selection set is the first { outside the header's
			// variable definitions, default values and directive arguments.
			j, nest := i+1, 0
			for ; j < len(toks); j++ {
				switch toks[j] {
				case "(", "[":
					nest++
				case ")", "]":
					nest--
				case "{":
					if nest == 0 {
						goto found
					}
					nest++
				case "}":
					nest--
				}
				if nest < 0 {
					return nil, false
				}
			}
			return nil, false
		found:
			end, ok := graphQLSkipBlock(toks, j)
			if !ok {
				return nil, false
			}
			if t != "fragment" {
				kinds = append(kinds, t)
			}
			i = end
		default:
			return nil, false
		}
	}
	return kinds, true
}

// graphQLSkipBlock returns the index after the bracket group opening at i.
func graphQLSkipBlock(toks []string, i int) (int, bool) {
	depth := 0
	for j := i; j < len(toks); j++ {
		switch toks[j] {
		case "{", "(", "[":
			depth++
		case "}", ")", "]":
			depth--
			if depth == 0 {
				return j + 1, true
			}
			if depth < 0 {
				return 0, false
			}
		}
	}
	return 0, false
}

// graphQLBOM is the byte-order mark GraphQL ignores as whitespace.
var graphQLBOM = string(rune(0xFEFF))

// graphQLTokens splits doc into names, brackets and single-character
// punctuators; each string or block string becomes one `"` token (so a
// string outside any bracket fails the top-level parse); comments,
// whitespace and commas are dropped. Only bracket balance and top-level
// keywords matter to the caller.
func graphQLTokens(doc string) ([]string, bool) {
	if !utf8.ValidString(doc) {
		return nil, false
	}
	var toks []string
	for i := 0; i < len(doc); {
		c := doc[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == ',':
			i++
		case strings.HasPrefix(doc[i:], graphQLBOM):
			i += len(graphQLBOM)
		case c == '#':
			for i < len(doc) && doc[i] != '\n' && doc[i] != '\r' {
				i++
			}
		case strings.HasPrefix(doc[i:], `"""`):
			end := strings.Index(doc[i+3:], `"""`)
			for end >= 0 && end > 0 && doc[i+3+end-1] == '\\' {
				next := strings.Index(doc[i+3+end+3:], `"""`)
				if next < 0 {
					return nil, false
				}
				end += 3 + next
			}
			if end < 0 {
				return nil, false
			}
			toks = append(toks, `"`)
			i += 3 + end + 3
		case c == '"':
			j := i + 1
			for ; j < len(doc); j++ {
				if doc[j] == '\\' {
					j++
					continue
				}
				if doc[j] == '"' || doc[j] == '\n' || doc[j] == '\r' {
					break
				}
			}
			if j >= len(doc) || doc[j] != '"' {
				return nil, false
			}
			toks = append(toks, `"`)
			i = j + 1
		case c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
			j := i + 1
			for j < len(doc) && (doc[j] == '_' || (doc[j] >= 'a' && doc[j] <= 'z') || (doc[j] >= 'A' && doc[j] <= 'Z') || (doc[j] >= '0' && doc[j] <= '9')) {
				j++
			}
			toks = append(toks, doc[i:j])
			i = j
		case c < utf8.RuneSelf && c > ' ':
			toks = append(toks, string(c))
			i++
		default:
			// Any other character is outside the GraphQL source grammar.
			return nil, false
		}
	}
	return toks, true
}
