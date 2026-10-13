// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMemoQueue_FilterByDuplicate asserts memo queue's filtering: a
// meeting with a transcript but already-existing full file is NOT
// emitted; a fresh meeting is. Sources come from the SQLite store (the
// only source since Granola sealed the desktop cache).
func TestMemoQueue_FilterByDuplicate(t *testing.T) {
	tmp := t.TempDir()
	s := newIsolatedStore(t)
	now := "2026-05-01T10:00:00Z"
	for _, id := range []string{"old", "new"} {
		insertMeeting(t, s, seedMeeting{ID: id, Title: id + " meeting", CreatedAt: now})
		for i := 0; i < 5; i++ {
			insertSegment(t, s, id, i, "microphone", "m")
		}
		for i := 0; i < 5; i++ {
			insertSegment(t, s, id, 5+i, "speaker", "s")
		}
	}
	s.Close()

	// Pre-create full_old.md to simulate already-extracted
	root := filepath.Join(tmp, "root")
	_ = os.MkdirAll(root, 0o755)
	_ = os.WriteFile(filepath.Join(root, "full_old.md"), []byte("x"), 0o644)

	// Build queue records by calling the inner helper paths directly.
	c, err := openGranolaCache()
	if err != nil {
		t.Fatalf("openGranolaCache: %v", err)
	}
	emitted := []string{}
	for _, id := range c.SortedDocumentIDs() {
		segs := c.TranscriptByID(id)
		// mic+far, never len(segs): unclassified segments must not count toward
		// the threshold, or this harness reproduces the bypass it should catch.
		mic, far, _, _ := countSources(segs)
		if !transcriptUsable(mic, mic+far) {
			continue
		}
		d := c.DocumentByID(id)
		if dup, _ := findDuplicates(root, id, d.Title, meetingDay(d.CreatedAt)); dup != "" {
			continue
		}
		emitted = append(emitted, id)
	}
	if len(emitted) != 1 || emitted[0] != "new" {
		t.Errorf("expected ['new'], got %v", emitted)
	}
}

// TestMemoRun_OneMeetingProducesThreeFiles smoke-tests runOneMemo.
func TestMemoRun_OneMeetingProducesThreeFiles(t *testing.T) {
	tmp := t.TempDir()
	s := newIsolatedStore(t)
	insertMeeting(t, s, seedMeeting{ID: "m1", Title: "M1", CreatedAt: "2026-05-01T10:00:00Z"})
	for i := 0; i < 5; i++ {
		insertSegment(t, s, "m1", i, "microphone", "m")
	}
	for i := 0; i < 5; i++ {
		insertSegment(t, s, "m1", 5+i, "speaker", "s")
	}
	s.Close()

	outDir := filepath.Join(tmp, "out")
	root := filepath.Join(tmp, "root")
	_ = os.MkdirAll(root, 0o755)
	rec := runOneMemo("m1", outDir, root, false)
	if rec.Status != "new" {
		t.Fatalf("expected status=new, got %q (err=%q)", rec.Status, rec.Error)
	}
	if len(rec.Files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(rec.Files))
	}
	// Check at least one is full_m1.md
	gotFull := false
	for _, f := range rec.Files {
		if strings.HasSuffix(f, "full_m1.md") {
			gotFull = true
		}
	}
	if !gotFull {
		t.Errorf("missing full_m1.md among %v", rec.Files)
	}
}
