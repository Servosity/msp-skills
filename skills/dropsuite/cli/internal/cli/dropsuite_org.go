// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: Dropsuite organization selector (--org) and credential redaction.
// Lives in its own file so generate --force preserves it.

package cli

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"dropsuite-pp-cli/internal/client"
	"dropsuite-pp-cli/internal/config"
)

// Dropsuite sends every request with X-Access-Token. GET /users, GET
// /users/{id} and /users/generate_support_sso_link take the reseller's own
// Authentication Token (S1 "Admin Token"); everything else takes ONE
// organization's authentication_token, which GET /users returns: backup data
// (/accounts, /onedrives, ...) and the organization's /users/{id}/delegated,
// retention_policies, tenants and tenant accounts routes (S1 "User's
// Authentication token"). --org lets a user name the organization instead of
// copying that token around.
//
// The selector state is package-level on purpose: a CLI process serves
// exactly one invocation (the MCP server shells out to the CLI binary for
// every mirrored command and never runs PersistentPreRunE in-process, so the
// override stays empty there).
var (
	// orgSelector is the root --org value.
	orgSelector string
	// orgTokenOverride is the resolved organization token. It is set only for
	// organization-scoped generated commands and applied by the client hook.
	orgTokenOverride string
	// orgOverrideID is the id of the organization orgTokenOverride belongs to.
	orgOverrideID string
)

// orgScopedCommands are the top-level generated commands whose routes need an
// organization token rather than the reseller admin token.
var orgScopedCommands = map[string]bool{
	"accounts":             true,
	"calendars":            true,
	"contacts":             true,
	"tasks":                true,
	"onedrives":            true,
	"google-drives":        true,
	"sharepoint-domains":   true,
	"sharepoint-sites":     true,
	"teams-domains":        true,
	"teams":                true,
	"shared-drive-domains": true,
	"shared-drives":        true,
	"shared-drive-backups": true,
	"journals":             true,
	"portal":               true,
}

// orgFromArgCommands are generated "organizations" subcommands whose first
// positional is the organization id and whose route needs that organization's
// own token. Without --org the positional selects the organization.
var orgFromArgCommands = map[string]bool{
	"list-delegated-users":    true,
	"list-retention-policies": true,
	"list-tenants":            true,
	"list-tenant-accounts":    true,
}

// orgAwareNovelCommands read orgSelector themselves (scope to one organization).
var orgAwareNovelCommands = map[string]bool{
	"health":       true,
	"coverage":     true,
	"billing":      true,
	"evidence":     true,
	"hygiene":      true,
	"restore-test": true,
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		if root.PersistentFlags().Lookup("org") == nil {
			root.PersistentFlags().StringVar(&orgSelector, "org", "",
				"Organization id, owner email or external_id. Organization-scoped commands (accounts, onedrives, sharepoint-*, teams*, shared-drive*, google-drives, calendars, contacts, tasks, journals, portal, and organizations list-tenants/list-tenant-accounts/list-delegated-users/list-retention-policies) run with that organization's token, resolved from the organizations list with your reseller token")
		}
		prev := root.PersistentPreRunE
		root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
			if prev != nil {
				if err := prev(cmd, args); err != nil {
					return err
				}
			}
			return applyOrgSelector(cmd, args, flags)
		}
	})
}

// applyOrgOverride points a new client at the --org organization's token. The
// one client hook (ds_guard.go) calls it above the origin pin.
func applyOrgOverride(c *client.Client) {
	if orgTokenOverride != "" && c.Config != nil {
		c.Config.AccessToken = orgTokenOverride
		c.Config.AuthHeaderVal = ""
		dropAccessTokenHeader(c.Config)
		guardOrgRedirects(c, orgOverrideID)
		// The response cache keys on the configured credential (or a
		// platform session); never let one organization read another's
		// cached body.
		c.NoCache = true
	}
}

// topLevelName returns the name of the root child that cmd belongs to.
func topLevelName(cmd *cobra.Command) string {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Parent() != nil && c.Parent().Parent() == nil {
			return c.Name()
		}
	}
	return ""
}

