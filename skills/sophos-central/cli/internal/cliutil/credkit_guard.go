// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring).

package cliutil

import (
	"net/url"

	"sophos-central-pp-cli/internal/credkit"
)

// OutboundGuard is the credential kit's outbound guard, around every transport
// the API client (internal/client), the token client, the --deliver webhook
// and the Spacefast client send through. The probe and feedback clients call
// credkit.Transport directly.
// Production code never reassigns it. In-process unit tests do, to the
// identity: they never run credkit.Start, so the kit has no declarations and no
// base origin and would refuse every credentialed request to a loopback test
// server. The built binaries' leak tests (internal/credwire) prove the guard
// itself.
var OutboundGuard = credkit.Transport

// Hold is credkit.Hold. Production code never reassigns it. In-process unit
// tests do, to a no-op: a held value lasts for the process and the kit
// withholds whatever carries it, so one test's fixture would withhold the
// next test's output.
var Hold = credkit.Hold

// TokenForm encodes a token request form with grant_type first. The kit also
// reads a form body as one key=value line, so a body that starts with a
// credential field (url.Values.Encode sorts client_id first) held everything
// after it, "grant_type=client_credentials" included. A piece of
// "credentials" then withheld every output that said the word, the kit's own
// notice ("it carried a credential") too: doctor on environment credentials
// printed nothing and exited 74. Each field is still held by its own name.
// ponytail: a dodge of a kit reading, drop it once the kit stops holding a
// form's tail under its first key.
func TokenForm(form url.Values) string {
	rest := url.Values{}
	for k, v := range form {
		if k != "grant_type" {
			rest[k] = v
		}
	}
	gt, ok := form["grant_type"]
	if !ok {
		return rest.Encode()
	}
	out := url.Values{"grant_type": gt}.Encode()
	if r := rest.Encode(); r != "" {
		out += "&" + r
	}
	return out
}
