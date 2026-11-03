// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: endpoint-security-odata-paging).

package client

import (
	"net/http"
	"strconv"
	"strings"
)

// odataTransport fixes Endpoint Security list paging on the wire. Those lists
// take OData options ($top, $skip, $count) and answer HTTP 400 to the plain
// names, but generated sync pages them with count=<page size> (count is the
// boolean $count option, not a page size) and skip=<offset>. On
// /rest/endpoint-security/ requests only, and only for plain names present
// without their $ form: a numeric count becomes $top (and asks $count=true so
// total_items comes back), a boolean count becomes $count, top becomes $top
// and skip becomes $skip. Every other request passes through untouched.
type odataTransport struct{ base http.RoundTripper }

func (t odataTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !strings.Contains(req.URL.Path, "/rest/endpoint-security/") || req.URL.RawQuery == "" {
		return t.base.RoundTrip(req)
	}
	q := req.URL.Query()
	changed := false
	move := func(plain, odata string) {
		if v, ok := q[plain]; ok {
			if _, has := q[odata]; !has {
				q[odata] = v
			}
			q.Del(plain)
			changed = true
		}
	}
	if v := q.Get("count"); v != "" {
		if _, err := strconv.Atoi(v); err == nil {
			if _, has := q["$top"]; !has {
				q.Set("$top", v)
			}
			if _, has := q["$count"]; !has {
				q.Set("$count", "true")
			}
			q.Del("count")
			changed = true
		}
	}
	move("count", "$count")
	move("top", "$top")
	move("skip", "$skip")
	if !changed {
		return t.base.RoundTrip(req)
	}
	r := req.Clone(req.Context())
	r.URL.RawQuery = q.Encode()
	return t.base.RoundTrip(r)
}

// InstallODataQueryTransport wraps the client's transport with odataTransport.
func InstallODataQueryTransport(c *Client) error {
	if c == nil || c.HTTPClient == nil {
		return nil
	}
	base := c.HTTPClient.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	if _, wrapped := base.(odataTransport); !wrapped {
		c.HTTPClient.Transport = odataTransport{base: base}
	}
	return nil
}