func applyOrgSelector(cmd *cobra.Command, args []string, flags *rootFlags) error {
	orgTokenOverride, orgOverrideID = "", ""
	selector := strings.TrimSpace(orgSelector)
	top := topLevelName(cmd)
	if top == "sync" && os.Getenv(defaultOrgEnv) != "" {
		// Routing would fetch one organization's backup data into the
		// reseller-wide store, which has no organization column.
		return usageErr(fmt.Errorf("sync cannot run with %s set: the local store does not separate organizations; unset it to sync the reseller-level resources", defaultOrgEnv))
	}
	scoped := orgScopedCommands[top] || (top == "organizations" && orgFromArgCommands[cmd.Name()])
	if scoped && (selector != "" || os.Getenv(defaultOrgEnv) != "" || (top == "organizations" && len(args) > 0)) {
		// The local store is shared by every organization under the reseller
		// credential and has no organization column: an organization-scoped
		// read must not write through to it or fall back to it.
		if flags.dataSource == "local" {
			return usageErr(fmt.Errorf("--data-source local cannot be combined with an organization token (--org, DROPSUITE_ORG or an organization id argument): the local store does not separate organizations"))
		}
		flags.dataSource = "live"
	}
	if top == "organizations" && orgFromArgCommands[cmd.Name()] {
		if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
			if selector != "" && selector != strings.TrimSpace(args[0]) {
				return usageErr(fmt.Errorf("--org %q and the organization id argument %q disagree; pass one", selector, args[0]))
			}
			if selector == "" {
				selector = strings.TrimSpace(args[0])
			}
		}
		if selector == "" {
			return nil // missing positional: the generated command reports it
		}
	} else {
		if selector == "" || orgAwareNovelCommands[top] {
			return nil
		}
		if !orgScopedCommands[top] {
			return usageErr(fmt.Errorf("--org applies to organization-scoped commands (accounts, onedrives, calendars, contacts, tasks, sharepoint-*, teams*, shared-drive*, google-drives, journals, portal, organizations list-tenants/list-tenant-accounts/list-delegated-users/list-retention-policies) and to health, coverage, billing, evidence, hygiene and restore-test; %q does not take it", top))
		}
	}
	if flags.dryRun {
		return nil
	}
	ctx, cancel := boundCtx(cmd.Context(), flags)
	defer cancel()
	c, err := adminClient(flags)
	if err != nil {
		return err
	}
	orgs, err := fetchOrgs(ctx, c)
	if err != nil {
		return err
	}
	org, err := matchOrg(orgs, selector)
	if err != nil {
		return err
	}
	if org.token == "" {
		return authErr(fmt.Errorf("organization %s has no authentication_token in the organizations list; check the reseller token's access", org.label()))
	}
	orgTokenOverride, orgOverrideID = org.token, org.ID
	return nil
}

// ---- credential redaction ----------------------------------------------

// revealKey marks a request whose successful response must keep credential
// fields (the fan-out code needs each organization's authentication_token in
// memory). Error bodies are always scrubbed.
type revealKey struct{}

func withSecrets(ctx context.Context) context.Context {
	return context.WithValue(ctx, revealKey{}, true)
}

const redactedValue = "[redacted]"

// minEchoedCredential is the shortest request credential masked where the
// API echoes it back; shorter values would mangle ordinary text.
const minEchoedCredential = 8

type redactingTransport struct {
	next http.RoundTripper
}

func installRedaction(c *client.Client) {
	if c == nil || c.HTTPClient == nil {
		return
	}
	if _, ok := c.HTTPClient.Transport.(redactingTransport); ok {
		return
	}
	next := c.HTTPClient.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	c.HTTPClient.Transport = redactingTransport{next: next}
}

