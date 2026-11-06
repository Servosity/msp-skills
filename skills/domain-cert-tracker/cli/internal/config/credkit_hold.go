// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json config-reads-held-before-parse).
// Every read of a config file, at startup or later (the MCP server reloads
// the file on every tool call), holds the file's values with the credential
// kit BEFORE the file is parsed. The kit's startup discovery holds the file
// it finds once; a file written after that is held here, when it is read.
// The readers are the kit's own (credkit.HoldBody, credkit.HoldURL): this
// file only says which values of a parsed config are credentials.

package config

import (
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"

	"domain-cert-tracker-pp-cli/internal/credkit"

	"github.com/pelletier/go-toml/v2"
)

// NotCredentials are the top-level keys whose values are not credentials.
// internal/credwire declares each as a credkit Identifier (config:<key>), so
// the kit exempts them too; a credential inside base_url (its password and
// query values) is still held.
var NotCredentials = []string{"base_url", "ca_cert", "tls_server_name"}

func notCredential(section, key string) bool {
	if section != "" {
		return false
	}
	for _, k := range NotCredentials {
		if key == k {
			return true
		}
	}
	return false
}

// holdConfigFile holds a config file's values: a file that does not parse goes
// to the kit's reader whole (it holds every token of bytes that are no
// document); a file that parses has each value held as the parser decoded it.
func holdConfigFile(data []byte) {
	var tree map[string]any
	if toml.Unmarshal(data, &tree) != nil {
		credkit.HoldBody(data)
		return
	}
	holdDecoded("", tree)
}

// HoldBaseURL holds the credentials inside a base URL with the kit's URL reader.
func HoldBaseURL(raw string) { credkit.HoldURL("userinfo", raw) }

// sourceFor is the kit source a value under key in section is held as:
// header:<name> for a [headers] entry, config:<key> for a top-level key,
// config:<section>.<key> else.
func sourceFor(section, key string) string {
	if section == "headers" {
		return "header:" + strings.ToLower(key)
	}
	if section == "" {
		return "config:" + key
	}
	return "config:" + section + "." + key
}

// holdDecoded holds each leaf of a parsed config as the parser decoded it.
func holdDecoded(section string, tree map[string]any) {
	for k, v := range tree {
		switch {
		case section == "" && k == "headers":
			if m, ok := v.(map[string]any); ok {
				for name, hv := range m {
					holdLeaf("header:"+strings.ToLower(name), hv)
				}
				continue
			}
		case notCredential(section, k):
			if s, ok := v.(string); ok {
				if k == "base_url" {
					HoldBaseURL(s)
				}
				continue
			}
		}
		if m, ok := v.(map[string]any); ok {
			holdDecoded(strings.TrimPrefix(section+"."+k, "."), m)
			continue
		}
		holdLeaf(sourceFor(section, k), v)
	}
}

func holdLeaf(source string, v any) {
	hold := func(s string) {
		if s = strings.TrimSpace(s); s != "" {
			credkit.Hold(credkit.Source(source), s)
		}
	}
	switch x := v.(type) {
	case string:
		hold(x)
	case int64:
		hold(strconv.FormatInt(x, 10))
	case float64:
		hold(strconv.FormatFloat(x, 'g', -1, 64))
	case bool:
	case []any:
		for _, e := range x {
			holdLeaf(source, e)
		}
	case map[string]any:
		for k, e := range x {
			holdLeaf(source+"."+k, e)
		}
	default:
		hold(fmt.Sprint(x))
	}
}

// CredentialLikeName reports a flag or JSON member name that names a credential.
func CredentialLikeName(name string) bool {
	name = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), "_", "-"))
	for _, marker := range []string{"token", "secret", "password", "credential", "api-key", "apikey", "authorization", "cookie", "private-key", "client-secret", "passwd", "pwd", "auth", "bearer", "session", "signature", "key"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

// HoldProfileStoreIn is HoldProfileStore on dir's profiles.json. Only a regular
// file is opened, so a pipe by that name cannot hang startup.
func HoldProfileStoreIn(dir string) {
	store := os.DirFS(dir)
	if info, err := fs.Stat(store, "profiles.json"); err != nil || !info.Mode().IsRegular() {
		return
	}
	if data, err := fs.ReadFile(store, "profiles.json"); err == nil {
		HoldProfileStore(data)
	}
}

// HoldProfileStore holds a run profile store (profiles.json) with the kit's body
// reader: the declared credential names in every duplicate, and every token of
// a store that does not parse. A value under a credential-named flag is never
// loaded (runProfileValueAllowed), so it is never applied or shown.
func HoldProfileStore(data []byte) { credkit.HoldBody(data) }
