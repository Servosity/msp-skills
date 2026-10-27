// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated), cove only: the JSON-RPC envelope of each typed
// endpoint tool. makeAPIHandler calls coveSeedRPCEnvelope before it binds the
// agent's arguments, so a call that passes only its parameters sends the
// jsonrpc, method and id each tool description advertises as defaults (the
// CLI twin's flag defaults), and a "method" naming anything but the tool's own
// spec method is refused before a client is used. The generated client's
// request guard (internal/client/cove_rpc_method_guard.go) still refuses any
// non-read body on its own. Hand-fix code: cove-rpc-read-method-guard.

package mcp

import (
	"fmt"

	"cove-pp-cli/internal/coverpc"
)

// coveSeedRPCEnvelope writes the advertised envelope defaults into body and
// drops an empty or null "method" argument (some hosts send "" for an unset
// optional string) so the default stands; a jsonrpc or id argument the agent
// passes still overrides its default when the arguments are bound. A tool not
// in the endpoint table is left alone.
func coveSeedRPCEnvelope(tool string, args, body map[string]any) error {
	method, ok := coverpc.EndpointMethodForTool(tool)
	if !ok {
		return nil
	}
	switch v := args["method"].(type) {
	case nil:
		delete(args, "method")
	case string:
		if v == "" {
			delete(args, "method")
		} else if v != method {
			return fmt.Errorf("%s always calls the JSON-RPC method %s; \"method\": %q is refused. Omit \"method\"; to call another method use `cove-cli call <Method>` (a method other than Get... or Enumerate... also needs --yes, which MCP cannot pass)", tool, method, v)
		}
	default:
		return fmt.Errorf("%s: \"method\" must be omitted or the string %q", tool, method)
	}
	body["jsonrpc"] = "2.0"
	body["method"] = method
	body["id"] = 1
	return nil
}
