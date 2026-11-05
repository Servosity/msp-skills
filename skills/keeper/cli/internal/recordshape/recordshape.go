// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package recordshape names what only vault record content looks like. An admin view
// never needs it, so an answer carrying it is refused whole. Keeper's own reports use
// it per row; the client uses it on what no row projection reaches: error bodies and
// the Service Mode GETs.
package recordshape

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

// Err is the refusal.
var Err = errors.New("refused: Keeper returned record-content fields; nothing from this response is shown")

// Keys matches field names that only vault record content carries.
var Keys = regexp.MustCompile(`(?i)^(password|passwords|secret|secrets|totp|one_?time_?code|otp|notes|custom|fields|custom_fields|attachments?|files?|file_ref|login_url|url|record_data|data_json|private_?key|key_pair|passkey|share_url|one_?time_?shares?)$`)

// Values matches values that are credentials whatever field holds them.
var Values = regexp.MustCompile(`(?i)(otpauth://|-----BEGIN [A-Z ]*PRIVATE KEY-----|keeper:///)`)

// Found reports record content anywhere in a response body, JSON or not.
func Found(body []byte) bool {
	if Values.Match(body) {
		return true
	}
	var v any
	return json.Unmarshal(body, &v) == nil && found(v)
}

func found(v any) bool {
	switch t := v.(type) {
	case string:
		return Values.MatchString(t)
	case []any:
		for _, e := range t {
			if found(e) {
				return true
			}
		}
	case map[string]any:
		for k, e := range t {
			if Keys.MatchString(strings.TrimSpace(k)) || found(e) {
				return true
			}
		}
	}
	return false
}
