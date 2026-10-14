// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/cisco-meraki/handfixes.json
// under "meraki-response-path-keeps-object".

package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestApplyResponsePathKeepsMerakiObjects(t *testing.T) {
	batch := json.RawMessage(`{"id":"1000","confirmed":true,"status":{"completed":false,"failed":true,"errors":["boom"]},"actions":[{"resource":"/x","operation":"update"}]}`)
	if got := string(applyResponsePath(batch, "actions")); !strings.Contains(got, `"failed":true`) {
		t.Fatalf("action batch status dropped: %s", got)
	}
	envelope := json.RawMessage(`{"items":[{"id":"a"}],"meta":{"counts":{"items":{"total":1}}}}`)
	if got := string(applyResponsePath(envelope, "items")); got != `[{"id":"a"}]` {
		t.Fatalf("items+meta envelope not unwrapped: %s", got)
	}
}
