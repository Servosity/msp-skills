// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated). The one definition of what a SentinelOne
// credential looks like, shared by every place that could persist one or hand
// one to an MCP agent: sync and workflow archive (never fetch a secret-only
// resource), the read write-through, the local store's single write choke
// point, the HTTP response cache, the MCP endpoint table (secret reads need
// confirm) and the MCP tool-result middleware.

package cliutil

import (
	"bytes"
	"encoding/json"
	"net/url"
	"path"
	"regexp"
	"strings"
)

// secretReadPaths are the GET endpoints whose answer IS a credential. The
// vendor spec declares no response schemas (every 2xx is text/plain), so the
// list is read from each operation's description in
// ~/printing-press/library/sentinelone/spec.yaml:
var secretReadPaths = []string{
	// GET /agents/passphrases (Get_Passphrase): "Show the passphrase for the
	// Agents that match the filter ... You need the passphrase for most
	// SentinelCtl commands". The per-endpoint secret that unlocks the agent.
	"/agents/passphrases",
	// GET /accounts/{account_id}/uninstall-password/view
	// (Get_Uninstall_Password): "Get the uninstall password to uninstall
	// several Agents of one Account with one command".
	"/accounts/{account_id}/uninstall-password/view",
	// GET /users/login/by-token (Login_by_Token): "Log in with user token".
	// The answer is a new session token.
	"/users/login/by-token",
	// GET /ranger/cred-groups/details (Get_Cred_group_details): the rows of
	// the Ranger credential groups, the credentials Ranger uses to reach and
	// deploy to unmanaged devices. Treated as credential-bearing (fail closed):
	// the spec does not say which fields the rows echo back.
	"/ranger/cred-groups/details",
	// GET /ranger/cred-groups (Get_Cred_groups): "the data for each row in the
	// Cred Groups table", the groups those credentials belong to. Same reason,
	// same fail-closed treatment: the spec documents no response fields.
	"/ranger/cred-groups",
}

// Not in the list, with the reason, so the next reviewer does not re-derive it:
//   - GET /sites, /sites/{site_id}, /groups, /groups/{group_id}: ordinary
//     rows that carry one secret field (registrationToken). The field rule
//     below drops it; gating the whole read would gate the site list.
//   - GET /users/{user_id}/api-token-details (API_Token_by_User_ID): "the
//     details of the API token" (created/expiry metadata), not the token. A
//     token-named field in its answer is dropped by the field rule anyway.
//   - GET /user (User_by_token): the user record of the calling token.
//   - GET /accounts/{account_id}/uninstall-password/metadata: who created or
//     revoked the password and when, not the password.
//   - GET /settings/smtp, /settings/active-directory, /settings/sso,
//     /settings/syslog, /settings/microsoft, /settings/sms and the marketplace
//     application reads: configuration whose credential fields, if any, are
//     password/secret/token/key named and dropped by the field rule.
//   - Non-GET endpoints that answer with a credential (users login,
//     generate-api-token, generate-iframe-token, generate-recovery-codes,
//     request-app, auth/app, auth/recovery-code, sites and groups
//     regenerate-key, uninstall-password generate): already need confirm over
//     MCP (every non-GET does), are never response-cached (only GET is), and
//     their write-through to the store passes the field rule.

// neverStoredResources are the sync resources (sync's own partition names)
// whose endpoint is a secretReadPaths entry. sync and workflow archive refuse
// them, and the store keeps no content for them.
var neverStoredResources = []struct{ resource, path string }{
	{"agents-passphrases", "/agents/passphrases"},
	{"ranger", "/ranger/cred-groups/details"},
	{"ranger-cred-groups", "/ranger/cred-groups"},
}

// SecretReadPaths returns the GET endpoint templates whose answer is a
// credential (see secretReadPaths).
func SecretReadPaths() []string {
	return append([]string(nil), secretReadPaths...)
}