func (t redactingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	reveal, _ := req.Context().Value(revealKey{}).(bool)
	req = req.Clone(req.Context())
	req.Header.Set("Accept-Encoding", "identity")
	creds := []string{req.Header.Get("X-Access-Token"), req.Header.Get("X-Reseller-Token")}
	resp, err := t.next.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil {
		return resp, scrubErr(err, creds)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, scrubErr(err, creds)
	}
	body, err = decodeForRedaction(contentEncoding(resp.Header), body)
	if err != nil {
		return nil, scrubErr(err, creds)
	}
	var out []byte
	withheld := false
	if reveal && resp.StatusCode >= 200 && resp.StatusCode < 300 && json.Valid(stripJSONPrefix(body)) {
		// The fan-out code reads tokens from this JSON in memory only. Any
		// other success body (HTML, malformed JSON) is scrubbed, since the
		// client may quote it in an error.
		out = body
	} else {
		out = redactSecretFields(body, creds...)
		if echoesCredential(string(out), creds) {
			out = []byte(withheldBody)
		}
		withheld = string(out) == withheldBody
	}
	if withheld && resp.StatusCode < 400 {
		// A withheld success must not read as an empty result.
		resp.StatusCode, resp.Status = http.StatusBadGateway, "502 Bad Gateway"
	}
	for k, vs := range resp.Header {
		for i, v := range vs {
			for _, c := range creds {
				if len(c) >= minEchoedCredential {
					v = strings.ReplaceAll(v, c, redactedValue)
				}
			}
			if echoesCredential(v, creds) {
				v = redactedValue
			}
			resp.Header[k][i] = v
		}
	}
	resp.Body = io.NopCloser(bytes.NewReader(out))
	resp.ContentLength = int64(len(out))
	resp.Header.Del("Content-Encoding")
	resp.Header.Set("Content-Length", strconv.Itoa(len(out)))
	return resp, nil
}

// withheldBody replaces a response that still carries a request credential
// in some encoded form after the scrub.
const withheldBody = `{"error":"response withheld: it repeated a credential this request sent"}`

var (
	percentEscape = regexp.MustCompile(`%[0-9a-fA-F]{2}`)
)

// scrubErr withholds a transport error whose text repeats a request
// credential in any decoded form.
func scrubErr(err error, creds []string) error {
	if err != nil && echoesCredential(err.Error(), creds) {
		return errors.New("request failed; the error repeated a credential this request sent and was withheld")
	}
	return err
}

// maxDecodeRounds bounds layered decoding; text still changing after that
// many rounds is treated as carrying a credential (withheld), never as clean.
const maxDecodeRounds = 64

