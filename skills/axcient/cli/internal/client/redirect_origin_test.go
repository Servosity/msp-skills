// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"net/http"
	"testing"
	"time"

	"axcient-pp-cli/internal/config"
)

// A redirect hop gets the API key only on the origin the request started
// from: same scheme and same host. An https -> http hop to the same host
// would send the key in clear text, so it is stripped like a cross-host hop.
func TestRedirectKeepsKeyOnlyOnSameOrigin(t *testing.T) {
	const key = "redirect-origin-key-7Qm2"
	c := New(&config.Config{BaseURL: "https://axapi.axcient.com", AxcientApiKey: key}, 5*time.Second, 0)
	start, _ := http.NewRequest("GET", "https://axapi.axcient.com/x360recover/v1/client", nil)
	for _, tc := range []struct {
		to   string
		want bool
	}{
		{"https://axapi.axcient.com/elsewhere", true},
		{"HTTPS://axapi.axcient.com/elsewhere", true},
		{"http://axapi.axcient.com/elsewhere", false},
		{"https://evil.example/elsewhere", false},
	} {
		hop, _ := http.NewRequest("GET", tc.to, nil)
		hop.Header.Set("X-Api-Key", key) // Go copies a custom header to every hop
		if err := c.HTTPClient.CheckRedirect(hop, []*http.Request{start}); err != nil {
			t.Fatalf("%s: %v", tc.to, err)
		}
		if got := hop.Header.Get("X-Api-Key") != ""; got != tc.want {
			t.Errorf("redirect to %s: key sent %v, want %v", tc.to, got, tc.want)
		}
	}
}
