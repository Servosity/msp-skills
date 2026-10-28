// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated). Pinned by this connector's handfixes.json
// under the "output-chokepoint" entry.

package main

import (
	"testing"

	"kaseya-vsa-pp-cli/internal/cliutil"
)

// TestClientMaskerFeedsTheChokepoint: the MCP binary links the client
// package, whose init registers its credential masker with the chokepoint
// (review r1 P1-B), so every tool result is checked against every encoding
// the masker knows.
func TestClientMaskerFeedsTheChokepoint(t *testing.T) {
	if cliutil.EchoDetectorsRegistered() < 1 {
		t.Fatal("no echo detector registered: the client masker is not in the output chokepoint")
	}
}
