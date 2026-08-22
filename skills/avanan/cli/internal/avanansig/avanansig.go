// Copyright 2026 geekbrownbear and contributors. Licensed under Apache-2.0. See LICENSE.

// Package avanansig implements the Avanan SmartAPI request-signing scheme.
//
// Avanan runs two generations of auth against the same logical API:
//
//   - Legacy hosts (*.avanan.net) use a signed handshake. Every request — the
//     token handshake included — carries x-av-req-id, x-av-app-id, x-av-date,
//     and x-av-sig. Non-handshake requests additionally carry x-av-token.
//   - Infinity Portal hosts (cloudinfra*) exchange an access key for a bearer
//     token and sign nothing.
//
// The signature is a plain SHA-256 over the base64 of a concatenation — NOT an
// HMAC. Getting this wrong produces a 401 that looks exactly like a bad
// credential, which is why the algorithm lives in its own package with tests
// rather than inline in a transport.
//
// Sources, all three of which agree:
//   - Avanan API Reference Guide, "Calculating API Request Signature
//     (x-av-sig) for Token Generation" (worked example reproduced in the tests)
//   - demisto/content Packs/CheckPointHEC/Integrations/CheckPointHEC
//   - gocovi/RewstPS examples/Invoke-AvananRestMethod.ps1
//
// The published docs omit the requestString term entirely; both shipping
// clients include it on every non-handshake call. We follow the clients.
package avanansig

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"strings"
	"time"
)

// DateFormat is the timestamp layout Avanan expects in x-av-date.
//
// The server PARSES this value; it does not merely echo it. A trailing "Z"
// makes the parse fail and the API answers HTTP 500 - on every request,
// including one carrying a deliberately fabricated app id, which is why the
// failure looks like an outage or an unprovisioned key rather than a client
// bug. Verified live against smart-api-production-1-us on 2026-08-20:
// microsecond precision with no zone suffix returns a JWT, millisecond
// precision with "Z" returns 500, and x-av-token is irrelevant either way.
//
// This matches Avanan's own MSP sample client, which sends
// datetime.datetime.utcnow().isoformat(). Do not "restore" the documented
// "2021-04-10T00:00:00.000Z" shape from the API Reference Guide: that literal
// appears in the guide's worked signature example, not as an accepted
// x-av-date value.
const DateFormat = "2006-01-02T15:04:05.000000"

// FormatDate renders t as an Avanan x-av-date value. The time is converted to
// UTC first; sending a local timestamp with a Z suffix would be a lie the
// server cannot detect but which shifts the signed window.
func FormatDate(t time.Time) string {
	return t.UTC().Format(DateFormat)
}

// Sign computes the x-av-sig value.
//
//	sig = hex(sha256(base64(reqID + appID + date + requestString + secret)))
//
// requestString is empty for the token handshake and is the path plus encoded
// query for every other call. Callers must pass exactly the string that will
// be sent on the wire — see RequestString.
func Sign(reqID, appID, date, requestString, secret string) string {
	preimage := reqID + appID + date + requestString + secret
	encoded := base64.StdEncoding.EncodeToString([]byte(preimage))
	sum := sha256.Sum256([]byte(encoded))
	return hex.EncodeToString(sum[:])
}

// RequestString builds the signed request-string term from a request URL.
//
// The server signs the path exactly as it receives it, so this must be derived
// from the final URL after query parameters are attached — not from a template
// or a pre-encoding copy. A mismatch here is the single most common cause of a
// 401 against a credential that is actually valid.
func RequestString(u *url.URL) string {
	if u == nil {
		return ""
	}
	s := u.EscapedPath()
	if u.RawQuery != "" {
		s += "?" + u.RawQuery
	}
	return s
}

// IsInfinityHost reports whether host uses Infinity Portal bearer auth rather
// than the legacy signed handshake.
//
// Check Point fronts the Infinity Portal at cloudinfra-gw*.portal.checkpoint.com;
// the XSOAR client makes the same determination by testing for "cloudinfra" in
// the base URL.
func IsInfinityHost(host string) bool {
	h := strings.ToLower(host)
	return strings.Contains(h, "cloudinfra")
}

// InfinityPathPrefix is prepended to the version segment on Infinity Portal
// hosts. Legacy hosts serve /v1.0/scopes; Infinity serves
// /app/hec-api/v1.0/scopes.
const InfinityPathPrefix = "/app/hec-api"

// ApplyInfinityPrefix rewrites a legacy API path to its Infinity Portal
// equivalent. Paths that already carry the prefix are returned unchanged, so
// this is safe to apply more than once.
func ApplyInfinityPrefix(path string) string {
	if path == "" || strings.HasPrefix(path, InfinityPathPrefix+"/") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return InfinityPathPrefix + path
}

// Region maps a short region code to its regional API host. Regions are hard
// isolated: credentials issued for one region cannot read another region's
// data, so picking the wrong host surfaces as an authorization failure rather
// than an empty result.
var regionHosts = map[string]string{
	"us":   "smart-api-production-1-us.avanan.net",
	"eu":   "smart-api-production-1-eu.avanan.net",
	"ca":   "smart-api-production-1-ca.avanan.net",
	"ap":   "smart-api-production-5-ap.avanan.net",
	"uk":   "smart-api-production-1-euw2.avanan.net",
	"euw2": "smart-api-production-1-euw2.avanan.net",
	"uae":  "smart-api-production-1-mec1.avanan.net",
	"mec1": "smart-api-production-1-mec1.avanan.net",
	"in":   "smart-api-production-1-aps1.avanan.net",
	"aps1": "smart-api-production-1-aps1.avanan.net",
}

// RegionBaseURL returns the API base URL for a region code, and reports
// whether the code was recognized.
func RegionBaseURL(region string) (string, bool) {
	host, ok := regionHosts[strings.ToLower(strings.TrimSpace(region))]
	if !ok {
		return "", false
	}
	return "https://" + host, true
}

// Regions returns the recognized canonical legacy region codes, for help text
// and error messages.
func Regions() []string {
	return []string{"us", "eu", "ca", "ap", "uk", "uae", "in"}
}

// infinityHosts maps a short code to an Infinity Portal gateway host.
//
// Only gateways this project has actually reached are listed. Check Point
// fronts further regional gateways, but their host names are not published in
// a form we can verify, and a wrong host here fails identically to a wrong
// credential — so tenants on any other gateway point the CLI at it directly
// with --base-url or AVANAN_BASE_URL rather than through a guessed code.
var infinityHosts = map[string]string{
	"infinity":    "cloudinfra-gw.portal.checkpoint.com",
	"infinity-us": "cloudinfra-gw-us.portal.checkpoint.com",
}

// InfinityRegionBaseURL returns the API base URL for an Infinity Portal region
// code, and reports whether the code was recognized.
func InfinityRegionBaseURL(region string) (string, bool) {
	host, ok := infinityHosts[strings.ToLower(strings.TrimSpace(region))]
	if !ok {
		return "", false
	}
	return "https://" + host, true
}

// InfinityRegions returns the recognized Infinity Portal region codes.
func InfinityRegions() []string {
	return []string{"infinity", "infinity-us"}
}
