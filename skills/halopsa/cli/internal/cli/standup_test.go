// Hand-written novel feature. Not generated.
package cli

import (
	"testing"
	"time"
)

func TestParseSince(t *testing.T) {
	now := time.Now()
	y, mo, d := now.Date()

	t.Run("today", func(t *testing.T) {
		got, err := parseSince("today")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := time.Date(y, mo, d, 0, 0, 0, 0, now.Location())
		if !got.Equal(want) {
			t.Errorf("today = %v, want %v", got, want)
		}
	})

	t.Run("duration", func(t *testing.T) {
		got, err := parseSince("24h")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.After(now) {
			t.Errorf("24h ago should not be in the future: %v", got)
		}
	})

	t.Run("absolute", func(t *testing.T) {
		got, err := parseSince("2026-05-20 09:00")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Hour() != 9 || got.Minute() != 0 {
			t.Errorf("absolute time = %v, want 09:00", got)
		}
	})

	// Documented help example `changed-since 09:00` ("since 9am today") must parse.
	t.Run("bare clock HH:MM is today at that time", func(t *testing.T) {
		got, err := parseSince("09:00")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := time.Date(y, mo, d, 9, 0, 0, 0, now.Location())
		if !got.Equal(want) {
			t.Errorf("09:00 = %v, want %v", got, want)
		}
	})

	t.Run("bare clock HH:MM:SS is today at that time", func(t *testing.T) {
		got, err := parseSince("14:30:15")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := time.Date(y, mo, d, 14, 30, 15, 0, now.Location())
		if !got.Equal(want) {
			t.Errorf("14:30:15 = %v, want %v", got, want)
		}
	})

	t.Run("garbage errors", func(t *testing.T) {
		if _, err := parseSince("not-a-time"); err == nil {
			t.Error("expected error for unparseable input")
		}
	})
}
