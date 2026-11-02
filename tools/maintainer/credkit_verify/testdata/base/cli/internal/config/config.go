// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Auth struct {
	RefreshToken string `toml:"refresh_token" json:"refresh_token"`
}

type Config struct {
	BaseURL      string            `toml:"base_url" json:"base_url"`
	Token        string            `toml:"token" json:"token"`
	ClientSecret string            `toml:"client_secret" json:"client_secret"`
	Profile      string            `toml:"profile" json:"profile"`
	Headers      map[string]string `toml:"headers" json:"headers"`
	Path         string            `toml:"-" json:"-"`
	Timeout      int               `toml:"timeout" json:"timeout"`
	Auth
}

func Path() string { return filepath.Join(os.TempDir(), "acme.toml") }

func Load() (*Config, error) {
	b, err := os.ReadFile(Path())
	c := Config{Path: Path()}
	if err == nil {
		err = json.Unmarshal(b, &c)
	}
	return &c, err
}
