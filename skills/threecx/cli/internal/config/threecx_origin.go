// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

// threecx handfix (saved-credential-bound-to-phone-system): a credential on disk
// belongs to the phone system it was saved for. The base URL is the template
// https://{fqdn}/xapi/v1, so an origin compared before {fqdn} is filled in binds
// nothing, and THREECX_FQDN or THREECX_BASE_URL naming another client's phone
// system would receive this one's secret. A credential in the environment is the
// operator's choice for this run and is not bound.

import (
	"fmt"
	"os"
	"strings"

	"threecx-pp-cli/internal/cliutil"
)

// withFQDN fills the base URL's {fqdn}; with no FQDN it stays a template, which
// matches no origin.
func withFQDN(base, fqdn string) string {
	if fqdn == "" {
		return base
	}
	return strings.ReplaceAll(base, "{fqdn}", fqdn)
}

// targetFQDN is the FQDN this run sends to, the order Load resolves it in.
func (c *Config) targetFQDN() string {
	if v := strings.TrimSpace(cliutil.EnvOverride("THREECX_FQDN")); v != "" {
		return v
	}
	if os.Getenv("PRINTING_PRESS_VERIFY") == "1" {
		return "fqdn_placeholder"
	}
	return strings.TrimSpace(c.TemplateVars["fqdn"])
}

// refuseFileCredentialsElsewhere drops credentials a config file holds when the
// environment points this run at another phone system than that file names. A
// file naming no phone system records no binding, and stays usable.
func (c *Config) refuseFileCredentialsElsewhere() {
	if !c.hasCredentialFields() && len(c.Headers) == 0 {
		return
	}
	own := withFQDN(c.BaseURL, strings.TrimSpace(c.TemplateVars["fqdn"]))
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
// explicit config: it is bound to the phone system it records, or with no record
// to the one its config file names, and a store and config naming neither stay
// usable.
func (c *Config) keepProfileCredential(creds *cliutil.Credentials, path string) bool {
	own := strings.TrimSpace(creds.BaseURL)
	if _, ok := cliutil.NormalizeCredentialOrigin(own); !ok {
		own = withFQDN(c.BaseURL, strings.TrimSpace(c.TemplateVars["fqdn"]))
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
