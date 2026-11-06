// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/cliutil/testenv"
)

func runAuthCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"auth"}, args...))
	err := cmd.Execute()
	return out.String(), err
}

// TestAuthSetupDescribesESET (handfixes.json: auth-setup-describes-eset):
// `auth setup` printed the generated API-key template ("Get a key at", the
// guide index, `echo "$TOKEN" | auth set-token`, POSIX only, "ESET MSP
// Administrator" without the 2). ESET Connect has no API keys: the steps
// must create the API user, set the three variables in both shells, and run
// `auth login`.
func TestAuthSetupDescribesESET(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	t.Setenv("ESET_PROTECT_USERNAME", "")
	t.Setenv("ESET_PROTECT_PASSWORD", "")
	t.Setenv("ESET_PROTECT_TOKEN", "")

	out, err := runAuthCmd(t, "setup")
	if err != nil {
		t.Fatalf("auth setup: %v", err)
	}
	for _, want := range []string{
		"https://help.eset.com/eset_connect/en-US/create_api_user_account.html",
		"ESET PROTECT Hub, ESET Business Account or\n   ESET MSP Administrator 2",
		"Integrations toggle",
		`export ESET_PROTECT_USERNAME="`, `export ESET_PROTECT_PASSWORD="`, `export ESET_PROTECT_REGION="eu"`,
		`$env:ESET_PROTECT_USERNAME = "`, `$env:ESET_PROTECT_PASSWORD = "`, `$env:ESET_PROTECT_REGION = "eu"`,
		"eu, de, us, ca or jpn",
		esetCLIName + " auth login",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("auth setup output lacks %q:\n%s", want, out)
		}
	}
	for _, banned := range []string{"Get a key", "set-token", "$TOKEN", "ESET_PROTECT_TOKEN"} {
		if strings.Contains(out, banned) {
			t.Errorf("auth setup output still carries the API-key template (%q):\n%s", banned, out)
		}
	}
	// ESET: every new API user, Hub included, receives the invitation email and
	// must finish creating the account; a Hub user must ALSO sign in to Hub once.
	flat := strings.Join(strings.Fields(out), " ")
	for _, want := range []string{
		"Every new API user (ESET PROTECT Hub, ESET Business Account or ESET MSP Administrator 2) receives an invitation email and must finish creating the account",
		"A new ESET PROTECT Hub user must also sign in to ESET PROTECT Hub once",
	} {
		if !strings.Contains(flat, want) {
			t.Errorf("auth setup must say %q (Hub users are invited too):\n%s", want, out)
		}
	}
	if strings.Contains(flat, "In ESET Business Account and ESET MSP Administrator 2 the new user accepts") {
		t.Errorf("auth setup still implies ESET PROTECT Hub users skip the invitation:\n%s", out)
	}

	t.Setenv(cliutil.VerifyEnvVar, "1")
	out, err = runAuthCmd(t, "setup", "--launch")
	if err != nil || !strings.Contains(out, "would launch: "+EsetAPIUserURL) {
		t.Fatalf("auth setup --launch must open ESET's create-API-user guide: %v\n%s", err, out)
	}
	t.Setenv(cliutil.VerifyEnvVar, "")

	out, err = runAuthCmd(t, "status")
	if err == nil {
		t.Fatalf("auth status with no credentials must fail:\n%s", out)
	}
	if strings.Contains(out, "set-token") || !strings.Contains(out, "auth login") {
		t.Fatalf("auth status still points at the API-key template:\n%s", out)
	}
	t.Setenv("ESET_PROTECT_USERNAME", "api@msp.example")
	t.Setenv("ESET_PROTECT_PASSWORD", "s3cret")
	out, _ = runAuthCmd(t, "status")
	if !strings.Contains(out, "ESET_PROTECT_USERNAME and ESET_PROTECT_PASSWORD are set") {
		t.Fatalf("auth status with a configured API user must say how to mint the token:\n%s", out)
	}
}

// TestNoGenericCredentialTemplateInGoStrings closes the class: no Go file in
// the module names the console without its "2", and no non-test file tells
// an operator to "get a key" (ESET Connect has none).
func TestNoGenericCredentialTemplateInGoStrings(t *testing.T) {
	bareName := regexp.MustCompile(`ESET MSP ` + `Administrator(?:[^ 2]| [^2]|$)`)
	keyTemplate := "Get a " + "key at"
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		for i, line := range strings.Split(string(data), "\n") {
			if bareName.MatchString(line) {
				t.Errorf("%s:%d names the ESET MSP console without its 2: %s", path, i+1, strings.TrimSpace(line))
			}
			if !strings.HasSuffix(path, "_test.go") && strings.Contains(line, keyTemplate) {
				t.Errorf("%s:%d tells the operator to get an API key: %s", path, i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
