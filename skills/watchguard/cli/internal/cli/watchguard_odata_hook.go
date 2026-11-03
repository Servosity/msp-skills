// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: endpoint-security-odata-paging).

package cli

import "watchguard-pp-cli/internal/client"

func init() {
	registerClientHook(client.InstallODataQueryTransport)
}
