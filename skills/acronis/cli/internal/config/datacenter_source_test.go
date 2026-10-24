// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDatacenterChosenIgnoresTheBuiltInDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ACRONIS_DATACENTER", "")
	t.Setenv("ACRONIS_BASE_URL", "")
	empty := filepath.Join(dir, "empty.toml")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(empty)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TemplateVars["datacenter"] != "eu2-cloud" {
		t.Fatalf("expected the eu2-cloud fill-in, got %q", cfg.TemplateVars["datacenter"])
	}
	if cfg.DatacenterChosen() {
		t.Fatal("the built-in eu2-cloud fill-in counted as an operator choice")
	}

	chosen := filepath.Join(dir, "chosen.toml")
	if err := os.WriteFile(chosen, []byte("[template_vars]\ndatacenter = \"us-cloud\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if cfg, err = Load(chosen); err != nil {
		t.Fatal(err)
	}
	if !cfg.DatacenterChosen() {
		t.Fatal("template_vars.datacenter in the config file was not counted as a choice")
	}

	t.Setenv("ACRONIS_DATACENTER", "au-cloud")
	if cfg, err = Load(empty); err != nil {
		t.Fatal(err)
	}
	if !cfg.DatacenterChosen() {
		t.Fatal("ACRONIS_DATACENTER was not counted as a choice")
	}
}
