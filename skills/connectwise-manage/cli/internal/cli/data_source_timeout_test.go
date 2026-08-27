// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"errors"
	"testing"
)

// TestIsTimeoutError guards the auto-mode fallback predicate (issue #146): a
// request-deadline failure must read as a timeout so `search` auto mode falls
// back to local FTS, while a connection-level error or nil must not.
func TestIsTimeoutError(t *testing.T) {
	if !isTimeoutError(context.DeadlineExceeded) {
		t.Error("context.DeadlineExceeded should be a timeout")
	}
	if !isTimeoutError(errors.New(`Post "https://x/search": context deadline exceeded`)) {
		t.Error(`"context deadline exceeded" string should be a timeout`)
	}
	if isTimeoutError(nil) {
		t.Error("nil must not be a timeout")
	}
	if isTimeoutError(errors.New("connection refused")) {
		t.Error("connection refused is a network error, not a timeout")
	}
}
