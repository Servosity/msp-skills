// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-read-only-resolved-path-refusal" entry.

package redact

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"testing"

	"dnsfilter-pp-cli/internal/store"
)

// TestIsCredentialRequestPath: every spelling a caller-supplied placeholder can
// give a credential segment is caught on the resolved path; ordinary paths
// (including ids that merely contain the words) are not.
func TestIsCredentialRequestPath(t *testing.T) {
	hit := []string{
		"/v1/user_agents/uninstall_pin",
		"/v1/user_agents/UNINSTALL_PIN",
		"/v1/user_agents/uninstall_pin/",
		"/v1/user_agents//uninstall_pin",
		"/v1/user_agents/uninstall_pin%2F",
		"/v1/user_agents/uninstall%5Fpin",
		"/v1/user_agents/uninstall%255Fpin",
		"/v1/user_agents/uninstall%5Fpin%zz",
		"/v1/user_agents/uninstall_pin.json",
		"/v1/user_agents/uninstall_pin;v=1",
		"/v1/user_agents/uninstall-pin",
		"/v1/user_agents/..%2Funinstall_pin",
		"/v1/users/..%2Fapi_keys",
		"/v1/users/..%5Capi_keys",
		"/v1/networks/7%2Fsecret_key",
		"/v1/networks/7/Secret-Key?x=1",
		"/v1/psa_integrations/redirect_link",
		"/v1/user_agents/uninstall_pin%3Fx%3D1",
		"/v1/user_agents/%20uninstall_pin%20",
		"/v1/user_agents/uninstall-pin.json",
		"/v1/user_agents/uninstall_pin.io",
		"/v1/api_keys.json",
		"/uninstall-pin.json",
	}
	for _, p := range hit {
		if !IsCredentialRequestPath(p) {
			t.Errorf("credential path not caught: %s", p)
		}
	}
	miss := []string{
		"/v1/networks/7",
		"/v1/networks/all?search=secret_key",
		"/v1/user_agents/all",
		"/v1/user_agents/12345",
		"/v1/user_agents/uninstall_pinned",
		"/v1/policies/9/add_whitelist_domain",
		"/v1/api_keys_report",
		"/secret-key.io",
		"/api-keys.example.com",
		"/v1/notes/policies/7/redirect-link.example.org",
		"/v1/notes/policies/7/uninstall-pin.corp.local",
	}
	for _, p := range miss {
		if IsCredentialRequestPath(p) {
			t.Errorf("ordinary path refused: %s", p)
		}
	}
}

// TestCredentialPathSegmentsMatchResponseCache keeps one set: the response
// cache in internal/client refuses to store exactly these segments.
func TestCredentialPathSegmentsMatchResponseCache(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "client", "client.go"))
	if err != nil {
		t.Fatal(err)
	}
	fn := regexp.MustCompile(`(?s)func responseCacheCredentialPath\(path string\) bool \{.*?case ([^:]+):`).FindSubmatch(src)
	if fn == nil {
		t.Fatal("responseCacheCredentialPath case list not found in internal/client/client.go")
	}
	var cache []string
	for _, m := range regexp.MustCompile(`"([^"]+)"`).FindAllSubmatch(fn[1], -1) {
		cache = append(cache, string(m[1]))
	}
	var ours []string
	for k := range CredentialPathSegments {
		ours = append(ours, k)
	}
	sort.Strings(cache)
	sort.Strings(ours)
	if !reflect.DeepEqual(cache, ours) {
		t.Fatalf("credential path sets drifted: response cache %v, MCP %v", cache, ours)
	}
}

// TestCredentialKeyRuleIsTheStoreRule: the MCP funnel and the local mirror
// share one key rule, and it withholds a PIN (the roaming client uninstall
// PIN is a credential).
func TestCredentialKeyRuleIsTheStoreRule(t *testing.T) {
	corpus := []string{"secret_key", "client_secret", "token", "access_token", "payment_token", "password", "reset_password_url",
		"api_key", "X-Api-Key", "credentials", "uninstall_pin", "pin", "PIN", "name", "id", "secret_key_id", "token_type",
		"has_secret_key", "last_four", "pinned", "spin_count", "pin_code_enabled", "organization_id", "ep-grd-subscriber-secret"}
	for _, k := range corpus {
		if store.IsSecretKey(k) != IsCredentialKey(k) {
			t.Errorf("%q: store=%v MCP=%v; the rules must be one definition", k, store.IsSecretKey(k), IsCredentialKey(k))
		}
	}
	for _, k := range []string{"uninstall_pin", "pin", "secret_key", "token"} {
		if !IsCredentialKey(k) {
			t.Errorf("%q must be withheld", k)
		}
	}
	for _, k := range []string{"name", "id", "last_four", "pinned", "organization_id"} {
		if IsCredentialKey(k) {
			t.Errorf("%q must not be withheld", k)
		}
	}
	if WithheldFieldPlaceholder != store.WithheldValuePlaceholder {
		t.Fatal("placeholder text must be the store's")
	}
}
