// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/url"
	"strings"

	"threatlocker-pp-cli/internal/cliutil"
)

// Hand-wired (handfixes.json: dependent-sync-path-helpers). Press 4.34.0 emits
// replacePathParam and pathParamSegmentValue from helpers.go.tmpl only when the
// spec has a {path} parameter, but sync.go.tmpl calls them for every dependent
// resource. ThreatLocker has no path parameters and two query-keyed dependents
// (application_files, maintenance), so the generated tree does not compile
// without these. Bodies are byte-identical to the press template. Delete this
// file when the press emits them for dependents too (the build then fails with
// a duplicate declaration, which is the signal).

// replacePathParam escapes path-param values before substituting them so
// reserved characters within each segment remain safe in the request URL.
func replacePathParam(path, name, value string) string {
	return strings.ReplaceAll(path, "{"+name+"}", cliutil.EscapePathParam(value))
}

func pathParamSegmentValue(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return value
	}
	escapedPath := strings.TrimRight(parsed.EscapedPath(), "/")
	if escapedPath == "" {
		return value
	}
	segment := escapedPath
	if i := strings.LastIndex(segment, "/"); i >= 0 {
		segment = segment[i+1:]
	}
	if segment == "" {
		return value
	}
	if decoded, err := url.PathUnescape(segment); err == nil {
		return decoded
	}
	return segment
}
