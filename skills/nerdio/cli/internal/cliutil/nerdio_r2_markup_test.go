// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import "testing"

// Codex r2 open P1 (class holder incomplete), fixed after the stop rule: the
// markup walk stopped at the decoder's first error, so anything after a
// malformed token (a numeric element name, a stray '<', an unterminated
// attribute) was never walked. A lexer that cannot fail now walks beside the
// decoder: a credential-named start tag marks everything up to its end tag,
// or the end of the body when it never closes.
func TestR2MarkupWalkSurvivesAParseError(t *testing.T) {
	recordHolds(t)
	for _, c := range []struct{ body, secret string }{
		{`<1><credentials><item>BadPrefixSecret-29Oct-a</item></credentials>`, "BadPrefixSecret-29Oct-a"},
		{`<< <credentials><item>BadPrefixSecret-29Oct-b</item></credentials>`, "BadPrefixSecret-29Oct-b"},
		{`<a x="unterminated><token>BadPrefixSecret-29Oct-c</token>`, "BadPrefixSecret-29Oct-c"},
		{`<1/><password>` + "BadPrefixSecret-29Oct-d", "BadPrefixSecret-29Oct-d"},
		{`<1><x secret="` + "BadPrefixSecret-29Oct-e" + `"/>`, "BadPrefixSecret-29Oct-e"},
		{`<1><credentials><![CDATA[BadPrefixSecret-29Oct-f]]></credentials>`, "BadPrefixSecret-29Oct-f"},
		{`<1><credentials>Bad&#80;refixSecret-29Oct-g</credentials>`, "BadPrefixSecret-29Oct-g"},
	} {
		ShowTenantSecretMembers([]byte(c.body))
		if !TenantSecretEchoes(c.secret) {
			t.Errorf("%q: the value under a credential-named element after a malformed token was not registered", c.body)
		}
	}
	// A non-credential element's text stays data.
	ShowTenantSecretMembers([]byte(`<1><item>OrdinaryDataValue-29Oct</item>`))
	if TenantSecretEchoes("OrdinaryDataValue-29Oct") {
		t.Error("an ordinary element's text was registered")
	}
}

// The lexer skips namespace declarations, as the decoder walk does: a
// credential-named element's xmlns:ns="u" names a schema, and holding "u"
// withheld every output carrying the letter as a token.
func TestR2MarkupLexerSkipsNamespaceDeclarations(t *testing.T) {
	held := []string{}
	walkMarkupLexer(`<r><ns:Secrets xmlns:ns="u" xmlns="urn:x"><v>LexNs-29Oct-h</v></ns:Secrets></r>`, SecretFieldName, func(v string) { held = append(held, v) })
	for _, v := range held {
		if v == "u" || v == "urn:x" {
			t.Errorf("the lexer held a namespace declaration %q", v)
		}
	}
	if len(held) != 1 || held[0] != "LexNs-29Oct-h" {
		t.Errorf("want only the credential text held, got %q", held)
	}
}
