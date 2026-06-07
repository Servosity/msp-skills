// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"encoding/base64"
	"testing"
)

func TestDeriveGradientToken(t *testing.T) {
	cases := []struct {
		name       string
		vendor     string
		partner    string
		wantToken  string
		wantSource string
	}{
		{
			name:       "both keys set",
			vendor:     "vk_123",
			partner:    "pk_456",
			wantToken:  base64.StdEncoding.EncodeToString([]byte("vk_123:pk_456")),
			wantSource: "env:GRADIENT_VENDOR_API_KEY+GRADIENT_PARTNER_API_KEY",
		},
		{name: "missing vendor", vendor: "", partner: "pk_456", wantToken: "", wantSource: ""},
		{name: "missing partner", vendor: "vk_123", partner: "", wantToken: "", wantSource: ""},
		{name: "both missing", vendor: "", partner: "", wantToken: "", wantSource: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvVendorAPIKey, tc.vendor)
			t.Setenv(EnvPartnerAPIKey, tc.partner)
			token, source := DeriveGradientToken()
			if token != tc.wantToken {
				t.Errorf("token = %q, want %q", token, tc.wantToken)
			}
			if source != tc.wantSource {
				t.Errorf("source = %q, want %q", source, tc.wantSource)
			}
		})
	}
}

func TestLoadDerivesTokenFromKeyPair(t *testing.T) {
	t.Setenv("GRADIENT_TOKEN", "")
	t.Setenv(EnvVendorAPIKey, "vk_abc")
	t.Setenv(EnvPartnerAPIKey, "pk_def")
	t.Setenv("GRADIENT_CONFIG", t.TempDir()+"/nonexistent.toml")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := base64.StdEncoding.EncodeToString([]byte("vk_abc:pk_def"))
	if cfg.GradientToken != want {
		t.Errorf("GradientToken = %q, want derived %q", cfg.GradientToken, want)
	}
	if cfg.AuthSource != "env:GRADIENT_VENDOR_API_KEY+GRADIENT_PARTNER_API_KEY" {
		t.Errorf("AuthSource = %q", cfg.AuthSource)
	}
}

func TestLoadEnvTokenBeatsKeyPair(t *testing.T) {
	t.Setenv("GRADIENT_TOKEN", "explicit-token")
	t.Setenv(EnvVendorAPIKey, "vk_abc")
	t.Setenv(EnvPartnerAPIKey, "pk_def")
	t.Setenv("GRADIENT_CONFIG", t.TempDir()+"/nonexistent.toml")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GradientToken != "explicit-token" {
		t.Errorf("GradientToken = %q, want explicit env token to win", cfg.GradientToken)
	}
}
