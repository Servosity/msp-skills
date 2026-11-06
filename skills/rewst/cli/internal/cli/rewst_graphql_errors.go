// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// rewst handfix graphql-errors-are-failures: Rewst is one GraphQL endpoint
// (POST /graphql) and, like most GraphQL gateways, answers a failed operation
// (not authorized, a validation error, an unknown argument) with HTTP 200 and
// an errors array. The generated client reads only the status, so a typed
// command printed {"results": {"errors": [...]}} and exited 0, sync stored the
// error envelope as data, and an MCP mirror call returned it as a result: an
// MSP reading "no data" where the API said "failed". Any failure signal wins:
// a 2xx answer from the GraphQL endpoint whose top-level errors member is
// present and not empty (an array with any element, or any other non-null
// value), or whose top-level error member is (a gateway or proxy answering
// in its own shape), becomes an HTTP error for every caller (CLI, novels,
// sync, MCP), whatever data rides beside it (a partial result is a failure
// here): 401 for an authentication code (Rewst's AUTH_ERR), 403 for a
// permission code, 400 for a request the gateway rejected, otherwise 502, so
// the client neither retries an authorization failure as a server error nor
// reports it as one. The body is kept as the error text, which the credential
// kit checks like every other output.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"rewst-pp-cli/internal/client"
)

// graphQLErrorStatus is the status a 2xx GraphQL answer carrying errors is
// rewritten to when no error code says more (the gateway answered, the
// operation failed).
const graphQLErrorStatus = http.StatusBadGateway

type graphQLErrorsTripper struct{ next http.RoundTripper }

func (t graphQLErrorsTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil || resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp, err
	}
	if req == nil || req.URL == nil || !strings.HasSuffix(strings.TrimRight(req.URL.Path, "/"), "/graphql") {
		return resp, nil
	}
	body, rerr := client.ReadBody(resp.Body)
	_ = resp.Body.Close()
	if rerr != nil {
		return nil, rerr
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	plain, derr := client.DecodeBody(resp.Header.Get("Content-Encoding"), body)
	if derr != nil {
		plain = body
	}
	if graphQLAnswerFailed(plain) {
		resp.StatusCode = graphQLFailureStatus(plain)
		resp.Status = fmt.Sprintf("%d GraphQL errors", resp.StatusCode)
	}
	return resp, nil
}

// graphQLAnswerFailed reports a GraphQL answer whose top-level errors (or
// error) member is present and carries anything: a non-empty array, an
// object, a string, a number or true. A body that is not one JSON object is left to the client
// (it already refuses what it cannot read).
func graphQLAnswerFailed(body []byte) bool {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(body), &top); err != nil {
		return false
	}
	for _, name := range []string{"errors", "error"} {
		if raw, ok := top[name]; ok && populated(raw) {
			return true
		}
	}
	return false
}

// graphQLErrorsPopulated reports a body whose top-level errors member (the
// GraphQL one, not a proxy's error) is populated.
func graphQLErrorsPopulated(body []byte) bool {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(body), &top); err != nil {
		return false
	}
	raw, ok := top["errors"]
	return ok && populated(raw)
}

// graphQLFailureStatus is the HTTP status a failed GraphQL answer becomes:
// the strongest error code wins (authentication over permission over a
// rejected request); an answer with no recognised code is a 502.
func graphQLFailureStatus(body []byte) int {
	var top struct {
		Errors []struct {
			Extensions struct {
				Code any `json:"code"`
			} `json:"extensions"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(bytes.TrimSpace(body), &top) // an errors member that is not a list keeps the default
	status, rank := graphQLErrorStatus, 0
	for _, e := range top.Errors {
		code, _ := e.Extensions.Code.(string)
		s, r := graphQLErrorStatus, 0
		switch strings.ToUpper(strings.TrimSpace(code)) {
		case "AUTH_ERR", "UNAUTHENTICATED", "UNAUTHORIZED", "INVALID_TOKEN":
			s, r = http.StatusUnauthorized, 3
		case "FORBIDDEN", "PERMISSION_DENIED":
			s, r = http.StatusForbidden, 2
		case "BAD_REQUEST", "BAD_USER_INPUT", "GRAPHQL_VALIDATION_FAILED", "GRAPHQL_PARSE_FAILED":
			s, r = http.StatusBadRequest, 1
		}
		if r > rank {
			status, rank = s, r
		}
	}
	return status
}

func populated(raw json.RawMessage) bool {
	switch string(bytes.TrimSpace(raw)) {
	case "", "null", "[]", "false", `""`, "{}":
		return false
	}
	return true
}
