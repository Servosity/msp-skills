// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (not generated): exposes the client's JSON prefix sanitizer so
// the FortiOS secret guard (internal/cli/fortinet_secrets.go) scans exactly the
// bytes the client will parse.

package client

// SanitizeJSONResponse strips the UTF-8 BOM and JSONP/XSSI prefixes the client
// removes before parsing. It only ever trims a prefix, so the result is a
// suffix of body.
func SanitizeJSONResponse(body []byte) []byte { return sanitizeJSONResponse(body) }
