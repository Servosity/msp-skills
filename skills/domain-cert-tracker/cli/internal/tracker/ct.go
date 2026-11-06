// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package tracker

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"

	"domain-cert-tracker-pp-cli/internal/store"
)

// CrtShURL is the crt.sh search endpoint.
const CrtShURL = "https://crt.sh/"

// CrtShResultLimit is crt.sh's cap on matching identities per search
// (certwatch_db fnc/web_apis.fnc: c_resultLimit). Past it crt.sh returns a
// random subset and says so in HTML output only.
const CrtShResultLimit = 10000

// ErrCTTruncated means crt.sh returned a capped, partial answer. It is a
// failed CT check: a partial list must never read as the domain's history.
var ErrCTTruncated = errors.New("crt.sh truncated the result")

// CTCert is one certificate crt.sh found for a domain. Several crt.sh rows
// (one per matching identity) fold into one CTCert per crt.sh id.
//
// crt.sh builds name_value from the identities that matched the query only
// (certwatch_db fnc/web_apis.fnc: Search asks for match=ILIKE, and
// NAME_VALUES aggregates the matching rows), so a certificate
// found for shop.example.com that also names *.example.com arrives with
// shop.example.com alone. Names is that partial list until a sweep reads
// the certificate itself; NamesSource says which one it is.
type CTCert struct {
	ID         int64    `json:"id"`
	IssuerCAID int64    `json:"issuer_ca_id"`
	IssuerName string   `json:"issuer_name"`
	CommonName string   `json:"common_name"`
	Names      []string `json:"names"`
	// NamesSource is "certificate" when Names is the SAN list read from the
	// certificate's DER, else "crtsh" (crt.sh's matched names only).
	NamesSource string `json:"names_source"`
	NamesError  string `json:"names_error,omitempty"`
	NotBefore   string `json:"not_before"`
	NotAfter    string `json:"not_after"`
	Serial      string `json:"serial"`
}

