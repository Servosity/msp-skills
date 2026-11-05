// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import "strings"

// prtg handfix (state-changing-get-confirms): the PRTG API v2 GET operations
// that change state. The bundled spec documents GET /session as "Renews an
// active user session."; every other GET is a read. One list, read by the MCP
// confirm gate, the MCP read-only switch and the fleet raw path check.
var stateChangingGETs = map[string]bool{"/session": true}

// StateChangingGET reports whether a GET of path changes PRTG state. The path
// may carry the /api/v2 prefix, a query or a trailing slash; letters are
// compared lowercased, so a case variant fails closed.
func StateChangingGET(path string) bool {
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	path = strings.ToLower(strings.TrimSpace(path))
	path = strings.TrimPrefix(path, "/api/v2")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return stateChangingGETs[strings.TrimRight(path, "/")]
}
