// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored Synthesize auth helper; survives regeneration as a whole file.

package config

import (
	"encoding/base64"
	"os"
)

// Env var names for the Synthesize API key pair. The official PowerShell SDK
// calls these VENDOR_API_KEY / PARTNER_API_KEY; this CLI namespaces them to
// avoid collisions with other vendor CLIs on the same machine.
const (
	EnvVendorAPIKey  = "GRADIENT_VENDOR_API_KEY"  // #nosec G101 -- env var NAME, not a credential value
	EnvPartnerAPIKey = "GRADIENT_PARTNER_API_KEY" // #nosec G101 -- env var NAME, not a credential value
)

// DeriveGradientToken builds the GRADIENT-TOKEN header value from the
// Synthesize key pair: base64("<vendorApiKey>:<partnerApiKey>"), exactly as
// the official SDK's BuildGradientToken helper does. Returns the token and an
// auth-source label, or empty strings when either key is unset.
func DeriveGradientToken() (token, source string) {
	vendor := os.Getenv(EnvVendorAPIKey)
	partner := os.Getenv(EnvPartnerAPIKey)
	if vendor == "" || partner == "" {
		return "", ""
	}
	raw := vendor + ":" + partner
	return base64.StdEncoding.EncodeToString([]byte(raw)), "env:" + EnvVendorAPIKey + "+" + EnvPartnerAPIKey
}
