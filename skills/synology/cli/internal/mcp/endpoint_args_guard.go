// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "mcp-writes-refuse-unknown-args" entry.

package mcp

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"synology-pp-cli/internal/mcp/cobratree"
)

// dsmReservedQueryKeys select which DSM operation runs (api, method,
// version) or carry the session (_sid, SynoToken). Every endpoint tool calls
// /webapi/entry.cgi or query.cgi with these in its fixed path.
var dsmReservedQueryKeys = []string{"api", "method", "version", "_sid", "synotoken"}

// undeclaredEndpointArgError decides, before any request, whether an
// endpoint tool call may proceed with the arguments it was given.
//
// The generated makeAPIHandler forwards every argument the tool does not
// declare as a query parameter, and the client merges parameters into the
// path's own query with Set. On DSM every operation is a GET on one CGI path
// selected by api=/method=, so an undeclared {"method":"backup"} on the
// read-only abb_tasks tool started a backup, {"api":...,"method":"cancel"}
// on abb_settings cancelled one (both while SYNOLOGY_MCP_READ_ONLY=1), and
// {"dry-run":true} on abb_backup was forwarded as an inert query parameter
// while the backup ran live.
//
// Rules: a write tool (not annotated read-only) refuses every undeclared
// argument; a read tool keeps the pass-through for extra DSM filters but
// refuses any argument that would override a parameter the tool's own path
// fixes, or a reserved DSM key, compared case-insensitively, and every name
// the Cobra-mirror tools refuse (cobratree.BlockedArgumentNames: deliver,
// config, home, output, db, profile, base-url, ...), so "--deliver,
// --output, --db, --config and --home are refused over MCP" holds for
// every tool instead of reaching the NAS as query parameters.
func undeclaredEndpointArgError(args map[string]any, declared map[string]bool, pathTemplate string, readOnly bool) error {
	fixed := map[string]bool{}
	for _, k := range dsmReservedQueryKeys {
		fixed[strings.ToLower(k)] = true
	}
	if _, rawQuery, ok := strings.Cut(pathTemplate, "?"); ok {
		if q, err := url.ParseQuery(rawQuery); err == nil {
			for k := range q {
				fixed[strings.ToLower(k)] = true
			}
		}
	}
	blocked := map[string]bool{}
	for name := range cobratree.BlockedArgumentNames() {
		blocked[strings.ToLower(name)] = true
	}
	var unknown []string
	for k := range args {
		if !declared[k] {
			unknown = append(unknown, k)
		}
	}
	sort.Strings(unknown)
	for _, k := range unknown {
		if !readOnly {
			return fmt.Errorf("unknown MCP parameter %q on a write tool; use the tool schema's named parameters (nothing was sent)", k)
		}
		if fixed[strings.ToLower(k)] {
			return fmt.Errorf("MCP parameter %q would override the DSM operation this tool is fixed to; refused (nothing was sent)", k)
		}
		if blocked[strings.ToLower(k)] {
			return fmt.Errorf("MCP parameter %q is a CLI root or destination flag, which is refused over MCP and never forwarded to the NAS (nothing was sent)", k)
		}
	}
	return nil
}
