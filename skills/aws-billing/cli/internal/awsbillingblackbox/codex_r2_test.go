// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package awsbillingblackbox

// Codex review round 2 (aws-billing 4.34.2 reprint): a gap round 2 found in a
// round-1 class fix (the stop rule fired; fixed after it, unreviewed), seen
// failing on the round-1 code (seen/r2-holder-incomplete-seen-failing.log in
// the run directory): holder-incomplete, token-file paths were trimmed (the
// SDK opens them as written) and credential files were read only to 1 MiB.
// TestR2HelperGetsNoPieceOfAHeldCredential runs the credential-handed input
// on the binaries; the cache class is tested in internal/cli
// (awsbilling_codex_r2_test.go).

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	r2TokenSecret = "webTok" + "N4q8Zp2Lx-5e2a9c"
	r2AuthSecret  = "ctrAuth" + "K7w3Rm9Vb-8d1f3b"
	r2BigSecret   = "bigFile" + "J2t6Hy4Qn/7qW3eR9tY"
	r2DirSecret   = "lgdX5c1P" + "k8Ws-2c7e4a"
)

func TestR2HolderReadsSourcesAsTheSDKDoes(t *testing.T) {
	t.Parallel()
	e := newEnv(t, srcProfileFlag)
	// Token files named with a trailing space: the SDK opens the path as
	// written; the trimmed path does not exist.
	tok := filepath.Join(e.home, "webid.token")
	e.input(t, tok+" ", r2TokenSecret+"\n")
	auth := filepath.Join(e.home, "container.auth")
	e.input(t, auth+" ", r2AuthSecret)
	// A directory named with a trailing space; Windows cannot hold one (a
	// path through "logincache " is not found there, for the SDK and the
	// holder alike), so there the directory is named without it.
	dir := filepath.Join(e.home, "logincache") + " "
	if runtime.GOOS == "windows" {
		dir = strings.TrimSpace(dir)
	}
	e.input(t, filepath.Join(dir, "bb.json"), `{"refreshToken":"`+r2DirSecret+`"}`)
	// A shared credentials file past 1 MiB: the SDK reads all of it.
	creds := filepath.Join(e.home, ".aws", "credentials")
	old, err := os.ReadFile(creds)
	if err != nil {
		t.Fatal(err)
	}
	e.input(t, creds, string(old)+"\n#"+strings.Repeat("x", 1<<20)+"\n[bbbig]\naws_access_key_id = AKIA"+"BBBIGFILEKEY0001\naws_secret_access_key = "+r2BigSecret+"\n")
	e = e.with("AWS_WEB_IDENTITY_TOKEN_FILE="+tok+" ", "AWS_ROLE_ARN=arn:aws:iam::111111111111:role/bbweb",
		"AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE="+auth+" ", "AWS_LOGIN_CACHE_DIRECTORY="+dir)
	// Control: recall prints a query that is not held, so a missing value
	// below is the kit's hold, not an empty answer.
	if out, code := e.runCode(t, "", bbCLI, "recall", "bbControlQuery-4d7e1a", "--agent", "--no-learn"); !strings.Contains(out, "bbControlQuery-4d7e1a") {
		t.Fatalf("recall did not print an ordinary query (%s)", quietReason(out, code))
	}
	for name, s := range map[string]string{
		"AWS_WEB_IDENTITY_TOKEN_FILE named with a trailing space":            r2TokenSecret,
		"AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE named with a trailing space": r2AuthSecret,
		"AWS_LOGIN_CACHE_DIRECTORY named with a trailing space":              r2DirSecret,
		"a shared credentials file value past 1 MiB":                         r2BigSecret,
	} {
		if out := e.run(t, "recall", s, "--agent", "--no-learn"); strings.Contains(out, s) {
			t.Errorf("%s is not held: recall printed it:\n%.600s", name, around(out, s))
		}
	}
}

// Codex r2 input, class credential-handed-to-another-program: a held secret
// key split between an inherited variable (REPORT_LABEL, its first 26
// characters) and the Slack channel argument (its last 4). report
// --post-slack starts slack-pp-cli; nothing it receives (argv, environment,
// stdin) carries an 8-character piece of the key.
func TestR2HelperGetsNoPieceOfAHeldCredential(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the slack-pp-cli stand-in is a shell script")
	}
	t.Parallel()
	const secret = "Qp4uW8mK" + "p6R2nF3cT1xV7dH5zN9AB0"
	e := newEnv(t, srcEnv).with("AWS_SECRET_ACCESS_KEY=" + secret)
	e.aws.set(func(f *fakeAWS) { f.quiet = true })
	e.api.quiet = true
	db := filepath.Join(e.work, "r2.db")
	if out, code := e.runCode(t, "", bbCLI, "sync", "--from", "2026-08-01", "--to", "2026-10-01", "--skip-inventory", "--db", db, "--json"); code != 0 {
		t.Fatalf("sync for the local report: exit %d\n%.1500s", code, out)
	}
	out, _ := e.with("REPORT_LABEL="+secret[:26]).runCode(t, "", bbCLI, "report", "--db", db, "--period", "2026-09-01:2026-10-01",
		"--data-source", "local", "--out", filepath.Join(e.work, "r2.html"), "--post-slack", "--yes", "--slack-channel", secret[26:])
	runs := e.slackRuns(t)
	if len(runs) == 0 {
		t.Fatalf("report --post-slack --yes started no slack-pp-cli; the test did not run:\n%.1500s", out)
	}
	for id, r := range runs {
		got := r["argv"] + "\n" + r["env"] + "\n" + r["stdin"]
		for i := 0; i+8 <= len(secret); i++ {
			if strings.Contains(got, secret[i:i+8]) {
				t.Errorf("slack run %s received %q, a piece of the held key: %s", id, secret[i:i+8], around(got, secret[i:i+8]))
				break
			}
		}
	}
}
