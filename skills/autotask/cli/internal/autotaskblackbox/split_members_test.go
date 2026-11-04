// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package autotaskblackbox

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// output-units-reassembled, Codex r1 4.34.2 input as given: a held API token
// typed into feedback in two members with a member between them reaches
// neither the output nor feedback.jsonl whole.
func TestFeedbackSplitAcrossMembersWithheld(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credStored)
	e.env = append(e.env, "AUTOTASK_API_INTEGRATION_CODE="+"ABCDEFGH12345678")
	out := e.run(t, "feedback", `{"a":"ABCDEFGH","gap":"x","b":"12345678"}`, "--json")
	if strings.Contains(out, "ABCDEFGH") && strings.Contains(out, "12345678") {
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
		if s := string(data); strings.Contains(s, "ABCDEFGH") && strings.Contains(s, "12345678") {
			t.Errorf("%s carries both halves of the held token:\n%s", path, s)
		}
		return nil
	})
	// Either outcome closes the class: the entry is refused whole
	// (the kit's file sink checks the reassembled value)
	// or written withheld. A run that did neither proves nothing.
	if files == 0 && !strings.Contains(out, "credkit: withheld") {
		t.Fatalf("feedback neither wrote feedback.jsonl nor refused the entry; the test did not run:\n%s", out)
	}
}
