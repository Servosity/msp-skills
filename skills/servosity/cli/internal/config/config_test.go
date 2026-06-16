// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package config

import "testing"

// TestAuthHeader_TokenScheme guards the fix for issue #78: the Servosity API
// authenticates the MSP partner token via Django REST Framework's
// TokenAuthentication, which requires the "Token " scheme on the Authorization
// header. A bare token value is rejected with HTTP 403. AuthHeader() must
// prepend the scheme, while tolerating a value that already carries a
// recognized scheme (the historical SERVOSITY_MSP_TOKEN="Token <t>" workaround).
func TestAuthHeader_TokenScheme(t *testing.T) {
	cases := []struct {
		name      string
		cfg       Config
		want      string
	}{
		{
			name: "bare token gets the Token scheme",
			cfg:  Config{ServosityMspToken: "abc123"},
			want: "Token abc123",
		},
		{
			name: "value already carrying Token scheme is not double-prefixed",
			cfg:  Config{ServosityMspToken: "Token abc123"},
			want: "Token abc123",
		},
		{
			name: "lowercase token scheme is normalized to canonical Token",
			cfg:  Config{ServosityMspToken: "token abc123"},
			want: "Token abc123",
		},
		{
			name: "mistaken Bearer scheme is normalized to the DRF Token scheme",
			cfg:  Config{ServosityMspToken: "Bearer abc123"},
			want: "Token abc123",
		},
		{
			name: "empty token yields an empty header",
			cfg:  Config{ServosityMspToken: ""},
			want: "",
		},
		{
			name: "explicit AuthHeaderVal override is returned verbatim",
			cfg:  Config{AuthHeaderVal: "Token override", ServosityMspToken: "abc123"},
			want: "Token override",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.AuthHeader(); got != tc.want {
				t.Fatalf("AuthHeader() = %q, want %q", got, tc.want)
			}
		})
	}
}
