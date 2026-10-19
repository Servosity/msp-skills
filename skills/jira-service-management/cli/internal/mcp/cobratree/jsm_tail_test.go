// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"strings"
	"testing"
)

func TestJSMTailSinglePoll(t *testing.T) {
	got := strings.Join(jsmTailSinglePoll([]string{"tail", "--follow", "--interval=2s", "--follow=true", "request"}, 1), " ")
	if got != "tail --follow=false --interval=2s request" {
		t.Fatalf("argv = %q", got)
	}
}
