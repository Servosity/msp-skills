// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (netsapiens; handfixes.json netsapiens-credential-goes-only-to-the-configured-server):
// --path-context fills request-path placeholders; it never names the server.
// The client refuses any request off the configured server anyway (the pin in
// internal/client/netsapiens_origin_pin.go); this refuses the attempt early,
// with a usage error that names the variables that do change the server.

package cli

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// serverShapedKeys are path-context names that can only mean a host or its
// parts, whatever the base URL template looks like (compared folded: case,
// '-', '_' and '.' ignored).
var serverShapedKeys = map[string]bool{
	"server": true, "host": true, "hostname": true, "port": true, "scheme": true,
	"origin": true, "authority": true, "url": true, "baseurl": true, "apiurl": true,
	"apiroot": true, "endpoint": true,
}

var placeholderName = regexp.MustCompile(`\{([^{}/]+)\}`)

// originPlaceholders lists the placeholders in the scheme and authority of a
// base URL template ("https://{server}/ns-api/v2" gives server).
func originPlaceholders(baseURL string) map[string]bool {
	head := baseURL
	if i := strings.Index(head, "://"); i >= 0 {
		rest := head[i+3:]
		if j := strings.IndexAny(rest, "/?#"); j >= 0 {
			rest = rest[:j]
		}
		head = head[:i+3] + rest
	} else if j := strings.IndexAny(head, "/?#"); j >= 0 {
		head = head[:j]
	}
	out := map[string]bool{}
	for _, m := range placeholderName.FindAllStringSubmatch(head, -1) {
		out[foldKey(m[1])] = true
	}
	return out
}

func foldKey(k string) string {
	return strings.NewReplacer("-", "", "_", "", ".", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(k)))
}

// refuseServerPathContext refuses every --path-context key that would fill the
// server's scheme, host or port.
func refuseServerPathContext(pathContext map[string]string, baseURL string) error {
	inOrigin := originPlaceholders(baseURL)
	var bad []string
	for k := range pathContext {
		if f := foldKey(k); serverShapedKeys[f] || inOrigin[f] {
			bad = append(bad, k)
		}
	}
	if len(bad) == 0 {
		return nil
	}
	sort.Strings(bad)
	return fmt.Errorf("--path-context %s cannot change the server a credentialed request goes to; set NETSAPIENS_SERVER (or NETSAPIENS_BASE_URL) to change the server", strings.Join(bad, ", "))
}
