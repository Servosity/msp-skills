// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

// connectwise-automate handfix (mcp-endpoint-write-confirm): every typed endpoint tool that is
// not read-only (running a command on an agent, a fleet patch deployment,
// minting or refreshing an API token) returns a preview of the request and sends nothing until
// confirm is boolean true. The preview never repeats a credential argument's
// value (the password, two-factor passcode or token of apitoken_mint and
// apitoken_refresh): it names the argument and shows previewWithheldArgValue.
// Fail closed: the gate wraps every registered tool that lacks the read-only
// hint and is not a mirrored command (those carry their own gate,
// cobratree.RequireConfirmForMirroredWrites), so an endpoint a later print
// adds is gated too.

import (
	"context"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"connectwise-automate-pp-cli/internal/cli"
	"connectwise-automate-pp-cli/internal/mcp/bound"
)

// previewWithheldArgValue stands in for a credential argument in a preview.
const previewWithheldArgValue = "[credential: not shown]"

const endpointConfirmNote = "Preview only: nothing was sent to ConnectWise Automate. Call again with the same arguments and confirm set to boolean true to send it."

// RequireConfirmForEndpointWrites wraps every non-read-only, non-mirrored tool
// in the confirm gate. Call it after every tool is registered.
func RequireConfirmForEndpointWrites(s *server.MCPServer) {
	root := cli.RootCmd()
	for name, entry := range s.ListTools() {
		tool := entry.Tool
		if tool.Meta != nil && tool.Meta.AdditionalFields["pp:tenant-gate"] == "child-cli" {
			continue
		}
		if hint := tool.Annotations.ReadOnlyHint; hint != nil && *hint {
			continue
		}
		if _, gated := tool.InputSchema.Properties["confirm"]; gated {
			continue
		}
		method, path := endpointRequest(root, name)
		if tool.InputSchema.Properties == nil {
			tool.InputSchema.Properties = map[string]any{}
		}
		tool.InputSchema.Properties["confirm"] = map[string]any{
			"type":        "boolean",
			"description": "Set to boolean true to send this request. It changes data in your ConnectWise Automate server. Any other value, including the string \"true\", returns a preview and sends nothing.",
		}
		tool.Description = strings.TrimSpace(tool.Description + " Returns a preview and sends nothing until confirm is boolean true.")
		s.AddTool(tool, endpointConfirmGate(name, method, path, entry.Handler))
	}
}

// endpointRequest names the HTTP method and path template of the command a
// tool mirrors (tool accounts_update-one is command
// `accounts update-one`), for the preview; "" when unknown.
func endpointRequest(root *cobra.Command, tool string) (string, string) {
	if root == nil {
		return "", ""
	}
	cmd, _, err := root.Find(strings.Split(tool, "_"))
	if err != nil || cmd == nil || cmd == root {
		return "", ""
	}
	return cmd.Annotations["pp:method"], cmd.Annotations["pp:path"]
}

// terminalOnlyTools mint or refresh an API bearer token. Governance puts them
// in the human-in-the-loop Credential tier, and the credential kit withholds
// any answer that carries the new token, so over MCP the token would be minted
// and then lost. They send nothing over MCP and name the CLI command, which
// saves the new token to the credential store.
var terminalOnlyTools = map[string]string{
	"apitoken_mint":    "apitoken mint",
	"apitoken_refresh": "apitoken refresh",
}

func endpointConfirmGate(name, method, path string, send server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		if command, ok := terminalOnlyTools[name]; ok {
			return mcpToolError("Nothing was sent: minting or refreshing an API token is human-in-the-loop only. Run `connectwise-automate-cli " + command + "` in a terminal; it saves the new token to the credential store, where later commands and this server read it."), nil
		}
		args := req.GetArguments()
		rest := make(map[string]any, len(args))
		for k, v := range args {
			if k != "confirm" {
				rest[k] = v
			}
		}
		if confirmed, ok := args["confirm"].(bool); ok && confirmed {
			req.Params.Arguments = rest
			return send(ctx, req)
		}
		shown := make(map[string]any, len(rest))
		for k, v := range rest {
			if credentialArgument(k) {
				v = previewWithheldArgValue
			}
			shown[k] = v
		}
		preview := map[string]any{
			"preview":               true,
			"confirmation_required": true,
			"tool":                  name,
			"arguments":             shown,
			"note":                  endpointConfirmNote,
		}
		if method != "" {
			preview["method"], preview["path"] = method, path
		}
		text, err := bound.JSON(preview)
		if err != nil {
			return mcpToolError("encoding confirmation preview: " + err.Error()), nil
		}
		return mcplib.NewToolResultText(text), nil
	}
}

// previewCredentialArguments are the MCP arguments the credential kit holds:
// credwire.Declarations.MCPArguments, repeated here because the kit's checker
// allows the Declarations variable no use but credkit.Start's argument.
var previewCredentialArguments = []string{"password", "two-factor", "token"}

// credentialArgument reports one of previewCredentialArguments, compared as
// the kit does, ignoring case.
func credentialArgument(name string) bool {
	for _, a := range previewCredentialArguments {
		if strings.EqualFold(a, name) {
			return true
		}
	}
	return false
}
