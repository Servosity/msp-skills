// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/connectwise-rmm/handfixes.json
// under the "mcp-argument-types-enforced" entry.

package cobratree

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// mcpArgumentTypeRefusal refuses a tool call whose argument has a different
// JSON type than the flag it sets. cliArgsFromMCP turns any value into
// --name=value text, so without this a string "true" would set a bool flag
// (run --confirm, --yes) that the tool schema declares as boolean.
func mcpArgumentTypeRefusal(cmd *cobra.Command, args map[string]any) string {
	if cmd == nil {
		return ""
	}
	for name, v := range args {
		if v == nil {
			continue
		}
		flag := cmd.Flag(name)
		if flag == nil {
			continue // positional or raw args field
		}
		typ := flag.Value.Type()
		if !mcpValueFitsFlagType(typ, v) {
			return fmt.Sprintf("argument %q must be a JSON %s for --%s (got %T)", name, mcpJSONKind(typ), name, v)
		}
	}
	return ""
}

func mcpValueFitsFlagType(typ string, v any) bool {
	if strings.HasSuffix(typ, "Slice") || strings.HasSuffix(typ, "Array") {
		elem := strings.TrimSuffix(strings.TrimSuffix(typ, "Slice"), "Array")
		if items, ok := v.([]any); ok {
			for _, item := range items {
				if item != nil && !mcpValueFitsFlagType(elem, item) {
					return false
				}
			}
			return true
		}
		return mcpValueFitsFlagType(elem, v)
	}
	switch mcpJSONKind(typ) {
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "number":
		_, ok := v.(float64)
		return ok
	default:
		switch v.(type) {
		case string, float64:
			return true
		}
		return false
	}
}

func mcpJSONKind(typ string) string {
	switch typ {
	case "bool":
		return "boolean"
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "float32", "float64", "count":
		return "number"
	}
	return "string"
}