// jsonUnescapeOnce removes exactly one layer of JSON string escapes, reading
// left to right so an escaped backslash (\\) is never re-read as the start of
// another escape.
func jsonUnescapeOnce(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			b.WriteByte(c)
			continue
		}
		switch n := s[i+1]; n {
		case '"', '\\', '/':
			b.WriteByte(n)
			i++
		case 'n':
			b.WriteByte('\n')
			i++
		case 't':
			b.WriteByte('\t')
			i++
		case 'r':
			b.WriteByte('\r')
			i++
		case 'b':
			b.WriteByte('\b')
			i++
		case 'f':
			b.WriteByte('\f')
			i++
		case 'u', 'U':
			if i+5 < len(s) {
				// Four hex digits never exceed 0xFFFF; the bound check states
				// that for the int32 rune conversion.
				if v, err := strconv.ParseUint(s[i+2:i+6], 16, 32); err == nil && v <= unicode.MaxRune {
					b.WriteRune(rune(v))
					i += 5
					continue
				}
			}
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// urlUnescapeOnce decodes each valid %XX on its own, so one stray '%'
// elsewhere in the text cannot switch decoding off.
func urlUnescapeOnce(s string) string {
	return percentEscape.ReplaceAllStringFunc(s, func(m string) string {
		b, err := strconv.ParseUint(m[1:], 16, 8)
		if err != nil {
			return m
		}
		return string([]byte{byte(b)})
	})
}

// decodedForms returns the text plus every way a reader might decode it:
// each single decoder (one layer of JSON escapes, URL %XX escapes, HTML
// entities) and all of them applied round after round until nothing changes,
// so layered encodings peel in any order. exhausted is true when the text was
// still changing after maxDecodeRounds.
func decodedForms(s string) (forms []string, exhausted bool) {
	decoders := []func(string) string{jsonUnescapeOnce, urlUnescapeOnce, html.UnescapeString}
	forms = []string{s}
	for _, d := range decoders {
		forms = append(forms, d(s))
	}
	f := s
	for i := 0; ; i++ {
		g := f
		for _, d := range decoders {
			g = d(g)
		}
		if g == f {
			break
		}
		if i == maxDecodeRounds {
			return forms, true
		}
		f = g
		forms = append(forms, f)
	}
	return forms, false
}

// echoesCredential is the backstop behind the scrub: it reports whether any
// decoded form of the text still contains a request credential.
func echoesCredential(s string, creds []string) bool {
	forms, exhausted := decodedForms(s)
	if exhausted {
		return true
	}
	for _, c := range creds {
		if len(c) < minEchoedCredential {
			continue
		}
		for _, f := range forms {
			if strings.Contains(f, c) {
				return true
			}
		}
	}
	return false
}

// secretIdentifier matches a credential field written as an identifier
// (authentication_token, authenticationToken, reseller-token) in any markup:
// JSON, query string, form, XML or an HTML input name. Prose with a space,
// such as "invalid reseller token", does not match.
var secretIdentifier = regexp.MustCompile(`(?i)(authentication|reseller)[_-]?token`)

// isSecretKey reports whether a JSON key names a credential, in any spelling
// (authentication_token, authenticationToken, Reseller-Token, ...) and any
// encoding the key itself carries (decoded like values; fails closed when
// decoding does not settle).
func isSecretKey(k string) bool {
	forms, exhausted := decodedForms(k)
	if exhausted {
		return true
	}
	for _, f := range forms {
		n := strings.Map(func(r rune) rune {
			if r == '_' || r == '-' || r == ' ' {
				return -1
			}
			return unicode.ToLower(r)
		}, f)
		if strings.Contains(n, "authenticationtoken") || strings.Contains(n, "resellertoken") {
			return true
		}
	}
	return false
}

// mentionsSecretField reports whether any decoded form of the text names a
// credential field as an identifier. A body that is not valid JSON and does is withheld whole,
// and so is a JSON string value that carries one (serialized JSON in an error
// message): a textual scrub cannot prove it found every value (an
// unterminated string, for one).
func mentionsSecretField(s string) bool {
	forms, exhausted := decodedForms(s)
	if exhausted {
		return true
	}
	for _, f := range forms {
		if secretIdentifier.MatchString(f) {
			return true
		}
	}
	return false
}

const maxDecodedBody = 64 << 20

// contentEncoding returns the response's one content coding. Every
// Content-Encoding value and comma-separated entry counts; more than one real
// coding is reported as unsupported (decodeForRedaction then fails closed)
// rather than half-decoded.
func contentEncoding(h http.Header) string {
	codings := make([]string, 0, 1)
	for _, v := range h.Values("Content-Encoding") {
		for _, c := range strings.Split(v, ",") {
			if c = strings.ToLower(strings.TrimSpace(c)); c != "" && c != "identity" {
				codings = append(codings, c)
			}
		}
	}
	switch len(codings) {
	case 0:
		return ""
	case 1:
		return codings[0]
	}
	return "multiple"
}

// decodeForRedaction inflates a compressed body so the scrub sees plaintext.
// An encoding it cannot read, or an oversized body, fails closed rather than
// passing secrets through.
func decodeForRedaction(encoding string, body []byte) ([]byte, error) {
	var r io.Reader
	switch enc := strings.ToLower(strings.TrimSpace(encoding)); enc {
	case "", "identity":
		return body, nil
	case "gzip", "x-gzip":
		zr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("decoding gzip response: %w", err)
		}
		defer zr.Close()
		r = zr
	case "deflate":
		// HTTP deflate is usually zlib-wrapped; fall back to raw deflate.
		if zr, err := zlib.NewReader(bytes.NewReader(body)); err == nil {
			defer zr.Close()
			r = zr
		} else {
			fr := flate.NewReader(bytes.NewReader(body))
			defer fr.Close()
			r = fr
		}
	default:
		// The header value is not quoted: it is server-controlled text.
		return nil, fmt.Errorf("unsupported response Content-Encoding")
	}
	out, err := io.ReadAll(io.LimitReader(r, maxDecodedBody+1))
	if err != nil {
		return nil, fmt.Errorf("decoding %s response: %w", encoding, err)
	}
	if len(out) > maxDecodedBody {
		return nil, fmt.Errorf("decoded response exceeds %d bytes", maxDecodedBody)
	}
	return out, nil
}

