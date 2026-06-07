// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Tests for the speedtest bps->Mbps rounding helper.

package cli

import (
	"math"
	"testing"
)

func TestRound2(t *testing.T) {
	tests := []struct {
		in   float64
		want float64
	}{
		{0, 0},
		{1.005, 1.0}, // float repr makes 1.005 round to 1.0; documents behavior
		{1.234567, 1.23},
		{float64(94_000_000) / 1e6, 94.0}, // 94 Mbps download
		{float64(11_500_000) / 1e6, 11.5}, // 11.5 Mbps upload
		{float64(123_456_789) / 1e6, 123.46},
	}
	for _, tc := range tests {
		if got := round2(tc.in); math.Abs(got-tc.want) > 1e-9 {
			t.Fatalf("round2(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
