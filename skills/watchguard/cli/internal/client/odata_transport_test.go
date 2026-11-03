// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"net/http"
	"net/url"
	"testing"
)

type queryRecorder struct{ got url.Values }

func (r *queryRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	r.got = req.URL.Query()
	return &http.Response{StatusCode: 200, Body: http.NoBody, Header: http.Header{}}, nil
}

// hand-fix (handfixes.json endpoint-security-odata-paging): Endpoint Security
// lists take OData options; plain count/skip/top are rewritten there only.
func TestODataTransportRewritesEndpointSecurityPaging(t *testing.T) {
	for _, tc := range []struct {
		path, query string
		want        map[string]string
		absent      []string
	}{
		{"/rest/endpoint-security/management/api/v1/accounts/A/devices", "count=100&skip=200", map[string]string{"$top": "100", "$skip": "200", "$count": "true"}, []string{"count", "skip"}},
		{"/rest/endpoint-security/management/api/v1/accounts/A/devices", "top=50&count=true", map[string]string{"$top": "50", "$count": "true"}, []string{"top", "count"}},
		{"/rest/endpoint-security/management/api/v1/accounts/A/devices", "%24top=10&count=100", map[string]string{"$top": "10", "$count": "true"}, []string{"count"}},
		{"/rest/firebox/management/v1/info/A/devices", "count=100&skip=2", map[string]string{"count": "100", "skip": "2"}, []string{"$top", "$skip"}},
	} {
		rec := &queryRecorder{}
		req, _ := http.NewRequest(http.MethodGet, "https://api.example.test"+tc.path+"?"+tc.query, nil)
		if _, err := (odataTransport{base: rec}).RoundTrip(req); err != nil {
			t.Fatal(err)
		}
		for k, v := range tc.want {
			if rec.got.Get(k) != v {
				t.Errorf("%s?%s: %s=%q, want %q (sent %v)", tc.path, tc.query, k, rec.got.Get(k), v, rec.got)
			}
		}
		for _, k := range tc.absent {
			if _, ok := rec.got[k]; ok {
				t.Errorf("%s?%s: %s still sent (%v)", tc.path, tc.query, k, rec.got)
			}
		}
	}
}
