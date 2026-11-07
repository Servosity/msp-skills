// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rewstblackbox

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// output-units-reassembled, Codex r1 4.34.2 input as given: a held key typed
// into feedback in two members with a member between them reaches neither the
// output nor feedback.jsonl whole. Once for each environment credential (the
// API key and the MCP HTTP token).
func TestFeedbackSplitAcrossMembersWithheld(t *testing.T) {
	budget(t)
	for _, name := range []string{"REWST_API_TOKEN", "REWST_MCP_HTTP_TOKEN"} {
		t.Run(name, func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, credStored)
			e.env = append(withoutEnv(e.env, name), name+"=ABCDEFGH12345678")
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
			// Either outcome closes the class: the entry is refused whole
			// (journal-credential-hardening's RefuseHeld sees the reassembled
			// value) or written withheld. A run that did neither proves nothing.
			if files == 0 && !strings.Contains(out, "not saved: it repeats a secret value") {
				t.Fatalf("feedback neither wrote feedback.jsonl nor refused the entry; the test did not run:\n%s", out)
			}
		})
	}
}
