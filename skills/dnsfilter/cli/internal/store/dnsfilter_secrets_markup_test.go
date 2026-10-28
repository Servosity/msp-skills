// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "secrets-never-persisted" entry.

package store

import (
	"bytes"
	"encoding/json"
	"testing"
)

// A credential in an XML/HTML element or after a command-line flag has no
// separator for the pair rule; both shapes are withheld, in a stored row too.
func TestMarkupAndFlagValuesNameSecrets(t *testing.T) {
	for _, s := range []string{
		"<password>R17MARK01</password>",
		`<secret_key type="site">R17MARK02</secret_key>`,
		"<NKEY>R17MARK03</NKEY>",
		"<token>R17MARK07</token>",
		"<password>R17MARK08 (cut off",
		"deploy: agent.exe --nkey R17MARK04 /quiet",
		"run tool -password R17MARK05",
		"curl --token R17MARK06 https://x.example",
	} {
		if !TextNamesSecret(s) {
			t.Errorf("TextNamesSecret(%q) = false", s)
		}
		b, _ := json.Marshal(map[string]string{"note": s})
		if out := ScrubPersistedSecrets(b); bytes.Contains(out, []byte("R17MARK")) {
			t.Errorf("stored %s", out)
		}
	}
	for _, s := range []string{
		"<p>Password Policy</p>",
		"<token/>",
		"<b>Site name</b> --name HQ",
		"sync --full --resources networks",
		"<password></password>",
		"--password --verbose",
		"export DNSFILTER_API_KEY=<token> on macOS",
		"set <value> to the site name",
	} {
		if TextNamesSecret(s) {
			t.Errorf("TextNamesSecret(%q) = true, want an ordinary string", s)
		}
	}
}