// credentialTier is the credential tier: every resource whose rows ARE
// credentials, by every name an MCP agent could use for it. No MCP tool but
// sentinelone_execute with confirm: true may name one (the MCP middleware in
// cmd/sentinelone-mcp refuses the call before any handler runs), and a CLI
// child the MCP server spawned refuses one at the resource resolver
// (internal/cli/s1_credential_tier_mcp.go). Every secretReadPaths entry and
// every neverStoredResources entry is a member (TestCredentialTierCovers...
// pins that). Each entry:
//   - resource: the CLI's own name (sync, tail, export, the store partition).
//   - path: the GET endpoint whose answer is the credential, "" for a member
//     that is only a store table.
//   - names: every other name: spellings the brief's adversary used, the
//     typed store table, the sync resource of the same rows. Matching is by
//     credentialTierKey, so kebab, snake, camel, any case, a leading slash
//     and an API prefix (/web/api/v2.1) are all the same name.
//   - wordOnly: the resource name is an ordinary word (ranger is also the
//     product feature and the prefix of ranger-table-view, ranger-gateways,
//     rangerStatus), so it matches only as a whole name, never inside a
//     longer one.
var credentialTier = []struct {
	resource string
	path     string
	names    []string
	wordOnly bool
}{
	{resource: "agents-passphrases", path: "/agents/passphrases", names: []string{"agents-passphrase", "agents/passphrases"}},
	// The Ranger credential details (sync resource "ranger") and the Ranger
	// credential groups (sync resource "ranger-cred-groups", GET
	// /ranger/cred-groups) are both credential tables: neither is synced,
	// stored or readable by an MCP tool without confirm.
	{resource: "ranger", path: "/ranger/cred-groups/details", names: []string{"ranger-cred-groups-details", "ranger-cred-group-details"}, wordOnly: true},
	{resource: "ranger-cred-groups", path: "/ranger/cred-groups", names: []string{"ranger-cred-group", "ranger_cred_groups"}},
	// uninstall_password is the typed store table of the uninstall-password
	// command group.
	{resource: "uninstall-password", path: "/accounts/{account_id}/uninstall-password/view", names: []string{"uninstall_password", "accounts-uninstall-password", "uninstall-password-view", "accounts-uninstall-password-view"}},
	{resource: "login-by-token", path: "/users/login/by-token", names: []string{"users-login-by-token", "users/login/by-token"}},
	// Store tables that hold a POST's answer whose payload is a credential:
	// regenerate-key answers with the new site or group registration token.
	// The store's field rule drops the token on write; naming the table is
	// refused anyway, so sql cannot be pointed at one.
	{resource: "sites-regenerate-key", names: []string{"sites_regenerate_key"}},
	{resource: "groups-regenerate-key", names: []string{"groups_regenerate_key"}},
}

// CredentialTier is one member of the credential tier, as a refusal names it.
type CredentialTier struct {
	// Resource is the CLI's name for it, such as "ranger".
	Resource string
	// Path is the GET endpoint whose answer is the credential, or "" for a
	// member that is only a store table.
	Path string
}

// CredentialTiers returns every credential-tier member.
func CredentialTiers() []CredentialTier {
	out := make([]CredentialTier, 0, len(credentialTier))
	for _, e := range credentialTier {
		out = append(out, CredentialTier{Resource: e.resource, Path: e.path})
	}
	return out
}

// credentialTierNameKeys maps credentialTierKey(name) to its member, for the
// resource name, every listed name, the member's path, and every never-stored
// sync resource, so the tier cannot drift from what sync refuses.
var credentialTierNameKeys = func() map[string]CredentialTier {
	keys := map[string]CredentialTier{}
	for _, e := range credentialTier {
		m := CredentialTier{Resource: e.resource, Path: e.path}
		for _, n := range append([]string{e.resource, e.path}, e.names...) {
			if k := credentialTierKey(n); k != "" {
				keys[k] = m
			}
		}
	}
	for _, r := range neverStoredResources {
		if _, ok := keys[credentialTierKey(r.resource)]; !ok {
			keys[credentialTierKey(r.resource)] = CredentialTier{Resource: r.resource, Path: r.path}
		}
	}
	return keys
}()

// credentialTierSQLKeys are the member names safe to look for INSIDE longer
// text once every non-alphanumeric is removed (an SQL query split with ||,
// quotes, brackets or comments). A wordOnly resource name is left out: it is
// matched as a whole word only (credentialTierInSQLWords).
var credentialTierSQLKeys = func() map[string]CredentialTier {
	keys := map[string]CredentialTier{}
	for _, e := range credentialTier {
		m := CredentialTier{Resource: e.resource, Path: e.path}
		names := append([]string(nil), e.names...)
		if !e.wordOnly {
			names = append(names, e.resource)
		}
		if e.path != "" {
			var lit []string
			for _, seg := range secretPathSegments(e.path) {
				if !strings.HasPrefix(seg, "{") {
					lit = append(lit, seg)
				}
			}
			names = append(names, strings.Join(lit, "/"))
		}
		for _, n := range names {
			if k := alnumOnly(strings.ToLower(n)); k != "" {
				keys[k] = m
			}
		}
	}
	return keys
}()

