// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: pinned by skills/sophos-central/handfixes.json under
// "secret-responses-never-persisted".
//
// export and tail print whole API bodies for any synced resource, and both
// are MCP tools. GET /endpoint/v1/migrations returns each job's token, so
// without this an agent could read it with `export migrations` even though
// the migrations commands are CLI-only, and `export --output` would write it
// to disk. A human who needs the token runs `migrations get-jobs`.

package cli

import (
	"encoding/json"

	"sophos-central-pp-cli/internal/store"
)

func redactSecretOutput(data json.RawMessage) json.RawMessage {
	return store.RedactSecrets(data)
}
