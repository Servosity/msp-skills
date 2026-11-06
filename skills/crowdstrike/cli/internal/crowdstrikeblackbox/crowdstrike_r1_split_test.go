// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package crowdstrikeblackbox

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// output-units-reassembled, Codex r1 4.34.2 input as given: a held
// FALCON_CLIENT_ID typed into feedback in two members with a member between
// them reaches neither the output nor feedback.jsonl whole.
func TestFeedbackSplitAcrossMembersWithheld(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credStored)
	e.env = append(e.env, "FALCON_CLIENT_ID=aaaaaaaabbbbbbbb")
	out := e.run(t, "feedback", `{"a":"aaaaaaaa","gap":"x","b":"bbbbbbbb"}`, "--json")
	if strings.Contains(out, "aaaaaaaa") && strings.Contains(out, "bbbbbbbb") {
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
		if s := string(data); strings.Contains(s, "aaaaaaaa") && strings.Contains(s, "bbbbbbbb") {
			t.Errorf("%s carries both halves of the held key:\n%s", path, s)
		}
		return nil
	})
	if files == 0 {
		t.Fatalf("feedback wrote no feedback.jsonl; the test did not run:\n%s", out)
	}
}
