// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// threecx handfix (store-never-keeps-secrets): the one rule for which 3CX
// field names and shapes carry credentials, shared by fleet output redaction,
// the response redaction, what is held with the credential kit and the local
// store, so none of them can disagree. The kit (internal/credkit) does the
// matching and the withholding; nothing here decides what leaves.

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"threecx-pp-cli/internal/credkit"
)

// Hold is credkit.Hold for every credential this connector finds itself. A
// variable because the kit's held set is process-wide with no reset: an
// in-process test (no credkit.Start) would check every later statement and file
// against its fixtures. The built binaries are the proof of the hold
// (internal/threecxblackbox). Production code never reassigns it.
var Hold = credkit.Hold

// fieldSource is the kit source of a value found by name or by shape in a
// request, a response or a tool argument. No Origins entry names it, so it
// shares the "*" rule: the configured phone system only.
const fieldSource = "response:credential-field"

// secretFieldPattern matches XAPI fields that carry credentials or
// credential-like links: AuthPassword, VMPIN, PinNumber, LicenseKey, ApiKey,
// AccessKey, ManagementAPIKey, TranscribeKeyGrok, ClientSecret,
// SftpPrivateKey, SecretKey, a bare Key, ServiceAccountCredentials, access
// and refresh tokens, OAuth PKCE verifiers (PKCECodeVerifier, the Exchange
// body's verifier), and phone provisioning links (ProvLink,
// ProvisioningLinkExt, ...), which expose SIP credentials (checked against
// every property and parameter name in the 3CX XAPI spec; the test
// TestEverySpecNameThatCouldCarryACredentialIsJudged keeps it that way). It over-matches "token"; booleans,
// numbers and null under these names (AllowChangePassword, EnablePin,
// total_tokens) are kept by the walkers below.
var secretFieldPattern = regexp.MustCompile(`(?i)(passw(or)?d|secret|credential|token|api_?key|access_?key|license_?key|private_?key|secret_?key|keygrok|^key$|verifier|pin$|^pin|pinnumber|prov(ision(ing)?)?.?(link|url))`)

// pinFieldPattern is the PIN subset of secretFieldPattern. A PIN a response
// carries is redacted wherever it is named, but not held: 3CX PINs are a few
// digits and often equal the extension number, so holding every PIN of a listing
// would withhold every later unit holding those extensions (the ledger's
// response:VMPIN and response:PinNumber entries). A PIN the operator supplies, in
// a flag, a tool argument or a request body, is one value they chose to send, so
// it is held (HoldOperatorValues).
var pinFieldPattern = regexp.MustCompile(`(?i)(pin$|^pin|pinnumber)`)

// publicSecretLikeFields match the pattern but carry routing or metadata, not
// a credential (checked against every property in the 3CX XAPI spec), plus the
// fleet list's secret_env, which names the variable that holds a PBX's secret
// (<env_prefix>_CLIENT_SECRET), never the secret.
var publicSecretLikeFields = map[string]bool{
	"invalidkeyforwarddn": true, "keys": true, "nextpagetoken": true, "pinprotecttimeout": true,
	"sftphostkeyfingerprint": true, "tokenendpoint": true, "tokenendpointauthmethod": true,
	"secret_env": true,
}

// SecretFieldName reports a property name that looks like a credential.
func SecretFieldName(name string) bool {
	return !publicSecretLikeFields[strings.ToLower(name)] && secretFieldPattern.MatchString(name)
}

// RedactedValue replaces a credential-bearing value.
const RedactedValue = "[redacted]"

// RedactSecretFields walks a decoded JSON value and masks every
// credential-bearing value in place: strings, objects and arrays under a
// credential name (booleans, numbers and null kept: flags and counts), the
// Value and Contents of 3CX's ConcealedPassword {Concealed, Value} and
// ConcealedDataFile {Concealed, Contents, Name} shapes, any string Contents,
// and the Value of a name/value record ({"Name":"CLIENT_SECRET","Value":...})
// whose name is a credential name. The local store uses it.
func RedactSecretFields(v any) any {
	return redactSecretFields(v, map[string]bool{}, false)
}

// RedactSecretFieldsStrict is RedactSecretFields for a response of unknown
// shape (fleet exec runs any XAPI path): numbers under a credential name are
// masked too (PINs), and so is an unnamed scalar Value, since an OData
// property read (/Users(1)/AuthPassword answers {"value": ...}) or a $select
// projection cannot be judged by name.
func RedactSecretFieldsStrict(v any) any {
	return redactSecretFields(v, map[string]bool{}, true)
}

