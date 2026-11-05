// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"cisco-umbrella-pp-cli/internal/cliutil"
	"cisco-umbrella-pp-cli/internal/cliutil/testenv"
	"cisco-umbrella-pp-cli/internal/credkit"
)

// The two keys Declarations and the per-read hold each carry must be the same,
// and config:<key> exempts exactly NotCredentials.
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
	}
	// Written out in full so the wiring checker can read them; they stay one list.
	if !slices.Equal(Declarations.ConfigFields, cliutil.CredentialKeys) || !slices.Equal(Declarations.StoredRecords, cliutil.CredentialKeys) {
		t.Errorf("config credential keys disagree: %q, %q, %q", Declarations.ConfigFields, Declarations.StoredRecords, cliutil.CredentialKeys)
	}
	operator := append(slices.Clone(Declarations.ResponseFields), "key")
	if !slices.Equal(Declarations.CredentialFields, operator) || !slices.Equal(Declarations.MCPArguments, operator) {
		t.Errorf("CredentialFields and MCPArguments must be ResponseFields plus key: %q, %q", Declarations.CredentialFields, Declarations.MCPArguments)
	}
}

// Both origins a credential may go to are declared: the API base and Cisco's own
// host, which serves the default token URL. The MCP bearer goes nowhere.
func TestOriginsNameBothHostsAndNoOther(t *testing.T) {
	var star, bearer *credkit.Origin
	for i := range Declarations.Origins {
		switch Declarations.Origins[i].Source {
		case "*":
			star = &Declarations.Origins[i]
		case "env:CISCO_UMBRELLA_MCP_HTTP_TOKEN":
			bearer = &Declarations.Origins[i]
		}
	}
	if star == nil || !slices.Equal(star.Allowed, []string{"base_url", "https://api.umbrella.com"}) {
		t.Errorf("default origin rule: %+v", star)
	}
	if bearer == nil || len(bearer.Allowed) != 0 {
		t.Errorf("the MCP bearer may go to: %+v", bearer)
	}
}

// Class 2 at the Files seam: every --config and --home the command line can mean
// is named, in every spelling, with the files the default run reads.
func TestFilesFollowEveryConfigAndHomeFlag(t *testing.T) {
	root := testenv.Isolate(t)
	defaultDir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	home1, home2 := filepath.Join(root, "h1"), filepath.Join(root, "h2")
	cfg1, cfg2 := filepath.Join(root, "c1.toml"), filepath.Join(root, "c2.toml")
	files := pathsUnder([]string{"--home", home1, "networks", "--home=" + home2, "--config", cfg1, "list", "--config=" + cfg2})
	for _, want := range []string{cfg1, cfg2, filepath.Join(home1, "config", "config.toml"), filepath.Join(home2, "config", "config.toml"), filepath.Join(defaultDir, "config.toml")} {
		if !slices.ContainsFunc(files, func(f string) bool {
			return filepath.Clean(f) == filepath.Clean(want) || strings.HasSuffix(want, filepath.Base(f)) && strings.Contains(f, filepath.Base(filepath.Dir(want)))
		}) {
			t.Errorf("Files does not name %s: %q", want, files)
		}
	}
	if restored, err := cliutil.ConfigDir(); err != nil || strings.Contains(restored, home2) {
		t.Errorf("a --home override survived Files: %q (%v)", restored, err)
	}
}

func heldNow(t *testing.T, v string) bool {
	t.Helper()
	err := credkit.WriteFile(filepath.Join(t.TempDir(), "probe"), []byte(v), 0o600)
	if err != nil && !errors.Is(err, credkit.ErrWithheld) {
		t.Fatal(err)
	}
	return errors.Is(err, credkit.ErrWithheld)
}

// specStringFields are the string-valued fields of internal/types, generated from
// the spec, and the string request parameters of the press manifest.
func specStringFields(t *testing.T) map[string]bool {
	t.Helper()
	seen := map[string]bool{}
	src, err := os.ReadFile(filepath.Join("..", "types", "types.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile("(?m)^\\s+\\w+\\s+string\\s+`json:\"([^\",]+)").FindAllStringSubmatch(string(src), -1) {
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
				Type string `json:"type"`
			} `json:"params"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, tool := range manifest.Tools {
		for _, p := range tool.Params {
			if p.Type == "string" {
				seen[p.Name] = true
			}
		}
	}
	if len(seen) < 200 {
		t.Fatalf("read only %d string fields from the spec; the scan is broken", len(seen))
	}
	return seen
}

// notInSpec are the declared names no Umbrella schema uses: the OAuth exchange's
// own members, and the same pairs under the names other Cisco endpoints use.
var notInSpec = []string{"access_token", "refresh_token", "client_id", "client_secret", "accessKeyId", "secretKey", "psk", "secret"}

// The declared list is bound to the spec in both directions: every string-valued
// field the one rule calls a credential is declared (bar the flag-shaped tool
// parameters, which CredentialFlags covers), and every declared name is a field
// of the spec or a named exception. A field a reprint's spec adds fails here.
func TestDeclaredFieldsMatchTheSpec(t *testing.T) {
	spec := specStringFields(t)
	for n := range spec {
		if !cliutil.SecretFieldName(n) || strings.Contains(n, "-") {
			continue
		}
		if !slices.Contains(Declarations.ResponseFields, n) {
			t.Errorf("the spec's string field %q is a credential by the rule and not in ResponseFields", n)
		}
	}
	for _, n := range Declarations.ResponseFields {
		if !spec[n] && !slices.Contains(notInSpec, n) {
			t.Errorf("%q is declared, and the spec has no such string field", n)
		}
	}
	if slices.Contains(Declarations.ResponseFields, "key") {
		t.Error("key is object-valued in the spec: declaring it would hold every scalar of an API key's record")
	}
	for _, n := range []string{"clientSecret", "clientId", "secret", "secretKey", "secretAccessKey", "accessKeyId", "currentKeyId", "oldKeyId", "psk", "password", "token", "access_token", "refresh_token", "client_secret"} {
		for name, list := range map[string][]string{"ResponseFields": Declarations.ResponseFields, "CredentialFields": Declarations.CredentialFields, "MCPArguments": Declarations.MCPArguments} {
			if !slices.Contains(list, n) {
				t.Errorf("%s does not declare %s", name, n)
			}
		}
	}
	for _, list := range [][]string{Declarations.CredentialFields, Declarations.MCPArguments} {
		if !slices.Contains(list, "key") {
			t.Error("an operator-supplied key is not declared")
		}
	}
	for _, id := range []string{"id", "keyId", "creatorKeyId", "keyName", "deviceKey", "imageKey", "originId", "organizationId"} {
		if slices.Contains(Declarations.ResponseFields, id) {
			t.Errorf("%s is an ordinary identifier: holding it would withhold every listing", id)
		}
	}
}
