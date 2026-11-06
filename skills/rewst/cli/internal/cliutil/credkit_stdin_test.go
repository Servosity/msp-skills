// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import "testing"

func TestFeedbackStdinReadsTheCommandFromArgs(t *testing.T) {
	for _, c := range []struct {
		args []string
		want bool
	}{
		{[]string{"feedback", "--stdin"}, true},
		{[]string{"--json", "feedback", "--stdin"}, true},
		{[]string{"feedback", "note"}, false},
		{[]string{"users", "create", "--stdin", "--name", "feedback"}, false},
		{[]string{"--", "feedback", "--stdin"}, false},
		{[]string{"--stdin"}, false},
	} {
		if got := FeedbackStdin(c.args); got != c.want {
			t.Errorf("FeedbackStdin(%q) = %v, want %v", c.args, got, c.want)
		}
	}
}
