// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package msgraphblackbox

import (
	"os"
	"path/filepath"
	"testing"
)

// Startup ordering (startup-output-held-first): a config file that does not
// parse still has every value in it held before the CLI or the MCP server can
// print its parse error, wherever in the file the credential sits (the broken
// line itself, or past it).
func TestMalformedConfigNeverPrintsItsCredentials(t *testing.T) {
	sv := echoServer(t)
	for name, body := range map[string]string{
		"on-the-broken-line": "base_url = \"https://graph.microsoft.com/v1.0\"\ngraph_token = \"" + bbStored + "\" \"" + bbStored + "\"\n",
		"after-the-break":    "base_url = [unclosed\ngraph_token = \"" + bbStored + "\"\n",
		"bare-value":         "graph_token = " + bbStored + "\n",
	} {
		e := newEnv(t, sv, true)
		cfg := filepath.Join(e.home, name+".toml")
		if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		e.env = append(withoutEnv(e.env, "MICROSOFT_GRAPH_CONFIG"), "MICROSOFT_GRAPH_CONFIG="+cfg)
		for where, out := range map[string]string{
			name + " CLI version": e.run(t, "version"),
			name + " CLI doctor":  e.run(t, "doctor", "--json"),
			name + " MCP startup": e.mcpCalls(t, nil),
		} {
			assertNoEcho(t, where, out)
		}
	}
}
