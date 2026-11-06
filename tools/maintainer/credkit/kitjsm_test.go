// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"encoding/base64"
	"net/url"
	"testing"
	"unicode/utf8"
)

// KF-JSM-1: a Basic pair whose user the adopter declares an Identifier
// (basic-user) is held only for the pieces that overlap the password. Its
// user, the user's domain and links built from it are ordinary output; the
// password, the pair and every encoding of either stay withheld.

var jsmUser = Declarations{Identifiers: []Identifier{{Source: "basic-user", Reason: "the account's email"}}}

func holdBasic(t *testing.T, d Declarations, user, pass string) string {
	t.Helper()
	resetForTest(d)
	b64 := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
	Hold("header:authorization", "Basic "+b64)
	return b64
}

func TestKitJSMEmailUserIsOrdinaryOutput(t *testing.T) {
	const user, tok = "tech@acmemsp.com", "jsmtok-Pr0beEm4il9z"
	holdBasic(t, jsmUser, user, tok)
	for _, u := range []string{
		`{"reporter":"jane@acmemsp.com"}`,
		`{"emailAddress":"tech@acmemsp.com"}`,
		"tech@acmemsp.com",
		`{"self":"https://acmemsp.atlassian.net/rest/servicedeskapi/servicedesk/1"}`,
		"acmemsp.com",
		`{"owner":"agent@other.example","domainPrefix":"acmemsp","self":"https://acmemsp.my.auvik.com/"}`,
		"tech@acmemsp.com:", // the user with the colon, no password character
	} {
		if leaks(u) {
			t.Errorf("ordinary output withheld: %q", u)
		}
	}
}

func TestKitJSMSecretHalfStillWithheld(t *testing.T) {
	const user, tok = "tech@acmemsp.com", "jsmtok-Pr0beEm4il9z"
	b64 := holdBasic(t, jsmUser, user, tok)
	pair := user + ":" + tok
	for _, u := range []string{
		tok,
		`{"note":"key ` + tok + ` here"}`,
		"prefix" + tok + "suffix",
		pair,
		"Basic " + b64,
		b64,
		base64.RawURLEncoding.EncodeToString([]byte(pair)),
		base64.StdEncoding.EncodeToString([]byte(tok)),
		url.QueryEscape(pair),
		"https://" + url.UserPassword(user, tok).String() + "@acmemsp.atlassian.net/",
		rot13s(pair),
		"acmemsp.com:jsmt", // a straddling piece: 4 user chars, 4 password chars
		".com:jsmtok",
		"m:jsmtok-",
	} {
		if !leaks(u) {
			t.Errorf("secret half not withheld: %q", u)
		}
	}
}

// Every 8-character piece of the pair that overlaps the password is still a
// piece of a held value, for users the cut has to step around: non-ASCII,
// combining marks, characters NFKC expands, short users.
func TestKitJSMEveryPasswordPieceOfThePairHeld(t *testing.T) {
	for _, user := range []string{
		"tech@acmemsp.com", "abcdefg", "abcdefgh", "", "u",
		"jos\u00e9@caf\u00e9.example", "jose\u0301\u0301@cafe\u0301.ex\u0301", "\ufb03\ufb03\ufb03\ufb03\ufb03\ufb03\ufb03\ufb03",
		"ADMINISTRATOR@VSPHERE.LOCAL", "xxxe\u0301abcde", "xxxe\u0301\u0301bcde", "\u00c5ngstr\u00f6m\u212b\u212b\u212b\u212b",
	} {
		for _, pass := range []string{"jsmtok-Pr0beEm4il9z", "pw-12345", "p:w:9876543210"} {
			holdBasic(t, jsmUser, user, pass)
			pair := user + ":" + pass
			for _, form := range []string{pair, string(normalize([]byte(pair)))} {
				cs := charStarts([]byte(form))
				passStart := utf8.RuneCountInString(form) - utf8.RuneCountInString(pass)
				if form != pair {
					passStart = utf8.RuneCountInString(form) - utf8.RuneCountInString(string(normalize([]byte(pass))))
				}
				for i := max(0, passStart-pieceLen+1); i+pieceLen < len(cs); i++ {
					w := form[cs[i]:cs[i+pieceLen]]
					if !leaks(w) {
						t.Errorf("user %q pass %q: piece %q of the pair not withheld", user, pass, w)
					}
				}
			}
		}
	}
}

// The class: the user half is dropped only when the adopter declares it, and
// only when the password is a full piece long. Otherwise the pair stays held
// whole, exactly as before.
func TestKitJSMPairHeldWholeUnlessUserDeclared(t *testing.T) {
	holdBasic(t, Declarations{}, "tech@acmemsp.com", "jsmtok-Pr0beEm4il9z")
	if !isHeld("tech@acmemsp.com:jsmtok-Pr0beEm4il9z") || !isHeld("tech@acmemsp.com") {
		t.Errorf("undeclared user: pair or user not held whole: %q", heldValues())
	}
	// "key:x": the user half is the secret by its shape; the pair stays whole.
	holdBasic(t, jsmUser, "apikey-0123456789abcdef", "x")
	if !leaks("apikey-0123") || !isHeld("apikey-0123456789abcdef:x") {
		t.Errorf("short password: the pair was not held whole: %q", heldValues())
	}
	// A pair that is not valid UTF-8 is held whole (contract 2 counts it by bytes).
	holdBasic(t, jsmUser, "us\xffer-name-long", "pw-9876543210")
	if !isHeld("us\xffer-name-long:pw-9876543210") {
		t.Errorf("invalid UTF-8 pair not held whole: %q", heldValues())
	}
}

// The probe's exact case: an Atlassian answer naming a colleague at the same
// domain goes out through stdout unchanged.
func TestKitJSMProbeAnswer(t *testing.T) {
	holdBasic(t, jsmUser, "tech@acmemsp.com", "jsmtok-Pr0beEm4il9z")
	ans := `{"size":1,"start":0,"limit":50,"isLastPage":true,"values":[{"id":"1","projectKey":"SD","projectName":"Reporter jane@acmemsp.com"}]}`
	if leaks(ans) {
		t.Error("the probe's ordinary answer is withheld")
	}
	if !leaks(ans[:len(ans)-3] + ` jsmtok-Pr0beEm4il9z"}]}`) {
		t.Error("the probe's answer carrying the token is not withheld")
	}
}
