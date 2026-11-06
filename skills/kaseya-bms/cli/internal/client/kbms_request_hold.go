// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"encoding/json"

	"kaseya-bms-pp-cli/internal/cliutil"
)

// holdCredentialEndpointRequest holds every value of a request to an endpoint
// whose path names a credential (cliutil.CredentialPath), under any name, but
// the allowlisted identifiers (kaseya-bms handfix
// credential-endpoint-request-held; Codex review r1 class holder incomplete):
// the BMS login form carries credentials under names that are data elsewhere
// in the API ("Code", the OAuth authorization code; "Code" is an account code
// or an error code in eight other schemas), so a name rule cannot hold them.
// Called with the request before it is encoded, ahead of a --dry-run preview
// or a send.
func holdCredentialEndpointRequest(path string, params map[string]string, body any) {
	if !cliutil.CredentialPath(path) {
		return
	}
	fields := map[string]any{}
	for k, v := range params {
		fields[k] = v
	}
	cliutil.HoldCredentialEndpointValues(fields)
	switch b := body.(type) {
	case nil:
	case multipartRequestBody:
		form := map[string]any{}
		for k, v := range b.Fields {
			form[k] = v
		}
		cliutil.HoldCredentialEndpointValues(form)
	default:
		raw, err := json.Marshal(body)
		if err != nil {
			return
		}
		var m map[string]any
		if json.Unmarshal(raw, &m) == nil {
			cliutil.HoldCredentialEndpointValues(m)
			return
		}
		var v any
		if json.Unmarshal(raw, &v) == nil {
			cliutil.HoldCredentialEndpointValues(map[string]any{"credential": v})
		}
	}
}
