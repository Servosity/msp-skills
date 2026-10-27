// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the MCP server's one choke point for the
// credential tier, registered by one marked line in the generated main.go
// (handfixes.json secret-tier-never-exposed-to-mcp).

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"sentinelone-pp-cli/internal/cliutil"
	mcptools "sentinelone-pp-cli/internal/mcp"
)

// tierRefusalPrefix starts every refusal, so a test (and an agent)
// can tell this refusal from a handler's own error.
const tierRefusalPrefix = "refused: credential-tier resource"

// sqlTextRE marks an argument value that reads as SQL. The sql tool's values
// always get the SQL normalization; any other tool's value does when it
// carries a SELECT, so a future tool that takes SQL text is covered without
// naming it here.
var sqlTextRE = regexp.MustCompile(`(?i)\bselect\b`)

// refuseCredentialTier refuses, before the handler (and before the tenant
// gate) runs, every tool call whose arguments name a credential-tier member
// (cliutil.CredentialTiers: the agent passphrases, the Ranger credential
// details, the account uninstall password, a login-by-token session, the
// regenerate-key tables) at any depth: a resource or table name in any
// spelling, an API path, a path run inside a longer value, a percent-encoded
// form, and for SQL text the comment-, quote- and ||-stripped form. It has no
// per-tool list: tail, export, sync, search, sql, analytics, the workflow and
// insight mirrors, sentinelone_read, sentinelone_get, sentinelone_search, the
// intents and any tool registered later are all covered by name alone. The
// one call it lets through is sentinelone_execute with confirm set to a
// top-level boolean true, the only way an MCP agent reads a credential.
func refuseCredentialTier(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		if confirmedSecretRead(req) {
			return next(ctx, req)
		}
		m, found, err := credentialTierInArguments(req)
		if err != nil {
			return mcplib.NewToolResultError(fmt.Sprintf("%s check failed for %s, so the call is refused: %v", tierRefusalPrefix, req.Params.Name, err)), nil
		}
		if found {
			return mcplib.NewToolResultError(credentialTierRefusal(req.Params.Name, m)), nil
		}
		return next(ctx, req)
	}
}

// credentialTierInArguments walks every string (keys and values, at any
// depth, duplicate keys included) of the decoded arguments and of the raw
// wire arguments, since a handler may read either.
func credentialTierInArguments(req mcplib.CallToolRequest) (cliutil.CredentialTier, bool, error) {
	var docs [][]byte
	if req.Params.Arguments != nil {
		b, err := json.Marshal(req.Params.Arguments)
		if err != nil {
			return cliutil.CredentialTier{}, false, err
		}
		docs = append(docs, b)
	}
	if len(req.Params.RawArguments) > 0 {
		docs = append(docs, req.Params.RawArguments)
	}
	sqlTool := req.Params.Name == "sql"
	for _, doc := range docs {
		dec := json.NewDecoder(bytes.NewReader(doc))
		dec.UseNumber()
		for {
			tok, err := dec.Token()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return cliutil.CredentialTier{}, false, err
			}
			s, ok := tok.(string)
			if !ok || strings.TrimSpace(s) == "" {
				continue
			}
			if m, hit := cliutil.CredentialTierInText(s); hit {
				return m, true, nil
			}
			if sqlTool || sqlTextRE.MatchString(s) {
				if m, hit := cliutil.CredentialTierInSQL(s); hit {
					return m, true, nil
				}
			}
		}
	}
	return cliutil.CredentialTier{}, false, nil
}

func credentialTierRefusal(tool string, m cliutil.CredentialTier) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s was not run because an argument names %q", tierRefusalPrefix, tool, m.Resource)
	if m.Path != "" {
		fmt.Fprintf(&b, " (GET %s)", m.Path)
	}
	b.WriteString(", whose values are credentials. No MCP tool reads, lists, exports, tails, searches or queries it")
	ids := mcptools.CredentialTierEndpointIDs(m.Path)
	switch {
	case len(ids) > 0 && cliutil.IsMCPReadOnlyEnv():
		fmt.Fprintf(&b, "; only sentinelone_execute with endpoint_id %s and confirm set to boolean true returns it, and that is unavailable while %s=1", strings.Join(ids, " or "), cliutil.MCPReadOnlyEnvVar)
	case len(ids) > 0:
		fmt.Fprintf(&b, "; only sentinelone_execute with endpoint_id %s and confirm set to boolean true returns it", strings.Join(ids, " or "))
	default:
		b.WriteString("; the local store never keeps it")
	}
	if m.Resource == "ranger" {
		b.WriteString(". (\"ranger\" is this connector's name for the Ranger credential details; for the Ranger device inventory use ranger-table-view or the ranger_exposure tool.)")
	} else {
		b.WriteString(".")
	}
	return b.String()
}
