// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"

	"zoom-phone-pp-cli/internal/client"
	"zoom-phone-pp-cli/internal/cliutil"
)

// Zoom pages with the next_page_token query key. The spec names that parameter
// page_token (x-url-name next_page_token) so the press detects page_token
// pagination, but the generated --all and sync paginators send the spec name.
// Zoom has no page_token query parameter, so the transport renames it on the wire.
func init() {
	registerClientHook(func(c *client.Client) error {
		if c.HTTPClient == nil {
			return nil
		}
		base := c.HTTPClient.Transport
		if base == nil {
			base = cliutil.OutboundGuard(nil) // credkit-wiring
		}
		if _, ok := base.(pageTokenTransport); !ok {
			c.HTTPClient.Transport = pageTokenTransport{base: base}
		}
		return nil
	})
}

type pageTokenTransport struct{ base http.RoundTripper }

func (t pageTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	q := req.URL.Query()
	cursor, ok := q["page_token"]
	if !ok {
		return t.base.RoundTrip(req)
	}
	if _, has := q["next_page_token"]; !has {
		q["next_page_token"] = cursor
	}
	q.Del("page_token")
	clone := req.Clone(req.Context())
	clone.URL.RawQuery = q.Encode()
	return t.base.RoundTrip(clone)
}
