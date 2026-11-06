// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import "vsphere-pp-cli/internal/cliutil"

// readOpenedConfigHeld reads a config file and holds every value in it before
// the permission verdict, so a refusal cannot print a value the credential kit
// has not seen (handfixes.json credkit-wiring). The MCP server reloads the file
// on every tool call; a file written since startup is held here.
func readOpenedConfigHeld(path string) (cliutil.OpenedConfig, error) {
	opened, err := cliutil.ReadOpenedConfig(path)
	if err == nil {
		cliutil.HoldConfigFile(opened.Data)
	}
	return opened, err
}
