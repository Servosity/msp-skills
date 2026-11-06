// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package knowbe4blackbox

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Codex r1 class credential-split-across-persisted-records (its input): a
// padding record larger than any fixed history between the two halves of the
// held token, three invocations; feedback.jsonl must never hold both halves.
// The held token comes from a config file's access_token here
// (KNOWBE4_API_KEY runs in the two split tests beside this one).
func TestFeedbackSplitAcrossPaddedRecordsWithheld(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credStored)
	e.writeConfig(t, "access_token = \"RecordSplitSecretABCDEFGH\"\n")
	out := e.run(t, "feedback", "RecordSplit", "--no-learn")
	out += e.runIn(t, strings.Repeat("x", 4096)+"\n", "feedback", "--stdin", "--no-learn")
	out += e.runIn(t, strings.Repeat("y", 5000)+" SecretABCDEFGH\n", "feedback", "--stdin", "--no-learn")
	out += e.run(t, "feedback", "SecretABCDEFGH", "--no-learn")
	var files int
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Base(path) != "feedback.jsonl" {
			return nil
		}
		files++
		data, _ := os.ReadFile(path)
		if s := string(data); strings.Contains(s, "RecordSplit") && strings.Contains(s, "SecretABCDEFGH") {
			t.Errorf("%s carries both halves of the held token across the padding", path)
		}
		return nil
	})
	if files == 0 {
		t.Fatalf("feedback wrote no feedback.jsonl; the test did not run:\n%.2000s", out)
	}
}
