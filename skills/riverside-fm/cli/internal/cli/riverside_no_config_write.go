// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"riverside-fm-pp-cli/internal/client"
	"riverside-fm-pp-cli/internal/config"
)

// Issue #270 (handfixes.json: no-config-write-switch): on press 4.35.1 the
// sliding-session cookie rotation is persisted by the cookie jar, not by a
// config save, so the switch has to reach the jar too. Every CLI and MCP client
// passes through ApplyClientHooks.
func init() {
	registerClientHook(func(c *client.Client) error {
		if config.NoConfigWrite() && c.HTTPClient != nil {
			c.HTTPClient.Jar = client.MemoryOnlyJar(c.HTTPClient.Jar)
		}
		return nil
	})
}
