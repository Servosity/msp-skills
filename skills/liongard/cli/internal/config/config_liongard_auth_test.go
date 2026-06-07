package config

import (
	"encoding/base64"
	"testing"
)

// Liongard's X-ROAR-API-KEY header is base64("accessKeyId:accessKeySecret").
// These tests pin the composed-credential wiring hand-added during generation:
// a pre-encoded LIONGARD_API_KEY wins; otherwise the ID+SECRET pair is composed.
func TestLoad_ComposedRoarAuth(t *testing.T) {
	// Isolate from any real config file / env on the host.
	t.Setenv("LIONGARD_CONFIG", "/nonexistent/liongard-config-test.toml")

	t.Run("composed from id+secret", func(t *testing.T) {
		t.Setenv("LIONGARD_API_KEY", "")
		t.Setenv("LIONGARD_ACCESS_KEY_ID", "abc-id")
		t.Setenv("LIONGARD_ACCESS_KEY_SECRET", "shh-secret")

		cfg, err := Load("")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		got := cfg.AuthHeader()
		want := base64.StdEncoding.EncodeToString([]byte("abc-id:shh-secret"))
		if got != want {
			t.Fatalf("AuthHeader = %q, want composed %q", got, want)
		}
		// The composed value must decode back to id:secret.
		dec, derr := base64.StdEncoding.DecodeString(got)
		if derr != nil || string(dec) != "abc-id:shh-secret" {
			t.Fatalf("decoded header = %q (err %v), want %q", dec, derr, "abc-id:shh-secret")
		}
		if cfg.AuthSource != "env:LIONGARD_ACCESS_KEY_ID+SECRET" {
			t.Fatalf("AuthSource = %q, want env:LIONGARD_ACCESS_KEY_ID+SECRET", cfg.AuthSource)
		}
	})

	t.Run("pre-encoded api key wins over id+secret", func(t *testing.T) {
		t.Setenv("LIONGARD_API_KEY", "already-base64==")
		t.Setenv("LIONGARD_ACCESS_KEY_ID", "abc-id")
		t.Setenv("LIONGARD_ACCESS_KEY_SECRET", "shh-secret")

		cfg, err := Load("")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got := cfg.AuthHeader(); got != "already-base64==" {
			t.Fatalf("AuthHeader = %q, want pre-encoded value", got)
		}
		if cfg.AuthSource != "env:LIONGARD_API_KEY" {
			t.Fatalf("AuthSource = %q, want env:LIONGARD_API_KEY", cfg.AuthSource)
		}
	})

	t.Run("no credentials yields empty header", func(t *testing.T) {
		t.Setenv("LIONGARD_API_KEY", "")
		t.Setenv("LIONGARD_ACCESS_KEY_ID", "")
		t.Setenv("LIONGARD_ACCESS_KEY_SECRET", "")

		cfg, err := Load("")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got := cfg.AuthHeader(); got != "" {
			t.Fatalf("AuthHeader = %q, want empty", got)
		}
	})

	t.Run("instance resolves into base URL template var", func(t *testing.T) {
		t.Setenv("LIONGARD_INSTANCE", "us1")
		cfg, err := Load("")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.TemplateVars["instance"] != "us1" {
			t.Fatalf("TemplateVars[instance] = %q, want us1", cfg.TemplateVars["instance"])
		}
	})
}
