// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package nerdioblackbox

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Codex r1 4.34.2 input as given (now credkit-wiring): a held
// NERDIO_CLIENT_ID typed into feedback in two members with a member between
// them reaches neither the output nor feedback.jsonl whole (each member is an
// 8-character piece of it, kit contract 2(b)).
func TestFeedbackSplitAcrossMembersWithheld(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credStored)
	e.env = append(e.env, "NERDIO_CLIENT_ID="+"ABCDEFGH12345678")
	out := e.run(t, "feedback", `{"a":"ABCDEFGH","gap":"x","b":"12345678"}`, "--json")
	if strings.Contains(out, "ABCDEFGH") && strings.Contains(out, "12345678") {
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
		if s := string(data); strings.Contains(s, "ABCDEFGH") && strings.Contains(s, "12345678") {
			t.Errorf("%s carries both halves of the held key:\n%s", path, s)
		}
		return nil
	})
	if files == 0 {
		t.Fatalf("feedback wrote no feedback.jsonl; the test did not run:\n%s", out)
	}
}
