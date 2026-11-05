// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rocketcyberblackbox

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// credential-split-across-output-records, across processes (the liongard
// review's round-2 input): two feedback invocations each append one half of
// the held token to feedback.jsonl; the file must never hold both. Once for each
// environment credential (the API token and the MCP HTTP token).
func TestFeedbackSplitAcrossInvocationsWithheld(t *testing.T) {
	for _, name := range []string{"ROCKETCYBER_API_TOKEN", "ROCKETCYBER_MCP_HTTP_TOKEN"} {
		t.Run(name, func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, credStored)
			const tok = "RecordSplitSecretABCDEFGH"
			e.env = append(withoutEnv(e.env, name), name+"="+tok)
			out := e.run(t, "feedback", "RecordSplit", "--no-learn")
			out += e.run(t, "feedback", "SecretABCDEFGH", "--no-learn")
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
				if s := string(data); strings.Contains(s, "RecordSplit") && strings.Contains(s, "SecretABCDEFGH") {
					t.Errorf("%s carries both halves of the held token:\n%s", path, s)
				}
				return nil
			})
			if files == 0 {
				t.Fatalf("feedback wrote no feedback.jsonl; the test did not run:\n%s", out)
			}
		})
	}
}
