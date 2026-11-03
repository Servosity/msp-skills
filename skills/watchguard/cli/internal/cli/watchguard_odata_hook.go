// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: endpoint-security-odata-paging).

package cli

import "watchguard-pp-cli/internal/client"

func init() {
	registerClientHook(client.InstallODataQueryTransport)
}