// redactSecretFields memoises SecretFieldName per name: a large collection
// repeats the same few dozen keys, and the regex dominates the walk.
func redactSecretFields(v any, seen map[string]bool, strict bool) any {
	isSecret := func(k string) bool {
		hit, ok := seen[k]
		if !ok {
			hit = SecretFieldName(k)
			seen[k] = hit
		}
		return hit
	}
	switch t := v.(type) {
	case map[string]any:
		mode := byName
		if strict {
			mode = orUnnamed
		}
		for _, k := range concealedValueKeys(t, isSecret, mode) {
			t[k] = RedactedValue
		}
		for k, val := range t {
			if _, isString := val.(string); isString && k == "Contents" {
				t[k] = RedactedValue
				continue
			}
			if isSecret(k) {
				switch val.(type) {
				case nil, bool:
				case float64, json.Number:
					if strict || pinFieldPattern.MatchString(k) {
						t[k] = RedactedValue
						continue
					}
				default:
					t[k] = RedactedValue
					continue
				}
			}
			t[k] = redactSecretFields(val, seen, strict)
		}
		return t
	case []any:
		for i := range t {
			t[i] = redactSecretFields(t[i], seen, strict)
		}
		return t
	default:
		return v
	}
}

// valueMode says which scalar Value/value members of a record are judged
// credentials by shape: byName (a name/value record whose name is a
// credential name), orUnnamed (also a Value with no name: a $select
// projection or an OData property read cannot be judged), or anyValue (every
// Value: the answer of a function that mints a credential, whatever the
// record calls it, e.g. Pbx.CrmChoice {Key: "OAuth", Value: <token>}).
type valueMode int

const (
	byName valueMode = iota
	orUnnamed
	anyValue
)

// oauthCodeCompanions are member names that mark an object as an OAuth
// authorization-code exchange (McpServers/Pbx.Exchange {code, scope,
// tokenEndpoint, verifier}; RFC 6749 token requests): its "code" is then a
// bearer credential. Elsewhere "code" is an error or status code and stays.
var oauthCodeCompanions = map[string]bool{
	"verifier": true, "code_verifier": true, "codeverifier": true, "pkcecodeverifier": true,
	"tokenendpoint": true, "token_endpoint": true, "redirect_uri": true, "redirecturi": true, "grant_type": true,
}

// OAuthCodeName reports whether name is an authorization-code member ("code",
// "auth_code", "authorization_code") of an object (or a command line) whose
// other names, given by has, include an OAuth companion: the one shape rule
// the holder, the redaction and argv capture share.
func OAuthCodeName(name string, names []string) bool {
	switch strings.ToLower(strings.TrimLeft(name, "-")) {
	case "code", "auth_code", "auth-code", "authorization_code", "authorization-code", "authcode":
	default:
		return false
	}
	for _, n := range names {
		if oauthCodeCompanions[strings.ReplaceAll(strings.ToLower(strings.TrimLeft(n, "-")), "-", "_")] {
			return true
		}
	}
	return false
}