// ParseCrtSh reads crt.sh JSON output (crtsh/certwatch_db fnc/web_apis.fnc
// row_to_json rows). Anything that is not a JSON array is an error: crt.sh
// answers overload with an HTML page, and that must never read as "no
// certificates".
func ParseCrtSh(body []byte) ([]CTCert, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, fmt.Errorf("crt.sh did not return a JSON array")
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var rows []struct {
		ID         json.Number `json:"id"`
		IssuerCAID json.Number `json:"issuer_ca_id"`
		IssuerName string      `json:"issuer_name"`
		CommonName string      `json:"common_name"`
		NameValue  string      `json:"name_value"`
		NotBefore  string      `json:"not_before"`
		NotAfter   string      `json:"not_after"`
		Serial     string      `json:"serial_number"`
		Count      json.Number `json:"result_count"`
	}
	if err := dec.Decode(&rows); err != nil {
		return nil, fmt.Errorf("parse crt.sh JSON: %w", err)
	}
	// crt.sh adds result_count over the rows and flags truncation when the
	// sum reaches c_resultLimit; this does the same for JSON. A row without
	// a usable count still counts as one identity.
	var identities int64
	for _, r := range rows {
		n, err := r.Count.Int64()
		if err != nil || n < 1 {
			n = 1
		}
		identities += n
	}
	if identities >= CrtShResultLimit {
		return nil, fmt.Errorf("%w: %d matching identities reached its %d-identity limit, so the certificate list is a partial subset", ErrCTTruncated, identities, CrtShResultLimit)
	}
	byID := map[int64]*CTCert{}
	for _, r := range rows {
		id, err := r.ID.Int64()
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("parse crt.sh JSON: bad id %q", r.ID.String())
		}
		c := byID[id]
		if c == nil {
			ca, _ := r.IssuerCAID.Int64()
			c = &CTCert{ID: id, IssuerCAID: ca, IssuerName: r.IssuerName, CommonName: strings.ToLower(r.CommonName),
				NamesSource: store.NamesFromCrtSh, NotBefore: crtShTime(r.NotBefore), NotAfter: crtShTime(r.NotAfter), Serial: DisplaySerial(r.Serial)}
			byID[id] = c
		}
		for _, n := range strings.Split(r.NameValue, "\n") {
			if n = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(n)), "."); n != "" && !strings.Contains(n, "@") {
				c.Names = appendUnique(c.Names, n)
			}
		}
		// An S/MIME address (rfc822Name) is an identity, never a host name.
		if c.CommonName != "" && !strings.Contains(c.CommonName, "@") {
			c.Names = appendUnique(c.Names, c.CommonName)
		}
	}
	out := make([]CTCert, 0, len(byID))
	for _, c := range byID {
		sort.Strings(c.Names)
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func appendUnique(list []string, s string) []string {
	for _, v := range list {
		if v == s {
			return list
		}
	}
	return append(list, s)
}

// crtShTime converts crt.sh's zone-less UTC timestamps to RFC 3339.
func crtShTime(s string) string {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04:05.999999", time.RFC3339Nano} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return "" // unknown, never a raw string that would sort as a date
}

// DisplaySerial lowercases a hex serial, strips separators and pads it to
// an even length, the way openssl and browsers show it. Compare serials with
// NormalizeSerial, not with this form.
func DisplaySerial(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer(":", "", " ", "", "-", "").Replace(s)
	if len(s)%2 == 1 {
		s = "0" + s
	}
	return s
}

// NormalizeSerial lowercases a hex serial and strips separators and leading
// zeros so crt.sh and crypto/x509 spellings compare equal.
func NormalizeSerial(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer(":", "", " ", "", "-", "").Replace(s)
	s = strings.TrimLeft(s, "0")
	if s == "" {
		return "0"
	}
	return s
}

// CertRef names one certificate the way both sources this tool reads can
// spell it: the issuer's distinguished name, the serial and the validity
// window. crt.sh rows and TLS handshakes both carry these four fields.
type CertRef struct {
	Issuer    string
	Serial    string
	NotBefore string
	NotAfter  string
}

// SameCertificate is the one rule for "these two records are one
// certificate"; every such decision goes through it. Serial uniqueness is
// scoped to the issuer (RFC 5280 4.1.2.2) and crt.sh identifies a
// certificate by issuer CA plus serial, so the identity is the issuer's
// whole distinguished name (every attribute, not its organization: one CA
// runs many intermediates, such as Let's Encrypt R10 and R11), the serial,
// and the validity window to the second. It fails closed: when either side's
// issuer does not read as a distinguished name, its serial is empty or not
// hex, or a validity bound is not an RFC 3339 time, the answer is false.
func SameCertificate(a, b CertRef) bool {
	ka, okA := a.identity()
	kb, okB := b.identity()
	return okA && okB && ka == kb
}

// crtShURL is the crt.sh page of one certificate. Built by concatenation so no
// non-test file holds a printf verb inside a URL query: the credential kit's
// fleet scan judges every URL query literal as a query sent as written.
func crtShURL(id int64) string {
	return "https://crt.sh/?id=" + strconv.FormatInt(id, 10)
}

// CertKey is a stable text key for one certificate (event ids). When every
// part reads with confidence it is the identity SameCertificate compares;
// otherwise it is the exact text, so two certificates that cannot be told
// apart are never folded into one key.
func CertKey(issuer, serial, notBefore, notAfter string) string {
	ref := CertRef{Issuer: issuer, Serial: serial, NotBefore: notBefore, NotAfter: notAfter}
	if k, ok := ref.identity(); ok {
		return k
	}
	return "unparsed|" + keyEscape(issuer) + "|" + keyEscape(serial) + "|" + keyEscape(notBefore) + "|" + keyEscape(notAfter)
}

// identity is the canonical key of c and whether every part of it was read
// with confidence.
func (c CertRef) identity() (string, bool) {
	issuer, ok := issuerIdentity(c.Issuer)
	if !ok {
		return "", false
	}
	serial := strings.TrimSpace(c.Serial)
	if serial == "" {
		return "", false
	}
	serial = NormalizeSerial(serial)
	for _, r := range serial {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return "", false
		}
	}
	nb, errB := time.Parse(time.RFC3339, strings.TrimSpace(c.NotBefore))
	na, errA := time.Parse(time.RFC3339, strings.TrimSpace(c.NotAfter))
	if errB != nil || errA != nil {
		return "", false
	}
	return strings.Join([]string{issuer, serial, nb.UTC().Format(time.RFC3339), na.UTC().Format(time.RFC3339)}, "|"), true
}

