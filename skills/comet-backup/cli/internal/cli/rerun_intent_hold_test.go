// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

// dispatch-intent-clock-back: a lost reply leaves an intent; the clock moving
// back an hour must not let a second run dispatch the same backup again.
func TestRerunIntentHoldsAcrossAClockMovedBack(t *testing.T) {
	for _, age := range []time.Duration{-time.Hour, -2 * rerunIntentHold, 0, rerunIntentHold / 2} {
		if !rerunIntentHolds(age) {
			t.Errorf("an intent of age %s no longer refuses a dispatch", age)
		}
	}
	if rerunIntentHolds(rerunIntentHold + time.Second) {
		t.Error("an intent past its hold still refuses")
	}
}
