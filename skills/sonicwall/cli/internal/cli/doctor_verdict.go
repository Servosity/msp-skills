// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: makes `doctor` report a verdict that tracks reality.
//
// The generated doctor called any HTTP response to `GET /` "reachable" and
// printed "present, not verified" for the credential, so a base_url aimed at a
// web UI (every API path 404s) and an expired NSM session token both rendered
// healthy. It also pointed a fresh install at `auth set-token`, when the NSM
// token is minted by `nsm-login` from the MySonicWall API key.
//
// GET /tenants is the probe: an argument-free NSM read that answers an invalid
// or idle-expired token with the in-band E_INVALIDAUTH envelope (observed on
// the wire 2026-10-13), which sonicwall_nsm_envelope.go turns into HTTP 401.
//
// See issue #282 and skills/sonicwall/handfixes.json.

package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"sonicwall-pp-cli/internal/client"
)

// doctorBaseURLEnv and doctorRegionEnv are named in remedies so the operator is
// told the fix. NSM SaaS is regional: a token minted on one region's NSM is
// rejected by another's, so a 401 cannot tell an expired token from the wrong
// region, and every negative verdict names both variables.
const (
	doctorBaseURLEnv = "SONICWALL_BASE_URL"
	doctorRegionEnv  = "SONICWALL_REGION"
)

const (
	doctorProbePath = "/tenants"
	doctorProbeCmd  = "tenants list"
)

// doctorRegionRemedy is appended to every verdict that leaves the NSM host in doubt.
const doctorRegionRemedy = "If your NSM portal is not nsm-uswest.sonicwall.com, set " + doctorRegionEnv + " (for example eucentral). " + doctorBaseURLEnv + " is only for test servers: it redirects MySonicWall calls too, so unset it for real use."

// doctorAuthHint replaces the generated `auth set-token` hint: the NSM token
// comes from the MySonicWall API key, not from a value the operator pastes.
const doctorAuthHint = "Set SONICWALL_MSW_API_KEY and run `sonicwall-cli nsm-login`; it mints the NSM session token and saves it (tokens last 8 hours, 15 minutes idle)."

// doctorProbeBodyHasData reports whether body is NSM's tenant list: a
// non-empty JSON array whose every element is an object with a non-empty
// string id (the vendor schema's required field). The one other accepted shape
// is a list envelope whose every field is an array and whose keys are all
// list names (data, items, results, tenants, content), which a stub or proxy
// in front of NSM may use. Anything else (an error object, {}, [], text) is
// not proof that the token was accepted.
func doctorProbeBodyHasData(body []byte) bool {
	var doc any
	if !decodeOneJSONValue(bytes.TrimSpace(stripJSONPrefixes(body)), &doc) {
		return false
	}
	switch t := doc.(type) {
	case []any:
		if len(t) == 0 {
			return false
		}
		for _, el := range t {
			obj, ok := el.(map[string]any)
			if !ok {
				return false
			}
			if id, ok := obj["id"].(string); !ok || strings.TrimSpace(id) == "" {
				return false
			}
		}
		return true
	case map[string]any:
		if len(t) == 0 {
			return false
		}
		for k, v := range t {
			if !doctorListEnvelopeKeys[strings.ToLower(k)] {
				return false
			}
			if _, ok := v.([]any); !ok {
				return false
			}
		}
		return true
	}
	return false
}

var doctorListEnvelopeKeys = map[string]bool{"data": true, "items": true, "results": true, "tenants": true, "content": true}

// doctorProbeCredentials issues ONE authenticated GET and writes the
// credentials verdict, correcting the api verdict when the probe proves it was
// optimistic. GetNoCache skips the response-cache read, so an expired token
// cannot be reported healthy from a cached 200.
func doctorProbeCredentials(ctx context.Context, c *client.Client, bin string, report map[string]any) {
	if c == nil || c.Config == nil || c.Config.AuthHeader() == "" {
		report["credentials"] = "ERROR not verified: there is no NSM session token to send. " + doctorAuthHint
		return
	}
	body, err := c.GetNoCache(ctx, doctorProbePath, nil)
	status := 0
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		status = apiErr.StatusCode
	}
	switch {
	case err == nil && doctorProbeBodyHasData(body):
		report["credentials"] = fmt.Sprintf("valid (NSM token verified with GET %s)", doctorProbePath)
	case err == nil:
		// A 2xx that is not NSM's tenant list (empty, {} or not JSON) proves
		// nothing about the token, so it is never reported valid.
		report["credentials"] = fmt.Sprintf("WARN not verified (GET %s answered without a tenant list) - an empty list, an error object or a non-NSM body does not confirm the token. %s", doctorProbePath, doctorRegionRemedy)
		report["api"] = "WARN the host answered but not like NSM's API. " + doctorRegionRemedy
	case status == 401:
		report["credentials"] = fmt.Sprintf("ERROR rejected (HTTP 401 from %s) - the NSM session token is invalid, expired or idle-timed-out. Run `%s nsm-login` again. %s", doctorProbePath, bin, doctorRegionRemedy)
		report["api"] = "WARN the NSM host answered but rejected the token, so this is not confirmed to be your tenant's NSM. " + doctorRegionRemedy
	case status == 403:
		report["credentials"] = fmt.Sprintf("WARN scope-limited (HTTP 403 from %s) - the token is accepted but cannot list tenants.", doctorProbePath)
	case status == 404:
		report["credentials"] = fmt.Sprintf("ERROR not verified (HTTP 404 from %s) - the endpoint does not exist at this base_url, so the token was never checked.", doctorProbePath)
		report["api"] = fmt.Sprintf("ERROR the host answered but %s is not there - base_url is not the NSM API root. Unset %s (it redirects MySonicWall calls too) and set %s to your NSM portal's region.", doctorProbePath, doctorBaseURLEnv, doctorRegionEnv)
	case status >= 500:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s) - NSM returned a server error, so the token was never checked.", status, doctorProbePath)
	case status > 0:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s). Run `%s %s` to see the full response.", status, doctorProbePath, bin, doctorProbeCmd)
	default:
		report["credentials"] = fmt.Sprintf("ERROR not verified: %s. %s", err, doctorRegionRemedy)
	}
}
