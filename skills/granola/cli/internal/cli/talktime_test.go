// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"

	"granola-pp-cli/internal/granola"
)

func TestSourceSeconds_MicAndFarEnd(t *testing.T) {
	segs := []granola.TranscriptSegment{
		{Source: "microphone", StartTimestamp: "2026-05-01T10:00:00Z", EndTimestamp: "2026-05-01T10:00:30Z"},
		{Source: "microphone", StartTimestamp: "2026-05-01T10:00:30Z", EndTimestamp: "2026-05-01T10:01:00Z"},
		{Source: "speaker", StartTimestamp: "2026-05-01T10:01:00Z", EndTimestamp: "2026-05-01T10:01:45Z"},
		{Source: "speaker", StartTimestamp: "2026-05-01T10:01:45Z", EndTimestamp: "2026-05-01T10:02:30Z"},
	}
	mic, far, unknown := sourceSeconds(segs)
	if mic != 60 {
		t.Errorf("expected mic=60, got %v", mic)
	}
	if far != 90 {
		t.Errorf("expected far=90, got %v", far)
	}
	if unknown != 0 {
		t.Errorf("expected unknown=0, got %v", unknown)
	}
}

// TestSourceSeconds_DeadVocabularyIsUnknown is talktime's half of the
// dead-grammar guard. Before the fix this switch was duplicated here and
// matched "system", so `talktime` reported system_seconds: 0 for every real
// meeting while its total silently omitted the far end.
func TestSourceSeconds_DeadVocabularyIsUnknown(t *testing.T) {
	segs := []granola.TranscriptSegment{
		{Source: "microphone", StartTimestamp: "2026-05-01T10:00:00Z", EndTimestamp: "2026-05-01T10:00:30Z"},
		{Source: "system", StartTimestamp: "2026-05-01T10:00:30Z", EndTimestamp: "2026-05-01T10:01:00Z"},
	}
	mic, far, unknown := sourceSeconds(segs)
	if mic != 30 {
		t.Errorf("expected mic=30, got %v", mic)
	}
	if far != 0 {
		t.Errorf("dead vocabulary must not count as far end; got far=%v", far)
	}
	if unknown != 30 {
		t.Errorf("unclassified time must be tracked, not dropped; got unknown=%v", unknown)
	}
}

func TestAggregateBySources(t *testing.T) {
	segs := []granola.TranscriptSegment{
		{Source: "microphone", StartTimestamp: "2026-05-01T10:00:00Z", EndTimestamp: "2026-05-01T10:00:30Z", Confidence: 0.9},
		{Source: "speaker", StartTimestamp: "2026-05-01T10:00:30Z", EndTimestamp: "2026-05-01T10:01:00Z", Confidence: 0.8},
	}
	agg := aggregateBySources(segs)
	if agg["microphone_seconds"].(float64) != 30 {
		t.Errorf("mic_seconds wrong: %v", agg["microphone_seconds"])
	}
	if agg["system_seconds"].(float64) != 30 {
		t.Errorf("sys_seconds wrong: %v", agg["system_seconds"])
	}
	if agg["unknown_seconds"].(float64) != 0 {
		t.Errorf("unknown_seconds wrong: %v", agg["unknown_seconds"])
	}
	if agg["total_seconds"].(float64) != 60 {
		t.Errorf("total_seconds wrong: %v", agg["total_seconds"])
	}
	avg := agg["confidence_avg"].(float64)
	if avg < 0.84 || avg > 0.86 {
		t.Errorf("expected avg conf ~0.85, got %v", avg)
	}
}

// TestAggregateBySources_UnknownStaysInTotal guards the reporting fix: time
// from an unrecognized label must remain in total_seconds instead of
// disappearing from the roll-up.
func TestAggregateBySources_UnknownStaysInTotal(t *testing.T) {
	segs := []granola.TranscriptSegment{
		{Source: "microphone", StartTimestamp: "2026-05-01T10:00:00Z", EndTimestamp: "2026-05-01T10:00:30Z"},
		{Source: "bogus-channel", StartTimestamp: "2026-05-01T10:00:30Z", EndTimestamp: "2026-05-01T10:01:00Z"},
	}
	agg := aggregateBySources(segs)
	if agg["unknown_seconds"].(float64) != 30 {
		t.Errorf("expected unknown_seconds=30, got %v", agg["unknown_seconds"])
	}
	if agg["total_seconds"].(float64) != 60 {
		t.Errorf("unknown time must stay in the total; got %v", agg["total_seconds"])
	}
}