// redactSecretFields masks credential values anywhere in a JSON body, plus
// any echo of the request's own credentials (creds). The body is first
// stripped of the BOM and XSSI prefixes the client strips before parsing. A
// body that parses is always re-serialized from the scrubbed document, so
// duplicate or escaped keys cannot carry the original bytes through. A body
// that does not parse is withheld whole when it names a credential field (see
// mentionsSecretField) and is otherwise left as is.
func redactSecretFields(body []byte, creds ...string) []byte {
	echoes := make([]string, 0, len(creds))
	for _, c := range creds {
		if len(c) >= minEchoedCredential {
			echoes = append(echoes, c)
			body = bytes.ReplaceAll(body, []byte(c), []byte(redactedValue))
		}
	}
	body = stripJSONPrefix(body)
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil || dec.More() {
		if mentionsSecretField(string(body)) {
			return []byte(withheldBody)
		}
		return body
	}
	v = redactWalk(v, echoes)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return []byte(withheldBody)
	}
	return bytes.TrimRight(buf.Bytes(), "\n")
}

// stripJSONPrefix mirrors the client's sanitizeJSONResponse, so the scrub
// parses exactly the document the client will.
func stripJSONPrefix(body []byte) []byte {
	body = bytes.TrimPrefix(body, []byte("\xEF\xBB\xBF"))
	for _, p := range []string{")]}'\n", ")]}'", "{}&&", "for(;;);", "while(1);"} {
		if bytes.HasPrefix(body, []byte(p)) {
			return bytes.TrimLeft(bytes.TrimPrefix(body, []byte(p)), " \t\r\n")
		}
	}
	return body
}

func redactWalk(v any, echoes []string) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if isSecretKey(k) {
				// Any non-empty value, whatever its JSON type.
				if val != nil && val != "" {
					t[k] = redactedValue
				}
				continue
			}
			t[k] = redactWalk(val, echoes)
		}
	case []any:
		for i, item := range t {
			t[i] = redactWalk(item, echoes)
		}
	case string:
		if mentionsSecretField(t) {
			return redactedValue
		}
		for _, e := range echoes {
			t = strings.ReplaceAll(t, e, redactedValue)
		}
		return t
	}
	return v
}

// ---- organizations --------------------------------------------------------

// dsOrg is one tenant user (client organization). token never leaves memory.
type dsOrg struct {
	ID                  string
	OrganizationID      string
	Email               string
	Name                string
	ExternalID          string
	PlanID              string
	SeatsUsed           int64
	SeatsAvailable      int64
	StorageUsed         int64
	StorageAvailable    int64
	CustomerDeactivated bool
	Suspended           bool
	Archive             bool
	token               string
}

func (o dsOrg) label() string {
	if o.Email != "" {
		return o.ID + " (" + o.Email + ")"
	}
	return o.ID
}

func (o dsOrg) inactive() bool { return o.CustomerDeactivated || o.Suspended }

