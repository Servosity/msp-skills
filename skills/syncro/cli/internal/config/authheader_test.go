// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package config

import "testing"

// TestAuthHeaderBearerPrefix pins the live-auth contract: Syncro expects
// "Authorization: Bearer <token>". The generator emits a bare-token
// AuthHeader for this securityScheme shape (type apiKey, in header, name
// Authorization), which fails against every real tenant. This test exists so
// a regen that drops the hand-applied prefix guard fails loudly instead of
// shipping silently-broken auth. See the sentinelone reprint for the same
// pattern (x-auth-value-prefix is not consumed by the generator).
func TestAuthHeaderBearerPrefix(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "bare token gains Bearer prefix",
			cfg:  Config{SyncroApiKey: "T123abc"},
			want: "Bearer T123abc",
		},
		{
			name: "already-prefixed token is not double-prefixed",
			cfg:  Config{SyncroApiKey: "Bearer T123abc"},
			want: "Bearer T123abc",
		},
		{
			name: "explicit auth_header value wins verbatim",
			cfg:  Config{AuthHeaderVal: "Bearer override", SyncroApiKey: "ignored"},
			want: "Bearer override",
		},
		{
			name: "empty credential yields empty header",
			cfg:  Config{},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.AuthHeader(); got != tt.want {
				t.Fatalf("AuthHeader() = %q, want %q", got, tt.want)
			}
		})
	}
}