func memberNames(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// concealedValueKeys lists the members of m that carry a credential by shape
// rather than by name: Value/Contents of a Concealed record, the "code" of an
// OAuth authorization-code exchange, and the scalar Value of a record as mode
// says (see valueMode).
func concealedValueKeys(m map[string]any, isSecret func(string) bool, mode valueMode) []string {
	var out []string
	names := memberNames(m)
	for _, k := range names {
		if _, isString := m[k].(string); isString && OAuthCodeName(k, names) {
			out = append(out, k)
		}
	}
	if _, concealed := m["Concealed"]; concealed {
		for _, k := range []string{"Value", "Contents"} {
			if val, ok := m[k]; ok && val != nil {
				out = append(out, k)
			}
		}
	}
	name, named := "", false
	for _, nk := range []string{"Name", "Key", "name", "key"} {
		if n, ok := m[nk].(string); ok {
			name, named = n, true
			break
		}
	}
	for _, vk := range []string{"Value", "value"} {
		val, ok := m[vk]
		if !ok || val == nil {
			continue
		}
		switch val.(type) {
		case []any, map[string]any:
			continue // a collection or nested object is walked
		}
		if mode == anyValue || (mode == orUnnamed && !named) || (named && isSecret(name)) {
			out = append(out, vk)
		}
	}
	return out
}

// isMaskValue reports a value that already hides a credential, by shape only:
// empty, nothing but asterisks or bullets (3CX's "****" settings), or this
// program's own RedactedValue. A word is never a mask: a password that is
// literally "hidden" is a password.
func isMaskValue(v string) bool {
	v = strings.TrimSpace(v)
	return v == "" || strings.Trim(v, "*\u2022") == "" || v == RedactedValue
}

// HoldCredentialValues is the one walker that registers credentials found in
// structured data with the credential kit (handfixes.json credkit-wiring): every
// string under a credential-named key at any
// depth (all strings under a credential-named object or array), in a
// Concealed record or Contents, or the Value of a name/value record with a
// credential name, except masks and PINs. A string that is itself a JSON
// object or array is read too. Request bodies and params, responses, MCP tool
// arguments and output members all use it. An unnamed OData scalar
// ({"value": "..."}) is held only from a minting function (HoldMintedValues):
// most are plain property reads, and a long-lived MCP server would otherwise
// withhold ordinary data (a version string) for the rest of its session.
func HoldCredentialValues(v any) {
	holdCredentialValues(v, false, false, false, byName)
}

// HoldOperatorValues is HoldCredentialValues for what the operator supplies (a
// tool's arguments, a request's params and body), where a PIN is held too, and a
// number under a credential name counts as well as a string.
func HoldOperatorValues(v any) {
	holdCredentialValues(v, false, false, true, byName)
}

// HoldMintedValues is HoldCredentialValues for the response of a function
// that mints or grants a credential (MintsCredential): every scalar Value or
// value in it is the credential, whatever its record calls it (an unnamed
// {"value": "..."}, Pbx.CrmChoice {Key, Value}).
func HoldMintedValues(v any) {
	holdCredentialValues(v, false, false, false, anyValue)
}

// recordNameKeys name a name/value record; with a Value beside them they are
// the record's name, not a credential (Pbx.Choice, Pbx.CrmChoice {Key, Value}),
// so they are not held: a long-lived MCP server would otherwise withhold the
// word "OAuth" for the rest of its session.
var recordNameKeys = map[string]bool{"Name": true, "Key": true, "name": true, "key": true}

func holdCredentialValues(v any, secret, pin, pins bool, mode valueMode) {
	switch t := v.(type) {
	case duplicateMembers:
		// A record that repeats a member cannot be judged by shape, so every
		// Value it has counts.
		for _, variant := range t {
			holdCredentialValues(variant, secret, pin, pins, anyValue)
		}
	case map[string]any:
		byShape := map[string]bool{}
		for _, k := range concealedValueKeys(t, SecretFieldName, mode) {
			byShape[k] = true
		}
		_, hasValue := t["Value"]
		if _, lower := t["value"]; lower {
			hasValue = true
		}
		for k, child := range t {
			named := SecretFieldName(k) && !(hasValue && recordNameKeys[k])
			childSecret := secret || byShape[k] || k == "Contents" || named
			holdCredentialValues(child, childSecret, pin || (named && pinFieldPattern.MatchString(k)), pins, mode)
		}
	case []any:
		for _, child := range t {
			holdCredentialValues(child, secret, pin, pins, mode)
		}
	case float64:
		if secret && pins {
			Hold(fieldSource, strconv.FormatFloat(t, 'f', -1, 64))
		}
	case json.Number:
		if secret && pins {
			Hold(fieldSource, t.String())
		}
	case string:
		if secret && (!pin || pins) && !isMaskValue(t) {
			Hold(fieldSource, t)
			if trimmed := strings.TrimSpace(t); len(trimmed) > 7 && strings.EqualFold(trimmed[:7], "bearer ") {
				Hold(fieldSource, strings.TrimSpace(trimmed[7:]))
			}
		}
		if trimmed := strings.TrimSpace(t); len(trimmed) > 1 && (trimmed[0] == '{' || trimmed[0] == '[') {
			for _, inner := range decodeDocuments([]byte(trimmed)) {
				holdCredentialValues(inner, secret, pin, pins, mode)
			}
		}
	}
}

// duplicateMembers is an object that repeats a member, as one map per
// occurrence: the i-th has the i-th of each repeated member (the last when that
// member has fewer). encoding/json keeps only the last, and every member of a
// duplicate key is held.
type duplicateMembers []map[string]any

// maxJSONDepth is encoding/json's own nesting limit.
const maxJSONDepth = 10000

// decodeJSONValue reads one JSON value, keeping every member of a duplicate key.
func decodeJSONValue(dec *json.Decoder, depth int) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, isDelim := tok.(json.Delim)
	if !isDelim {
		return tok, nil
	}
	if depth >= maxJSONDepth {
		return nil, errors.New("JSON nested too deeply")
	}
	if delim == '[' {
		items := []any{}
		for dec.More() {
			item, err := decodeJSONValue(dec, depth+1)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		_, err := dec.Token()
		return items, err
	}
	members := map[string][]any{}
	most := 1
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil, err
		}
		name, _ := key.(string)
		val, err := decodeJSONValue(dec, depth+1)
		if err != nil {
			return nil, err
		}
		members[name] = append(members[name], val)
		most = max(most, len(members[name]))
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	variants := make(duplicateMembers, most)
	for i := range variants {
		variants[i] = make(map[string]any, len(members))
		for name, vals := range members {
			variants[i][name] = vals[min(i, len(vals)-1)]
		}
	}
	if most == 1 {
		return variants[0], nil
	}
	return variants, nil
}

