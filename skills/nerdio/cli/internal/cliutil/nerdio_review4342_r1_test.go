// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"encoding/base64"
	"encoding/hex"
	"slices"
	"strings"
	"testing"
)

// Codex review r1 (4.34.2 reprint), class tenant secret written to disk: a
// tenant secret under six characters was recognized only as a whole JSON
// string literal, so `secure-variables list --csv --select value --deliver
// file:/tmp/leak.csv` over [{"id":1,"name":"x","value":"abcde"}] wrote the
// clear value. Every path that does not answer the caller (a file, stderr,
// the store, an MCP child argv) now matches a tenant secret of any length
// the way it matches a held credential: as written, inside any text, and in
// every encoding the chokepoint withholds.
func TestShortTenantSecretWithheldInEveryRepresentation(t *testing.T) {
	recordHolds(t)
	const short = "qK5vJ"
	ShowTenantSecrets([]byte(`[{"id":1,"name":"x","value":"`+short+`"}]`), "value")
	sinks := map[string]string{
		"csv":       "id,name,value\n1,x," + short + "\n",
		"plain":     short + "\n",
		"table":     "ID  NAME  VALUE\n1   x     " + short + "\n",
		"yaml":      "- id: 1\n  value: " + short + "\n",
		"json":      `[{"id":1,"value":"` + short + `"}]` + "\n",
		"in a word": "pin=" + short + "z;\n",
		"base64":    "blob " + base64.StdEncoding.EncodeToString([]byte("value="+short)) + "\n",
		"hex":       "blob " + hex.EncodeToString([]byte(short)) + "\n",
	}
	for name, text := range sinks {
		if out := string(WithholdTenantSecrets([]byte(text))); strings.Contains(out, short) || out == text {
			t.Errorf("%s: a file sink kept the short tenant secret:\n%s", name, out)
		}
	}
	if !ArgvRepeatsSecret("teach\n--notes\nmy pin is " + short) {
		t.Errorf("an MCP child argv repeating the short tenant secret was not refused")
	}
}

// Codex review r1 (4.34.2 reprint), class holder incomplete: a credential-named
// markup element marked only its own text, so a credential container's
// descendants (<credentials><item>x</item></credentials>) printed in error
// output. The markup walk now follows the element tree the way the JSON walk
// does: everything under a credential-named element or attribute, at any
// depth, is a tenant secret (text, CDATA, entity-escaped text, attributes).
func TestMarkupCredentialContainersHoldEveryDescendant(t *testing.T) {
	recordHolds(t)
	cases := map[string]string{
		"XmlLeaked-r1-a7c2":   `<error><credentials><item>XmlLeaked-r1-a7c2</item></credentials></error>`,
		"XmlNested-r1-5e9d":   `<r><ns:Secrets xmlns:ns="u"><list><entry><v>XmlNested-r1-5e9d</v></entry></list></ns:Secrets></r>`,
		"XmlAttr-r1-3b8f":     `<r><password><item value="XmlAttr-r1-3b8f"/></password></r>`,
		"XmlCdata-r1-6c1a":    `<r><apiKeys><k><![CDATA[XmlCdata-r1-6c1a]]></k></apiKeys></r>`,
		"Xml&Entity-r1-2d4e":  `<r><tokens><t>Xml&amp;Entity-r1-2d4e</t></tokens></r>`,
		"XmlMixed-r1-8f3b":    `<r><credential>XmlLead-r1-0e6d <b>XmlMixed-r1-8f3b</b></credential></r>`,
		"XmlUnclosed-r1-1a9c": `<html><body><secret><p>XmlUnclosed-r1-1a9c</body></html>`,
	}
	for value, body := range cases {
		ShowTenantSecretMembers([]byte(body))
		if !TenantSecretEchoes(value) {
			t.Errorf("%s: the value under a credential container was not registered", body)
		}
	}
	// Control: a non-credential container's descendants stay data.
	ShowTenantSecretMembers([]byte(`<error><details><item>XmlPlainData-r1-4c7e</item></details></error>`))
	if TenantSecretEchoes("XmlPlainData-r1-4c7e") {
		t.Errorf("a value under a non-credential container was registered")
	}
}

// The markup family of the same class: a tenant-secret response that is not
// JSON (markup, "Name: value" text, or the bare secret) registered nothing,
// so a file sink wrote it clear. Its secrets are registered as written: under
// a credential- or value-named element at any depth, a named line, or (when
// the body names no member) every word of it.
func TestNonJSONTenantSecretBodiesAreRegistered(t *testing.T) {
	held := recordHolds(t)
	t.Setenv(MCPChildEnvVar, "")
	bodies := map[string]string{
		"MarkupLaps-r1-7d2c": `<result><password><v>MarkupLaps-r1-7d2c</v></password></result>`,
		"MarkupVar-r1-2b6e":  `<vars><var><name>Join</name><value>MarkupVar-r1-2b6e</value></var></vars>`,
		"LineLaps-r1-9a3f":   "accountName: localadmin\npassword: LineLaps-r1-9a3f\n",
		"BareLaps-r1-5c8d":   "BareLaps-r1-5c8d\n",
	}
	for secret, body := range bodies {
		ShowTenantSecrets([]byte(body), "value")
		if out := string(WithholdTenantSecrets([]byte("saved: " + secret + "\n"))); strings.Contains(out, secret) {
			t.Errorf("%q: a file sink kept the tenant secret of a non-JSON body:\n%s", body, out)
		}
		if slices.Contains(*held, secret) {
			t.Errorf("%q: the tenant secret was held as a credential (the caller could not see it)", body)
		}
	}
}
