// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

// prtg handfix (saved-credential-bound-to-core): a credential on disk belongs to
// the core it was saved for. The base URL is the template
// https://{server}/api/v2, so an origin compared before {server} is filled in
// binds nothing, and PRTG_SERVER or PRTG_BASE_URL naming another client's core
// would receive this one's key. A credential in the environment is the operator's
// choice for this run and is not bound. The spec's placeholder host names no core:
// a key saved before a server was chosen records no binding.

import (
	"fmt"
	"strings"

	"prtg-pp-cli/internal/cliutil"
)

const placeholderServer = "prtg.example.com"

// withServer fills the base URL's {server}; with no server it stays a template,
// which matches no origin.
func withServer(base, server string) string {
	if server == "" {
		return base
	}
	return strings.ReplaceAll(base, "{server}", server)
}

// ownServer is the server this config records; the placeholder records none.
func (c *Config) ownServer() string {
	v := strings.TrimSpace(c.TemplateVars["server"])
	if strings.EqualFold(v, placeholderServer) {
		return ""
	}
	return normalizeEndpointTemplateValue(v)
}

// targetServer is the server this run sends to, the order Load resolves it in.
func (c *Config) targetServer() string {
	if v := strings.TrimSpace(cliutil.EnvOverride("PRTG_SERVER")); v != "" {
		return normalizeEndpointTemplateValue(v)
	}
	return strings.TrimSpace(c.TemplateVars["server"])
}

// refuseFileCredentialsElsewhere drops credentials a config file holds when the
// environment points this run at another core than that file names. A file naming
// no core records no binding, and stays usable.
func (c *Config) refuseFileCredentialsElsewhere() {
	if !c.hasCredentialFields() && len(c.Headers) == 0 {
		return
	}
	own := withServer(c.BaseURL, c.ownServer())
	if _, ok := cliutil.NormalizeCredentialOrigin(own); !ok {
		return
	}
	target := c.credentialTargetURL()
	if cliutil.CredentialOriginsMatch(own, target) {
		return
	}
	c.addCredentialRefusal(cliutil.CredentialRefusal{
		Source:             "config-kind path",
		Path:               c.Path,
		Err:                fmt.Errorf("credentials in this file are for %s and were not applied to %s", own, target),
		CredentialsPresent: true,
	})
	c.clearCredentialFields()
	c.Headers = nil // contract section 1: [headers] values are credentials too
}

// keepProfileCredential is the same rule for the credential store beside an
// explicit config: it is bound to the core it records, or with no record to the
// one its config file names, and a store and config naming neither stay usable.
func (c *Config) keepProfileCredential(creds *cliutil.Credentials, path string) bool {
	own := strings.TrimSpace(creds.BaseURL)
	if _, ok := cliutil.NormalizeCredentialOrigin(own); !ok {
		own = withServer(c.BaseURL, c.ownServer())
	}
	if _, ok := cliutil.NormalizeCredentialOrigin(own); !ok {
		return true
	}
	target := c.credentialTargetURL()
	if cliutil.CredentialOriginsMatch(own, target) {
		return true
	}
	c.addCredentialRefusal(cliutil.CredentialRefusal{
		Source:             "credentials file",
		Path:               path,
		Err:                fmt.Errorf("credentials saved for %s were not applied to %s", own, target),
		CredentialsPresent: true,
	})
	return false
}
