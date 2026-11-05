// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored Synthesize auth helper; survives regeneration as a whole file.

package config

import (
	"encoding/base64"
	"strings"

	"gradient-pp-cli/internal/cliutil"
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
	// EnvOverride drops an unresolved .mcpb "${user_config.*}" placeholder, which
	// would otherwise be sent as a key.
	vendor := strings.TrimSpace(cliutil.EnvOverride(EnvVendorAPIKey))
	partner := strings.TrimSpace(cliutil.EnvOverride(EnvPartnerAPIKey))
	if vendor == "" || partner == "" {
		return "", ""
	}
	raw := vendor + ":" + partner
	token = base64.StdEncoding.EncodeToString([]byte(raw))
	// The header built from the pair is held the moment it is derived
	// (handfixes.json credkit-wiring). Both keys are held from startup
	// (Declarations.SecretEnv); the kit's decoders read the pair and its halves
	// out of the base64 header, so no derived part is held by hand.
	cliutil.Hold("minted:gradient-token", token)
	return token, "env:" + EnvVendorAPIKey + "+" + EnvPartnerAPIKey
}
