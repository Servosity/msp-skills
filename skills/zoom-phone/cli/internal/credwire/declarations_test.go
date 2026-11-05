// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"zoom-phone-pp-cli/internal/cli"
	"zoom-phone-pp-cli/internal/cliutil"
	"zoom-phone-pp-cli/internal/cliutil/testenv"
	"zoom-phone-pp-cli/internal/credkit"
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
	for _, s := range []credkit.Source{"env:ZOOM_PHONE_MCP_HTTP_TOKEN"} {
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

	files := credentialFiles([]string{"users", "list-phone", "--config", explicit})
	if !slices.Contains(files, explicit) || slices.Contains(files, defaultConfig) {
		t.Errorf("--config: Files %q; want %s and not the discovered default", files, explicit)
	}

	t.Setenv("ZOOM_PHONE_CONFIG", explicit)
	files = credentialFiles([]string{"users", "list-phone"})
	if !slices.Contains(files, explicit) || slices.Contains(files, defaultConfig) {
		t.Errorf("environment selection: Files %q; want %s and not the discovered default", files, explicit)
	}
	t.Setenv("ZOOM_PHONE_CONFIG", "")

	if files = credentialFiles(nil); !slices.Contains(files, defaultConfig) {
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
	files = credentialFiles([]string{"--home", home, "users", "list-phone"})
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
	if err := os.WriteFile(store, []byte("base_url = \"https://issued.example\"\naccess_token = \"unit-store-token\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if files = credentialFiles(nil); slices.Contains(files, store) {
		t.Errorf("a store recording a base URL is listed: %q", files)
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

// A spec field the one name rule calls a credential is declared. A reprint whose spec
// adds one fails here until it is judged. Ordinary identifiers stay out of every list,
// or every listing would be withheld.
func TestSpecCredentialFieldsAreDeclared(t *testing.T) {
	for n := range specFields(t) {
		name := strings.ReplaceAll(n, "-", "_")
		if cliutil.IsCredentialFieldName(n) && !slices.Contains(Declarations.ResponseFields, name) && !slices.Contains(Declarations.MCPArguments, name) {
			t.Errorf("the spec's %q is a credential by the rule: declare it", n)
		}
	}
	for _, list := range [][]string{Declarations.ResponseFields, Declarations.CredentialFields, Declarations.MCPArguments} {
		for _, n := range list {
			if !cliutil.IsCredentialFieldName(n) {
				t.Errorf("%q is declared, and the one name rule does not call it a credential", n)
			}
		}
		for _, id := range []string{"id", "name", "email", "user_id", "account_id", "extension_number", "site_id"} {
			if slices.Contains(list, id) {
				t.Errorf("%s is declared a credential, and it is an ordinary identifier", id)
			}
		}
	}
}

// A credential-named flag on any command is declared, and so is --client-id, half of
// the Basic credential. A reprint that adds one fails here until it is judged.
func TestEveryCredentialFlagIsDeclared(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		c.Flags().VisitAll(func(f *pflag.Flag) {
			named := cliutil.IsCredentialFieldName(f.Name) || f.Name == "client-id"
			if named && f.Value.Type() != "bool" && !slices.Contains(Declarations.CredentialFlags, "--"+f.Name) {
				t.Errorf("%s --%s is a credential by the rule: declare it", c.CommandPath(), f.Name)
			}
		})
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(cli.RootCmd())
}
