// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import "testing"

func TestTokenSecret(t *testing.T) {
	for in, want := range map[string]string{
		"root@pam!monitor=0f3c9a51-8d2e-4b7a-9c61-5e2d8f4a7b13":             "0f3c9a51-8d2e-4b7a-9c61-5e2d8f4a7b13",
		"PVEAPIToken=root@pam!monitor=0f3c9a51-8d2e-4b7a-9c61-5e2d8f4a7b13": "0f3c9a51-8d2e-4b7a-9c61-5e2d8f4a7b13",
		"backup@pbs!ro:7d1e4c2a-5b9f-4e8d-a3c6-2f8b1d9e0a74":                "7d1e4c2a-5b9f-4e8d-a3c6-2f8b1d9e0a74",
		"pbsapitoken=backup@pbs!ro:7d1e4c2a-5b9f-4e8d-a3c6-2f8b1d9e0a74":    "7d1e4c2a-5b9f-4e8d-a3c6-2f8b1d9e0a74",
		"  root@pam!x=s3cr3t-value  ":                                       "s3cr3t-value",
		// Not a token's shape: held whole.
		"just-a-secret-value":   "just-a-secret-value",
		"root@pam!monitor":      "root@pam!monitor",
		"root@pam!monitor=":     "root@pam!monitor=",
		"!monitor=abc":          "!monitor=abc",
		"rootpam!monitor=abcde": "rootpam!monitor=abcde",
	} {
		if got := TokenSecret(in); got != want {
			t.Errorf("TokenSecret(%q) = %q, want %q", in, got, want)
		}
	}
}
