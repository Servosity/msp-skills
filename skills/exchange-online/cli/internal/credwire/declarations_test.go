// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/pflag"

	"exchange-online-pp-cli/internal/cli"
	"exchange-online-pp-cli/internal/cliutil"
	"exchange-online-pp-cli/internal/cliutil/testenv"
	"exchange-online-pp-cli/internal/credkit"
)

// config:<key> exempts exactly NotCredentials, and no key is both.
func TestConfigKeyListsAgree(t *testing.T) {
	for _, k := range cliutil.NotCredentials {
		if !slices.ContainsFunc(Declarations.Identifiers, func(id credkit.Identifier) bool { return id.Source == credkit.Source("config:"+k) }) {
			t.Errorf("cliutil.NotCredentials names %s; Identifiers has no config:%s", k, k)
		}
	}
	for _, id := range Declarations.Identifiers {
		if k, ok := strings.CutPrefix(string(id.Source), "config:"); ok && !slices.Contains(cliutil.NotCredentials, k) {
			t.Errorf("Identifiers exempts %s; cliutil.NotCredentials does not", id.Source)
		}
		if id.Reason == "" {
			t.Errorf("Identifiers: %s has no reason", id.Source)
		}
	}
	for _, k := range Declarations.ConfigFields {
		if slices.Contains(cliutil.NotCredentials, k) {
			t.Errorf("%s is declared a credential and exempted", k)
		}
		if !slices.Contains(Declarations.StoredRecords, k) {
			t.Errorf("%s is a config credential and not a stored record", k)
		}
	}
}

// A token goes to the configured API base only, and the MCP bearers go nowhere.
func TestOriginsNameTheBaseAndNoOther(t *testing.T) {
	seen := map[credkit.Source][]string{}
	for _, o := range Declarations.Origins {
		seen[o.Source] = o.Allowed
	}
	if !slices.Equal(seen["*"], []string{"base_url"}) {
		t.Errorf("default origin rule: %q", seen["*"])
	}
	for _, s := range []credkit.Source{"env:EXCHANGE_ONLINE_MCP_HTTP_TOKEN"} {
		if allowed, ok := seen[s]; !ok || len(allowed) != 0 {
			t.Errorf("%s may go to %q (declared: %v)", s, allowed, ok)
		}
	}
}

