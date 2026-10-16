// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: makes `doctor` report a verdict that tracks reality
// (handfixes.json: doctor-verdict-tracks-reality).
//
// The generated doctor called any HTTP response to `GET /` "reachable" and
// printed "present, not verified" for the credential, so a base_url aimed at
// the console's web UI (every API path 404s) and a deleted API key both
// rendered healthy. GravityZone has no GET endpoints at all, so the probe is
// one JSON-RPC call, general#getApiKeyDetails: it needs no parameters and is
// answered for any valid key, whatever API areas the key has.
//
// See issue #282.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"bitdefender-pp-cli/internal/client"
	"bitdefender-pp-cli/internal/cliutil"
)

const (
	doctorBaseURLEnv    = "BITDEFENDER_BASE_URL"
	doctorProbeArea     = "/v1.0/jsonrpc/general"
	doctorProbeMethod   = "getApiKeyDetails"
	doctorProbeCmd      = "api-key"
	doctorProbeMaxBytes = 1 << 20
)

const doctorBaseRemedy = "Set " + doctorBaseURLEnv + " to the Access URL shown in Control Center under My Account > Control Center API, including /api (the cloud default is https://cloud.gravityzone.bitdefender.com/api)."

// doctorProbe posts one JSON-RPC getApiKeyDetails call with the configured
// Authorization header and returns the raw status and body. The request URL
// carries no #method fragment, so the JSON-RPC client transport passes it
// through untouched and the response can be classified as received.
func doctorProbe(ctx context.Context, c *client.Client, authHeader string) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	payload := []byte(`{"jsonrpc":"2.0","method":"` + doctorProbeMethod + `","params":{},"id":"doctor"}`)
	url := strings.TrimRight(c.Config.BaseURL, "/") + doctorProbeArea
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, err
	}
	// Same static headers and User-Agent real commands send, so a proxy that
	// gates on either answers the probe as it answers them.
	for k, v := range c.Config.Headers {
		req.Header.Set(k, v)
	}
	ua := cliutil.EnvOverride("BITDEFENDER_USER_AGENT")
	if ua == "" {
		ua = "bitdefender-cli/1.0"
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, doctorProbeMaxBytes))
	return resp.StatusCode, body, err
}

// doctorRPCEnvelope decodes a JSON-RPC 2.0 response. ok is false for any body
// that is not one.
func doctorRPCEnvelope(body []byte) (result json.RawMessage, rpcErr *rpcError, ok bool) {
	var env struct {
		JSONRPC string          `json:"jsonrpc"`
		Result  json.RawMessage `json:"result"`
		Error   *rpcError       `json:"error"`
	}
	if json.Unmarshal(bytes.TrimSpace(body), &env) != nil || env.JSONRPC != "2.0" {
		return nil, nil, false
	}
	if env.Error == nil && len(env.Result) == 0 {
		return nil, nil, false
	}
	return env.Result, env.Error, true
}

// doctorListEnvelope reports whether body is a non-empty JSON object whose
// every field is an array under a list name (data, items, results). That is
// not Control Center's own shape: it is what a stub or proxy in front of it
// may answer, and it is accepted only after the key was sent and the call
// answered 2xx.
func doctorListEnvelope(body []byte) bool {
	var doc map[string]any
	if json.Unmarshal(bytes.TrimSpace(body), &doc) != nil || len(doc) == 0 {
		return false
	}
	for k, v := range doc {
		switch strings.ToLower(k) {
		case "data", "items", "results":
		default:
			return false
		}
		if _, ok := v.([]any); !ok {
			return false
		}
	}
	return true
}

// doctorProbeCredentials writes the credentials verdict from one authenticated
// getApiKeyDetails call, and corrects the api verdict when the probe proves
// the generated `GET /` check was optimistic.
func doctorProbeCredentials(ctx context.Context, c *client.Client, authHeader string, report map[string]any) {
	call := doctorProbeArea + "#" + doctorProbeMethod
	status, body, err := doctorProbe(ctx, c, authHeader)
	result, rpcErr, isRPC := doctorRPCEnvelope(body)
	ok2xx := status >= 200 && status < 300
	switch {
	case err != nil:
		report["credentials"] = fmt.Sprintf("ERROR not verified: %s. %s", err, doctorBaseRemedy)
	case status == http.StatusUnauthorized:
		report["credentials"] = fmt.Sprintf("ERROR rejected (HTTP 401 from %s) - the API key is invalid, disabled or deleted. Create a key in Control Center under My Account > Control Center API and set BITDEFENDER_API_KEY.", call)
		report["api"] = "WARN the host answered but rejected the key, so this is not confirmed to be your Control Center. " + doctorBaseRemedy
	case status == http.StatusForbidden:
		report["credentials"] = fmt.Sprintf("WARN scope-limited (HTTP 403 from %s) - the key was accepted but may not call this API. Run `bitdefender-cli %s` once the key has the API areas you need.", call, doctorProbeCmd)
	case status == http.StatusNotFound:
		report["credentials"] = fmt.Sprintf("ERROR not verified (HTTP 404 from %s) - the endpoint does not exist at this base_url, so the key was never checked.", call)
		report["api"] = fmt.Sprintf("ERROR the host answered but %s is not there - base_url is not the Control Center API root. %s", doctorProbeArea, doctorBaseRemedy)
	case status >= 500:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s) - Control Center returned a server error, so the key was never checked.", status, call)
	case ok2xx && isRPC && rpcErr != nil:
		report["credentials"] = fmt.Sprintf("ERROR rejected (JSON-RPC error %d from %s: %s) - Control Center refused the call, so the key is not confirmed.", rpcErr.Code, call, rpcErr.Message)
	case ok2xx && isRPC && !bytes.Equal(bytes.TrimSpace(result), []byte("null")):
		report["credentials"] = fmt.Sprintf("valid (API key verified with %s)", call)
	case ok2xx && doctorListEnvelope(body):
		report["credentials"] = fmt.Sprintf("valid (HTTP %d list response to %s)", status, call)
	case ok2xx:
		report["credentials"] = fmt.Sprintf("WARN not verified (%s answered %d without a JSON-RPC result) - %s", call, status, doctorBaseRemedy)
		report["api"] = "WARN the host answered but not like the Control Center API. " + doctorBaseRemedy
	default:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s). Run `bitdefender-cli %s` to see the full response.", status, call, doctorProbeCmd)
	}
}
