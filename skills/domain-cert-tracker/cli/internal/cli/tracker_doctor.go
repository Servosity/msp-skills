// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"domain-cert-tracker-pp-cli/internal/client"
)

// rdapDoctorVerdict replaces the generated doctor's API and credential rows.
// This connector has no account or key, so a reachable homepage proves
// nothing: base_url is healthy only when it answers an RDAP help query
// (RFC 9082 section 3.1.6) with an RDAP JSON object. Every other answer is
// reported as a negative verdict that names what was wrong.
func rdapDoctorVerdict(ctx context.Context, c *client.Client) (api, credentials string) {
	body, err := c.GetNoCache(ctx, "/help", nil)
	var apiErr *client.APIError
	switch {
	case err == nil:
		if !isRDAPResponse(body) {
			return "invalid: base_url answered /help without an RDAP JSON object; point it at an RDAP server such as https://rdap.org",
				"skipped (base_url is not an RDAP server)"
		}
		return "reachable (RDAP /help answered)", "none needed: RDAP, CT, DNS and TLS lookups are public"
	case errors.As(err, &apiErr) && (apiErr.StatusCode == 401 || apiErr.StatusCode == 403):
		return fmt.Sprintf("refused: base_url answered HTTP %d at /help; point it at a public RDAP server such as https://rdap.org", apiErr.StatusCode),
			fmt.Sprintf("rejected: base_url wants credentials (HTTP %d); this tool needs none from a public RDAP server", apiErr.StatusCode)
	case errors.As(err, &apiErr):
		return fmt.Sprintf("invalid: base_url answered HTTP %d at /help, so it is not an RDAP server", apiErr.StatusCode),
			"skipped (base_url is not an RDAP server)"
	default:
		return fmt.Sprintf("unreachable: %s", err), "skipped (API unreachable)"
	}
}

// isRDAPResponse reports whether body is a non-error JSON object answer:
// null, arrays, {} and an object carrying an error (RFC 9083 section 6
// errorCode, or an "error" key) are not. rdapConformance (RFC 9083 section
// 4.1), when present, must be a non-empty array of non-empty strings.
func isRDAPResponse(body []byte) bool {
	var obj map[string]json.RawMessage
	if json.Unmarshal(body, &obj) != nil || len(obj) == 0 {
		return false
	}
	for k := range obj {
		switch strings.ToLower(k) {
		case "errorcode", "error", "errors":
			return false
		}
	}
	raw, ok := obj["rdapConformance"]
	if !ok {
		return true
	}
	var conf []*string
	if json.Unmarshal(raw, &conf) != nil || len(conf) == 0 {
		return false
	}
	for _, v := range conf {
		if v == nil || strings.TrimSpace(*v) == "" {
			return false
		}
	}
	return true
}
