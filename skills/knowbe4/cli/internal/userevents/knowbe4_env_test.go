// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package userevents

import "testing"

// An MCPB host that skips the optional User Event API key leaves the literal
// ${user_config.*} template in the environment: it reads as unset, never as a
// key to send. KNOWBE4_USER_EVENT_BASE_URL must be a plain http(s) origin.
func TestEnvPlaceholdersReadAsUnset(t *testing.T) {
	t.Setenv(EnvKey, "${user_config.knowbe4_user_event_api_key}")
	t.Setenv(BaseURLEnv, "")
	if _, err := New(0); err == nil {
		t.Fatal("an unresolved user_config placeholder was accepted as the key")
	}
	t.Setenv(EnvKey, "ueKeyUnit-4b1d")
	t.Setenv("KNOWBE4_REGION", "${user_config.knowbe4_region}")
	c, err := New(0)
	if err != nil || c.BaseURL() != "https://api.events.knowbe4.com" {
		t.Fatalf("placeholder region: base %v, err %v", c, err)
	}
	for _, bad := range []string{"ftp://x.example", "https://u:p@x.example", "https://x.example/?k=v", "x.example", "https://"} {
		t.Setenv(BaseURLEnv, bad)
		if _, err := New(0); err == nil {
			t.Errorf("%s accepted as %s", bad, BaseURLEnv)
		}
	}
	t.Setenv(BaseURLEnv, "http://127.0.0.1:8080/")
	if c, err := New(0); err != nil || c.BaseURL() != "http://127.0.0.1:8080" {
		t.Fatalf("override: %v %v", c, err)
	}
}
