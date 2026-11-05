// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package gradientblackbox

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Codex r1 4.34.2 class credential split across output members (once the
// output-units-reassembled hand-fix, now the shared credential kit, credkit-wiring):
// a held vendor key typed into feedback in two members with a member between
// them reaches neither the output nor feedback.jsonl whole. The key is the
// reviewed input's shape (two 8-character halves), low-entropy.
func TestFeedbackSplitAcrossMembersWithheld(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credStored)
	e.env = append(e.env, "GRADIENT_VENDOR_API_KEY=fsplwkzzwqmkmqzk")
	out := e.run(t, "feedback", `{"a":"fsplwkzz","gap":"x","b":"wqmkmqzk"}`, "--json")
	if strings.Contains(out, "fsplwkzz") && strings.Contains(out, "wqmkmqzk") {
		t.Errorf("the output carries both halves of the held key:\n%s", out)
	}
	var files int
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		if filepath.Base(path) == "feedback.jsonl" {
			files++
		}
		if s := string(data); strings.Contains(s, "fsplwkzz") && strings.Contains(s, "wqmkmqzk") {
			t.Errorf("%s carries both halves of the held key:\n%s", path, s)
		}
		return nil
	})
	if files == 0 {
		t.Fatalf("feedback wrote no feedback.jsonl; the test did not run:\n%s", out)
	}
}
