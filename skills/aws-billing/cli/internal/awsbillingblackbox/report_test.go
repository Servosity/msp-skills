// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package awsbillingblackbox

// report-post-needs-yes, mcp-mirror-write-confirm (aws-billing:sends-with-yes)
// and child-env-without-aws-credentials, on the built binaries: the one
// external write is report --post-slack, which runs a slack-pp-cli child (a
// stand-in on PATH records its argv, stdin and environment). The CLI previews
// unless --yes; the MCP report tool previews until confirm is boolean true
// (the gate then adds --yes) and never writes a report file; the child starts
// without any AWS credential variable or a variable outside its allowlist.

import (
	"runtime"
	"strings"
	"testing"
)

// awsCredentialEnvNames are the variables a helper process must not inherit.
var awsCredentialEnvNames = []string{"AWS_ACCESS_KEY_ID=", "AWS_SECRET_ACCESS_KEY=", "AWS_SESSION_TOKEN=", "AWS_SECURITY_TOKEN=",
	"AWS_CONTAINER_AUTHORIZATION_TOKEN", "AWS_WEB_IDENTITY_TOKEN_FILE=", "AWS_BEARER_TOKEN_", "AWS_BILLING_MCP_HTTP_TOKEN="}

// checkSlackRuns checks every recorded slack-pp-cli run: no AWS credential
// variable in its environment (which did reach it: the marker variable is
// there), and nothing it was handed holds a credential.
func checkSlackRuns(t *testing.T, e *bbEnv) map[string]map[string]string {
	t.Helper()
	runs := e.slackRuns(t)
	for id, r := range runs {
		if !strings.Contains(r["env"], "HOME="+e.home) || !strings.Contains(r["env"], "AWS_REGION=us-east-1") {
			t.Errorf("slack run %s: its environment was not recorded:\n%.600s", id, r["env"])
		}
		// The helper environment is an allowlist (Codex r2 class
		// credential-handed-to-another-program): an arbitrary variable is
		// not inherited.
		if strings.Contains(r["env"], "BB_CHILD_MARKER=") {
			t.Errorf("slack run %s inherited a variable outside the helper allowlist (BB_CHILD_MARKER)", id)
		}
		for _, line := range strings.Split(r["env"], "\n") {
			for _, name := range awsCredentialEnvNames {
				if strings.HasPrefix(line, name) {
					t.Errorf("slack run %s started with %s", id, strings.SplitN(line, "=", 2)[0])
				}
			}
		}
		assertNoEcho(t, "slack run "+id+" argv", r["argv"])
		assertNoEcho(t, "slack run "+id+" stdin", r["stdin"])
		assertNoEcho(t, "slack run "+id+" environment", r["env"])
	}
	return runs
}

// slackPosts counts the summary posts and the uploads among runs.
func slackPosts(runs map[string]map[string]string) (messages, uploads int) {
	for _, r := range runs {
		switch {
		case strings.HasPrefix(r["argv"], "messages\npost_message\n--channel=C123\n--text="):
			messages++
		case strings.HasPrefix(r["argv"], "files\nupload\n--stdin\n"):
			uploads++
		}
	}
	return messages, uploads
}

func TestReportPostsToSlackOnlyWithYes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the slack-pp-cli stand-in is a shell script")
	}
	t.Parallel()
	e := newEnv(t, srcEnv)
	e.aws.set(func(f *fakeAWS) { f.quiet = true }) // a report with an echo is withheld whole (the file write)
	e.api.quiet = true
	args := []string{"report", "--period", "this-month", "--data-source", "live", "--post-slack", "--slack-channel", "C123"}
	preview := e.run(t, args...)
	if !strings.Contains(preview, "Preview only: nothing was posted") {
		t.Errorf("report --post-slack without --yes printed no preview:\n%.1500s", preview)
	}
	if runs := e.slackRuns(t); len(runs) != 0 {
		t.Fatalf("report --post-slack without --yes started slack-pp-cli %d time(s)", len(runs))
	}
	posted := e.run(t, append(args, "--yes")...)
	if !strings.Contains(posted, "posted report to Slack channel C123") {
		t.Errorf("report --post-slack --yes did not post:\n%.1500s", posted)
	}
	runs := checkSlackRuns(t, e)
	if m, u := slackPosts(runs); m != 1 || u != 1 {
		t.Fatalf("want one summary post and one upload, got %d and %d: %v", m, u, runs)
	}
	for _, r := range runs {
		if strings.HasPrefix(r["argv"], "files\n") && !strings.Contains(r["stdin"], "HQ-Service-Compute") {
			t.Errorf("the uploaded report lost the bill:\n%.1500s", r["stdin"])
		}
	}
	assertNoEcho(t, "report output", preview+posted)
}

func TestMCPReportPostsOnlyWithConfirmAndWritesNoFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the slack-pp-cli stand-in is a shell script")
	}
	t.Parallel()
	e := newEnv(t, srcEnv)
	e.aws.set(func(f *fakeAWS) { f.quiet = true }) // a report with an echo is withheld whole (the file write)
	e.api.quiet = true
	report := func(post bool, confirm any) map[string]any {
		args := map[string]any{"period": "this-month", "data-source": "live"}
		if post {
			args["post-slack"], args["slack-channel"] = true, "C123"
		}
		if confirm != nil {
			args["confirm"] = confirm
		}
		return map[string]any{"name": "report", "arguments": args}
	}
	out := e.mcpPhases(t, []map[string]any{report(true, nil)}, []map[string]any{report(true, "true")}, []map[string]any{report(false, true)})
	replies := mcpReplies(out)
	for id := 1; id <= 2; id++ {
		if !strings.Contains(replies[id], "Preview only: nothing was posted") {
			t.Errorf("call %d (no boolean confirm) did not preview:\n%.1500s", id, replies[id])
		}
	}
	if runs := e.slackRuns(t); len(runs) != 0 {
		t.Fatalf("the MCP report tool posted without confirm: %d slack run(s)", len(runs))
	}
	if !strings.Contains(replies[3], "HQ-Service-Compute") {
		t.Errorf("report with confirm true and no post returned no summary:\n%.1500s", replies[3])
	}
	sent := e.mcpPhases(t, []map[string]any{report(true, true)})
	if !strings.Contains(mcpReplies(sent)[1], "posted report to Slack channel C123") {
		t.Errorf("confirm true did not post:\n%.1500s", sent)
	}
	runs := checkSlackRuns(t, e)
	if m, u := slackPosts(runs); m != 1 || u != 1 {
		t.Fatalf("want one summary post and one upload, got %d and %d: %v", m, u, runs)
	}
	// An MCP call never writes a report file, previewing or posting.
	for rel := range e.writtenFiles(t) {
		if strings.HasSuffix(rel, ".html") || strings.HasSuffix(rel, ".pdf") {
			t.Errorf("an MCP report call wrote %s", rel)
		}
	}
	assertNoEcho(t, "MCP session", out+sent)
}
