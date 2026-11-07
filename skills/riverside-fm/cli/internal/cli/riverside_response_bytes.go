// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"

	"riverside-fm-pp-cli/internal/client"
)

// responseFileBytes returns the bytes a command should write to disk for one
// API response. The client hands back any non-JSON content type wrapped in its
// base64 _pp_binary envelope, and Riverside serves the per-participant HLS
// manifest (/api/v4/vod/...) as application/x-mpegurl, so writing the response
// as-is would save the envelope instead of the manifest. JSON passes through
// unchanged. Every novel command that saves an API response routes it here.
func responseFileBytes(data json.RawMessage) []byte {
	if raw, _, ok := client.UnwrapBinaryResponse(data); ok {
		return raw
	}
	return data
}
