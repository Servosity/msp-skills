// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// GravityZone's Public API is JSON-RPC 2.0: one POST URL per API area
// (/v1.x/jsonrpc/<area>) with the method name in the body. The spec writes
// each method as /v1.x/jsonrpc/<area>#<method>, and net/http never sends a URL
// fragment, so this transport turns the fragment plus the generated command's
// body (the method params) into the envelope, then hands back the `result`
// member, or the `error` member as an HTTP error status.
// Source: https://www.bitdefender.com/business/support/en/77209-125277-public-api.html

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"bitdefender-pp-cli/internal/client"
)

func init() { registerClientHook(installJSONRPCTransport) }

func installJSONRPCTransport(c *client.Client) error {
	if c == nil || c.HTTPClient == nil {
		return nil
	}
	if _, ok := c.HTTPClient.Transport.(*jsonrpcTransport); ok {
		return nil
	}
	base := c.HTTPClient.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	c.HTTPClient.Transport = &jsonrpcTransport{base: base}
	return nil
}

type jsonrpcTransport struct{ base http.RoundTripper }

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      string          `json:"id"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (t *jsonrpcTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := refusePlainHTTP(req.URL.Scheme, req.URL.Hostname()); err != nil {
		return nil, err
	}
	method := req.URL.Fragment
	if method == "" || req.Method != http.MethodPost || !strings.Contains(req.URL.Path, "/jsonrpc/") {
		return t.base.RoundTrip(req)
	}
	params := json.RawMessage(`{}`)
	if req.Body != nil {
		raw, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading JSON-RPC params: %w", err)
		}
		if s := bytes.TrimSpace(raw); len(s) > 0 && !bytes.Equal(s, []byte("null")) {
			params = s
		}
	}
	id := newRPCID()
	env, err := json.Marshal(rpcRequest{JSONRPC: "2.0", Method: method, Params: params, ID: id})
	if err != nil {
		return nil, fmt.Errorf("encoding JSON-RPC envelope: %w", err)
	}
	out := req.Clone(req.Context())
	u := *req.URL
	u.Fragment, u.RawFragment = "", ""
	out.URL = &u
	out.Body = io.NopCloser(bytes.NewReader(env))
	out.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(env)), nil }
	out.ContentLength = int64(len(env))
	out.Header.Set("Content-Type", "application/json")
	resp, err := t.base.RoundTrip(out)
	if err != nil || resp == nil {
		return resp, err
	}
	return unwrapRPCResponse(resp, id)
}

// refusePlainHTTP keeps the API key (sent as HTTP Basic on every call) off
// the wire in clear text: a base URL that is http:// to anything but a
// loopback test server is refused before the request leaves the machine.
func refusePlainHTTP(scheme, host string) error {
	if !strings.EqualFold(scheme, "http") {
		return nil
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("refusing to send the GravityZone API key over plain HTTP to %s: set BITDEFENDER_BASE_URL to an https:// URL (plain http is allowed only for a loopback test server)", host)
}

// maxRPCResponseBytes bounds one decoded JSON-RPC response. The largest
// documented page (getIncidentsList, perPage up to 10000) stays well under it.
var maxRPCResponseBytes int64 = 256 << 20

// unwrapRPCResponse replaces a JSON-RPC envelope with its result, or with its
// error object under a 4xx status (Control Center answers most failures with
// HTTP 200 plus an `error` member). Non-envelope bodies pass through untouched.
func unwrapRPCResponse(resp *http.Response, wantID string) (*http.Response, error) {
	defer resp.Body.Close()
	var src io.Reader = resp.Body
	// net/http only decompresses transparently when it chose Accept-Encoding
	// itself; decode an explicit gzip response here so the envelope is visible.
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		zr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("decoding gzip JSON-RPC response: %w", err)
		}
		defer zr.Close()
		src = zr
		resp.Header.Del("Content-Encoding")
		resp.Uncompressed = true
	}
	raw, err := io.ReadAll(io.LimitReader(src, maxRPCResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading JSON-RPC response: %w", err)
	}
	if int64(len(raw)) > maxRPCResponseBytes {
		return nil, fmt.Errorf("JSON-RPC response exceeds %d MiB; request a smaller page (perPage)", maxRPCResponseBytes>>20)
	}
	body := raw
	var env struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result"` // "null" when present as null, empty when absent
		Error   *rpcError       `json:"error"`
	}
	decodeErr := json.Unmarshal(raw, &env)
	switch {
	case decodeErr == nil && env.Error != nil:
		// An error object fails the call whatever its version or id says.
		body, _ = json.Marshal(map[string]any{"error": env.Error})
		if resp.StatusCode < 400 {
			resp.StatusCode = rpcErrorStatus(env.Error.Code)
			resp.Status = fmt.Sprintf("%d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
		}
	case resp.StatusCode >= 400:
		// Non-envelope failures (401 text, gateway pages) keep their status.
	case decodeErr != nil || env.JSONRPC != "2.0" || len(env.Result) == 0:
		return nil, fmt.Errorf("malformed JSON-RPC response: want jsonrpc 2.0 with a result or an error")
	case string(env.ID) != strconv.Quote(wantID):
		return nil, fmt.Errorf("JSON-RPC response id %s does not match request id %q", env.ID, wantID)
	default:
		body = env.Result
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
	return resp, nil
}

// rpcErrorStatus maps the documented JSON-RPC error codes onto the HTTP
// statuses the generated client already turns into typed exit codes.
// -32000 ("Server error") is what Control Center returns for bad input such as
// a missing required parameter, so it is a 400, not a retryable 5xx.
func rpcErrorStatus(code int) int {
	switch code {
	case -32001:
		return http.StatusForbidden
	case -32002:
		return http.StatusNotFound
	case -32003:
		return http.StatusTooManyRequests
	default:
		return http.StatusBadRequest
	}
}

func newRPCID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "1"
	}
	return hex.EncodeToString(b[:])
}
