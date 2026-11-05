// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"io"
)

// printStampedDryRunOutput formats a hand-built plan through the output flags
// (--agent, --select, --compact), then stamps the dry-run contract keys on the
// envelope those flags produced, as the generated commands do.
func printStampedDryRunOutput(w io.Writer, data json.RawMessage, flags *rootFlags, agentMeta map[string]any, action string) error {
	var buf bytes.Buffer
	formatErr := printOutputWithFlagsMeta(&buf, data, flags, agentMeta)
	if formatErr != nil && buf.Len() == 0 {
		return formatErr
	}
	stamped, err := stampDryRunEnvelope(bytes.TrimSpace(buf.Bytes()), action, false)
	if err != nil {
		return err
	}
	if err := printOutput(w, stamped, true); err != nil {
		return err
	}
	return formatErr
}