var apiPrefixRE = regexp.MustCompile(`^(?:web/)?api/v[0-9.]+/`)

// credentialTierKey is the spelling-blind form of one name: lower-cased, the
// scheme and host of a URL dropped, the query string of a path dropped, any
// leading slash and API prefix (/web/api/v2.1/) dropped, then every character
// but a-z, 0-9 and . removed (so ranger_cred_groups, Ranger-Cred-Groups,
// rangerCredGroups and /ranger/cred-groups are one key) and dots trimmed at
// the ends. Inner dots are kept so an MCP endpoint id such as
// users.login-by-token is not a resource name.
func credentialTierKey(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	if i := strings.Index(s, "://"); i >= 0 {
		rest := s[i+3:]
		if j := strings.IndexByte(rest, '/'); j >= 0 {
			s = rest[j:]
		} else {
			s = ""
		}
	}
	if strings.ContainsRune(s, '/') {
		if i := strings.IndexAny(s, "?#"); i >= 0 {
			s = s[:i]
		}
		s = apiPrefixRE.ReplaceAllString(strings.TrimLeft(s, "/"), "")
	}
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' {
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), ".")
}

func alnumOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CredentialTierResource reports whether name is a credential-tier resource
// in any spelling (agents-passphrases, Agents_Passphrases, agentsPassphrases,
// /ranger, /web/api/v2.1/ranger, RANGER), and which.
func CredentialTierResource(name string) (CredentialTier, bool) {
	m, ok := credentialTierNameKeys[credentialTierKey(name)]
	return m, ok
}

// CredentialTierTable reports whether a local-store table name (typed table
// such as uninstall_password, or a resources.resource_type partition) is a
// credential-tier member. Same spelling rules as CredentialTierResource.
func CredentialTierTable(table string) (CredentialTier, bool) {
	return CredentialTierResource(table)
}

// CredentialTierPath reports whether an API path reaches a credential-tier
// endpoint: a template, a resolved path, a full URL, with a query string, an
// API prefix, percent-encoding (also double), dot segments or backslashes,
// and also when the credential path is only a run INSIDE the value (a path
// parameter such as 123/uninstall-password/view that a server decoding %2F
// would route to the uninstall password). A run is any two or more trailing
// segments of the endpoint template ({placeholder} matches one segment).
func CredentialTierPath(apiPath string) (CredentialTier, bool) {
	for _, cand := range pathCandidates(apiPath) {
		segs := cleanPathSegments(cand)
		if len(segs) == 0 {
			continue
		}
		for _, e := range credentialTier {
			if e.path == "" {
				continue
			}
			want := secretPathSegments(e.path)
			for k := 2; k <= len(want); k++ {
				if segmentRunIn(segs, want[len(want)-k:]) {
					return CredentialTier{Resource: e.resource, Path: e.path}, true
				}
			}
		}
	}
	return CredentialTier{}, false
}

func pathCandidates(p string) []string {
	out := []string{p}
	cur := p
	for i := 0; i < 3 && strings.ContainsRune(cur, '%'); i++ {
		next, err := url.PathUnescape(cur)
		if err != nil {
			next, err = url.QueryUnescape(cur)
		}
		if err != nil || next == cur {
			break
		}
		out = append(out, next)
		cur = next
	}
	return out
}

