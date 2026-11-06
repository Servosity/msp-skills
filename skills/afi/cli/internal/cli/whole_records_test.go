// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json export-hands-the-kit-whole-records).

package cli

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"text/tabwriter"
)

// writeLog keeps each Write it receives.
type writeLog struct{ writes []string }

func (w *writeLog) Write(p []byte) (int, error) {
	w.writes = append(w.writes, string(p))
	return len(p), nil
}

func TestWholeLinesHandsOnWholeLines(t *testing.T) {
	var log writeLog
	l := &wholeLines{w: &log}
	for _, p := range []string{"ab", "c\nde", "f\ng\nh", "", "i"} {
		if n, err := l.Write([]byte(p)); n != len(p) || err != nil {
			t.Fatalf("Write(%q) = %d, %v", p, n, err)
		}
	}
	if err := l.Flush(); err != nil {
		t.Fatal(err)
	}
	want := []string{"abc\n", "def\n", "g\n", "hi"}
	if fmt.Sprint(log.writes) != fmt.Sprint(want) {
		t.Fatalf("writes %q, want %q", log.writes, want)
	}
}

// TestExportRecordsWritesWholeRecords: every Write is one or more whole
// records with their newlines, at most exportBatchBytes unless one record is
// larger, and nothing is lost or reordered.
func TestExportRecordsWritesWholeRecords(t *testing.T) {
	var log writeLog
	r := &exportRecords{w: &log}
	var want []string
	for i := range 300 {
		rec := fmt.Sprintf(`{"device_id": "d%d", "note": %q}`, i, strings.Repeat("n", i%40))
		if i == 150 {
			rec = fmt.Sprintf(`{"device_id": "d%d", "note": %q}`, i, strings.Repeat("big ", 3*exportBatchBytes/4))
		}
		want = append(want, rec)
		if err := r.add([]byte(rec)); err != nil {
			t.Fatal(err)
		}
		if i%100 == 99 {
			if err := r.flush(); err != nil { // a page boundary
				t.Fatal(err)
			}
		}
	}
	if err := r.flush(); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, w := range log.writes {
		if !strings.HasSuffix(w, "\n") {
			t.Fatalf("a write ends inside a record: %.80q", w[max(0, len(w)-80):])
		}
		lines := strings.Split(strings.TrimSuffix(w, "\n"), "\n")
		if len(w) > exportBatchBytes && len(lines) > 1 {
			t.Fatalf("a %d-byte write carries %d records; the batch bound is %d", len(w), len(lines), exportBatchBytes)
		}
		got = append(got, lines...)
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("records lost or reordered: %d written, %d wanted", len(got), len(want))
	}
	if len(log.writes) < 6 {
		t.Fatalf("%d writes for three pages: the page boundaries did not flush", len(log.writes))
	}
}

// TestTableWriterWritesWholeLines: the table is byte for byte what a bare
// tabwriter prints, and every Write is one whole line.
func TestTableWriterWritesWholeLines(t *testing.T) {
	rows := []string{"NAME\tSTATUS\tNOTE", "acme-dc01\tok\t{\"a\": [1, 2]}", "a-much-longer-device-name\tfailed\t"}
	var log writeLog
	tw := newTabWriter(&log)
	var bare bytes.Buffer
	ref := tabwriter.NewWriter(&bare, 2, 4, 2, ' ', 0)
	for _, r := range rows {
		fmt.Fprintln(tw, r)
		fmt.Fprintln(ref, r)
	}
	if err := tw.Flush(); err != nil {
		t.Fatal(err)
	}
	_ = ref.Flush()
	if got := strings.Join(log.writes, ""); got != bare.String() {
		t.Fatalf("table changed:\n%s\nwant:\n%s", got, bare.String())
	}
	if len(log.writes) != len(rows) {
		t.Fatalf("%d writes for %d lines: %q", len(log.writes), len(rows), log.writes)
	}
	for _, w := range log.writes {
		if strings.Count(w, "\n") != 1 || !strings.HasSuffix(w, "\n") {
			t.Fatalf("a write is not one whole line: %q", w)
		}
	}
}

// TestNoChunkingWritersOverKitSinks is the guard for the class
// streamed-output-truncated-by-kit: the credential kit checks each Write to a
// file (and each run of stdout between pauses) as one unit, so a writer that
// cuts output at a buffer size hands it units ending inside a record. A
// 4096-byte bufio.Writer under export did that, and an ordinary 233-device
// export stopped after 20 KiB. Every package of the module except the vendored
// kit must write whole records: no bufio, csv or bare tabwriter writer, and no
// io.Copy except into io.Discard.
func TestNoChunkingWritersOverKitSinks(t *testing.T) {
	root := filepath.Join("..", "..")
	bad := regexp.MustCompile(`bufio\.NewWriter(Size)?\(|bufio\.NewReadWriter\(|csv\.NewWriter\(|tabwriter\.NewWriter\(|io\.Copy(N|Buffer)?\(`)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "internal/credkit" || strings.HasPrefix(rel, ".") && rel != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(b), "\n") {
			code, _, _ := strings.Cut(line, "//")
			for _, m := range bad.FindAllStringIndex(code, -1) {
				call := code[m[0]:]
				switch {
				case strings.HasPrefix(call, "io.Copy(io.Discard,"):
				case strings.HasPrefix(call, "tabwriter.NewWriter(") && rel == "internal/cli/whole_records.go":
				default:
					t.Errorf("%s:%d hands the credential kit chunks, not whole records: %s", rel, i+1, strings.TrimSpace(line))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
