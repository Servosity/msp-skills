// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package runzeroblackbox

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// output-units-reassembled, Codex r1 4.34.2 input as given: a held key typed
// into feedback in two members with a member between them reaches neither the
// output nor feedback.jsonl whole. Once for the key in RUNZERO_API_KEY and
// once for a key stored with auth set-token.
func TestFeedbackSplitAcrossMembersWithheld(t *testing.T) {
	t.Parallel() // its own fake runZero and home
	for name, env := range map[string][]string{
		"RUNZERO_API_KEY": {"RUNZERO_API_KEY=ABCDEFGH12345678"}, // gitleaks:allow synthetic test fixture
		"stored":          nil,
	} {
		t.Run(name, func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, credEnv)
			e.env = append(withoutEnv(e.env, "RUNZERO_API_KEY"), env...)
			if env == nil {
				e.storeKey(t, "ABCDEFGH12345678")
			}
			out := e.run(t, "feedback", `{"a":"ABCDEFGH","gap":"x","b":"12345678"}`, "--json")
			if strings.Contains(out, "ABCDEFGH") && strings.Contains(out, "12345678") {
				t.Errorf("the output carries both halves of the held key:\n%s", out)
			}
			var files int
			_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
				// The credential store and the test's own config file hold the
				// key by design.
				if err != nil || d.IsDir() || strings.HasPrefix(d.Name(), "credentials") || strings.Contains(path, string(filepath.Separator)+"cfg"+string(filepath.Separator)) {
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
			// Either outcome closes the class: the entry is withheld whole by
			// the credential kit (each half is a held piece of 8 characters),
			// in the ledger or on stdout. A run that did neither proves nothing.
			if files == 0 && !strings.Contains(out, kitNotice("stdout")) {
				t.Fatalf("feedback neither wrote feedback.jsonl nor refused the entry; the test did not run:\n%s", out)
			}
		})
	}
}
