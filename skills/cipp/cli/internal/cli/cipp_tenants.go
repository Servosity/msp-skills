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

// parseTenantArray maps a raw JSON array of CIPP tenant objects into
// tenantRefs, reading fields case-insensitively. Objects with no resolvable
// defaultDomainName are dropped — a tenant we cannot scope a request to is not
// usable for fan-out.
func parseTenantArray(data json.RawMessage) ([]tenantRef, error) {
	raw, err := cippListRows(data, "tenant list", tenantRecordFields, "defaultDomainName")
	if err != nil {
		return nil, err
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

// cippListRows turns a CIPP list response into its row objects (hand-fix
// handfixes.json tenant-list-message-is-not-a-fleet). It is the one parser for
// the lists whose completeness a report depends on (the tenant list, the GDAP
// relationship list). Fail closed: every element must be an object that carries
// the list's identity key (idKeys, matched case-insensitively, non-empty), and
// the whole response, envelope and every element, goes through the one document
// walker (cippScan): any failure signal anywhere refuses it. Any other element
// - a string, a CIPP result object such as {"state":"error","resultText":"Failed
// ..."}, a row without its identity - refuses the whole list, because a report
// built from a partial list looks complete. Bare arrays, Results/data/value/items
// envelopes and a lone object are read the same way. The body is decoded
// strictly (cippDecodeStrict: a duplicate key refuses it). rowExcept names the
// top-level row keys the walker leaves out (only the tenant list sets it).
func cippListRows(data json.RawMessage, what string, rowExcept map[string]bool, idKeys ...string) ([]map[string]any, error) {
	v, err := cippDecodeStrict(data)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", what, err)
	}
	if failure := cippResponseFailure(v, rowExcept); failure != "" {
		return nil, fmt.Errorf("CIPP returned a failure with the %s, so it would be incomplete: %s", what, failure)
	}
	rowsOf := func(items []any) ([]map[string]any, error) {
		out := make([]map[string]any, 0, len(items))
		var bad []string
		for _, it := range items {
			m, ok := it.(map[string]any)
			switch {
			case !ok:
				bad = append(bad, fmt.Sprint(it))
			case tenantFieldLookup(m, idKeys...) == "":
				bad = append(bad, "an entry with no "+strings.Join(idKeys, "/")+": "+cippEntryText(m))
			default:
				out = append(out, m)
			}
		}
		if len(bad) > 0 {
			return nil, fmt.Errorf("CIPP returned a message with the %s, so it would be incomplete: %s", what, strings.Join(bad, "; "))
		}
		return out, nil
	}
	switch x := v.(type) {
	case []any:
		return rowsOf(x)
	case map[string]any:
		for _, want := range tenantEnvelopeKeys {
			for k, inner := range x {
				if !strings.EqualFold(k, want) {
					continue
				}
				switch y := inner.(type) {
				case []any:
					return rowsOf(y)
				case string:
					return nil, fmt.Errorf("CIPP returned a message instead of the %s: %s", what, y)
				}
			}
		}
		return rowsOf([]any{x})
	case string:
		return nil, fmt.Errorf("CIPP returned a message instead of the %s: %s", what, x)
	}
	return nil, fmt.Errorf("parsing %s: unexpected JSON shape", what)
}

// cippEntryText is a short rendering of a refused entry for the error.
func cippEntryText(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "(unprintable entry)"
	}
	if len(b) > 200 {
		return string(b[:200]) + "..."
	}
	return string(b)
}

// tenantRecordFields are the error-named fields CIPP keeps on every tenant
// record (CIPP-API Get-Tenants.ps1 and Get-GraphToken.ps1: the last Graph
// error for that tenant and a count of them). They describe one tenant's
// health, not a failure of the response, so the tenant list's validation (and
// nothing else: not write results, not any other list) leaves them out of each
// tenant row, and a tenant that CIPP cannot currently reach does not refuse the
// whole fleet (hand-fix handfixes.json tenant-list-message-is-not-a-fleet).
var tenantRecordFields = map[string]bool{"lastgrapherror": true, "lastgraphtokenerror": true, "grapherrorcount": true}