// Only what the run takes its base URL from is listed for the kit. A discovered
// default under an explicit selection, another --home, and a store recording the base
// URL its token was issued for are held, never listed: the first base_url in the
// list becomes the one origin a credential may go to (item 8b).
func TestFilesListOnlyWhatTheRunReads(t *testing.T) {
	root := testenv.Isolate(t)
	defaultDir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(defaultDir, 0o700); err != nil {
		t.Fatal(err)
	}
	defaultConfig := filepath.Join(defaultDir, "config.toml")
	if err := os.WriteFile(defaultConfig, []byte("base_url = \"https://discovered.example\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	explicit := filepath.Join(root, "named.toml")

	files := pathsUnder([]string{"mailboxes", "list", "--config", explicit})
	if !slices.Contains(files, explicit) || slices.Contains(files, defaultConfig) {
		t.Errorf("--config: Files %q; want %s and not the discovered default", files, explicit)
	}

	t.Setenv("EXCHANGE_ONLINE_CONFIG", explicit)
	files = pathsUnder([]string{"mailboxes", "list"})
	if !slices.Contains(files, explicit) || slices.Contains(files, defaultConfig) {
		t.Errorf("environment selection: Files %q; want %s and not the discovered default", files, explicit)
	}
	t.Setenv("EXCHANGE_ONLINE_CONFIG", "")

	if files = pathsUnder(nil); !slices.Contains(files, defaultConfig) {
		t.Errorf("no selection: Files %q; want the default %s", files, defaultConfig)
	}

	// Load falls back to the legacy file in the real home when a home has no config,
	// so only a home with its own config leaves that file out.
	home := filepath.Join(root, "h1")
	if err := os.MkdirAll(filepath.Join(home, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config", "config.toml"), []byte("# nothing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	files = pathsUnder([]string{"--home", home, "mailboxes", "list"})
	if slices.Contains(files, defaultConfig) {
		t.Errorf("--home: Files %q names the real home's config", files)
	}
	if restored, err := cliutil.ConfigDir(); err != nil || restored != defaultDir {
		t.Errorf("a --home override survived Files: %q (%v)", restored, err)
	}

	store, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(store), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store, []byte("online_token = \"unit-store-token\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if files = pathsUnder(nil); !slices.Contains(files, store) {
		t.Errorf("Files %q; want the credential store %s", files, store)
	}
	if err := os.WriteFile(store, []byte("base_url = \"https://issued.example\"\nonline_token = \"unit-store-token\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if files = pathsUnder(nil); slices.Contains(files, store) {
		t.Errorf("a store recording a base URL is listed: %q", files)
	}
}

// The set-token words are matched in order, so nothing else reads a pipe.
func TestSetTokenArguments(t *testing.T) {
	for _, tc := range []struct {
		args []string
		rest []string
		ok   bool
	}{
		{[]string{"auth", "set-token"}, []string{}, true},
		{[]string{"--json", "auth", "set-token", "k1"}, []string{"k1"}, true},
		{[]string{"search", "set-token"}, nil, false},
		{[]string{"set-token", "auth"}, nil, false},
		{[]string{"mailboxes", "list"}, nil, false},
	} {
		rest, ok := setTokenArguments(tc.args)
		if ok != tc.ok || !slices.Equal(rest, tc.rest) {
			t.Errorf("%q: got %q %v", tc.args, rest, ok)
		}
	}
}

// specFields are the fields of internal/types and the parameters of the press
// manifest, both generated from the spec.
func specFields(t *testing.T) map[string]bool {
	t.Helper()
	seen := map[string]bool{}
	src, err := os.ReadFile(filepath.Join("..", "types", "types.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile("json:\"([^\",]+)").FindAllStringSubmatch(string(src), -1) {
		seen[m[1]] = true
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "press", "tools-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Tools []struct {
			Params []struct {
				Name string `json:"name"`
			} `json:"params"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, tool := range manifest.Tools {
		for _, p := range tool.Params {
			seen[p.Name] = true
		}
	}
	if len(seen) < 20 {
		t.Fatalf("read only %d names from the spec; the scan is broken", len(seen))
	}
	return seen
}

// No cmdlet field or parameter in the spec has a credential name. A reprint whose
// spec adds one fails here until it is judged and declared. Ordinary identifiers stay
// out of every list, or every listing would be withheld.
func TestDeclaredFieldsAreNotSpecFields(t *testing.T) {
	for n := range specFields(t) {
		if cliutil.SecretFieldName(n) {
			t.Errorf("the spec's %q is a credential by the rule: declare it", n)
		}
	}
	for name, list := range map[string][]string{"ResponseFields": Declarations.ResponseFields, "CredentialFields": Declarations.CredentialFields, "MCPArguments": Declarations.MCPArguments} {
		for _, n := range list {
			if !cliutil.SecretFieldName(n) {
				t.Errorf("%s declares %q, which the one name rule does not call a credential", name, n)
			}
		}
		for _, id := range []string{"Identity", "Guid", "Name", "ExternalDirectoryObjectId", "UserPrincipalName", "Mailbox", "tenantId"} {
			if slices.Contains(list, id) {
				t.Errorf("%s holds %s, an ordinary identifier", name, id)
			}
		}
	}
}

// A flag the command gains or loses changes what counts as a key on its line.
func TestSetTokenFlagsMatchTheCommand(t *testing.T) {
	cmd, _, err := cli.RootCmd().Find([]string{"auth", "set-token"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	cmd.InitDefaultHelpFlag()
	cmd.Flags().AddFlagSet(cmd.InheritedFlags())
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		got[f.Name] = f.NoOptDefVal == ""
		if f.Shorthand != "" {
			got[f.Shorthand] = f.NoOptDefVal == ""
		}
	})
	if !maps.Equal(got, setTokenFlags) {
		var lines []string
		for name, takes := range got {
			lines = append(lines, fmt.Sprintf("\t%q: %v,", name, takes))
		}
		slices.Sort(lines)
		t.Errorf("setTokenFlags is out of date; the command declares:\n%s", strings.Join(lines, "\n"))
	}
}
