package cli

import "testing"

func TestTopNShare(t *testing.T) {
	custs := []custMRR{
		{MRRCents: 5000}, {MRRCents: 3000}, {MRRCents: 1000}, {MRRCents: 1000},
	}
	total := int64(10000)
	cases := []struct {
		n    int
		want float64
	}{
		{1, 50},   // 5000/10000
		{2, 80},   // 8000/10000
		{4, 100},  // all
		{10, 100}, // n > len clamps
	}
	for _, c := range cases {
		if got := topNShare(custs, total, c.n); got != c.want {
			t.Errorf("topNShare(n=%d) = %v, want %v", c.n, got, c.want)
		}
	}
	if got := topNShare(custs, 0, 1); got != 0 {
		t.Errorf("topNShare with zero total = %v, want 0", got)
	}
}

func TestPercentileDesc(t *testing.T) {
	// descending-sorted: index 0 = largest
	custs := []custMRR{
		{MRRCents: 100}, {MRRCents: 80}, {MRRCents: 60}, {MRRCents: 40}, {MRRCents: 20},
	}
	// p90 should be a large customer (top end), p50 the middle, p10 the small end.
	if got := percentileDesc(custs, 50); got != 60 {
		t.Errorf("p50 = %v, want 60 (median)", got)
	}
	if got := percentileDesc(custs, 90); got < 80 {
		t.Errorf("p90 = %v, want a large-customer value (>=80)", got)
	}
	if got := percentileDesc(nil, 50); got != 0 {
		t.Errorf("p50 of empty = %v, want 0", got)
	}
}

func TestKindToType(t *testing.T) {
	// quantity_based_component is RECURRING per-unit in Chargify, not usage.
	rec := []string{"baseline", "on_off_component", "delay_capture", "quantity_based_component"}
	for _, k := range rec {
		if got := kindToType(k); got != "recurring" {
			t.Errorf("kindToType(%q) = %q, want recurring", k, got)
		}
	}
	if got := kindToType("metered_component"); got != "usage" {
		t.Errorf("kindToType(metered_component) = %q, want usage", got)
	}
	if got := kindToType("setup_fee"); got != "services" {
		t.Errorf("kindToType(setup_fee) = %q, want services", got)
	}
}

func TestIsInvoluntaryCancellation(t *testing.T) {
	for _, m := range []string{"dunning", "automatic", "remittance_failure", "payment_failure"} {
		if !isInvoluntaryCancellation(m) {
			t.Errorf("isInvoluntaryCancellation(%q) = false, want true", m)
		}
	}
	for _, m := range []string{"merchant", "customer_request", "", "manual"} {
		if isInvoluntaryCancellation(m) {
			t.Errorf("isInvoluntaryCancellation(%q) = true, want false", m)
		}
	}
}

func TestMonthOf(t *testing.T) {
	if got := monthOf("2026-01-15"); got != "2026-01" {
		t.Errorf("monthOf(2026-01-15) = %q, want 2026-01", got)
	}
	if got := monthOf(""); got != "" {
		t.Errorf("monthOf(empty) = %q, want empty", got)
	}
}
