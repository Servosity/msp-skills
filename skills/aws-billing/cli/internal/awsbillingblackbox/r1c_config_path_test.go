// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package awsbillingblackbox

// Class held-set-poisoned-by-a-path-the-sdk-never-opens (Windows CI, PR #607):
// the startup holder named the kit every reading of a web_identity_token_file
// value in ~/.aws/config, the untrimmed one included. On Windows that value is
// " C:\...": not a file name at all, so the kit could not tell it from an
// unreadable credential file, marked the held set unknown and withheld every
// write, its own notices included; the program ran and printed nothing. The
// SDK opens only the trimmed value with its comment cut, so that is the one
// path the holder names. On Unix the same kit rule is driven by a regular file
// named " " in the working directory, which makes the untrimmed relative path
// fail with "not a directory" instead of "does not exist".

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const r1cTokenSecret = "cfgTok" + "R7m2Qx9Lw4-6b1d8f"

func TestR1cConfigTokenFileNamedAsTheSDKOpensIt(t *testing.T) {
	t.Parallel()
	e := newEnv(t, srcProfileFlag)
	token := filepath.Join(e.home, ".aws", "cfgweb.token")
	e.input(t, token, r1cTokenSecret+"\n")
	cfgPath := filepath.Join(e.home, ".aws", "config")
	cfg, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	e.input(t, cfgPath, string(cfg)+"\n[profile bbcfgweb]\nrole_arn = arn:aws:iam::111111111111:role/bbweb\n"+
		"web_identity_token_file = "+token+" # the token file\nregion = us-east-1\n")
	if runtime.GOOS != "windows" {
		// A Windows path that keeps its leading blank is already no file name.
		e.input(t, filepath.Join(e.work, " "), "")
	}

	const control = "bbCfgControlQuery-9e4a2c"
	if out, code := e.runCode(t, "", bbCLI, "recall", control, "--agent", "--no-learn"); !strings.Contains(out, control) {
		t.Fatalf("recall did not print an ordinary query (%s)", quietReason(out, code))
	}
	if out := e.run(t, "recall", r1cTokenSecret, "--agent", "--no-learn"); strings.Contains(out, r1cTokenSecret) {
		t.Errorf("the token file a profile names is not held: recall printed it:\n%.600s", around(out, r1cTokenSecret))
	}
}

// quietReason says why a run printed less than expected without quoting its
// output, which may carry a held fixture value: the exit code, the size of the
// output and whether the credential kit withheld a unit.
func quietReason(out string, code int) string {
	notice := "no withhold notice"
	if strings.Contains(out, "[credkit:") {
		notice = "a credential kit withhold notice"
	}
	return "exit " + strconv.Itoa(code) + ", " + strconv.Itoa(len(strings.TrimSpace(out))) + " bytes of output, " + notice
}