func cleanPathSegments(p string) []string {
	p = strings.ReplaceAll(p, `\`, "/")
	segs := secretPathSegments(p)
	if len(segs) == 0 {
		return nil
	}
	cleaned := path.Clean("/" + strings.Join(segs, "/"))
	return secretPathSegments(cleaned)
}

func segmentRunIn(segs, want []string) bool {
	for start := 0; start+len(want) <= len(segs); start++ {
		match := true
		for i, w := range want {
			if strings.HasPrefix(w, "{") && strings.HasSuffix(w, "}") {
				continue
			}
			if segs[start+i] != w {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// credentialTierTokenRE splits free text into the words and paths an argument
// value could name a resource with: letters, digits and . _ - / { } %.
var credentialTierTokenRE = regexp.MustCompile(`[A-Za-z0-9._/\-{}%\\]+`)

// CredentialTierInText reports whether any part of an MCP argument value
// names a credential-tier member: the whole value (so "ranger cred groups"
// counts), each word or path in it (so "sites,agents-passphrases",
// "resource=ranger" and a JSON string carrying "ranger" count), each in its
// percent-decoded form too, by resource or table name or by API path.
func CredentialTierInText(text string) (CredentialTier, bool) {
	for _, cand := range pathCandidates(text) {
		if m, ok := credentialTierNameKeys[credentialTierKey(cand)]; ok {
			return m, true
		}
		if strings.ContainsAny(cand, `/\`) {
			if m, ok := CredentialTierPath(cand); ok {
				return m, true
			}
		}
		for _, tok := range credentialTierTokenRE.FindAllString(cand, -1) {
			if m, ok := credentialTierNameKeys[credentialTierKey(tok)]; ok {
				return m, true
			}
			if strings.ContainsAny(tok, `/\`) {
				if m, ok := CredentialTierPath(tok); ok {
					return m, true
				}
			}
		}
	}
	return CredentialTier{}, false
}

var (
	sqlBlockCommentRE = regexp.MustCompile(`(?s)/\*.*?(?:\*/|$)`)
	sqlLineCommentRE  = regexp.MustCompile(`--[^\n]*`)
	// sqlConcatRE removes the || operator with the quotes around it, so
	// 'agents-pass' || 'phrases' reads 'agents-passphrases'.
	sqlConcatRE   = regexp.MustCompile("[\"'`\\]]?\\s*\\|\\|\\s*[\"'`\\[]?")
	sqlQuotingRE  = regexp.MustCompile("[\"'`\\[\\]]")
	sqlWordSplitR = regexp.MustCompile(`[^a-z0-9_\-]+`)
)

// CredentialTierInSQL reports whether SQL text mentions a credential-tier
// member once it is normalized: lower-cased, comments removed, || and the
// quotes around it removed, quotes, brackets and backticks removed. A member
// name is then found inside the text with every non-alphanumeric removed
// (so 'agents-pass'||'phrases', concat('agents-pass','phrases'),
// [Uninstall_Password] and uninstall/**/_password all count), a wordOnly name
// (ranger) as a whole word ('ran'||'ger' counts, rangerStatus and
// ranger-table-view do not), and every word and path through
// CredentialTierInText.
//
// This is a guard on the query text, not the protection: the local store
// holds no credential-tier rows by construction (sync and workflow archive
// never fetch them, the store keeps no content for a never-stored resource,
// the field rule drops every secret-named field at the store's one write
// choke point, and the one-time scrub cleans a store an older build wrote).
// So a query that builds a name the normalization cannot see (char(),
// replace(), printf(), LIKE 'agents-pass%', a json path computed at run time
// or read through json_each) finds nothing to return.
func CredentialTierInSQL(query string) (CredentialTier, bool) {
	if m, ok := CredentialTierInText(query); ok {
		return m, true
	}
	s := strings.ToLower(query)
	s = sqlBlockCommentRE.ReplaceAllString(s, "")
	s = sqlLineCommentRE.ReplaceAllString(s, " ")
	s = sqlConcatRE.ReplaceAllString(s, "")
	squashed := alnumOnly(s)
	for k, m := range credentialTierSQLKeys {
		if strings.Contains(squashed, k) {
			return m, true
		}
	}
	unquoted := sqlQuotingRE.ReplaceAllString(s, " ")
	for _, w := range sqlWordSplitR.Split(unquoted, -1) {
		if m, ok := credentialTierNameKeys[credentialTierKey(w)]; ok {
			return m, true
		}
	}
	return CredentialTierInText(unquoted)
}

// NeverStoredResources returns the sync resource names that are never fetched
// into or kept in the local store, mapped to their endpoint.
func NeverStoredResources() map[string]string {
	out := make(map[string]string, len(neverStoredResources))
	for _, r := range neverStoredResources {
		out[r.resource] = r.path
	}
	return out
}

// IsNeverStoredResource reports whether a store partition or sync resource
// name is one whose rows are credentials. Matching ignores case and treats _
// and - alike, so a snake_case write-through partition is caught too.
func IsNeverStoredResource(resourceType string) bool {
	n := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(resourceType)), "_", "-")
	for _, r := range neverStoredResources {
		if r.resource == n {
			return true
		}
	}
	return false
}

