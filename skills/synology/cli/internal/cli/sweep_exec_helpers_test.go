// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): shared helpers for the press 4.33.5 template
// class tests recorded in skills/synology/handfixes.json.

package cli

import (
	"os"
	"path/filepath"
	"testing"

	"synology-pp-cli/internal/cliutil/testenv"
)

// sweepIsolate gives the test a throwaway home with learning off, no NAS
// address and no DSM credentials, so nothing reaches the operator's files or a
// real NAS.
func sweepIsolate(t *testing.T) string {
	t.Helper()
	home := testenv.Isolate(t)
	for _, k := range []string{"SYNOLOGY_BASE_URL", "SYNOLOGY_ACCOUNT", "SYNOLOGY_PASSWORD", "SYNOLOGY_DEVICE_ID",
		"SYNOLOGY_OTP_CODE", "SYNOLOGY_TLS_FINGERPRINT", "SYNOLOGY_SKIP_TLS_VERIFY", "SYNOLOGY_FEEDBACK_ENDPOINT",
		"SYNOLOGY_FEEDBACK_AUTO_SEND", "PRINTING_PRESS_VERIFY", "PRINTING_PRESS_DOGFOOD", "PRINTING_PRESS_CLIENT_PROFILE"} {
		t.Setenv(k, "")
	}
	t.Setenv("SYNOLOGY_NO_LEARN", "1")
	return home
}

// sweepExecute runs the real Execute() (the binary's entry point, with its
// stdout/stderr writers and --deliver wiring) with args, capturing the
// process streams in files, and returns them with the exit code.
func sweepExecute(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	dir := t.TempDir()
	outF, err := os.Create(filepath.Join(dir, "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	errF, err := os.Create(filepath.Join(dir, "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	oldArgs, oldOut, oldErr := os.Args, os.Stdout, os.Stderr
	oldNoColor, oldHuman := noColor, humanFriendly
	os.Args = append([]string{"synology-cli"}, args...)
	os.Stdout, os.Stderr = outF, errF
	defer func() {
		os.Args, os.Stdout, os.Stderr = oldArgs, oldOut, oldErr
		noColor, humanFriendly = oldNoColor, oldHuman
	}()
	if runErr := Execute(); runErr != nil {
		code = ExitCode(runErr)
	}
	_ = outF.Close()
	_ = errF.Close()
	o, _ := os.ReadFile(filepath.Join(dir, "stdout"))
	e, _ := os.ReadFile(filepath.Join(dir, "stderr"))
	return string(o), string(e), code
}
