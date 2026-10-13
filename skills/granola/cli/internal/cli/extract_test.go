// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestExtract_ThreeStreams asserts the three streams stay separate in
// the full markdown — H2 sections named exactly "Notes (Human)",
// "AI Summary", and "Transcript". The doc + transcript are seeded into the
// SQLite store (the only source since Granola sealed the desktop cache).
func TestExtract_ThreeStreams(t *testing.T) {
	tmp := t.TempDir()
	s := newIsolatedStore(t)
	insertMeeting(t, s, seedMeeting{
		ID: "abc", Title: "Test Meeting",
		CreatedAt: "2026-05-01T10:00:00Z", UpdatedAt: "2026-05-01T11:00:00Z",
		NotesPlain: "hello world",
	})
	for i := 0; i < 5; i++ {
		insertSegment(t, s, "abc", i, "microphone", "mic-"+strconv.Itoa(i+1))
	}
	for i := 0; i < 5; i++ {
		insertSegment(t, s, "abc", 5+i, "speaker", "sys-"+strconv.Itoa(i+1))
	}
	s.Close()

	outDir := filepath.Join(tmp, "out")
	// allowLive=false so the live API is never dialed.
	res, err := runExtract("abc", outDir, "", false)
	if err != nil {
		t.Fatalf("runExtract: %v", err)
	}
	if len(res.Files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(res.Files))
	}
	fullBytes, err := os.ReadFile(filepath.Join(outDir, "full_abc.md"))
	if err != nil {
		t.Fatalf("read full: %v", err)
	}
	body := string(fullBytes)
	for _, section := range []string{"## Notes (Human)", "## AI Summary", "## Transcript"} {
		if !strings.Contains(body, section) {
			t.Errorf("missing section %q in full file", section)
		}
	}
	// Three streams stay separate: human notes "hello world" only in
	// notes section, mic-1 only in transcript section.
	notesIdx := strings.Index(body, "## Notes (Human)")
	summaryIdx := strings.Index(body, "## AI Summary")
	transcriptIdx := strings.Index(body, "## Transcript")
	if !(notesIdx < summaryIdx && summaryIdx < transcriptIdx) {
		t.Errorf("section order wrong: notes=%d summary=%d transcript=%d", notesIdx, summaryIdx, transcriptIdx)
	}
	if !strings.Contains(body[notesIdx:summaryIdx], "hello world") {
		t.Errorf("expected 'hello world' in Notes section")
	}
	if !strings.Contains(body[transcriptIdx:], "mic-1") {
		t.Errorf("expected 'mic-1' in Transcript section")
	}

	// metadata file has yaml frontmatter
	metaBytes, _ := os.ReadFile(filepath.Join(outDir, "metadata_abc.md"))
	if !strings.HasPrefix(string(metaBytes), "---\n") || !strings.Contains(string(metaBytes), "id: abc") {
		t.Errorf("metadata file missing frontmatter")
	}
}

func TestExtract_MissingTranscriptExits2(t *testing.T) {
	tmp := t.TempDir()
	s := newIsolatedStore(t)
	insertMeeting(t, s, seedMeeting{ID: "xyz", Title: "No Transcript", CreatedAt: "2026-05-01T10:00:00Z"})
	s.Close()
	_, err := runExtract("xyz", filepath.Join(tmp, "o"), "", false)
	if err == nil {
		t.Fatalf("expected error")
	}
	var ce *cliError
	if !As(err, &ce) {
		t.Fatalf("expected cliError, got %v", err)
	}
	if ce.code != 2 {
		t.Errorf("expected exit code 2, got %d", ce.code)
	}
}
