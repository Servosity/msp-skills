// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/url"

	"knowbe4-pp-cli/internal/config"
	"knowbe4-pp-cli/internal/userevents"
)

// newUserEventsClient builds the User Event API client (the second credential,
// KNOWBE4_USER_EVENT_API_KEY) behind the same guard as every Reporting API
// client (knowbe4 handfix user-event-origin-pinned): the key goes only to the
// scheme, host and port of the region's User Event API host
// (https://api.events.knowbe4.com, or https://api-<region>.events.knowbe4.com),
// every redirect off that origin is refused before it is followed, and a
// client built under --dry-run sends nothing. A request that repeats the
// Reporting API key whole in any part is refused (refuseOtherAPICredentials);
// the credential kit's outbound guard under it refuses a piece or an encoding
// of any key off its declared origin (credwire).
func newUserEventsClient(flags *rootFlags) (*userevents.Client, error) {
	c, err := userevents.New(flags.timeout)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(flags.configPath)
	if err != nil {
		return nil, configErr(err)
	}
	// The Reporting client's CA trust (--ca-cert, KNOWBE4_CA_CERT, ca_cert),
	// applied before the pin wraps the transport.
	flags.applyTLSFlags(cfg)
	if err := c.TrustCA(cfg.CACert); err != nil {
		return nil, configErr(err)
	}
	pin := ""
	if u, perr := url.Parse(c.BaseURL()); perr == nil {
		pin = normalizedOrigin(u)
	}
	installPin(c.HTTP, pin, flags.dryRun)
	// r1-credential-to-other-api-host: the Reporting key, from whichever
	// source the configuration reads it, never reaches the User Event host.
	refuseOtherAPICredentials(c.HTTP, otherThan(config.ReportingCredentials(cfg), userEventCredentials()))
	return c, nil
}
