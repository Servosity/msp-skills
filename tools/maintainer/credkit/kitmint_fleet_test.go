// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	fleetBasicMint = regexp.MustCompile(`\.SetBasicAuth\(clientID, clientSecret\)`)
	fleetTokenURL  = regexp.MustCompile(`tokenURL = "(https://[^"]+)"`)
	fleetBaseURL   = regexp.MustCompile(`Default:\s*("https://[^"]+"|[A-Za-z_][A-Za-z0-9_]*)`)
	fleetAnyOrigin = regexp.MustCompile(`\{Source: "\*", Allowed: \[\]string\{([^}]*)\}\}`)
	fleetURLConst  = regexp.MustCompile(`(?m)^\s*(?:const\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=\s*"(https://[^"]+)"`)
)

// Issues #691 and #698: a client-credentials mint that sends the client pair
// as HTTP Basic to a token host the held access token may not go to is
// withheld whenever the token carries the client ID at the Basic header's
// base64 alignment (an Entra or Zoom token does): on renewal, and on the first
// mint after a restart that loaded an expired token. Each kit adopter whose
// Basic mint's default token host is not where its "*" origin lets the token
// go must send the pair in the form body instead. Declaring
// header:authorization for the token host is not enough: it covers the
// header only once a Bearer request has bound the token there, never after a
// restart (zoom-phone, seen failing). A token URL the connector derives at
// run time (no literal default) is logged, not judged.
func TestKitFleetBasicMintRenewalIsNotWithheld(t *testing.T) {
	slugs, _ := filepath.Glob("../../../skills/*/cli")
	if len(slugs) == 0 {
		t.Skip("no skills tree beside the kit")
	}
	checked := 0
	for _, cli := range slugs {
		decl, err := os.ReadFile(filepath.Join(cli, "internal/credwire/declarations.go"))
		if err != nil {
			continue // not a kit adopter
		}
		for _, name := range []string{"internal/client/client.go", "internal/cli/auth.go"} {
			src, err := os.ReadFile(filepath.Join(cli, name))
			if err != nil || !fleetBasicMint.Match(src) {
				continue
			}
			m := fleetTokenURL.FindSubmatch(src)
			if m == nil {
				t.Logf("%s/%s: a Basic mint whose token URL is derived at run time", cli, name)
				continue
			}
			checked++
			token := fleetOrigin(string(m[1]))
			if fleetOriginIn(decl, fleetAnyOrigin, token) {
				continue
			}
			if b := fleetBaseURL.FindSubmatch(decl); b != nil && fleetOrigin(fleetResolve(decl, string(b[1]))) == token {
				continue
			}
			t.Errorf("%s/%s: the Basic client-credentials mint to %s is withheld on renewal and after a restart when the token carries the client ID; send the client pair in the form body, grant_type first (cliutil.TokenForm)", cli, name, token)
		}
	}
	t.Logf("Basic client-credentials mints checked: %d", checked)
}

// fleetOrigin is scheme://host of raw, its template braces kept.
func fleetOrigin(raw string) string {
	scheme, rest, _ := strings.Cut(raw, "://")
	host, _, _ := strings.Cut(rest, "/")
	return scheme + "://" + host
}

// fleetOriginIn reports whether the Origin re matches in decl allows origin,
// by a literal or by a package constant naming it.
func fleetOriginIn(decl []byte, re *regexp.Regexp, origin string) bool {
	m := re.FindSubmatch(decl)
	if m == nil {
		return false
	}
	for _, item := range strings.Split(string(m[1]), ",") {
		if fleetOrigin(fleetResolve(decl, item)) == origin {
			return true
		}
	}
	return false
}

// fleetResolve is item unquoted, or the URL of the package constant it names.
func fleetResolve(decl []byte, item string) string {
	item = strings.TrimSpace(item)
	for _, c := range fleetURLConst.FindAllSubmatch(decl, -1) {
		if string(c[1]) == item {
			return string(c[2])
		}
	}
	return strings.Trim(item, `"`)
}
