// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// comet-backup hand-fix (import-confirmed-idempotent): import counts a record
// as created only when Comet's reply confirms it, and --idempotent skips the
// records that already exist on the server, so a rerun after a crash or a
// partial failure finishes the import instead of failing on every record the
// first run already created. Proven by TestImportWriteSimulation
// (comet_write_sim_test.go).

package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"comet-backup-pp-cli/internal/client"
	"comet-backup-pp-cli/internal/cliutil"
)

// cometImportKeys names, for each importable Comet create endpoint, the
// request field that identifies the record it creates and the read that
// lists the records that already exist. Comet keys user accounts by name, so
// a second add-user for a name is refused by the server itself.
var cometImportKeys = map[string]struct{ field, listPath string }{
	"/api/v1/admin/add-user": {field: "TargetUser", listPath: "/api/v1/admin/list-users"},
}

type cometImportGuard struct {
	field    string
	existing map[string]bool // set only with --idempotent
	noSend   bool            // --dry-run or the verify harness: nothing reaches the server
}

// newCometImportGuard prepares the checks for one import into path. With
// --idempotent it reads the records that exist now; a resource whose records
// cannot be listed refuses --idempotent rather than guessing.
func newCometImportGuard(ctx context.Context, c *client.Client, path string, idempotent bool) (*cometImportGuard, error) {
	k, known := cometImportKeys[path]
	g := &cometImportGuard{field: k.field, noSend: c.DryRun || (cliutil.IsVerifyEnv() && !cliutil.IsVerifyLiveHTTPEnv())}
	if !idempotent || g.noSend {
		return g, nil
	}
	if !known {
		return nil, usageErr(fmt.Errorf("--idempotent: import cannot tell which records of %s already exist", path))
	}
	raw, err := cometRead(ctx, c, k.listPath, nil)
	if err != nil {
		return nil, err
	}
	var names []string
	if err := json.Unmarshal(raw, &names); err != nil {
		return nil, fmt.Errorf("--idempotent: reading the records that already exist: %w", err)
	}
	g.existing = make(map[string]bool, len(names))
	for _, n := range names {
		g.existing[n] = true
	}
	return g, nil
}

// exists reports whether --idempotent found body's record on the server.
func (g *cometImportGuard) exists(body map[string]any) bool {
	if g == nil || g.existing == nil {
		return false
	}
	name, _ := body[g.field].(string)
	return name != "" && g.existing[name]
}

// confirm returns nil only for a Comet reply whose Status is 2xx. An empty
// body, a non-JSON body, or a failure envelope the client let through (one
// with no Message, such as {"Status":403}) may or may not have created the
// record, so it counts as failed and is never retried here.
func (g *cometImportGuard) confirm(data json.RawMessage) error {
	if g != nil && g.noSend {
		return nil
	}
	var reply struct{ Status *int }
	if err := json.Unmarshal(data, &reply); err != nil || reply.Status == nil {
		return fmt.Errorf("the create reply was not a Comet status reply; check the server before importing this record again")
	}
	if *reply.Status < 200 || *reply.Status > 299 {
		return fmt.Errorf("the create reply carried Comet Status %d; check the server before importing this record again", *reply.Status)
	}
	return nil
}
