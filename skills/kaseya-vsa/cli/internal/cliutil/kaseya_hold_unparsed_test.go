// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"strings"
	"testing"
)

func TestHoldUnparsedValuesHoldsEverySecretShape(t *testing.T) {
	const secret = "Zq7Unparsed9SecretValue42"
	shapes := []string{
		"token_secret = " + secret,
		secret,
		"token_secret = \"" + secret + "\"\ntoken_secret = \"" + secret + "\"",
		"[auth]\ntoken_secret = '" + secret + "\n",
		"x = 1\ntoken_secret = [\"" + secret + "\", ]]",
		strings.Repeat("pad = \"filler-value-0001\"\n", 5000) + "token_secret = \"" + secret + "\" = broken",
	}
	for i, body := range shapes {
		HoldUnparsedValues([]byte(body))
		if out := FunnelHeld("upstream said: " + secret); strings.Contains(out, secret) {
			t.Fatalf("shape %d: secret survived the funnel after HoldUnparsedValues: %q", i, out)
		}
	}
}
