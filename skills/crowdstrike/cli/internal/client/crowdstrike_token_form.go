// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import "net/url"

// ClientCredentialsTokenForm is the one client_credentials token request body
// for every mint path: the automatic mint, `auth login` and the fleet
// member_cid mint. CrowdStrike's /oauth2/token takes client_id and
// client_secret as form fields (FalconPy's oauth2.token sends them in the
// body, as did 0.1.10); 4.34.2 renders HTTP Basic, which the vendor does not
// document. member_cid, when set, mints a Flight Control token scoped to that
// child CID. crowdstrike handfix: token-request-body-credentials.
func ClientCredentialsTokenForm(clientID, clientSecret, memberCID string) url.Values {
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	}
	if memberCID != "" {
		form.Set("member_cid", memberCID)
	}
	if scope := resolveClientCredentialsScope(); scope != "" {
		form.Set("scope", scope)
	}
	return form
}
