// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cipp-pp-cli/internal/client"
	"cipp-pp-cli/internal/store"
)

// tenantRef is the minimal tenant identity the novel cross-tenant features
// need. CIPP scopes nearly every endpoint by tenantFilter, whose value is a
// tenant's defaultDomainName (or the literal "AllTenants"). DisplayName is for
// human-facing output; CustomerID is carried for completeness.
type tenantRef struct {
	DisplayName       string `json:"displayName"`
	DefaultDomainName string `json:"defaultDomainName"`
	CustomerID        string `json:"customerId"`
}

// tenantFieldLookup resolves a value from a decoded tenant object using a set
// of case-insensitive candidate keys. CIPP responses are inconsistent about
// casing (displayName vs DisplayName, customerId vs CustomerId), so every read
// is case-insensitive rather than pinned to one rendering.
func tenantFieldLookup(obj map[string]any, candidates ...string) string {
	for _, want := range candidates {
		for k, v := range obj {
			if strings.EqualFold(k, want) {
				if v == nil {
					continue
				}
				if s, ok := v.(string); ok {
					if s != "" {
						return s
					}
					continue
				}
				return fmt.Sprintf("%v", v)
			}
		}
	}
	return ""
}

// tenantEnvelopeKeys are the wrapper keys a CIPP deployment may nest the
// tenant array under. Matching is case-insensitive, so one entry covers both
// the REST-convention and .NET renderings ("results" also matches "Results").
// Current CIPP returns {"Results": [...]}; older builds return a bare array.
var tenantEnvelopeKeys = []string{"results", "data", "value", "items"}

// tenantEnvelopeArray pulls the tenant array out of a wrapper object. Returns
// false when no candidate key holds an array, which is the signal to fall back
// to treating the payload as a lone tenant object.
func tenantEnvelopeArray(obj map[string]any) ([]map[string]any, bool) {
	for _, want := range tenantEnvelopeKeys {
		for k, v := range obj {
			if !strings.EqualFold(k, want) {
				continue
			}
			items, ok := v.([]any)
			if !ok {
				continue
			}
			out := make([]map[string]any, 0, len(items))
			for _, it := range items {
				if m, ok := it.(map[string]any); ok {
					out = append(out, m)
				}
			}
			return out, true
		}
	}
	return nil, false
}

// parseTenantArray maps a raw JSON array of CIPP tenant objects into
// tenantRefs, reading fields case-insensitively. Objects with no resolvable
// defaultDomainName are dropped — a tenant we cannot scope a request to is not
// usable for fan-out.
func parseTenantArray(data json.RawMessage) ([]tenantRef, error) {
	var raw []map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		// Some CIPP deployments wrap the array; try a single object too.
		var single map[string]any
		if jerr := json.Unmarshal(data, &single); jerr != nil {
			return nil, fmt.Errorf("parsing tenant list: %w", err)
		}
		// A wrapper must be unwrapped BEFORE the lone-tenant fallback: a
		// {"Results": [...]} payload treated as one tenant resolves no
		// defaultDomainName, so it is dropped and every --all-tenants fan-out
		// silently reports zero tenants against a healthy CIPP.
		if inner, ok := tenantEnvelopeArray(single); ok {
			raw = inner
		} else {
			raw = []map[string]any{single}
		}
	}
	out := make([]tenantRef, 0, len(raw))
	for _, obj := range raw {
		domain := tenantFieldLookup(obj, "defaultDomainName")
		if domain == "" {
			continue
		}
		out = append(out, tenantRef{
			DisplayName:       tenantFieldLookup(obj, "displayName"),
			DefaultDomainName: domain,
			CustomerID:        tenantFieldLookup(obj, "customerId", "customerID"),
		})
	}
	return out, nil
}

// fetchTenants resolves the fleet from the live API by POSTing /ListTenants
// with tenantFilter=AllTenants and an empty body.
func fetchTenants(ctx context.Context, c *client.Client) ([]tenantRef, error) {
	data, _, err := c.PostWithParams(ctx, "/ListTenants", map[string]string{"tenantFilter": "AllTenants"}, map[string]any{})
	if err != nil {
		return nil, err
	}
	return parseTenantArray(data)
}

// loadTenantsFromStore resolves the fleet from the local SQLite store so
// offline / --local fan-out works without a live call. Reads resource_type
// "tenants".
func loadTenantsFromStore(ctx context.Context, dbPath string) ([]tenantRef, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("cipp-cli")
	}
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local database: %w", err)
	}
	defer db.Close()

	rows, err := db.List("tenants", 100000)
	if err != nil {
		return nil, fmt.Errorf("reading tenants from store: %w", err)
	}
	out := make([]tenantRef, 0, len(rows))
	for _, r := range rows {
		var obj map[string]any
		if json.Unmarshal(r, &obj) != nil {
			continue
		}
		domain := tenantFieldLookup(obj, "defaultDomainName")
		if domain == "" {
			continue
		}
		out = append(out, tenantRef{
			DisplayName:       tenantFieldLookup(obj, "displayName"),
			DefaultDomainName: domain,
			CustomerID:        tenantFieldLookup(obj, "customerId", "customerID"),
		})
	}
	return out, nil
}
