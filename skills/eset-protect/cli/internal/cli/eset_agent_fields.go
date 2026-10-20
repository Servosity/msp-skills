// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generator output); pinned by skills/eset-protect/handfixes.json
// under "agent-output-keeps-eset-identity".
//
// The generated --compact / --agent field floor and the --quiet identity
// pick know "id", "*Id" and "*_at". ESET names its identity "uuid", links
// objects through "*Uuid" / "*Uuids", stamps time in "*Time" fields, and
// carries state in "severity" and "is*" flags, so --agent dropped the uuid an
// agent needs for its next call and every timestamp, and --quiet printed the
// first alphabetical scalar (the redacted deviceToken, a category enum).

package cli

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"eset-protect-pp-cli/internal/client"
	"eset-protect-pp-cli/internal/secrets"
)

// esetCompactField reports whether compaction keeps an ESET field: uuid and
// every *Uuid / *Uuids link, every *Time stamp, severity, is* state flags
// and hash* identities (executables), in camel or snake case.
func esetCompactField(name string) bool {
	switch strings.ToLower(name) {
	case "uuid", "severity", "displayname":
		return true
	}
	for _, suffix := range []string{"Uuid", "Uuids", "Time", "Severity", "Sha1", "Sha256"} {
		if strings.HasSuffix(name, suffix) && len(name) > len(suffix) {
			return true
		}
	}
	lower := strings.ToLower(name)
	for _, suffix := range []string{"_uuid", "_uuids", "_time", "_severity"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	if strings.HasPrefix(name, "is") && len(name) > 2 {
		r, _ := utf8.DecodeRuneInString(name[2:])
		return unicode.IsUpper(r)
	}
	return strings.HasPrefix(name, "hash") && len(name) > 4
}

// esetPlaceholderValue reports a value the CLI wrote in place of a credential
// or a withheld text: never an identity.
func esetPlaceholderValue(s string) bool {
	return s == client.RedactedValue || s == secrets.Redacted || s == secrets.Withheld
}