// IsSecretBearingPath reports whether an API path (a template such as
// /accounts/{account_id}/uninstall-password/view, or a resolved path, with or
// without a query string or an API base prefix) is one of secretReadPaths.
// A {placeholder} segment matches any one segment. Matching ignores case.
func IsSecretBearingPath(path string) bool {
	segs := secretPathSegments(path)
	if len(segs) == 0 {
		return false
	}
	for _, pattern := range secretReadPaths {
		want := secretPathSegments(pattern)
		if len(want) > len(segs) {
			continue
		}
		tail := segs[len(segs)-len(want):]
		match := true
		for i, w := range want {
			if strings.HasPrefix(w, "{") && strings.HasSuffix(w, "}") {
				if tail[i] == "" {
					match = false
					break
				}
				continue
			}
			if tail[i] != w {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func secretPathSegments(path string) []string {
	p := strings.TrimSpace(path)
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	if i := strings.Index(p, "://"); i >= 0 {
		rest := p[i+3:]
		if j := strings.IndexByte(rest, '/'); j >= 0 {
			p = rest[j:]
		} else {
			p = ""
		}
	}
	p = strings.ToLower(strings.Trim(p, "/"))
	if p == "" {
		return nil
	}
	var out []string
	for _, s := range strings.Split(p, "/") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// secretFieldParts is the field-name rule. A JSON key is a secret when its
// normalized name (lower-cased, with _ - . and spaces removed) contains one of
// these. Why each:
//   - passphrase: the agent passphrase (GET /agents/passphrases).
//   - password, passwd, passcode: the account uninstall password, SMTP / AD /
//     Ranger credential passwords, any echoed login password.
//   - secret: client secrets, shared secrets, the 2FA app enrollment secret.
//   - token: registrationToken on sites and groups (the site/group enrollment
//     key), session and API tokens from login, iframe tokens, download tokens.
//   - apikey: third-party integration keys (marketplace, syslog/SIEM).
//   - privatekey, privkey: TLS client keys (syslog over TLS, SSO signing).
//   - recoverycode: 2FA recovery codes (users generate-recovery-codes).
//   - qrcode: the 2FA app enrollment QR payload, which encodes the TOTP seed.
var secretFieldParts = []string{
	"passphrase", "password", "passwd", "passcode", "secret", "token",
	"apikey", "privatekey", "privkey", "recoverycode", "qrcode",
}

// SecretFieldParts returns the field-name rule's substrings (see
// secretFieldParts), for callers that prefilter in SQL.
func SecretFieldParts() []string {
	return append([]string(nil), secretFieldParts...)
}

// paginationTokenNames contain "token" but are opaque page cursors, not
// credentials, so a page envelope keeps its continuation. (SentinelOne pages
// with nextCursor, which never matches.)
var paginationTokenNames = map[string]bool{
	"nexttoken": true, "nextpagetoken": true, "pagetoken": true,
	"prevtoken": true, "previoustoken": true, "prevpagetoken": true,
	"previouspagetoken": true, "continuationtoken": true, "paginationtoken": true,
}

// pairNameKeys and pairValueKey cover the name/value shape configuration
// fields often take ({"name": "apiKey", "value": "..."}): when one of these
// keys names a secret, the sibling "value" is the secret.
var pairNameKeys = map[string]bool{
	"name": true, "key": true, "field": true, "fieldname": true, "id": true,
	"label": true, "property": true, "propertyname": true,
}

const pairValueKey = "value"

// WithheldMarker replaces a secret value in an MCP tool result.
const WithheldMarker = "[withheld: credential]"

func normalizeSecretFieldName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch r {
		case '_', '-', '.', ' ', '\t':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// IsSecretFieldName reports whether a JSON key names a credential value (see
// secretFieldParts). Case, _ - . and spaces are ignored, so registrationToken,
// registration_token and REGISTRATION-TOKEN all match; nextCursor and the
// pagination token names do not.
func IsSecretFieldName(name string) bool {
	n := normalizeSecretFieldName(name)
	if n == "" || paginationTokenNames[n] {
		return false
	}
	for _, part := range secretFieldParts {
		if strings.Contains(n, part) {
			return true
		}
	}
	return false
}

// secretFieldValue reports whether a value under a secret-named key can hold
// a secret: booleans and nulls (passwordExpired: true, token: null) cannot,
// so they stay readable and never mark a body as secret-bearing.
func secretFieldValue(v any) bool {
	switch v.(type) {
	case bool, nil:
		return false
	}
	return true
}

// tokenCountName reports whether a key is secret-named only through "token"
// (total_tokens, tokenCount, promptTokens): with a number under it, that is a
// count, not a credential. Every SentinelOne credential token is a string. A
// number under any other secret name (a numeric passcode) stays secret.
func tokenCountName(name string) bool {
	n := normalizeSecretFieldName(name)
	if !strings.Contains(n, "token") {
		return false
	}
	for _, part := range secretFieldParts {
		if part != "token" && strings.Contains(n, part) {
			return false
		}
	}
	return true
}

// isSecretField is the whole field rule for one key/value pair.
func isSecretField(key string, v any) bool {
	if !IsSecretFieldName(key) || !secretFieldValue(v) {
		return false
	}
	switch v.(type) {
	case json.Number, float64, int, int64:
		return !tokenCountName(key)
	}
	return true
}

// mayHoldSecretField is the cheap prefilter: false means no key in data can
// match the field rule. Any backslash sends data to the parser, because a JSON
// escape (\u0074oken) can spell a key the byte scan would miss.
func mayHoldSecretField(data []byte) bool {
	if bytes.IndexByte(data, '\\') >= 0 {
		return true
	}
	n := []byte(normalizeSecretFieldName(string(data)))
	for _, part := range secretFieldParts {
		if bytes.Contains(n, []byte(part)) {
			return true
		}
	}
	return false
}

// RedactSecretFields returns raw with every secret-named key (and the value
// of a name/value pair whose name is secret) removed at any depth, and
// whether anything was removed. Unchanged or non-JSON input is returned as-is;
// numbers keep their exact text.
func RedactSecretFields(raw json.RawMessage) (json.RawMessage, bool) {
	if !mayHoldSecretField(raw) {
		return raw, false
	}
	v, ok := decodeSecretJSON(raw)
	if !ok {
		return raw, false
	}
	if !dropSecretFields(v) {
		return raw, false
	}
	out, err := encodeSecretJSON(v)
	if err != nil {
		return raw, false
	}
	return out, true
}

// ContainsSecretField reports whether raw holds a secret-named field at any
// depth (see RedactSecretFields).
func ContainsSecretField(raw json.RawMessage) bool {
	_, removed := RedactSecretFields(raw)
	return removed
}

// ResponseMayCarrySecret is the response cache's test: a JSON body with a
// secret field, or a body that is not one JSON document (CSV export, text,
// NDJSON) yet mentions a secret-shaped name (a Passphrase column, a
// token=... line), is never written to disk. A body with no secret-shaped
// name at all is cacheable.
func ResponseMayCarrySecret(raw []byte) bool {
	if !mayHoldSecretField(raw) {
		return false
	}
	if _, ok := decodeSecretJSON(raw); !ok {
		return true
	}
	return ContainsSecretField(raw)
}

func decodeSecretJSON(raw []byte) (any, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return nil, false // more than one JSON value: not a document we rewrite
	}
	return v, true
}

func encodeSecretJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func secretPairValueKeys(t map[string]any) []string {
	named := false
	for k, child := range t {
		if !pairNameKeys[normalizeSecretFieldName(k)] {
			continue
		}
		if s, ok := child.(string); ok && IsSecretFieldName(s) {
			named = true
			break
		}
	}
	if !named {
		return nil
	}
	var keys []string
	for k, child := range t {
		if normalizeSecretFieldName(k) == pairValueKey && secretFieldValue(child) {
			keys = append(keys, k)
		}
	}
	return keys
}

func dropSecretFields(v any) bool {
	removed := false
	switch t := v.(type) {
	case map[string]any:
		for _, k := range secretPairValueKeys(t) {
			delete(t, k)
			removed = true
		}
		for k, child := range t {
			if isSecretField(k, child) {
				delete(t, k)
				removed = true
				continue
			}
			if dropSecretFields(child) {
				removed = true
			}
		}
	case []any:
		for _, child := range t {
			if dropSecretFields(child) {
				removed = true
			}
		}
	}
	return removed
}

// WithholdSecretFieldsText replaces every secret value in an MCP tool result
// with WithheldMarker. A JSON document is walked (secret-named keys and
// name/value pairs at any depth, and JSON or JSON-like text inside string
// values, such as a sql row's data column or a bounded preview cut mid-
// document); any other text gets the key/value scan. Text with nothing to
// withhold is returned unchanged.
func WithholdSecretFieldsText(text string) string {
	if !mayHoldSecretField([]byte(text)) {
		return text
	}
	if v, ok := decodeSecretJSON([]byte(text)); ok {
		if out, changed := withholdSecretValue(v); changed {
			if b, err := encodeSecretJSON(out); err == nil {
				return string(b)
			}
		}
		return text
	}
	return withholdSecretPairs(text)
}

func withholdSecretValue(v any) (any, bool) {
	switch t := v.(type) {
	case map[string]any:
		changed := false
		for _, k := range secretPairValueKeys(t) {
			if s, ok := t[k].(string); ok && s == WithheldMarker {
				continue
			}
			t[k] = WithheldMarker
			changed = true
		}
		for k, child := range t {
			if isSecretField(k, child) {
				if s, ok := child.(string); ok && s == WithheldMarker {
					continue
				}
				t[k] = WithheldMarker
				changed = true
				continue
			}
			if out, c := withholdSecretValue(child); c {
				t[k] = out
				changed = true
			}
		}
		return t, changed
	case []any:
		changed := false
		for i, child := range t {
			if out, c := withholdSecretValue(child); c {
				t[i] = out
				changed = true
			}
		}
		return t, changed
	case string:
		if out := withholdSecretString(t); out != t {
			return out, true
		}
	}
	return v, false
}

func withholdSecretString(s string) string {
	if !mayHoldSecretField([]byte(s)) {
		return s
	}
	if inner, ok := decodeSecretJSON([]byte(s)); ok {
		switch inner.(type) {
		case map[string]any, []any:
			if out, changed := withholdSecretValue(inner); changed {
				if b, err := encodeSecretJSON(out); err == nil {
					return string(b)
				}
			}
			return s
		}
	}
	return withholdSecretPairs(s)
}

// quotedSecretPairRE finds "key": value pairs in JSON-like text, including a
// string value cut off at the end of a preview.
var quotedSecretPairRE = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"(\s*:\s*)("(?:[^"\\]|\\.)*(?:"|$)|[^\s,}\]\[{"]+)`)

// bareSecretPairRE finds key: value and key=value pairs in plain text, keyed
// by a secret-shaped word.
var bareSecretPairRE = regexp.MustCompile(`(?i)([A-Za-z0-9_.\-]*(?:pass|secret|token|api[_\-.]?key|priv(?:ate)?[_\-.]?key|recovery[_\-.]?code|qr[_\-.]?code)[A-Za-z0-9_.\-]*)(\s*[:=]\s*)("[^"]*"?|[^\s,;&}\]"]+)`)

func withholdSecretPairs(text string) string {
	quoted := `"` + WithheldMarker + `"`
	out := quotedSecretPairRE.ReplaceAllStringFunc(text, func(m string) string {
		sub := quotedSecretPairRE.FindStringSubmatch(m)
		key := sub[1]
		var decoded string
		if err := json.Unmarshal([]byte(`"`+key+`"`), &decoded); err == nil {
			key = decoded
		}
		if !IsSecretFieldName(key) || plainNonSecretValue(sub[3]) || sub[3] == quoted || (tokenCountName(key) && bareNumber(sub[3])) {
			return m
		}
		return `"` + sub[1] + `"` + sub[2] + quoted
	})
	return bareSecretPairRE.ReplaceAllStringFunc(out, func(m string) string {
		sub := bareSecretPairRE.FindStringSubmatch(m)
		if !IsSecretFieldName(sub[1]) || plainNonSecretValue(sub[3]) || strings.HasPrefix(sub[3], `"[withheld`) || (tokenCountName(sub[1]) && bareNumber(sub[3])) {
			return m
		}
		return sub[1] + sub[2] + WithheldMarker
	})
}

func plainNonSecretValue(v string) bool {
	switch strings.ToLower(strings.Trim(strings.TrimSpace(v), `"`)) {
	case "true", "false", "null", "":
		return true
	}
	return false
}

func bareNumber(v string) bool {
	var n json.Number
	return json.Unmarshal([]byte(strings.TrimSpace(v)), &n) == nil
}
