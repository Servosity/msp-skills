// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// regionInFile returns the [template_vars] region config.toml holds.
func regionInFile(t *testing.T, path string) (string, bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		TemplateVars map[string]string `toml:"template_vars"`
	}
	if err := toml.Unmarshal(data, &f); err != nil {
		t.Fatalf("config.toml does not parse: %v\n%s", err, data)
	}
	v, ok := f.TemplateVars["region"]
	return v, ok
}

// hand-fix guard (handfixes.json region-never-persisted, adversarial round
// 4): every token save wrote the env or default region into config.toml, and
// Load honors a config-file region, so after one `ESET_PROTECT_REGION=us
// auth login` an unset variable meant us, not the documented eu default. A
// save may write back only the region the file itself held; the env still
// wins at load time.
func TestRegionNeverPersistedFromEnvOrDefault(t *testing.T) {
	for _, k := range []string{"ESET_PROTECT_USERNAME", "ESET_PROTECT_PASSWORD", "ESET_PROTECT_TOKEN", "ESET_PROTECT_REGION", "ESET_PROTECT_CONFIG", "ESET_PROTECT_BASE_URL"} {
		t.Setenv(k, "")
	}
	save := func(t *testing.T, cfg *Config) {
		t.Helper()
		if err := cfg.SaveTokens("eset:x:0000000000000000", "", "test-access", "", time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		// A second save in the same process (a refresh) must not either.
		if err := cfg.SaveTokens("eset:x:0000000000000000", "", "test-access-2", "", time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	load := func(t *testing.T, path string) *Config {
		t.Helper()
		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		return cfg
	}

	for _, env := range []string{"us", " CA ", "jpn"} {
		path := filepath.Join(t.TempDir(), "config.toml")
		t.Setenv("ESET_PROTECT_REGION", env)
		cfg := load(t, path)
		if cfg.TemplateVars["region"] != strings.ToLower(strings.TrimSpace(env)) || cfg.RegionSource != RegionSourceEnv {
			t.Fatalf("env region %q not applied: %q (%s)", env, cfg.TemplateVars["region"], cfg.RegionSource)
		}
		save(t, cfg)
		if v, ok := regionInFile(t, path); ok {
			t.Fatalf("ESET_PROTECT_REGION=%q was persisted to config.toml as %q", env, v)
		}
		t.Setenv("ESET_PROTECT_REGION", "")
		if cfg := load(t, path); cfg.TemplateVars["region"] != "eu" || cfg.RegionSource != "" || cfg.AccessToken != "test-access-2" {
			t.Fatalf("after a save under ESET_PROTECT_REGION=%q, an unset variable gives region %q from %q (token %q); want the eu default", env, cfg.TemplateVars["region"], cfg.RegionSource, cfg.AccessToken)
		}
	}

	// The default is never written either.
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg := load(t, path)
	save(t, cfg)
	if v, ok := regionInFile(t, path); ok {
		t.Fatalf("the default region was persisted as %q", v)
	}

	// A region the user wrote into config.toml is kept, honored without the
	// variable, overridden by it, and never replaced by it on save.
	path = filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("base_url = \"https://{region}.device-management.eset.systems\"\n\n[template_vars]\nregion = \"de\"\nother = \"kept\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg = load(t, path)
	if cfg.TemplateVars["region"] != "de" || cfg.RegionSource != RegionSourceFile {
		t.Fatalf("file region not honored: %q (%s)", cfg.TemplateVars["region"], cfg.RegionSource)
	}
	save(t, cfg)
	if v, _ := regionInFile(t, path); v != "de" {
		t.Fatalf("the file's own region became %q after a save", v)
	}
	t.Setenv("ESET_PROTECT_REGION", "us")
	cfg = load(t, path)
	if cfg.TemplateVars["region"] != "us" || cfg.RegionSource != RegionSourceEnv {
		t.Fatalf("env did not win over the file: %q (%s)", cfg.TemplateVars["region"], cfg.RegionSource)
	}
	save(t, cfg)
	if v, _ := regionInFile(t, path); v != "de" {
		t.Fatalf("ESET_PROTECT_REGION=us replaced the file's region: %q", v)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "kept") {
		t.Fatalf("another template var was dropped:\n%s", data)
	}
}