// decodeDocuments reads every JSON document in body, up to the first that does
// not parse.
func decodeDocuments(body []byte) []any {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var docs []any
	for {
		doc, err := decodeJSONValue(dec, 0)
		if err != nil {
			return docs
		}
		docs = append(docs, doc)
	}
}

// HoldCredentialFields decodes a JSON body and holds its credential values;
// minted marks the response of a minting function (HoldMintedValues).
func HoldCredentialFields(body []byte, minted ...bool) {
	for _, doc := range decodeDocuments(body) {
		if len(minted) > 0 && minted[0] {
			HoldMintedValues(doc)
			continue
		}
		HoldCredentialValues(doc)
	}
}

// mintingFunction matches the XAPI functions whose answer may be a new
// credential or an access grant: every function (Pbx.<Name>, its arguments
// excluded) whose name holds Generate, Regenerate, Token, OAuth, Key, Secret,
// Password, Credential or Link (GenerateApiKey, GenerateAppToken,
// GenerateProvLink, APIToken, GetMicrosoftAccessToken, GetOAuth,
// GetRequestHelpLink, RegeneratePasswords, RegenerateWebCredentials, ...).
// Over-matching only holds the values of an answer that carries no
// credential.
var mintingFunction = regexp.MustCompile(`(?i)/Pbx\.[A-Za-z0-9_]*(Generate|Token|OAuth|Key|Secret|Passw|Credential|Link)`)

// MintsCredential reports whether an XAPI request path calls a function
// whose answer is a new credential.
func MintsCredential(path string) bool { return mintingFunction.MatchString(path) }

// allocatingFunction matches the XAPI functions that allocate an object as a
// plain GET (InitTrunk and the like).
var allocatingFunction = regexp.MustCompile(`/Pbx\.Init`)

// StateChangingGET reports an XAPI GET that mints or grants a credential or
// allocates an object: the generated tables read these as reads, so each gate
// that lets a read through without confirm asks this first.
func StateChangingGET(path string) bool {
	return MintsCredential(path) || allocatingFunction.MatchString(path)
}

// HoldRequestCredentials holds the credential values of an outgoing request
// (query params named like a credential, and the JSON body) before it is
// previewed or sent.
func HoldRequestCredentials(params map[string]string, body []byte) {
	m := make(map[string]any, len(params))
	for k, v := range params {
		m[k] = v
	}
	HoldOperatorValues(m)
	for _, doc := range decodeDocuments(body) {
		HoldOperatorValues(doc)
	}
}

// RedactBody is a JSON response body with every value under a credential name
// or shape replaced by RedactedValue (RedactSecretFields), so a listing that
// carries a password stays readable once the password is held: the kit withholds
// a whole unit that repeats a held value. Numbers keep their spelling. A body
// that is not JSON, or holds nothing to redact, comes back unchanged.
func RedactBody(body []byte) []byte {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil || dec.More() {
		return body
	}
	before, err := json.Marshal(v)
	if err != nil {
		return body
	}
	after, err := json.Marshal(RedactSecretFields(v))
	if err != nil || bytes.Equal(before, after) {
		return body
	}
	return after
}
