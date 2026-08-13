// Copyright 2026 damienstevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
)

// TestParseTenantArrayShapes locks the three payload shapes CIPP deployments
// actually return. The {"Results": [...]} case is the regression guard: before
// the envelope unwrap, that payload was treated as a single tenant object,
// resolved no defaultDomainName, and every --all-tenants fan-out reported zero
// tenants against a healthy CIPP.
func TestParseTenantArrayShapes(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string // expected defaultDomainName values, in order
	}{
		{
			name: "bare array",
			body: `[{"displayName":"Contoso","defaultDomainName":"contoso.onmicrosoft.com"}]`,
			want: []string{"contoso.onmicrosoft.com"},
		},
		{
			name: "Results envelope",
			body: `{"Results":[{"displayName":"Contoso","defaultDomainName":"contoso.onmicrosoft.com"},
			         {"displayName":"Fabrikam","defaultDomainName":"fabrikam.onmicrosoft.com"}],
			         "Metadata":{"count":2}}`,
			want: []string{"contoso.onmicrosoft.com", "fabrikam.onmicrosoft.com"},
		},
		{
			name: "lowercase results envelope",
			body: `{"results":[{"defaultDomainName":"contoso.onmicrosoft.com"}]}`,
			want: []string{"contoso.onmicrosoft.com"},
		},
		{
			name: "lone tenant object",
			body: `{"displayName":"Contoso","defaultDomainName":"contoso.onmicrosoft.com"}`,
			want: []string{"contoso.onmicrosoft.com"},
		},
		{
			name: "objects without a domain are dropped",
			body: `[{"displayName":"No domain"},{"defaultDomainName":"ok.onmicrosoft.com"}]`,
			want: []string{"ok.onmicrosoft.com"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseTenantArray(json.RawMessage(tc.body))
			if err != nil {
				t.Fatalf("parseTenantArray: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %d tenants, want %d (%+v)", len(got), len(tc.want), got)
			}
			for i, want := range tc.want {
				if got[i].DefaultDomainName != want {
					t.Errorf("tenant %d domain = %q, want %q", i, got[i].DefaultDomainName, want)
				}
			}
		})
	}
}

// TestParseTenantArrayRejectsGarbage keeps the error path honest — a payload
// that is neither an array nor an object must surface as an error rather than
// silently degrading to zero tenants.
func TestParseTenantArrayRejectsGarbage(t *testing.T) {
	if _, err := parseTenantArray(json.RawMessage(`"not json we can use"`)); err == nil {
		t.Fatal("expected an error for a non-object, non-array payload")
	}
}
