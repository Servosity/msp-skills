// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"reflect"
	"testing"

	"microsoft-graph-pp-cli/internal/credkit"
)

// credkit-wiring: the outbound guard every Graph request leaves through (the
// CLI's client hook puts it under the origin pin) is the credential kit's own
// transport. Only in-process tests in other packages replace it.
func TestOutboundGuardIsTheKit(t *testing.T) {
	if reflect.ValueOf(OutboundGuard).Pointer() != reflect.ValueOf(credkit.Transport).Pointer() {
		t.Fatal("client.OutboundGuard is not credkit.Transport: Graph requests would leave unchecked")
	}
}
