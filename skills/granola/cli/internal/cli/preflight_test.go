// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"granola-pp-cli/internal/granola"
)

// TestCountSources covers the LIVE grammar: the public REST API's
// speaker.source emits "microphone" and "speaker". Case and surrounding
// whitespace are tolerated.
func TestCountSources(t *testing.T) {
	segs := []granola.TranscriptSegment{
		{Source: "microphone"}, {Source: "microphone"}, {Source: "speaker"},
		{Source: "MICROPHONE"}, {Source: " Speaker "},
	}
	mic, far, _, unknown := countSources(segs)
	if mic != 3 {
		t.Errorf("expected mic=3, got %d", mic)
	}
	if far != 2 {
		t.Errorf("expected far=2, got %d", far)
	}
	if len(unknown) != 0 {
		t.Errorf("expected no unknown labels, got %v", unknown)
	}
}

// TestCountSources_DeadVocabularyIsUnknown locks the migration to the REST
// grammar. The pre-7.4x desktop cache's "system"/"speakers"/"mixed" labels
// must NOT be silently counted as the far end -- that store is app-private
// now, so seeing them means something upstream changed and the caller has to
// be told. This test fails if anyone re-adds the old `case`.
func TestCountSources_DeadVocabularyIsUnknown(t *testing.T) {
	segs := []granola.TranscriptSegment{
		{Source: "microphone"}, {Source: "system"}, {Source: "speakers"},
		{Source: "mixed"}, {Source: "system"},
	}
	mic, far, _, unknown := countSources(segs)
	if mic != 1 {
		t.Errorf("expected mic=1, got %d", mic)
	}
	if far != 0 {
		t.Errorf("dead vocabulary must not count as far end; got far=%d", far)
	}
	// Sorted and de-duplicated.
	want := []string{"mixed", "speakers", "system"}
	if len(unknown) != len(want) {
		t.Fatalf("expected unknown=%v, got %v", want, unknown)
	}
	for i := range want {
		if unknown[i] != want[i] {
			t.Errorf("unknown[%d]: got %q, want %q", i, unknown[i], want[i])
		}
	}
}

// TestUnknownIsMaterial_BlocksWholesaleRelabel is the guard for the hole an
// adversarial review found in the first cut of this fix: transcriptUsable
// counts EVERY segment, classified or not, so 5 microphone segments plus a
// wholly relabelled far end cleared `total >= 10` and returned exit 0 with a
// one-sided verdict -- the original bug in a new costume. A material relabel
// must be judged unusable; one or two stray labels must not be.
func TestUnknownIsMaterial_BlocksWholesaleRelabel(t *testing.T) {
	relabelled := func(mics, unknowns int) []granola.TranscriptSegment {
		var segs []granola.TranscriptSegment
		for i := 0; i < mics; i++ {
			segs = append(segs, granola.TranscriptSegment{Source: "microphone"})
		}
		for i := 0; i < unknowns; i++ {
			segs = append(segs, granola.TranscriptSegment{Source: "farfield"})
		}
		return segs
	}

	// Wholesale relabel. Two independent guards must both reject it: the
	// classified-only threshold, and the material-unknown rule.
	segs := relabelled(5, 5)
	mic, far, unknownSegs, unknown := countSources(segs)
	if transcriptUsable(mic, len(segs)) == false {
		t.Fatal("precondition: counting RAW segments would pass here, else this test proves nothing")
	}
	if transcriptUsable(mic, mic+far) {
		t.Errorf("classified-only threshold must reject 5 classified segments; mic=%d far=%d", mic, far)
	}
	if !unknownIsMaterial(unknownSegs) {
		t.Errorf("5 relabelled segments must be material; unknownSegs=%d", unknownSegs)
	}
	if far != 0 || len(unknown) != 1 {
		t.Errorf("expected far=0 and one unknown label, got far=%d unknown=%v", far, unknown)
	}

	// The boundary case a second adversarial pass found: 6 microphone + 4
	// relabelled reaches 10 RAW segments while staying UNDER the
	// material-unknown cutoff, so the count rule alone let it through. Only
	// the classified-only threshold closes it.
	sub := relabelled(6, 4)
	mic, far, unknownSegs, _ = countSources(sub)
	if unknownIsMaterial(unknownSegs) {
		t.Fatal("precondition: 4 unknown segments must be UNDER the material cutoff, else this test proves nothing")
	}
	if !transcriptUsable(mic, len(sub)) {
		t.Fatal("precondition: counting RAW segments would pass here, else this test proves nothing")
	}
	if transcriptUsable(mic, mic+far) {
		t.Errorf("6 microphone + 4 relabelled must fail: only %d segments are classified", mic+far)
	}

	// A stray label on an otherwise healthy transcript stays advisory.
	var healthy []granola.TranscriptSegment
	for i := 0; i < 50; i++ {
		healthy = append(healthy, granola.TranscriptSegment{Source: "microphone"})
	}
	for i := 0; i < 40; i++ {
		healthy = append(healthy, granola.TranscriptSegment{Source: "speaker"})
	}
	healthy = append(healthy, granola.TranscriptSegment{Source: "farfield"})
	mic, far, unknownSegs, _ = countSources(healthy)
	if unknownIsMaterial(unknownSegs) {
		t.Errorf("a single stray label must not be material; unknownSegs=%d", unknownSegs)
	}
	if !transcriptUsable(mic, mic+far) {
		t.Error("a healthy transcript with one stray label must still be usable")
	}
}

