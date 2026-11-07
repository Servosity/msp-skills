// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"encoding/base64"
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// KF-JSM-1: a Basic pair whose user the adopter declares an Identifier
// (basic-user) is held except for its pieces lying wholly inside "user:",
// decided by position alone. Its
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
		"ADMINISTRATOR@VSPHERE.LOCAL", "tech@acme.\ufb01nance", "tech@acme.o\ufb03ce", "tech@A\u030a\u0301\u0316\ufb03ab", "tech@acme.xA\u030a\u0301abcd", "xxxxe\u0301abcd", "tech@\ufb03\ufb03\ufb03\ufb03\ufb03\ufb03", "ab\ufb01cdefgh", "xxxe\u0301abcde", "xxxe\u0301\u0301bcde", "\u00c5ngstr\u00f6m\u212b\u212b\u212b\u212b",
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

// heldWhole reports whether v is held with no piece of it released.
func heldWhole(v string) bool {
	reg.Lock()
	defer reg.Unlock()
	e := reg.vals[v]
	return e != nil && !e.userCut
}

// The class: the user half is dropped only when the adopter declares it.
// Otherwise the pair stays held whole, exactly as before; so does a pair held
// whole from any other source, and a pair that is not valid UTF-8.
func TestKitJSMPairHeldWholeUnlessUserDeclared(t *testing.T) {
	holdBasic(t, Declarations{}, "tech@acmemsp.com", "jsmtok-Pr0beEm4il9z")
	if !heldWhole("tech@acmemsp.com:jsmtok-Pr0beEm4il9z") || !isHeld("tech@acmemsp.com") || !leaks("acmemsp.com") {
		t.Errorf("undeclared user: pair or user not held whole: %q", heldValues())
	}
	// A short password: still every piece of the pair it is in is held.
	holdBasic(t, jsmUser, "apikey-0123456789abcdef", "x")
	if !leaks("abcdef:x") || !leaks("x") || heldWhole("apikey-0123456789abcdef:x") {
		t.Errorf("short password: %q", heldValues())
	}
	// Held whole from another source too: whole (strongest wins), either order.
	holdBasic(t, jsmUser, "tech@acmemsp.com", "jsmtok-Pr0beEm4il9z")
	Hold("minted:pair", "tech@acmemsp.com:jsmtok-Pr0beEm4il9z")
	if !heldWhole("tech@acmemsp.com:jsmtok-Pr0beEm4il9z") || !leaks("acmemsp.com") {
		t.Error("a pair held whole elsewhere lost its user pieces")
	}
	Hold("header:authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("tech@acmemsp.com:jsmtok-Pr0beEm4il9z")))
	if !heldWhole("tech@acmemsp.com:jsmtok-Pr0beEm4il9z") {
		t.Error("a later Basic header cut a pair held whole")
	}
	// A pair that is not valid UTF-8 is held whole (contract 2 counts it by bytes).
	holdBasic(t, jsmUser, "us\xffer-name-long", "pw-9876543210")
	if !isHeld("us\xffer-name-long:pw-9876543210") || !leaks("er-name-") {
		t.Errorf("invalid UTF-8 pair not held whole: %q", heldValues())
	}
}

// A child built on the kit inherits the cut, and a pair held whole on
// either side stays whole.
func TestKitJSMCutInherited(t *testing.T) {
	holdBasic(t, jsmUser, "tech@acmemsp.com", "jsmtok-Pr0beEm4il9z")
	recs := heldRecords()
	var buf strings.Builder
	if err := json.NewEncoder(&buf).Encode(recs); err != nil {
		t.Fatal(err)
	}
	resetForTest(jsmUser)
	holdInherited(strings.NewReader(buf.String()))
	if leaks("acmemsp.com") || !leaks("m:jsmtok-") || heldWhole("tech@acmemsp.com:jsmtok-Pr0beEm4il9z") {
		t.Errorf("child: cut not inherited or password piece passed: %q", heldValues())
	}
	Hold("minted:pair", "tech@acmemsp.com:jsmtok-Pr0beEm4il9z")
	holdInherited(strings.NewReader(buf.String()))
	if !heldWhole("tech@acmemsp.com:jsmtok-Pr0beEm4il9z") {
		t.Error("child: an inherited cut weakened a pair held whole")
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

// Codex r1: a cut whose user side normalizes longer (a ligature) must not
// hold a piece that lies wholly inside the user.
func TestKitJSMLigatureUserSiteLink(t *testing.T) {
	// Codex r2: a cut 5 characters back as written, 6 normalized, is enough.
	holdBasic(t, jsmUser, "tech@acme.o\ufb03ce", "jsmtok-Pr0beEm4il9z")
	if leaks("https://acme.office/") || !leaks("e.o\ufb03ce:j") || !leaks("office:j") {
		t.Errorf("ligature user: site link withheld or password piece passed: %q", heldValues())
	}
	holdBasic(t, jsmUser, "tech@acme.\ufb01nance", "jsmtok-Pr0beEm4il9z")
	for _, u := range []string{"https://acme.finance/", "acme.\ufb01nance", "tech@acme.finance"} {
		if leaks(u) {
			t.Errorf("ordinary output withheld: %q", u)
		}
	}
	if !leaks("finance:jsmt") || !leaks("jsmtok-Pr0beEm4il9z") {
		t.Error("the password side of a ligature user's pair is not withheld")
	}
}

// Codex r1: a password that is public text while running is not held on its
// own (contract 2); the pair's pieces carrying it still are.
func TestKitJSMPublicTextSideKeepsThePair(t *testing.T) {
	holdBasic(t, jsmUser, "alice-credkit", " withheld from stdout")
	if !leaks(" withheld from stdout") {
		t.Errorf("a password whose part is public text is not withheld: %q", heldValues())
	}
}

// Codex r1 (P2): a binding already held for the raw pair is still inherited
// by the parts of a Basic header carrying it.
func TestKitJSMPairBindingInherited(t *testing.T) {
	const user, pass = "tech@acmemsp.com", "jsmtok-Pr0beEm4il9z"
	resetForTest(Declarations{
		Identifiers: []Identifier{{Source: "basic-user", Reason: "the account's email"}},
		Origins:     []Origin{{Source: "minted:pair", Allowed: []string{"https://a.example"}}},
	})
	Hold("minted:pair", user+":"+pass)
	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
	reg.Lock()
	holdHeaderLocked(http.Header{"Authorization": {auth}})
	reg.Unlock()
	u, _ := url.Parse("https://a.example/")
	o, _ := originOf(u)
	if m, unknown := originMatcher(o, false); unknown || m != nil && m.check([]byte(auth)) != clean {
		t.Error("the Basic header was refused at the origin its pair is bound to")
	}
	u, _ = url.Parse("https://b.example/")
	o, _ = originOf(u)
	if m, unknown := originMatcher(o, false); !unknown && (m == nil || m.check([]byte(auth)) == clean) {
		t.Error("the Basic header may go to an origin its pair is not bound to")
	}
}

// Codex r3: stacked combining marks reorder when a piece is normalized on its
// own; the piece as written still has to be withheld, and a piece wholly
// inside the user is released whatever its marks.
func TestKitJSMStackedMarks(t *testing.T) {
	holdBasic(t, jsmUser, "tech@A\u030a\u0301\u0316\ufb03ab", "jsmtok-Pr0beEm4il9z")
	if !leaks("\u030a\u0301\u0316\ufb03ab:j") {
		t.Errorf("a piece of the pair as written passed: %q", heldValues())
	}
	holdBasic(t, jsmUser, "tech@acme.xA\u030a\u0301abcd", "jsmtok-Pr0beEm4il9z")
	if leaks("https://acme.x\u01fbabcd/") {
		t.Errorf("ordinary link withheld: %q", heldValues())
	}
}

// mainBasic is what main's kit holds for a Basic header whose user is an
// Identifier: the header, its base64, the pair whole and the password.
func mainBasic(user, pass string) *matcher {
	b64 := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
	return newMatcher([]string{"Basic " + b64, b64, user + ":" + pass, pass})
}

// spellings is each way an output can carry piece w that main's matcher may
// still find: as written, case changed, rot13, percent and base64 encoded.
func spellings(w string) []string {
	alt := []rune(w)
	for i := range alt {
		if i%2 == 0 {
			alt[i] = unicode.ToUpper(alt[i])
		}
	}
	return []string{w, strings.ToUpper(w), string(alt), rot13s(w), strings.ToUpper(rot13s(w)),
		url.QueryEscape(w), base64.StdEncoding.EncodeToString([]byte(w)), "x " + w + " x"}
}

// checkPositional asserts the positional rule for one pair: every spelling of
// every 8-character piece of the pair (as written, and of the pair
// normalized) that overlaps the password is withheld whenever main's kit
// withholds it; and the user alone prints, unless what main holds besides the
// pair (the password, the header, its base64) already withholds it.
func checkPositional(t *testing.T, user, pass string) {
	t.Helper()
	holdBasic(t, jsmUser, user, pass)
	pair := user + ":" + pass
	mainM := mainBasic(user, pass)
	b64 := base64.StdEncoding.EncodeToString([]byte(pair))
	others := newMatcher([]string{"Basic " + b64, b64, pass})
	if leaks(user) && (others == nil || others.check([]byte(user)) == clean) {
		t.Errorf("user %q pass %q: the user alone is withheld", user, pass)
	}
	np := string(normalize([]byte(pass)))
	for _, f := range []struct{ form, userSide string }{
		{pair, user + ":"},
		{string(normalize([]byte(pair))), string(normalize([]byte(user + ":")))},
	} {
		if !strings.HasSuffix(f.form, np) && f.form != pair {
			continue
		}
		cs := charStarts([]byte(f.form))
		ucs := utf8.RuneCountInString(f.userSide)
		if f.form != pair && !strings.HasPrefix(f.form, f.userSide) {
			ucs = 0 // the normalized pair does not split at the colon: every piece counts
		}
		for i := 0; i+pieceLen < len(cs); i++ {
			if i+pieceLen <= ucs {
				continue // wholly inside "user:": released
			}
			for _, u := range spellings(f.form[cs[i]:cs[i+pieceLen]]) {
				if mainM != nil && mainM.check([]byte(u)) != clean && !leaks(u) {
					t.Errorf("user %q pass %q: %q withheld by main, printed now", user, pass, u)
				}
			}
		}
	}
}

// Codex r4: Ascii85's "y" shorthand made "yy" decode to the 8-space password;
// the upper-case spelling of a pair piece main withholds must stay withheld.
func TestKitJSMRound4AsciiShorthand(t *testing.T) {
	holdBasic(t, jsmUser, "yyyyyy", strings.Repeat(" ", 8))
	if u := "YY:" + strings.Repeat(" ", 5); !leaks(u) {
		t.Errorf("%q withheld by main, printed now", u)
	}
	checkPositional(t, "yyyyyy", strings.Repeat(" ", 8))
}

// The positional rule over generated pairs: emails and repeated letters for
// the user; spaces, marks, ligatures, the Ascii85 shorthand and short values
// for the password.
func TestKitJSMPositionalProperty(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	pick := func(alpha []string, lo, hi int) string {
		var b strings.Builder
		for range lo + r.IntN(hi-lo+1) {
			b.WriteString(alpha[r.IntN(len(alpha))])
		}
		return b.String()
	}
	ua := []string{"a", "e", "y", "y", "z", "t", "c", "h", "m", "s", "p", ".", "-"}
	pa := []string{" ", " ", "y", "Y", "z", "!", "0", "9", ":", "́", "̊", "ﬃ", "A", "k", "~"}
	n := 160
	if testing.Short() {
		n = 40
	}
	for range n {
		user := pick(ua, 1, 10)
		if r.IntN(2) == 0 {
			user += "@" + pick(ua, 1, 8) + "." + pick([]string{"com", "net", "yy", "example"}, 1, 1)
		}
		checkPositional(t, user, pick(pa, 1, 18))
	}
	for _, c := range [][2]string{
		{"yyyyyy", strings.Repeat(" ", 8)}, {"yyyyyyyy", strings.Repeat(" ", 8)}, {"tech@acmemsp.com", "x"},
		{"tech@Ǻ̖ﬃab", "jsmtok-Pr0beEm4il9z"}, {"tech@acme.oﬃce", "́́́́́́́́"},
		{"zzzzzzzz@zz.zz", "zzzzzzzz"}, {"a", "yyyy    "},
	} {
		checkPositional(t, c[0], c[1])
	}
}
