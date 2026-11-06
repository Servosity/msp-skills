// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"

	"kaseya-bms-pp-cli/internal/cli"
	"kaseya-bms-pp-cli/internal/client"
	"kaseya-bms-pp-cli/internal/platform"
)

// newMCPClientForCall builds the execute tool's client (kaseya-bms handfix
// mcp-preview-never-mints). A call that will only return a preview (a write
// method without boolean confirm: true) gets a client that does not mint: the
// generated handler builds its client before the confirm check, and a preview
// must send nothing, the token request included.
func newMCPClientForCall(ctx context.Context, sends bool) (*client.Client, *platform.Session, error) {
	cfg, err := newMCPConfig()
	if err != nil {
		return nil, nil, err
	}
	if !sends {
		defer cli.WithoutMintFor(cfg)()
	}
	return newMCPClientFromConfig(ctx, cfg)
}
