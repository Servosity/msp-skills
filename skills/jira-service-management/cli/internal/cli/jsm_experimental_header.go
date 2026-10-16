// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"strings"

	"jira-service-management-pp-cli/internal/client"
)

// Several request-API endpoints (request types, customers, request feedback)
// are "experimental" and answer HTTP 412 without X-ExperimentalApi: opt-in.
// The generated commands send it per call, but sync and the MCP execute tool
// do not, so `sync --resources requesttype` failed. Atlassian ignores the
// header on stable endpoints, so every servicedeskapi request carries it.
func init() {
	registerClientHook(func(c *client.Client) error {
		if c.HTTPClient == nil {
			return nil
		}
		if _, done := c.HTTPClient.Transport.(jsmExperimentalHeader); done {
			return nil
		}
		next := c.HTTPClient.Transport
		if next == nil {
			next = http.DefaultTransport
		}
		c.HTTPClient.Transport = jsmExperimentalHeader{next: next}
		return nil
	})
}

type jsmExperimentalHeader struct{ next http.RoundTripper }

func (t jsmExperimentalHeader) RoundTrip(r *http.Request) (*http.Response, error) {
	if strings.Contains(r.URL.Path, "/rest/servicedeskapi/") && r.Header.Get("X-ExperimentalApi") == "" {
		r = r.Clone(r.Context())
		r.Header.Set("X-ExperimentalApi", "opt-in")
	}
	return t.next.RoundTrip(r)
}
