// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package abnormalblackbox

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Codex r1 4.34.2 input as given: a held API token typed into feedback in
// two members with a member between them reaches neither the output nor
// feedback.jsonl whole (the kit reads each write as one unit).
func TestFeedbackSplitAcrossMembersWithheld(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credStored)
	e.env = append(e.env, "ABNORMAL_API_TOKEN=ABCDABCD12121212")
	out := e.run(t, "feedback", `{"a":"ABCDABCD","gap":"x","b":"12121212"}`, "--json")
	if strings.Contains(out, "ABCDABCD") && strings.Contains(out, "12121212") {
		t.Errorf("the output carries both halves of the held token:\n%s", out)
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
		if s := string(data); strings.Contains(s, "ABCDABCD") && strings.Contains(s, "12121212") {
			t.Errorf("%s carries both halves of the held token:\n%s", path, s)
		}
		return nil
	})
	// The kit's file sink reads the entry as one unit and withholds it. A run
	// that wrote no feedback.jsonl proves nothing.
	if files == 0 {
		t.Fatalf("feedback wrote no feedback.jsonl; the test did not run:\n%s", out)
	}
}