// TestTranscriptUsable pins the ratified gate: enough near-side speech plus
// enough CLASSIFIED content. A solo voice memo (zero far end) must pass -- the
// old mic>=5 && far>=5 rule failed every one of them. The second argument is
// mic+far, never len(segs); see the doc comment on transcriptUsable.
func TestTranscriptUsable(t *testing.T) {
	cases := []struct {
		name            string
		mic, classified int
		want            bool
	}{
		{"solo memo", 60, 60, true},
		{"normal two-sided call", 55, 103, true},
		{"exactly at threshold", 5, 10, true},
		{"empty", 0, 0, false},
		{"stub, too few classified", 6, 6, false},
		{"far end present but near side unclassifiable", 0, 90, false},
		{"6 mic + 4 relabelled: only 6 classified", 6, 6, false},
	}
	for _, c := range cases {
		if got := transcriptUsable(c.mic, c.classified); got != c.want {
			t.Errorf("%s: transcriptUsable(%d,%d) = %v, want %v", c.name, c.mic, c.classified, got, c.want)
		}
	}
}

func TestFindDuplicateFile(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "full_abc.md"), []byte("x"), 0o644)
	got := findDuplicateFile(tmp, "abc", "Anything")
	if got == "" {
		t.Errorf("expected hit on abc, got empty")
	}
	// Title-fuzzy match
	os.WriteFile(filepath.Join(tmp, "the_big_meeting.md"), []byte("x"), 0o644)
	got = findDuplicateFile(tmp, "zzz", "the big meeting")
	if got == "" {
		t.Errorf("expected fuzzy hit, got empty")
	}
}

func TestPreflight_SyntheticCache_OK(t *testing.T) {
	tmp := t.TempDir()
	cachePath := filepath.Join(tmp, "cache.json")
	cache := buildSyntheticCache(map[string]docFixture{
		"good": {Title: "Good", CreatedAt: "2026-05-01T10:00:00Z", HasTranscript: true},
	})
	data, _ := json.Marshal(cache)
	_ = os.WriteFile(cachePath, data, 0o644)
	t.Setenv("GRANOLA_CACHE_PATH", cachePath)
	c, err := openGranolaCache()
	if err != nil {
		t.Fatal(err)
	}
	segs := c.TranscriptByID("good")
	mic, far, _, unknown := countSources(segs)
	if len(unknown) != 0 {
		t.Errorf("fixture must use the live grammar; unknown labels: %v", unknown)
	}
	if !transcriptUsable(mic, mic+far) {
		t.Errorf("synthetic cache: mic=%d far=%d total=%d", mic, far, len(segs))
	}
}

// TestPreflight_RealStoreLabels_Pass is the regression guard for the actual
// bug: the live store holds "microphone" + "speaker" segments, and the gate
// counted zero far-end because it only matched "system"/"speakers". This
// fixture mirrors the real store, so it fails against the pre-fix gate.
func TestPreflight_RealStoreLabels_Pass(t *testing.T) {
	var segs []granola.TranscriptSegment
	for i := 0; i < 53; i++ {
		segs = append(segs, granola.TranscriptSegment{Source: "microphone"})
	}
	for i := 0; i < 41; i++ {
		segs = append(segs, granola.TranscriptSegment{Source: "speaker"})
	}
	mic, far, _, unknown := countSources(segs)
	if mic != 53 || far != 41 {
		t.Errorf("live-store labels: got mic=%d far=%d, want 53/41", mic, far)
	}
	if len(unknown) != 0 {
		t.Errorf("live-store labels must all classify; got unknown=%v", unknown)
	}
	if !transcriptUsable(mic, mic+far) {
		t.Error("a real two-sided transcript must pass the gate")
	}
}
