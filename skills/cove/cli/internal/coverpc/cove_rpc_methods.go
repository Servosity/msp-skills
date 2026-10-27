// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated), cove only: the one rule for which Cove
// JSON-RPC methods only read, shared by the `call` command (which sends any
// other method only with --yes) and the generated client's request guard
// (which never sends any other method), plus the JSON-RPC method each raw
// generated endpoint is specified to call. Hand-fix codes:
// cove-call-confirms-writes, cove-rpc-read-method-guard.

package coverpc

import "strings"

// readMethodPrefixes name the JSON-RPC methods that only read: Cove names its
// reads Get... and Enumerate.... Every other method (Add, Modify, Remove, Set,
// Send, ... and any method this build has never seen) may change the tenant.
var readMethodPrefixes = []string{"Get", "Enumerate"}

// IsReadMethod reports whether method, as the server decodes it, is a read: a
// read prefix, then an ASCII upper-case letter, then only ASCII letters and
// digits (GetServerInfo, EnumerateAccounts). Matching is exact and on the
// decoded name, so "Getaway", "GETRemove", "getAccountInfoById", a padded or
// NUL-split name, and any non-ASCII letter are not reads.
func IsReadMethod(method string) bool {
	for _, p := range readMethodPrefixes {
		rest, ok := strings.CutPrefix(method, p)
		if !ok || rest == "" || rest[0] < 'A' || rest[0] > 'Z' {
			continue
		}
		for i := 0; i < len(rest); i++ {
			b := rest[i]
			if !(b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9') {
				return false
			}
		}
		return true
	}
	return false
}

// endpointMethods is the JSON-RPC method the spec names for each raw generated
// endpoint, keyed by its pp:endpoint id (the MCP tool name with "_" for ".").
// Source: the press spec's body "method" default. Every one is a read.
var endpointMethods = map[string]string{
	"audit.list":       "EnumerateAuditActions",
	"backup-jobs.list": "EnumerateJobs",
	"columns.list":     "EnumerateColumns",
	"devices.get":      "GetAccountInfoById",
	"devices.list":     "EnumerateAccounts",
	"devices.stats":    "EnumerateAccountStatistics",
	"labels.list":      "EnumerateAllLabels",
	"locations.list":   "EnumerateLocations",
	"partners.get":     "GetPartnerInfoById",
	"partners.list":    "EnumeratePartners",
	"products.list":    "EnumerateProducts",
	"server.info":      "GetServerInfo",
	"storage.list":     "EnumerateStorages",
	"storage.nodes":    "EnumerateAllStorageNodes",
	"storage.stats":    "EnumerateStorageStatistics",
	"users.list":       "EnumerateUsers",
}

// RPCPath is the single JSON-RPC endpoint path every Cove method POSTs to.
const RPCPath = rpcPath

// EndpointMethod returns the JSON-RPC method a raw generated endpoint is
// specified to call, by its pp:endpoint id.
func EndpointMethod(endpoint string) (string, bool) {
	m, ok := endpointMethods[endpoint]
	return m, ok
}

// EndpointMethodForTool returns the JSON-RPC method a typed MCP endpoint tool
// is specified to call, by its tool name (the endpoint id with "_" for ".").
func EndpointMethodForTool(tool string) (string, bool) {
	for endpoint, m := range endpointMethods {
		if strings.ReplaceAll(endpoint, ".", "_") == tool {
			return m, true
		}
	}
	return "", false
}

// EndpointIDs lists every endpoint id the table names (for conformance tests).
func EndpointIDs() []string {
	ids := make([]string, 0, len(endpointMethods))
	for id := range endpointMethods {
		ids = append(ids, id)
	}
	return ids
}
