// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"autotask-pp-cli/internal/cliutil/testenv"
)

// config-file-holds-no-credential: a base URL that carries a userinfo
// password and a query credential (here from AUTOTASK_BASE_URL) is saved to
// config.toml without either; the host and path stay.
func TestSavedConfigFileKeepsNoCredential(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("AUTOTASK_BASE_URL", "https://ro:PersistPw-1c9e7a@webservices5.autotask.net/atservicesrest/V1.0?api_key="+"PersistKey-55b3")
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveBaseURL("https://ro:PersistPw-1c9e7a@webservices14.autotask.net/atservicesrest/V1.0?api_key=" + "PersistKey-55b3"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"PersistPw-1c9e7a", "PersistKey-55b3"} {
		if strings.Contains(string(got), v) {
			t.Errorf("config.toml holds %q:\n%s", v, got)
		}
	}
	if !strings.Contains(string(got), "webservices14.autotask.net/atservicesrest/V1.0") {
		t.Errorf("config.toml lost the base URL itself:\n%s", got)
	}
}

// r1-zone-save-keeps-credentials: zone's SaveBaseURL stores a config file's
// credentials in the credential store, then writes config.toml without them.
func TestZoneSaveKeepsConfigFileCredentials(t *testing.T) {
	testenv.Isolate(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := "api_integration_code = \"ZoneSaveCode-1q2w\"\npsa_user_name = \"zone-save@example.com\"\npsa_secret = \"ZoneSaveSecret-3e4r\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveBaseURL("https://webservices14.autotask.net/atservicesrest/V1.0"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if strings.Contains(string(got), "ZoneSaveSecret-3e4r") {
		t.Errorf("config.toml still holds the secret:\n%s", got)
	}
	again, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if again.AutotaskApiIntegrationCode != "ZoneSaveCode-1q2w" || again.AutotaskPsaUserName != "zone-save@example.com" || again.AutotaskPsaSecret != "ZoneSaveSecret-3e4r" {
		t.Errorf("credentials lost after the zone save: code %q user %q secret set %v", again.AutotaskApiIntegrationCode, again.AutotaskPsaUserName, again.AutotaskPsaSecret != "")
	}
	if !strings.Contains(again.BaseURL, "webservices14") {
		t.Errorf("base URL not saved: %q", again.BaseURL)
	}
}
