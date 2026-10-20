// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: makes `doctor` report a verdict that tracks reality.
//
// The generated doctor called any HTTP response to `GET /` "reachable" and
// printed "present, not verified" for the credential, so a base_url aimed at a
// web UI (every API path 404s) and a rejected key both rendered healthy. It
// also called the v3 key "required" when either key alone powers its own
// commands (see HAND-FIX(manifest-key-optional)).
//
// Probes: GET /clients/health?page_size=1 with the v3 (ScalePad) key, an
// argument-free read that answers with a {data:[...], total_count, next_cursor}
// envelope; GET /backups/overview with the v2 (Backup Radar) key, an
// argument-free read that answers with integer counts. Each key is probed only
// when it is configured, and the credentials verdict is positive only when a
// probe returned the vendor's own shape.
//
// See msp-skills issue #282 and skills/backup-radar/handfixes.json.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"backup-radar-pp-cli/internal/client"
	"backup-radar-pp-cli/internal/cliutil"
)

const (
	doctorV3Env       = "BACKUP_RADAR_API_KEY"
	doctorV2Env       = "BACKUP_RADAR_V2_API_KEY"
	doctorBaseURLEnv  = "BACKUP_RADAR_BASE_URL"
	doctorV3ProbePath = "/clients/health"
	doctorV3ProbeCmd  = "health list --page-size 1"
	doctorV2ProbePath = "/backups/overview"
	doctorV2ProbeCmd  = "backups overview"
)

// doctorAuthHint replaces the generated single-key hint: either key alone is
// enough for its own commands.
const doctorAuthHint = "Set " + doctorV3Env + " (ScalePad API key: v3 health, devices, ingest, retire, push) and/or " + doctorV2Env + " (Backup Radar API v2 key: backups list, get, results, filters, inactive, retired, overview). Either key alone powers its own commands; `backup-radar-cli auth set-token` saves the v3 key."

// doctorBaseURLRemedy is appended to every verdict that leaves the v3 host in doubt.
const doctorBaseURLRemedy = "The v3 API root is https://api.scalepad.com/backup-radar/v3 (US) or https://eu.api.scalepad.com/backup-radar/v3 (EU); set " + doctorBaseURLEnv + " or base_url in config.toml to one of them."

// doctorDecodeObject decodes exactly one non-empty JSON object; anything
// else (an array, text, trailing data, {}) is not a vendor envelope.
func doctorDecodeObject(body []byte) (map[string]any, bool) {
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(body)))
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil || len(doc) == 0 {
		return nil, false
	}
	if dec.More() {
		return nil, false
	}
	return doc, true
}

// doctorV3BodyHasData reports whether body is the v3 client-health envelope:
// a JSON object whose data field is an array (empty is fine: a tenant with no
// clients still proves the key). Every other shape (an error object, {}, [],
// text) is not proof that the key was accepted.
func doctorV3BodyHasData(body []byte) bool {
	doc, ok := doctorDecodeObject(body)
	if !ok {
		return false
	}
	for _, k := range []string{"data", "items", "results"} {
		if v, ok := doc[k]; ok {
			if _, isList := v.([]any); isList {
				return true
			}
		}
	}
	return false
}

// doctorV2BodyHasCounts reports whether body is the v2 overview: a JSON
// object with a numeric backups count (the vendor schema's fields are all
// integers; a stub's list envelope of arrays also counts).
func doctorV2BodyHasCounts(body []byte) bool {
	doc, ok := doctorDecodeObject(body)
	if !ok {
		return false
	}
	for _, k := range []string{"backups", "activePolicies", "workstations", "office365"} {
		if _, ok := doc[k].(float64); ok {
			return true
		}
	}
	return doctorV3BodyHasData(body)
}

// doctorProbeKeys issues ONE authenticated GET per configured key and writes
// the credentials verdict (and credentials_v2 when the v2 key is set),
// correcting the api verdict when the probe proves it was optimistic.
// GetNoCache skips the response-cache read, so a rejected key cannot be
// reported healthy from a cached 200.
func doctorProbeKeys(ctx context.Context, c *client.Client, bin string, report map[string]any, v3Reachable bool) {
	v3 := c != nil && c.Config != nil && c.Config.AuthHeader() != ""
	v2 := strings.TrimSpace(cliutil.EnvOverride(doctorV2Env)) != ""
	// A stored v3 key that was refused keeps doctor's "refused" verdict; it is
	// not the same as no key. HAND-FIX(doctor-keeps-refused-v3-verdict)
	refused := !v3 && c != nil && c.Config != nil && c.Config.HasCredentialRefusals()
	if !v3 && !v2 {
		if !refused {
			report["credentials"] = "ERROR not verified: no API key is configured, so nothing was probed. " + doctorAuthHint
		}
		return
	}
	switch {
	case refused:
	case v3 && !v3Reachable:
		// The v3 host did not answer at the network level; the v2 host is a
		// different endpoint, so its probe below still runs.
		report["credentials"] = "skipped (v3 API unreachable, so the v3 key was not probed). " + doctorBaseURLRemedy
	case v3:
		report["credentials"] = doctorProbeOne(ctx, c, bin, "v3 key", doctorV3ProbePath, doctorV3ProbeCmd, doctorV3BodyHasData, report, true)
	default:
		report["credentials"] = "INFO " + doctorV3Env + " not set: v3 commands (health, devices, ingest, retire, push), scorecard, the unmapped reason of unwatched and the v3 half of check and timeline are unavailable. " + doctorAuthHint
	}
	if v2 {
		report["credentials_v2"] = doctorProbeOne(ctx, c.WithTier("v2"), bin, "v2 key", doctorV2ProbePath, doctorV2ProbeCmd, doctorV2BodyHasCounts, report, false)
	}
}

func doctorProbeOne(ctx context.Context, c *client.Client, bin, label, path, cmd string, accept func([]byte) bool, report map[string]any, fixAPI bool) string {
	body, err := c.GetNoCache(ctx, path, map[string]string{"page_size": "1"})
	status := 0
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		status = apiErr.StatusCode
	}
	switch {
	case err == nil && accept(body):
		return fmt.Sprintf("valid (%s verified with GET %s)", label, path)
	case err == nil:
		if fixAPI {
			report["api"] = "WARN the host answered but not like the Backup Radar API. " + doctorBaseURLRemedy
		}
		return fmt.Sprintf("WARN not verified (GET %s answered without the expected envelope) - an error object or a non-API body does not confirm the %s.", path, label)
	case status == 401 || status == 403:
		if fixAPI {
			report["api"] = "WARN the host answered but rejected the key, so this is not confirmed to be your tenant's API."
		}
		return fmt.Sprintf("ERROR rejected (HTTP %d from %s) - the %s is invalid, revoked or lacks access. Create a new key and set it again. %s", status, path, label, doctorAuthHint)
	case status == 404:
		if fixAPI {
			report["api"] = fmt.Sprintf("ERROR the host answered but %s is not there - base_url is not the Backup Radar v3 API root. %s", path, doctorBaseURLRemedy)
		}
		return fmt.Sprintf("ERROR not verified (HTTP 404 from %s) - the endpoint does not exist at this base_url, so the %s was never checked.", path, label)
	case status >= 500:
		return fmt.Sprintf("WARN not verified (HTTP %d from %s) - the API returned a server error, so the %s was never checked.", status, path, label)
	case status > 0:
		return fmt.Sprintf("WARN not verified (HTTP %d from %s). Run `%s %s` to see the full response.", status, path, bin, cmd)
	default:
		return fmt.Sprintf("ERROR not verified: %s", err)
	}
}
