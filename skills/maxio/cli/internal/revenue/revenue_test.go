package revenue

import (
	"testing"
	"time"
)

func TestClassifyBucket(t *testing.T) {
	cases := []struct {
		category string
		amount   int64
		want     string
	}{
		{"imported", 0, BucketImported},
		{"imported", 5000, BucketImported},
		{"signup", 12000, BucketNew},
		{"signup_with_trial", 0, BucketNew},
		{"trial_conversion", 9900, BucketNew},
		{"reactivation", 4000, BucketReactivation},
		{"cancellation", -12000, BucketChurn},
		{"expiration", -8000, BucketChurn},
		{"delayed_cancellation", -1000, BucketChurn},
		{"plan_change", 5000, BucketExpansion},
		{"plan_change", -5000, BucketContraction},
		{"component_allocation_change", 250, BucketExpansion},
		{"metered_usage", -250, BucketContraction},
		{"proration", 0, BucketExpansion}, // non-negative defaults to expansion
		{"UNKNOWN_CATEGORY", 100, BucketExpansion},
		{"UNKNOWN_CATEGORY", -100, BucketContraction},
		{"  Signup  ", 100, BucketNew}, // trimmed + case-insensitive
	}
	for _, c := range cases {
		if got := classifyBucket(c.category, c.amount); got != c.want {
			t.Errorf("classifyBucket(%q,%d) = %q, want %q", c.category, c.amount, got, c.want)
		}
	}
}

func TestCents(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "$0.00"},
		{5, "$0.05"},
		{100, "$1.00"},
		{12345, "$123.45"},
		{16995789, "$169,957.89"},
		{203949468, "$2,039,494.68"},
		{-46080, "-$460.80"},
		{-1433906, "-$14,339.06"},
		{1000000, "$10,000.00"},
	}
	for _, c := range cases {
		if got := Cents(c.in); got != c.want {
			t.Errorf("Cents(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestWithThousands(t *testing.T) {
	cases := map[string]string{
		"0":       "0",
		"12":      "12",
		"123":     "123",
		"1234":    "1,234",
		"12345":   "12,345",
		"123456":  "123,456",
		"1234567": "1,234,567",
	}
	for in, want := range cases {
		if got := withThousands(in); got != want {
			t.Errorf("withThousands(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"2026-01-01", "2026-01-01", false},
		{"30d", "2026-05-10", false},
		{"1w", "2026-06-02", false},
		{"2w", "2026-05-26", false},
		{"6m", "2025-12-09", false},
		{"1y", "2025-06-09", false},
		{"nonsense", "", true},
		{"10x", "", true},
		{"-5d", "", true}, // negative window must not resolve to a future date
		{"0d", "2026-06-09", false},
	}
	for _, c := range cases {
		got, err := ParseSince(c.in, now)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseSince(%q) expected error, got nil", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseSince(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseSince(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMonthKey(t *testing.T) {
	cases := map[string]string{
		"2026-06-09T16:45:56-04:00": "2026-06",
		"2023-07-19":                "2023-07",
		"short":                     "short",
	}
	for in, want := range cases {
		if got := MonthKey(in); got != want {
			t.Errorf("MonthKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMovementKeyStable(t *testing.T) {
	m := rawMovement{Timestamp: "2026-06-09T16:45:56-04:00", Category: "signup", AmountInCents: 12000, Description: "Acme Co", LineItems: []byte("[]")}
	if movementKey(m) != movementKey(m) {
		t.Fatal("movementKey is not deterministic")
	}
	m2 := m
	m2.AmountInCents = 12001
	if movementKey(m) == movementKey(m2) {
		t.Error("movementKey collided on differing amount")
	}
}
