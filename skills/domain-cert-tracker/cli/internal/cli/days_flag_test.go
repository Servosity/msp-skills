// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

// A window under one day would be read downstream as "use the default 30",
// so it is refused instead.
func TestParseDaysFlagRefusesUnderOneDay(t *testing.T) {
	for _, v := range []string{"0", "0d", "12h", "-1"} {
		if _, err := parseDaysFlag("days", v); err == nil {
			t.Errorf("parseDaysFlag(%q) accepted", v)
		}
	}
	if d, err := parseDaysFlag("days", "7"); err != nil || d != 7 {
		t.Fatalf("parseDaysFlag(7) = %d, %v", d, err)
	}
}
