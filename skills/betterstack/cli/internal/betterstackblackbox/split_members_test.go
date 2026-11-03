// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package betterstackblackbox

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Codex r1 4.34.2 input as given: a held API token typed into feedback in two
// members with a member between them reaches neither the output nor
// feedback.jsonl whole.
func TestFeedbackSplitAcrossMembersWithheld(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credStored)
	e.env = append(e.env, "BETTERSTACK_API_TOKEN="+"ABCDEFGH"+"12345678")
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
	// The kit's file sink writes the entry withheld (each half is an
	// 8-character piece of the held token). A run that wrote no feedback.jsonl
	// proves nothing.
	if files == 0 {
		t.Fatalf("feedback wrote no feedback.jsonl; the test did not run:\n%s", out)
	}
}
