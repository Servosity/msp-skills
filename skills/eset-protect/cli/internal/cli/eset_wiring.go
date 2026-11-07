// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generator output); pinned by skills/eset-protect/handfixes.json
// under "extensions-wired-after-start".

package cli

import "sync"

var esetWiring sync.Once

// wireExtensions registers this connector's novel commands and its client hook.
// They were init functions, which run before credkit.Start: everything one
// references counts as code that may print before the output layer is up, so
// they run from here instead, the first time a command tree or the MCP server
// is built.
func wireExtensions() {
	esetWiring.Do(func() {
		wireEsetAuth()
		wireIncidentsContext()
		wireExecutablesFind()
		wireDevicesProfile()
		wireFleetStale()
		wireReportVulnerabilities()
		wireDetectionsLatest()
		wireTriage()
		wirePoliciesDecode()
		wireDetectionsSpread()
	})
}
