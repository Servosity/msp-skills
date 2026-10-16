// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jira-service-management-pp-cli/internal/cliutil"
	"jira-service-management-pp-cli/internal/cliutil/testenv"
)

// cmdb discover --save writes cloudId/workspaceId into the config file and
// must never rewrite the credentials file or persist env overrides.
func TestSaveTemplateVarsLeavesCredentialsAlone(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("JIRA_SERVICE_MANAGEMENT_BASE_URL", "http://127.0.0.1:1/override")
	t.Setenv("JIRA_SERVICE_MANAGEMENT_SITE", "acme")
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	credsPath, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(credsPath), 0o700); err != nil {
		t.Fatal(err)
	}
	creds := []byte("service_management_email = 'a@b.c'\nservice_management_api_token = 'tok'\n")
	if err := os.WriteFile(credsPath, creds, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTemplateVars(map[string]string{"cloudId": "c-1", "workspaceId": "w-1"}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(credsPath)
	if string(after) != string(creds) {
		t.Fatalf("credentials file rewritten:\n%s", after)
	}
	written, _ := os.ReadFile(cfgPath)
	w := string(written)
	if !strings.Contains(w, "c-1") || !strings.Contains(w, "w-1") {
		t.Fatalf("template vars not saved: %s", w)
	}
	if strings.Contains(w, "override") || strings.Contains(w, "acme") || strings.Contains(w, "tok") {
		t.Fatalf("env overrides or credentials leaked into config: %s", w)
	}
}

// An explicit --config file with inline credentials must not be rewritten:
// migrating them would overwrite the default tenant's credentials file.
func TestSaveTemplateVarsRefusesInlineCredentials(t *testing.T) {
	testenv.Isolate(t)
	credsPath, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(credsPath), 0o700); err != nil {
		t.Fatal(err)
	}
	tenantA := []byte("service_management_email = 'a@a.test'\nservice_management_api_token = 'tok-a'\n")
	if err := os.WriteFile(credsPath, tenantA, 0o600); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(t.TempDir(), "tenant-b.toml")
	tenantB := []byte("service_management_email = 'b@b.test'\nservice_management_api_token = 'tok-b'\n")
	if err := os.WriteFile(cfgPath, tenantB, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTemplateVars(map[string]string{"cloudId": "c-1"}); err == nil {
		t.Fatal("want an error for a config with inline credentials")
	}
	if got, _ := os.ReadFile(credsPath); string(got) != string(tenantA) {
		t.Fatalf("default credentials rewritten: %s", got)
	}
	if got, _ := os.ReadFile(cfgPath); string(got) != string(tenantB) {
		t.Fatalf("tenant config rewritten: %s", got)
	}
}