func orgFromMap(m map[string]any) dsOrg {
	return dsOrg{
		ID:                  str(m, "id"),
		OrganizationID:      str(m, "organization_id"),
		Email:               str(m, "email"),
		Name:                str(m, "organization_name"),
		ExternalID:          str(m, "external_id"),
		PlanID:              str(m, "plan_id"),
		SeatsUsed:           num(m, "seats_used"),
		SeatsAvailable:      num(m, "seats_available"),
		StorageUsed:         num(m, "storage_used"),
		StorageAvailable:    num(m, "storage_available"),
		CustomerDeactivated: boolv(m, "customer_deactivated"),
		Suspended:           boolv(m, "flg_suspended"),
		Archive:             boolv(m, "archive"),
		token:               str(m, "authentication_token"),
	}
}

// adminClient builds a client with the configured (reseller admin) token and
// no response cache, so unredacted fan-out bodies never land on disk.
func adminClient(flags *rootFlags) (*client.Client, error) {
	c, err := flags.newClient()
	if err != nil {
		return nil, err
	}
	c.NoCache = true
	return c, nil
}

// orgClient builds a client that sends one organization's token.
func orgClient(flags *rootFlags, org dsOrg) (*client.Client, error) {
	if org.token == "" {
		return nil, fmt.Errorf("organization %s has no authentication_token", org.label())
	}
	c, err := flags.newClient()
	if err != nil {
		return nil, err
	}
	c.NoCache = true
	c.Config.AccessToken = org.token
	c.Config.AuthHeaderVal = ""
	dropAccessTokenHeader(c.Config)
	// The token is explicit; default-organization routing must not touch it.
	if rt, ok := c.HTTPClient.Transport.(orgRoutingTransport); ok {
		c.HTTPClient.Transport = rt.next
	}
	guardOrgRedirects(c, org.ID)
	return c, nil
}

// guardOrgRedirects stops a client that carries one organization's token
// from following a redirect anywhere but the same path (query changes only):
// any other path could belong to another organization.
func guardOrgRedirects(c *client.Client, orgID string) {
	if c.HTTPClient == nil || orgID == "" {
		return
	}
	prev := c.HTTPClient.CheckRedirect
	c.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 0 && req.URL.Path != via[0].URL.Path {
			return fmt.Errorf("refusing a redirect from %s to %s with organization %s's token", via[0].URL.Path, req.URL.Path, orgID)
		}
		if prev != nil {
			return prev(req, via)
		}
		return nil
	}
}

// dropAccessTokenHeader removes a configured X-Access-Token header, which the
// client would otherwise send in place of the selected organization's token.
func dropAccessTokenHeader(cfg *config.Config) {
	for k := range cfg.Headers {
		if strings.EqualFold(k, "X-Access-Token") {
			delete(cfg.Headers, k)
		}
	}
}

// fetchOrgs lists every organization with tokens kept (in memory only). A
// failure carries the documented exit code (4 auth, 5 API, 7 rate limit)
// instead of a bare 1, and names both reseller tokens on 401/403. Commands
// and the --org resolver use it; the routing transport uses fetchOrgsRaw so
// a generated command still classifies (and prints) the error itself.
func fetchOrgs(ctx context.Context, c *client.Client) ([]dsOrg, error) {
	orgs, err := fetchOrgsRaw(ctx, c)
	if err == nil {
		return orgs, nil
	}
	var listErr *orgListError
	if !errors.As(err, &listErr) {
		return nil, err
	}
	switch msg := listErr.err.Error(); {
	case strings.Contains(msg, "HTTP 401") || strings.Contains(msg, "HTTP 403"):
		return nil, authErr(fmt.Errorf("%w\nhint: listing organizations needs both reseller-admin tokens from Settings > API Settings:"+
			" DROPSUITE_ACCESS_TOKEN (Authentication Token) and DROPSUITE_RESELLER_TOKEN (Reseller Token)."+
			"\n      Run 'dropsuite-cli doctor' to check auth status.", err))
	case strings.Contains(msg, "HTTP 404"):
		return nil, apiErr(fmt.Errorf("%w\nhint: this host has no organization list; check DROPSUITE_HOST (your partner site's API host).", err))
	}
	return nil, classifyAPIErrorOnly(err)
}

