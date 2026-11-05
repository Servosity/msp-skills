// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"sync"
	"testing"
)

// Building a root for the allow-list runs after the real parse, so it cannot reset
// the output flags that parse set.
func TestProfileFlagWalkKeepsOutputFlags(t *testing.T) {
	savedHuman, savedColor := humanFriendly, noColor
	t.Cleanup(func() { humanFriendly, noColor = savedHuman, savedColor })
	humanFriendly, noColor = true, true
	runProfileFlagsOnce = sync.Once{}
	if !runProfileValueAllowed("timeout") {
		t.Error("timeout is no run flag")
	}
	if !humanFriendly || !noColor {
		t.Errorf("humanFriendly=%v noColor=%v after the walk", humanFriendly, noColor)
	}
}
