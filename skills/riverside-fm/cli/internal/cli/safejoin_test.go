// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: proves the download path guard BOTH directions.
//
// The download commands name files after ids that come straight out of API
// responses. Before safeJoin existed, an id of "x/../../../../tmp/pwn" turned
// filepath.Join(outDir, title+"-"+id+ext) into /tmp/pwn.mp4 - outside the
// directory the operator chose, with content the response body controls.
//
// A guard that only fires on hostile input is half a proof: one that also fires
// on ordinary titles would quietly stop downloading real files and nobody would
// notice, because the commands skip on "" rather than erroring loudly. Both
// halves are asserted here.

package cli

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSafeJoinRefusesEscapes(t *testing.T) {
	root := t.TempDir()

	hostile := []struct {
		name string
		part string
	}{
		{"posix traversal mid-component", "x/../../../../tmp/pwn"},
		{"leading traversal", "../../etc/cron.d/evil"},
		{"bare dotdot", ".."},
		{"dot", "."},
		{"triple dot", "..."},
		{"windows separator", `..\..\Windows\System32\evil`},
		{"absolute posix path", "/etc/passwd"},
		{"nul byte", "ok\x00/../../escape"},
	}
	for _, tc := range hostile {
		t.Run(tc.name, func(t *testing.T) {
			got := safeJoin(root, tc.part)
			if got == "" {
				return // refused outright, which is the strongest answer
			}
			// Otherwise it must have been neutralised INTO the root.
			rel, err := filepath.Rel(root, got)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				t.Fatalf("safeJoin(%q, %q) = %q, which escapes %q", root, tc.part, got, root)
			}
			if strings.Contains(rel, string(filepath.Separator)) {
				t.Fatalf("safeJoin(%q, %q) = %q introduced a new path segment", root, tc.part, got)
			}
		})
	}
}

func TestSafeJoinAllowsOrdinaryNames(t *testing.T) {
	root := t.TempDir()

	benign := []string{
		"Episode 12 - Guest Interview-abc123.mp4",
		"untitled-9f8e7d.json",
		"Q3 Planning (final).vtt",
		"Ünïcödé Tïtlé-xyz.m3u8",
		"a",
	}
	for _, part := range benign {
		got := safeJoin(root, part)
		if got == "" {
			t.Fatalf("safeJoin(%q, %q) refused an ordinary filename", root, part)
		}
		if filepath.Dir(got) != root {
			t.Fatalf("safeJoin(%q, %q) = %q, expected a direct child of root", root, part, got)
		}
	}

	// Multi-part joins nest, and stay inside.
	nested := safeJoin(root, "My Studio", "Episode 1-id42")
	if nested == "" {
		t.Fatal("safeJoin refused an ordinary two-level path")
	}
	rel, err := filepath.Rel(root, nested)
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("nested join escaped: %q", nested)
	}
	if len(strings.Split(rel, string(filepath.Separator))) != 2 {
		t.Fatalf("expected two levels under root, got %q", rel)
	}
}

func TestSanitizeNeutralisesSeparatorsAndDotOnlyNames(t *testing.T) {
	for _, tc := range []struct{ in, notWanted string }{
		{"a/b", "/"},
		{`a\b`, `\`},
	} {
		if got := sanitize(tc.in); strings.Contains(got, tc.notWanted) {
			t.Fatalf("sanitize(%q) = %q still contains %q", tc.in, got, tc.notWanted)
		}
	}
	for _, in := range []string{"..", ".", "...", "  ..  "} {
		if got := sanitize(in); strings.Trim(got, ".") == "" {
			t.Fatalf("sanitize(%q) = %q is still a dots-only component", in, got)
		}
	}
	if got := sanitize("ok\x00name"); strings.ContainsRune(got, 0) {
		t.Fatalf("sanitize kept a NUL byte: %q", got)
	}
	if runtime.GOOS == "windows" {
		if got := sanitize("a:b"); strings.Contains(got, ":") {
			t.Fatalf("sanitize(%q) kept a drive separator: %q", "a:b", got)
		}
	}
}