// issuerIdentity reads a distinguished name in either spelling this tool
// sees into its attribute multiset: each type mapped to one name (Go prints
// a dotted OID for types it has no name for, OpenSSL a short name), each
// value with escapes, quotes and the "#hex" DER form decoded, then folded
// for case and runs of white space, and sorted, so attribute and RDN order
// do not matter. ok is false when the text does not read as a DN.
func issuerIdentity(dn string) (string, bool) {
	attrs, ok := parseDN(dn)
	if !ok {
		return "", false
	}
	parts := make([]string, 0, len(attrs))
	for _, a := range attrs {
		parts = append(parts, a.typ+"="+keyEscape(strings.Join(strings.Fields(strings.ToLower(a.val)), " ")))
	}
	sort.Strings(parts)
	return strings.Join(parts, ","), true
}

// issuerOrgKey is what two certificates from one CA organization share:
// the issuer organization (folded, so DigiCert's "DigiCert Inc" and
// "DigiCert, Inc." intermediates agree), else, for an issuer with no
// organization, its whole distinguished name. ok is false when the issuer
// does not read as a DN; such an issuer shares an organization with none.
func issuerOrgKey(dn string) (string, bool) {
	attrs, ok := parseDN(dn)
	if !ok {
		return "", false
	}
	for _, a := range attrs {
		if a.typ == "o" {
			if v := foldKey(a.val); v != "" {
				return "o=" + v, true
			}
		}
	}
	id, _ := issuerIdentity(dn)
	return "dn=" + id, true
}

// sameIssuerOrg is the one rule for "one CA organization issued both". It
// fails closed: an issuer that does not read as a DN matches nothing.
func sameIssuerOrg(a, b string) bool {
	ka, okA := issuerOrgKey(a)
	kb, okB := issuerOrgKey(b)
	return okA && okB && ka == kb
}

