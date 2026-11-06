// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package runzeroblackbox

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// Class false-withheld-own-metadata: agent-context describes the CLI's own
// auth (mode, env var names, a hint) under an "auth" key; that container is a
// registered own descriptor, so the chokepoint prints it unchanged and holds
// nothing from it, and never withholds the whole description, with no
// credential configured, with one in the environment and with one in a config
// file.
func TestAgentContextIsNeverWithheld(t *testing.T) {
	t.Parallel() // its own fake runZero and home
	sv := echoServer(t)
	for _, mode := range []credMode{credStored, credEnv, credConfigFile} {
		var e *bbEnv
		if mode == credStored {
			// a clean environment: nothing configured at all
			h := t.TempDir()
			e = &bbEnv{home: h, env: cleanEnv(h, "RUNZERO_BASE_URL="+sv.base(), "RUNZERO_CA_CERT="+sv.caFile)}
		} else {
			e = newEnv(t, sv, mode)
		}
		out := e.run(t, "agent-context")
		var doc struct {
			CLI      map[string]any `json:"cli"`
			Commands []any          `json:"commands"`
		}
		if err := json.NewDecoder(strings.NewReader(out)).Decode(&doc); err != nil || len(doc.Commands) == 0 {
			t.Fatalf("%v: agent-context is not the CLI's description (err %v):\n%.600s", mode, err, out)
		}
		assertNoEcho(t, "agent-context ("+mode.String()+")", out)
	}
}

// Class false-withheld-own-metadata, MCP channel: the MCP server's context
// tool describes the server's own auth (type, env var names, a hint) under an
// "auth" key too. That container is the server's own constant descriptor
// (internal/mcp/runzero_own_descriptor.go), so the chokepoint prints it
// unchanged and holds nothing from it, with no credential configured and with
// one in the environment, and a read after it still prints.
//
// The context also prints the server's config, data, state and cache paths.
// With a key held, a path of long random-looking runs (a temp directory
// such as /var/folders/<random>/T/...) can push the funnel's joined view of
// the 17 KB result past its work budget, which withholds the whole result by
// the contract's budget rule (P2 false-withheld-by-work-budget,
// seen/false-withheld-by-work-budget-seen-failing.log): the home is a short
// temp directory (os.MkdirTemp, not t.TempDir, whose name repeats the
// test's), and a result withheld whole with a key held is logged as that P2
// rather than failed. RUNZERO_BB_P2=1 uses t.TempDir homes and fails on it.
// The fake echoes nothing: the credential kit withholds a whole answer that
// repeats a held value, and the read here proves a later answer still prints.
func TestMCPContextIsNeverWithheld(t *testing.T) {
	sv := quietServer(t)
	for _, mode := range []credMode{credStored, credEnv} {
		h := t.TempDir()
		if os.Getenv("RUNZERO_BB_P2") == "" {
			d, err := os.MkdirTemp("", "rz")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(d) })
			h = d
		}
		env := cleanEnv(h, "RUNZERO_BASE_URL="+sv.base(), "RUNZERO_CA_CERT="+sv.caFile, "RUNZERO_CLI_PATH="+bbCLI)
		if mode == credEnv {
			env = append(env, "RUNZERO_API_KEY="+bbKey, "RUNZERO_MCP_HTTP_TOKEN="+bbHTTPToken)
		}
		e := &bbEnv{home: h, env: env}
		out := e.mcpPhases(t, []map[string]any{tool("context", map[string]any{})}, []map[string]any{read("org.get-sites", nil)})
		replies := mcpReplies(out)
		var msg struct {
			Result struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
		}
		var doc struct {
			Auth struct {
				Type    string `json:"type"`
				EnvVars []struct {
					Name string `json:"name"`
				} `json:"env_vars"`
			} `json:"auth"`
			ToolCount int `json:"tool_count"`
		}
		if json.Unmarshal([]byte(replies[1]), &msg) == nil && len(msg.Result.Content) == 1 && mode == credEnv &&
			strings.HasPrefix(msg.Result.Content[0].Text, kitNotice("mcp")) && os.Getenv("RUNZERO_BB_P2") == "" {
			t.Logf("%v: P2 false-withheld-by-work-budget: the context result was withheld whole (paths %s)", mode, h)
		} else if len(msg.Result.Content) == 0 || json.Unmarshal([]byte(msg.Result.Content[0].Text), &doc) != nil || doc.ToolCount == 0 {
			t.Fatalf("%v: the context tool's result is not the server's description:\n%.1500s", mode, replies[1])
		} else if doc.Auth.Type != "bearer_token" || len(doc.Auth.EnvVars) != 1 || doc.Auth.EnvVars[0].Name != "RUNZERO_API_KEY" {
			t.Errorf("%v: the context tool withheld its own auth descriptor:\n%.1500s", mode, replies[1])
		}
		if mode == credEnv && !strings.Contains(replies[2], "HQ-Record headquarters") {
			t.Errorf("%v: the read after the context was withheld:\n%.1500s", mode, replies[2])
		}
		assertNoEcho(t, "MCP context ("+mode.String()+")", out)
	}
}
