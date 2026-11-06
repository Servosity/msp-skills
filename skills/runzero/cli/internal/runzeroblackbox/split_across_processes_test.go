// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package runzeroblackbox

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// credential-split-across-output-records, across processes (the liongard
// review's round-2 input): two feedback invocations each append one half of
// the held credential to feedback.jsonl; the file must never hold both. Once
// for each place the one key comes from: RUNZERO_API_KEY, the same pasted
// with its "Bearer " scheme, a config file's auth_header ("Bearer <key>"),
// its api_key, and the credential store (auth set-token).
func TestFeedbackSplitAcrossInvocationsWithheld(t *testing.T) {
	t.Parallel() // its own fake runZero and home
	const tok = "RecordSplitSecretABCDEFGH"
	for name, env := range map[string][]string{
		"RUNZERO_API_KEY":             {"RUNZERO_API_KEY=" + tok},
		"RUNZERO_API_KEY with scheme": {"RUNZERO_API_KEY=" + bearer(tok)},
		"auth_header":                 nil,
		"api_key":                     nil,
		"stored":                      nil,
	} {
		t.Run(name, func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, credEnv)
			e.env = append(withoutEnv(e.env, "RUNZERO_API_KEY"), env...)
			switch name {
			case "auth_header":
				e.writeConfig(t, fmt.Sprintf("auth_header = %q\n", bearer(tok)))
			case "api_key":
				e.writeConfig(t, fmt.Sprintf("api_key = %q\n", tok))
			case "stored":
				e.storeKey(t, tok)
			}
			out := e.run(t, "feedback", "RecordSplit", "--no-learn")
			out += e.run(t, "feedback", "SecretABCDEFGH", "--no-learn")
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
				if s := string(data); strings.Contains(s, "RecordSplit") && strings.Contains(s, "SecretABCDEFGH") {
					t.Errorf("%s carries both halves of the held key:\n%s", path, s)
				}
				return nil
			})
			if files == 0 {
				t.Fatalf("feedback wrote no feedback.jsonl; the test did not run:\n%s", out)
			}
		})
	}
}
