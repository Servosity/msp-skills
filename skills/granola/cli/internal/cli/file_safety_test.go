// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestValidateNoteIDForFilename(t *testing.T) {
	t.Parallel()
	bad := []string{"", "../etc/x", "a/b", "..", ".", "a b", "a.b", `a\b`, "a\x00b"}
	for _, id := range bad {
		if err := validateNoteIDForFilename(id); err == nil {
			t.Errorf("validateNoteIDForFilename(%q) = nil, want error", id)
		}
	}
	good := []string{"not_06Yq6JtogRihEr", "ff1186df-593b-4ce5-bb1d-70e265f4a811", "abc_123-XYZ"}
	for _, id := range good {
		if err := validateNoteIDForFilename(id); err != nil {
			t.Errorf("validateNoteIDForFilename(%q) = %v, want nil", id, err)
		}
	}
}

func TestSafeJoinContainedRejectsEscape(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{"../etc/x", "a/b", "..", ".", "/abs", `a\b`, "", "a\x00b"} {
		if _, err := safeJoinContained(dir, name); err == nil {
			t.Errorf("safeJoinContained(dir, %q) = nil error, want error", name)
		}
	}
	got, err := safeJoinContained(dir, "full_not_abc.md")
	if err != nil {
		t.Fatalf("safeJoinContained normal name: %v", err)
	}
	resolved := dir
	if r, e := filepath.EvalSymlinks(dir); e == nil {
		resolved = r
	}
	if filepath.Dir(got) != resolved {
		t.Fatalf("safe path %q is not directly under %q", got, resolved)
	}
}

func TestEnsureSafeDirAndWriteSafeFilePerms(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "memos")
	if err := ensureSafeDir(dir); err != nil {
		t.Fatalf("ensureSafeDir: %v", err)
	}
	di, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := di.Mode().Perm(); perm != 0o700 {
		t.Fatalf("dir perm = %o, want 700", perm)
	}

	path, err := safeJoinContained(dir, "full_not_abc.md")
	if err != nil {
		t.Fatal(err)
	}
	// Pre-create the file world-readable to prove writeSafeFile tightens an
	// existing file's mode (os.WriteFile alone would not).
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeSafeFile(path, []byte("new")); err != nil {
		t.Fatalf("writeSafeFile: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("file perm = %o, want 600", perm)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("content = %q, want new", data)
	}
}

func TestReadSetTokenValueStdin(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("  tok-123\n"))
	got, err := readSetTokenValue(cmd)
	if err != nil {
		t.Fatalf("readSetTokenValue: %v", err)
	}
	if got != "tok-123" {
		t.Fatalf("token = %q, want tok-123", got)
	}
}

func TestReadSetTokenValueEnvFallback(t *testing.T) {
	t.Setenv("GRANOLA_API_KEY", "env-tok")
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("   \n")) // whitespace-only stdin -> fall through to env
	got, err := readSetTokenValue(cmd)
	if err != nil {
		t.Fatalf("readSetTokenValue: %v", err)
	}
	if got != "env-tok" {
		t.Fatalf("token = %q, want env-tok", got)
	}
}

func TestSetTokenRejectsPositionalArg(t *testing.T) {
	cmd := newAuthSetTokenCmd(&rootFlags{})
	err := cmd.RunE(cmd, []string{"my-secret-token"})
	if err == nil {
		t.Fatalf("expected rejection of positional token, got nil")
	}
	if !strings.Contains(err.Error(), "stdin") || !strings.Contains(err.Error(), "GRANOLA_API_KEY") {
		t.Fatalf("error should explain the secure usage (stdin/GRANOLA_API_KEY): %v", err)
	}
	if strings.Contains(err.Error(), "my-secret-token") {
		t.Fatalf("rejection message leaked the token: %v", err)
	}
}
