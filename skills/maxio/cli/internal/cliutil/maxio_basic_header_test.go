// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil_test

import (
	"encoding/base64"
	"strings"
)

// basicDecodedAuthHeader returns the user:password text of a Basic header, so
// the template's credential-precedence assertions can look for the raw secret
// (maxio renders `Basic {token}:x`; tools/apply_edits.py basic-constant-template-tests).
func basicDecodedAuthHeader(h string) string {
	enc, ok := strings.CutPrefix(h, "Basic ")
	if !ok {
		return h
	}
	dec, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return h
	}
	return string(dec)
}
