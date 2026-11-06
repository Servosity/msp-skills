// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// comet-backup handfix (store-never-keeps-secrets): the one rule for which Comet
// field names carry credentials, shared by output redaction and the local
// store, so the two can never disagree.

import (
	"bytes"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"

	"comet-backup-pp-cli/internal/credkit"
)

// Hold is credkit.Hold for every credential this connector finds itself. A
// variable because the kit's held set is process-wide with no reset: an
// in-process test (no credkit.Start) would check every later statement and file
// against its fixtures. The built binaries are the proof of the hold
// (internal/cometblackbox). Production code never reassigns it.
var Hold = credkit.Hold

// secretFieldPattern matches Comet fields that carry credentials, read on the
// normalized name (normalizedFieldName: lowercase, letters and digits only, so
// S3SecretKey, s3_secret_key and s3-secret-key are one name). Sources: the
// comet-go-sdk structs the spec cites (UserProfileConfig, DestinationConfig,
// Organization branding, BucketProperties, ConstellationCheckReport): every
// password (FTPPassword, SFTPPassword, SMBPassword, SSHPassword, SMTPPassword,
// BindPassword, PfxFilePassword, the PKCS11/PKCS12 code-signing passwords, a
// user's PasswordHash and PasswordRecovery), the Apple ID pass, passphrases,
// secrets (S3SecretKey, ClientSecret, the Azure code-signing app secret), auth
// tokens, and every name ending in "key": storage keys (S3AccessKey,
// AZBAccountKey, CometBucketKey, ReadWriteKey, AccessKey, SecretKey, AppKey,
// PartnerKey, Key), encryption and TOTP keys (EncryptedEncryptionKey, TOTPKey)
// and private keys (SFTPPrivateKey, SSHPrivateKey). It also matches the admin
// request fields (Password, SessionKey, TOTP, WebAuthnSign, Token) and the new-password
// fields of user writes (TargetPassword, NewPassword). The public half of a
// storage key pair (S3AccessKey) is a credential too. Booleans and numbers
// under a matching name (AllowPasswordLogin, PasswordFormat) are kept by the
// walkers, so the deliberate over-match costs nothing.
//
// Some credentials sit under a name the operator chose, not Comet: the HTTP
// header maps of webhooks, PSA integrations and custom remote storage
// (WebhookOption.CustomHeaders, PSAConfig.CustomHeaders,
// CustomRemoteBucketSettings.CustomHeaders, where an "Authorization" key holds
// a bearer or basic credential) and the custom remote storage request body
// (CustomBody). Everything under a header map or a custom body is a credential
// whatever its key, and the standard credential headers (Authorization,
// Proxy-Authorization, Cookie) are named directly.
var secretFieldPattern = regexp.MustCompile(`(webauthnsign|passw(or)?d|pass$|passphrase|secret|credential|token|apikey|privatekey|key$|^totp$|sessionkey|headers?$|custombody|authorization|cookie)`)

// publicSecretLikeFields match the pattern but are metadata or identifiers,
// not a credential (normalized names).
var publicSecretLikeFields = map[string]bool{
	"passwordcomplexity": true,
}

// SecretFieldName reports a property, flag, parameter or header name that
// looks like a credential, in any spelling.
func SecretFieldName(name string) bool {
	n := normalizedFieldName(name)
	return !publicSecretLikeFields[n] && secretFieldPattern.MatchString(n)
}

// normalizedFieldName lowercases ASCII letters and drops every separator
// (anything that is not a letter or digit), keeping non-ASCII bytes.
func normalizedFieldName(name string) string {
	b := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		switch c := name[i]; {
		case 'A' <= c && c <= 'Z':
			b = append(b, c+('a'-'A'))
		case 'a' <= c && c <= 'z', '0' <= c && c <= '9', c >= 0x80:
			b = append(b, c)
		}
	}
	return string(b)
}

// RedactedValue replaces a credential-bearing value.
const RedactedValue = "[redacted]"

// RedactSecretFields walks a decoded JSON value and masks every value under a
// credential name (containers included). Null, booleans and numbers are kept:
// flags and counts ("total_tokens"), never a credential.
func RedactSecretFields(v any) any {
	return redactSecretFields(v, map[string]bool{})
}

// redactSecretFields memoises SecretFieldName per name: a large collection
// repeats the same few dozen keys, and the regex dominates the walk.
func redactSecretFields(v any, seen map[string]bool) any {
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
		for k, val := range t {
			if isSecret(k) {
				switch val.(type) {
				case nil, bool, float64, json.Number:
				default:
					t[k] = RedactedValue
					continue
				}
			}
			t[k] = redactSecretFields(val, seen)
		}
		return t
	case []any:
		for i := range t {
			t[i] = redactSecretFields(t[i], seen)
		}
		return t
	case string:
		return redactJSONInString(t, seen)
	default:
		return v
	}
}

// redactJSONInString redacts a record carried as a JSON string (EngineProps
// SMBAUTH-*: WinSMBAuth, per comet-go-sdk). It returns s as written unless a
// credential was redacted, so an ordinary string is never reformatted.
func redactJSONInString(s string, seen map[string]bool) string {
	t := strings.TrimSpace(s)
	if t == "" || (t[0] != '{' && t[0] != '[') {
		return s
	}
	var doc any
	if json.Unmarshal([]byte(t), &doc) != nil {
		return s
	}
	before, err := json.Marshal(doc)
	if err != nil {
		return s
	}
	after, err := json.Marshal(redactSecretFields(doc, seen))
	if err != nil || bytes.Equal(before, after) {
		return s
	}
	return string(after)
}

// RedactBody is a JSON response body with every value under a credential name
// replaced by RedactedValue (RedactSecretFields), so a listing that carries a
// password stays readable once the password is held: the kit withholds a whole
// unit that repeats a held value. A body that is not JSON, or holds nothing to
// redact, comes back unchanged. Numbers keep their spelling.
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

// CarriesRedactedValue reports a request body or parameter that carries the
// marker reads print in place of a credential. Sent to Comet it would overwrite
// the real credential: a profile read with 'users get --json' and written back
// with 'users set-profile --profile-data' would store "[redacted]" as every
// Storage Vault's key (comet-backup handfix: redacted-marker-never-written).
func CarriesRedactedValue(params map[string]string, body []byte) bool {
	for _, v := range params {
		if strings.Contains(v, RedactedValue) {
			return true
		}
	}
	if bytes.Contains(body, []byte(RedactedValue)) {
		return true
	}
	if decoded, err := url.QueryUnescape(string(body)); err == nil {
		return strings.Contains(decoded, RedactedValue)
	}
	return false
}

// OmitRedactedMembers drops every member holding the redaction marker from a
// JSON document, whatever its depth, for a sink on another host: a marker under
// a credential's name reads to the kit as that credential's value, so it would
// refuse the whole delivery and withhold everything after it. A body that is not
// one JSON document is returned as it is, and the kit decides.
func OmitRedactedMembers(body []byte) []byte {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var doc any
	if dec.Decode(&doc) != nil || dec.More() {
		return body
	}
	out, err := json.Marshal(omitRedacted(doc))
	if err != nil {
		return body
	}
	return out
}

func omitRedacted(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if child == RedactedValue {
				delete(t, k)
				continue
			}
			t[k] = omitRedacted(child)
		}
	case []any:
		for i, child := range t {
			t[i] = omitRedacted(child)
		}
	}
	return v
}
