// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package awsbillingblackbox

// Startup ordering, on the built binaries: a credential in a malformed
// ~/.aws/credentials or a malformed --config file is held before either
// binary writes its first byte, so an error that echoes an argument (Cobra's
// unknown command, the MCP server's unknown transport) never shows it.

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStartupOutputNeverShowsAMalformedFileCredential(t *testing.T) {
	t.Parallel()
	const awsSecret = "startupAwsSecret7Kq9xZw"
	const cfgSecret = "startupCfgToken4Wm8pLq"
	e := newEnv(t, srcProfileEnv)
	e.input(t, filepath.Join(e.home, ".aws", "credentials"), "[bbfile]\naws_access_key_id = AKIA"+"BBSTARTUPKEY0001\naws_secret_access_key = "+
		awsSecret+"\nthis line is not ini [[\n")
	cfg := filepath.Join(e.home, "malformed.toml")
	e.input(t, cfg, "auth_header = \"Bearer "+cfgSecret+"\"\nbad = [\n")
	for _, c := range []struct {
		what, secret, bin string
		env               []string
		args              []string
	}{
		{"CLI, malformed ~/.aws/credentials", awsSecret, bbCLI, nil, []string{awsSecret}},
		{"CLI, malformed --config", cfgSecret, bbCLI, nil, []string{"--config", cfg, cfgSecret}},
		{"MCP, malformed ~/.aws/credentials", awsSecret, bbMCP, nil, []string{"--transport", awsSecret}},
		// The MCP server has no --config flag: AWS_BILLING_CONFIG names its file.
		{"MCP, malformed config file", cfgSecret, bbMCP, []string{"AWS_BILLING_CONFIG=" + cfg}, []string{"--transport", cfgSecret}},
	} {
		out, code := e.with(c.env...).runCode(t, "", c.bin, c.args...)
		if code == 0 {
			t.Errorf("%s: an unknown command or transport succeeded:\n%.600s", c.what, out)
		}
		if strings.Contains(out, c.secret) {
			t.Errorf("%s: startup output shows the credential:\n%.600s", c.what, around(out, c.secret))
		}
	}
}