// orgListError marks a failed GET /users so fetchOrgs classifies only the
// request failure, not a response it could not parse.
type orgListError struct{ err error }

func (e *orgListError) Error() string {
	return "listing organizations (GET /users with the reseller token): " + e.err.Error()
}
func (e *orgListError) Unwrap() error { return e.err }

// fetchOrgsRaw is fetchOrgs without exit-code classification.
func fetchOrgsRaw(ctx context.Context, c *client.Client) ([]dsOrg, error) {
	data, err := c.Get(withSecrets(ctx), "/users", nil)
	if err != nil {
		return nil, &orgListError{err: err}
	}
	items, _, err := decodeList(data)
	if err != nil {
		return nil, fmt.Errorf("parsing organizations: %w", err)
	}
	orgs := make([]dsOrg, 0, len(items))
	for _, m := range items {
		orgs = append(orgs, orgFromMap(m))
	}
	return orgs, nil
}

// matchOrg resolves --org against id, organization_id, external_id or email.
func matchOrg(orgs []dsOrg, ref string) (dsOrg, error) {
	ref = strings.TrimSpace(ref)
	// An exact organization id wins over another organization's alias.
	for _, o := range orgs {
		if o.ID != "" && o.ID == ref {
			return o, nil
		}
	}
	var hits []dsOrg
	for _, o := range orgs {
		if (o.ID != "" && o.ID == ref) ||
			(o.OrganizationID != "" && o.OrganizationID == ref) ||
			(o.ExternalID != "" && o.ExternalID == ref) ||
			(o.Email != "" && strings.EqualFold(o.Email, ref)) {
			hits = append(hits, o)
		}
	}
	switch len(hits) {
	case 0:
		return dsOrg{}, notFoundErr(fmt.Errorf("no organization matches --org %q (searched %d organizations by id, organization_id, external_id and owner email); list them with 'dropsuite-cli organizations list'", ref, len(orgs)))
	case 1:
		return hits[0], nil
	default:
		ids := make([]string, 0, len(hits))
		for _, h := range hits {
			ids = append(ids, h.label())
		}
		sort.Strings(ids)
		return dsOrg{}, usageErr(fmt.Errorf("--org %q matches %d organizations (%s); pass the organization id", ref, len(hits), strings.Join(ids, ", ")))
	}
}

// selectOrgs applies --org, or drops deactivated/suspended customers unless
// includeInactive. skipped lists what was dropped.
func selectOrgs(orgs []dsOrg, selector string, includeInactive bool) (picked []dsOrg, skipped []skippedOrg, err error) {
	if strings.TrimSpace(selector) != "" {
		o, err := matchOrg(orgs, selector)
		if err != nil {
			return nil, nil, err
		}
		return []dsOrg{o}, []skippedOrg{}, nil
	}
	picked = make([]dsOrg, 0, len(orgs))
	skipped = make([]skippedOrg, 0)
	for _, o := range orgs {
		if o.inactive() && !includeInactive {
			reason := "customer_deactivated"
			if o.Suspended {
				reason = "flg_suspended"
			}
			skipped = append(skipped, skippedOrg{OrgID: o.ID, OrgEmail: o.Email, Reason: reason})
			continue
		}
		picked = append(picked, o)
	}
	return picked, skipped, nil
}

// rejectArgs keeps argument-free commands from silently running with stray
// positionals.
func rejectArgs(args []string) error {
	if len(args) > 0 {
		return usageErr(fmt.Errorf("unexpected argument(s): %s", strings.Join(args, " ")))
	}
	return nil
}

type skippedOrg struct {
	OrgID    string `json:"org_id"`
	OrgEmail string `json:"org_email"`
	Reason   string `json:"reason"`
}

type fetchFailure struct {
	OrgID    string `json:"org_id,omitempty"`
	OrgEmail string `json:"org_email,omitempty"`
	Scope    string `json:"scope"`
	Error    string `json:"error"`
}
