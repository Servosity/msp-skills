// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package awsbillingblackbox

// Codex r1b class credential-used-but-not-held (r1b-signers-held), on the
// built binaries: in an assume-role chain the SDK signs the STS AssumeRole
// call with the source profile's credentials, which the credential provider
// wrapper never sees (it wraps only the outermost provider). STS refuses the
// call with an error whose Message quotes the access key ID that signed it;
// that key must never reach stdout, stderr or MCP output.
//
//   - a credential_process source (Codex's input): its key is minted at run
//     time and read by no startup source (a shell script, so not on Windows);
//   - static keys in the shared credentials file: held from startup, a
//     control that runs on every OS.
//
// The successful chain then runs end to end through the kit-wired binary:
// holding inside the transport's per-request hook does not deadlock against
// the kit's guard.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	r1bProcKey    = "ASIA" + "BBPROCSRCKEY0001"
	r1bProcSecret = "bbProcSourceSecret/6nQ2wE8rT4yU0iO"
)

// r1bChainProfiles adds a credential_process source profile and a role
// profile that assumes its role with it.
func r1bChainProfiles(t *testing.T, e *bbEnv) {
	t.Helper()
	script := filepath.Join(e.home, "procsrc.sh")
	e.input(t, script, "printf '{\"Version\":1,\"AccessKeyId\":\"%s\",\"SecretAccessKey\":\"%s\"}\\n' '"+r1bProcKey+"' '"+r1bProcSecret+"'\n")
	cfg, err := os.ReadFile(filepath.Join(e.home, ".aws", "config"))
	if err != nil {
		t.Fatal(err)
	}
	e.input(t, filepath.Join(e.home, ".aws", "config"), string(cfg)+
		"\n[profile bbprocsrc]\ncredential_process = sh "+script+"\nregion = us-east-1\n"+
		"\n[profile bbproctarget]\nrole_arn = arn:aws:iam::111111111111:role/bb\nsource_profile = bbprocsrc\nregion = us-east-1\n")
}

// r1bShowsKey reports whether out carries key in any form the fakes send.
func r1bShowsKey(out, key string) string {
	for _, f := range echoForms(key) {
		if strings.Contains(out, f) {
			return around(out, f)
		}
	}
	return ""
}

func TestR1bAssumeRoleSourceKeyNeverPrinted(t *testing.T) {
	t.Parallel()
	type chain struct {
		name, target, key string
		process           bool
	}
	chains := []chain{{"static keys in the shared credentials file", "bbrole", bbFileKey, false}}
	if runtime.GOOS != "windows" {
		chains = append(chains, chain{"credential_process", "bbproctarget", r1bProcKey, true})
	}
	for _, c := range chains {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t, srcProfileFlag)
			r1bChainProfiles(t, e)
			e.aws.set(func(f *fakeAWS) { f.quiet, f.assumeDeny = true, true })
			e.api.quiet = true
			out := e.run(t, "bill", "--profile-aws", c.target, "--data-source", "live")
			if e.aws.signed(c.key) == 0 || e.aws.count("AssumeRole") == 0 {
				t.Fatalf("control: AssumeRole was never signed with the source key; requests %v:\n%.1200s", e.aws.seen(), out)
			}
			if s := r1bShowsKey(out, c.key); s != "" {
				t.Errorf("CLI: the source profile's access key ID, quoted by STS, was printed: %s", s)
			}
			mcp := e.mcpCalls(t, []map[string]any{{"name": "bill", "arguments": map[string]any{"profile-aws": c.target, "data-source": "live"}}})
			if len(mcpReplies(mcp)) < 2 {
				t.Fatalf("the MCP session did not answer:\n%.2000s", mcp)
			}
			if s := r1bShowsKey(mcp, c.key); s != "" {
				t.Errorf("MCP: the source profile's access key ID, quoted by STS, was returned: %s", s)
			}

			// The chain works through the kit-wired binary when STS answers:
			// the role's credentials sign the service calls.
			e.aws.set(func(f *fakeAWS) { f.assumeDeny = false })
			ok, code := e.runCode(t, "", bbCLI, "bill", "--profile-aws", c.target, "--data-source", "live", "--json")
			if code != 0 || e.aws.mintedSigned() == 0 || e.aws.count("GetCostAndUsage") == 0 {
				t.Errorf("the role chain did not run end to end (exit %d, requests %v):\n%.1500s", code, e.aws.seen(), ok)
			}
		})
	}
}