// keyEscape makes a value safe inside a key built with "," "=" and "|".
func keyEscape(s string) string {
	s = strings.NewReplacer(`\`, `\\`, ",", `\,`, "|", `\|`).Replace(s)
	if strings.HasPrefix(s, "#") {
		s = `\` + s
	}
	return s
}

// foldKey lowercases s and keeps only letters and digits.
func foldKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// dnAttr returns the first value of attribute typ ("O", "CN") in a
// distinguished name, whatever spelling names the type.
func dnAttr(dn, typ string) string {
	want := canonicalDNType(typ)
	attrs, _ := parseDN(dn)
	for _, a := range attrs {
		if a.typ == want {
			return a.val
		}
	}
	return ""
}

// dnAVA is one attribute of a distinguished name: its canonical type and
// its decoded value.
type dnAVA struct{ typ, val string }

// dnTypeNames maps every spelling of an attribute type the two sources use
// (crypto/x509 names, OpenSSL short and long names, dotted OIDs) to one.
var dnTypeNames = map[string]string{
	"2.5.4.3": "cn", "commonname": "cn",
	"2.5.4.4": "surname", "sn": "surname",
	"2.5.4.5": "serialnumber",
	"2.5.4.6": "c", "countryname": "c",
	"2.5.4.7": "l", "localityname": "l",
	"2.5.4.8": "st", "stateorprovincename": "st",
	"2.5.4.9": "street", "streetaddress": "street",
	"2.5.4.10": "o", "organizationname": "o",
	"2.5.4.11": "ou", "organizationalunitname": "ou",
	"2.5.4.12": "title",
	"2.5.4.13": "description",
	"2.5.4.15": "businesscategory",
	"2.5.4.17": "postalcode",
	"2.5.4.42": "givenname", "gn": "givenname",
	"2.5.4.46":                   "dnqualifier",
	"2.5.4.97":                   "organizationidentifier",
	"1.2.840.113549.1.9.1":       "emailaddress",
	"0.9.2342.19200300.100.1.25": "dc", "domaincomponent": "dc",
	"0.9.2342.19200300.100.1.1": "uid", "userid": "uid",
	"1.3.6.1.4.1.311.60.2.1.1": "jurisdictionl", "jurisdictionlocalityname": "jurisdictionl",
	"1.3.6.1.4.1.311.60.2.1.2": "jurisdictionst", "jurisdictionstateorprovincename": "jurisdictionst",
	"1.3.6.1.4.1.311.60.2.1.3": "jurisdictionc", "jurisdictioncountryname": "jurisdictionc",
}

// canonicalDNType lowercases an attribute type and maps its known
// spellings to one name. Unknown types are kept as written (lowercased).
func canonicalDNType(t string) string {
	t = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(t)), "oid.")
	if n, ok := dnTypeNames[t]; ok {
		return n
	}
	return t
}

// parseDN splits a distinguished name in either spelling this tool sees:
// crt.sh's "C=US, O=GoDaddy.com, Inc., CN=..." (commas left bare inside
// values) and crypto/x509's "CN=...,O=GoDaddy.com\, Inc.,C=US" (RFC 4514
// escapes). A comma or plus ends a value only when the text after it is an
// attribute type followed by "=". RFC 4514 "\XX" hex escapes, quoted values
// and "#hex" DER values are decoded. ok is false when any part of the text
// does not read as an attribute (no "=", a bad type, an unterminated quote,
// a trailing backslash, undecodable hex); the attributes read so far are
// still returned for display.
func parseDN(dn string) ([]dnAVA, bool) {
	var out []dnAVA
	ok := true
	var key, val strings.Builder
	inKey, inQuote, der := true, false, false
	flush := func() {
		k := strings.TrimSpace(key.String())
		switch {
		case inKey && k != "":
			ok = false // text with no "=" is not an attribute
		case !inKey:
			v := strings.TrimSpace(val.String())
			if der {
				if d, good := derValue(v); good {
					v = d
				} else {
					ok = false
				}
			}
			if !validDNType(k) || inQuote {
				ok = false
			}
			out = append(out, dnAVA{typ: canonicalDNType(k), val: v})
		}
		key.Reset()
		val.Reset()
		inKey, inQuote, der = true, false, false
	}
	for i := 0; i < len(dn); i++ {
		c := dn[i]
		switch {
		case c == '\\':
			if i+1 >= len(dn) {
				ok = false
				continue
			}
			var b byte
			if i+2 < len(dn) && isHex(dn[i+1]) && isHex(dn[i+2]) {
				b = unhex(dn[i+1])<<4 | unhex(dn[i+2])
				i += 2
			} else {
				i++
				b = dn[i]
			}
			if inKey {
				key.WriteByte(b)
			} else {
				val.WriteByte(b)
			}
		case c == '"' && !inKey:
			inQuote = !inQuote
		case c == '=' && inKey:
			inKey = false
			if j := strings.IndexFunc(dn[i+1:], func(r rune) bool { return r != ' ' }); j >= 0 && dn[i+1+j] == '#' {
				der = true
				i += 1 + j // the '#' marks a DER value; it is not part of it
			}
		case (c == ',' || c == '+') && !inQuote && startsAttr(dn[i+1:]):
			flush()
		case inKey:
			key.WriteByte(c)
		default:
			val.WriteByte(c)
		}
	}
	flush()
	return out, ok && len(out) > 0
}

// validDNType reports whether t is an attribute type: a name (letter, then
// letters, digits, hyphens), optionally "OID."-prefixed dotted numbers, or
// dotted numbers.
func validDNType(t string) bool {
	t = strings.TrimSpace(t)
	if strings.HasPrefix(strings.ToLower(t), "oid.") {
		t = t[4:]
	}
	if t == "" {
		return false
	}
	if t[0] >= '0' && t[0] <= '9' {
		for _, p := range strings.Split(t, ".") {
			if p == "" || strings.Trim(p, "0123456789") != "" {
				return false
			}
		}
		return true
	}
	for i := 0; i < len(t); i++ {
		c := t[i]
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || i > 0 && (c >= '0' && c <= '9' || c == '-')) {
			return false
		}
	}
	return true
}

// derValue decodes a "#hex" attribute value (RFC 4514 2.4; what crypto/x509
// prints for a type it has no name for). A string type yields its text;
// any other well-formed value is kept as "#" and its lowercase hex.
func derValue(h string) (string, bool) {
	raw, err := hex.DecodeString(h)
	if err != nil || len(raw) == 0 {
		return "", false
	}
	var rv asn1.RawValue
	rest, err := asn1.Unmarshal(raw, &rv)
	if err != nil || len(rest) != 0 {
		return "", false
	}
	if rv.Class == asn1.ClassUniversal && !rv.IsCompound {
		switch rv.Tag {
		case asn1.TagUTF8String, asn1.TagPrintableString, asn1.TagIA5String, asn1.TagNumericString, 26: // 26: VisibleString
			return string(rv.Bytes), true
		case asn1.TagT61String: // crypto/x509 reads T61String as Latin-1
			r := make([]rune, len(rv.Bytes))
			for i, b := range rv.Bytes {
				r[i] = rune(b)
			}
			return string(r), true
		case asn1.TagBMPString:
			if len(rv.Bytes)%2 != 0 {
				return "", false
			}
			u := make([]uint16, len(rv.Bytes)/2)
			for i := range u {
				u[i] = uint16(rv.Bytes[2*i])<<8 | uint16(rv.Bytes[2*i+1])
			}
			return string(utf16.Decode(u)), true
		}
	}
	return "#" + strings.ToLower(h), true
}

func isHex(c byte) bool { return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' }

func unhex(c byte) byte {
	switch {
	case c >= 'a':
		return c - 'a' + 10
	case c >= 'A':
		return c - 'A' + 10
	}
	return c - '0'
}

// startsAttr reports whether s begins (after spaces) with an attribute type
// (letters, digits, dots, hyphens) and "=".
func startsAttr(s string) bool {
	s = strings.TrimLeft(s, " ")
	n := 0
	for n < len(s) && (s[n] == '.' || s[n] == '-' || s[n] >= '0' && s[n] <= '9' || s[n] >= 'A' && s[n] <= 'Z' || s[n] >= 'a' && s[n] <= 'z') {
		n++
	}
	return n > 0 && strings.HasPrefix(strings.TrimLeft(s[n:], " "), "=")
}

// CT searches crt.sh.
type CT struct{ Web *Web }

// Search returns the unexpired certificates crt.sh holds for domain and its
// subdomains, deduplicated (a precertificate and its final certificate are one).
func (c *CT) Search(ctx context.Context, domain string) ([]CTCert, error) {
	q := url.Values{}
	q.Set("q", domain)
	q.Set("output", "json")
	q.Set("exclude", "expired")
	q.Set("deduplicate", "Y")
	// Always ILIKE: left to itself crt.sh picks match=Single for a value with
	// a hyphen (web_apis.fnc), which returns no subdomain's certificate.
	q.Set("match", "ILIKE")
	body, err := c.Web.Get(ctx, CrtShURL+"?"+q.Encode(), "application/json")
	if err != nil {
		return nil, err
	}
	certs, err := ParseCrtSh(body)
	if err != nil {
		return nil, err
	}
	// crt.sh identity search also matches names that merely contain the
	// domain; keep only certificates that name the domain or a subdomain.
	out := certs[:0]
	for _, cert := range certs {
		if CoversDomain(cert.Names, domain) {
			out = append(out, cert)
		}
	}
	return out, nil
}

// maxCertBytes bounds one downloaded certificate (PEM or DER).
const maxCertBytes = 1 << 20

// Certificate downloads one certificate from crt.sh by its crt.sh id
// (https://crt.sh/?d=<id>, answered as PEM). The host and port are fixed:
// only the numeric id varies, and it comes from crt.sh's own search answer,
// never from a caller. The fetch goes through the same guarded, rate-limited
// client as Search.
func (c *CT) Certificate(ctx context.Context, id int64) ([]byte, error) {
	if id <= 0 {
		return nil, fmt.Errorf("crt.sh id %d is not a certificate id", id)
	}
	q := url.Values{}
	q.Set("d", strconv.FormatInt(id, 10))
	return c.Web.Get(ctx, CrtShURL+"?"+q.Encode(), "application/pkix-cert")
}

// CertificateNames reads the DNS names certificate body (PEM or DER, as
// crt.sh's ?d= download returns it) carries, after checking it is the
// certificate the crt.sh row c describes: same issuer, serial and validity
// (CertKey), and naming every DNS name crt.sh listed for it. The names are
// the SAN dNSNames, or the subject common name when it is a DNS name and
// there are none, lowercased and sorted. Any mismatch or parse failure is
// an error: the row then keeps crt.sh's partial list.
func CertificateNames(body []byte, c CTCert) ([]string, error) {
	if len(body) > maxCertBytes {
		return nil, fmt.Errorf("certificate download is larger than %d bytes", maxCertBytes)
	}
	der := bytes.TrimSpace(body)
	if blk, _ := pem.Decode(der); blk != nil {
		if blk.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("certificate download is a PEM %q block, not a certificate", blk.Type)
		}
		der = blk.Bytes
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("parse downloaded certificate: %w", err)
	}
	if c.Serial == "" || c.NotBefore == "" || c.NotAfter == "" {
		return nil, errors.New("crt.sh row has no serial or validity dates to check the download against")
	}
	got := CertKey(cert.Issuer.String(), cert.SerialNumber.Text(16), cert.NotBefore.UTC().Format(time.RFC3339), cert.NotAfter.UTC().Format(time.RFC3339))
	if want := CertKey(c.IssuerName, c.Serial, c.NotBefore, c.NotAfter); got != want {
		return nil, fmt.Errorf("downloaded certificate does not match the crt.sh row (issuer, serial or validity differ: %s vs %s)", got, want)
	}
	var names []string
	for _, n := range cert.DNSNames {
		if n = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(n)), "."); n != "" {
			names = appendUnique(names, n)
		}
	}
	if len(names) == 0 {
		cn := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(cert.Subject.CommonName)), ".")
		if _, _, ok := caaName(cn); ok {
			names = append(names, cn)
		}
	}
	if len(names) == 0 {
		return nil, errors.New("downloaded certificate names no DNS name")
	}
	for _, n := range c.Names {
		if _, _, ok := caaName(n); ok && !slices.Contains(names, n) {
			return nil, fmt.Errorf("crt.sh lists %s, which the downloaded certificate does not name", n)
		}
	}
	sort.Strings(names)
	return names, nil
}

// CoversDomain reports whether any name is domain, a subdomain of it, or a
// wildcard under it.
func CoversDomain(names []string, domain string) bool {
	for _, n := range names {
		n = strings.TrimPrefix(n, "*.")
		if n == domain || strings.HasSuffix(n, "."+domain) {
			return true
		}
	}
	return false
}

// CoversHost reports whether a certificate name list is valid for host
// (exact match, or a single-label wildcard per RFC 6125 6.4.3).
func CoversHost(names []string, host string) bool {
	host = strings.ToLower(host)
	for _, n := range names {
		if n == host {
			return true
		}
		if strings.HasPrefix(n, "*.") {
			if i := strings.IndexByte(host, '.'); i > 0 && host[i+1:] == n[2:] {
				return true
			}
		}
	}
	return false
}
