// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package dsm

import (
	"io"
	"os"
	"testing"
)

// TestMain keeps the plain-http:// login warning (every httptest NAS is
// http://) out of the test log; the tests that check it swap the writer.
func TestMain(m *testing.M) {
	PlainHTTPWarnings = io.Discard
	os.Exit(m.Run())
}
