// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Exchange Online admin endpoints are cmdlet RPC over POST: the body is
// {"CmdletInput":{"CmdletName":"<cmdlet>","Parameters":{...}}}. The spec writes
// each cmdlet as <surface path>#<Cmdlet>, and net/http never sends a URL
// fragment, so this transport turns the fragment plus the generated command's
// body (the cmdlet parameters) into the envelope, routes the request with
// X-AnchorMailbox, follows @odata.nextLink pages, and hands back the merged
// `value` rows.
// Sources: https://learn.microsoft.com/en-us/exchange/reference/admin-api-get-started
// and CIPP-API Modules/CIPPCore/Public/GraphHelper/New-ExoRequest.ps1.

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"exchange-online-pp-cli/internal/client"
	"exchange-online-pp-cli/internal/cliutil"
	"exchange-online-pp-cli/internal/credkit"
)

// installEXOTransport is registered by exo_guard.go, after the origin pin.
func installEXOTransport(c *client.Client) error {
	if c == nil || c.HTTPClient == nil {
		return nil
	}
	if _, ok := c.HTTPClient.Transport.(*exoTransport); ok {
		return nil
	}
	base := c.HTTPClient.Transport
	if base == nil {
		base = credkit.Transport(http.DefaultTransport)
	}
	c.HTTPClient.Transport = &exoTransport{base: base}
	return nil
}

type exoTransport struct{ base http.RoundTripper }

// systemMailboxGUID is the organization system mailbox Microsoft documents as
// the app-only routing key; it is the same in every tenant.
const systemMailboxGUID = "bb558c35-97f1-4cb9-8ff7-d53741dc928c"

// anchorEnv overrides the routing key for calls with no target mailbox, for
// example UPN:admin@contoso.com with a delegated token.
const anchorEnv = "EXCHANGE_ONLINE_ANCHOR_MAILBOX"

var (
	cmdletRe = regexp.MustCompile(`^[A-Z][A-Za-z]+-[A-Za-z0-9]+$`)
	tenantRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*$`)
)

// anchorParam names the parameter holding the mailbox a cmdlet binds to. The
// request is routed to that mailbox, as Microsoft documents for /Mailbox and
// /MailboxFolderPermission and as CIPP does (-Anchor <user>) for inbox rules,
// mobile devices, auto-reply, forwarding and CAS changes.
var anchorParam = map[string]string{
	"Get-Mailbox":                       "Identity",
	"Set-Mailbox":                       "Identity",
	"Set-CASMailbox":                    "Identity",
	"Get-InboxRule":                     "Mailbox",
	"Remove-InboxRule":                  "Mailbox",
	"Get-MobileDevice":                  "Mailbox",
	"Get-MailboxAutoReplyConfiguration": "Identity",
	"Set-MailboxAutoReplyConfiguration": "Identity",
	"Get-MailboxFolderPermission":       "Identity",
	"Add-MailboxFolderPermission":       "Identity",
	"Set-MailboxFolderPermission":       "Identity",
	"Remove-MailboxFolderPermission":    "Identity",
}

// maxEXOPages bounds one call's @odata.nextLink walk; maxEXOBytes bounds the
// merged body. ponytail: fixed caps; add flags if a tenant ever needs more.
var (
	maxEXOPages       = 200
	maxEXOBytes int64 = 256 << 20
)

func (t *exoTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// exchange-online_read and _execute append query parameters to the path they
	// were given, which ends in the fragment: "#Get-Mailbox?%24select=Name". The
	// Admin API takes $select as a query parameter of the URL.
	cmdlet, fragmentQuery, _ := strings.Cut(req.URL.EscapedFragment(), "?")
	if cmdlet == "" || req.Method != http.MethodPost || !strings.HasPrefix(req.URL.Path, "/adminapi/") {
		return t.base.RoundTrip(req)
	}
	if !cmdletRe.MatchString(cmdlet) {
		return nil, fmt.Errorf("invalid cmdlet name %q", cmdlet)
	}
	// Validate the escaped path so an encoded separator (a%2Fb) cannot pass as a tenant.
	segs := strings.Split(strings.TrimPrefix(req.URL.EscapedPath(), "/"), "/")
	if len(segs) != 4 || segs[0] != "adminapi" || (segs[1] != "beta" && segs[1] != "v2.0") || segs[3] == "" {
		return nil, fmt.Errorf("unexpected Exchange admin path %q", req.URL.EscapedPath())
	}
	if !tenantRe.MatchString(segs[2]) {
		return nil, fmt.Errorf("tenant must be a tenant ID (GUID) or domain, got %q", segs[2])
	}
	tenant := segs[2]
	beta := segs[1] == "beta"

	params := map[string]any{}
	if req.Body != nil {
		raw, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading cmdlet parameters: %w", err)
		}
		if s := bytes.TrimSpace(raw); len(s) > 0 && !bytes.Equal(s, []byte("null")) {
			if err := json.Unmarshal(s, &params); err != nil || params == nil {
				return nil, fmt.Errorf("cmdlet parameters must be a JSON object")
			}
		}
	}
	normalizeParams(params)
	input := map[string]any{"CmdletName": cmdlet}
	// The v2.0 docs omit Parameters when there are none; CIPP always sends it on beta.
	if len(params) > 0 || beta {
		input["Parameters"] = params
	}
	env, err := json.Marshal(map[string]any{"CmdletInput": input})
	if err != nil {
		return nil, fmt.Errorf("encoding CmdletInput: %w", err)
	}

	u := *req.URL
	u.Fragment, u.RawFragment = "", ""
	if fragmentQuery != "" {
		if u.RawQuery != "" {
			u.RawQuery += "&"
		}
		u.RawQuery += fragmentQuery
	}
	// Every page goes out as the kit reads it: this one, and each nextLink below,
	// through the same helper. A literal $ in a query is ordinary OData.
	if u.RawQuery, err = cliutil.CanonicalQuery(u.RawQuery); err != nil {
		return nil, fmt.Errorf("the query of the %s request cannot be read: %w", cmdlet, err)
	}
	// Sync sends the public parameter name; the wire name is $select.
	if q := u.Query(); q.Has("properties") {
		if !q.Has("$select") {
			q.Set("$select", q.Get("properties"))
		}
		q.Del("properties")
		u.RawQuery = q.Encode()
	}
	anchor := req.Header.Get("X-AnchorMailbox")
	if anchor == "" {
		anchor = mailboxAnchor(cmdlet, params)
	}
	if anchor == "" {
		anchor = strings.TrimSpace(os.Getenv(anchorEnv))
	}
	if anchor == "" {
		anchor = "APP:SystemMailbox{" + systemMailboxGUID + "}@" + tenant
	}

	var rows []json.RawMessage
	var warnings []json.RawMessage
	var total int64
	var first *http.Response
	sawValue := false
	seenLinks := map[string]bool{}
	for page := 1; ; page++ {
		out := req.Clone(req.Context())
		pu := u
		out.URL = &pu
		out.Host = ""
		out.Body = io.NopCloser(bytes.NewReader(env))
		out.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(env)), nil }
		out.ContentLength = int64(len(env))
		out.Header.Set("Content-Type", "application/json; charset=utf-8")
		out.Header.Set("X-AnchorMailbox", anchor)
		// Exchange answers an unroled app with a 403 whose body is NUL bytes
		// labelled gzip (MSPbots exchange_online_mcp README); ask for identity.
		out.Header.Set("Accept-Encoding", "identity")
		if out.Header.Get("Prefer") == "" {
			out.Header.Set("Prefer", "odata.maxpagesize=1000")
		}
		resp, err := t.base.RoundTrip(out)
		if err != nil || resp == nil {
			return resp, err
		}
		body, err := readEXOBody(resp, maxEXOBytes-total)
		if err != nil {
			if resp.StatusCode >= 400 {
				// Keep the status: an unreadable error body is still that error.
				return withBody(resp, exoStatusBody(resp.StatusCode)), nil
			}
			return nil, err
		}
		total += int64(len(body))
		if resp.StatusCode >= 400 {
			if len(bytes.Trim(body, "\x00 \t\r\n")) == 0 {
				return withBody(resp, exoStatusBody(resp.StatusCode)), nil
			}
			return withBody(resp, exoErrorBody(body)), nil
		}
		if first == nil {
			first = resp
		}
		var pageBody struct {
			Value    json.RawMessage `json:"value"`
			Next     string          `json:"@odata.nextLink"`
			Warnings json.RawMessage `json:"@adminapi.warnings"`
		}
		if len(bytes.TrimSpace(body)) == 0 || json.Unmarshal(body, &pageBody) != nil {
			if page == 1 {
				// No envelope (for example 200 with an empty body after Set-*).
				return withBody(resp, body), nil
			}
			return nil, fmt.Errorf("page %d of %s was not a JSON object", page, cmdlet)
		}
		if len(pageBody.Warnings) > 0 && string(pageBody.Warnings) != "null" {
			var ws []json.RawMessage
			if json.Unmarshal(pageBody.Warnings, &ws) != nil {
				ws = []json.RawMessage{pageBody.Warnings}
			}
			warnings = append(warnings, ws...)
		}
		if len(pageBody.Value) > 0 && string(pageBody.Value) != "null" {
			sawValue = true
			var vals []json.RawMessage
			if err := json.Unmarshal(pageBody.Value, &vals); err != nil {
				vals = []json.RawMessage{pageBody.Value} // a single object
			}
			for _, v := range vals {
				rows = append(rows, stripODataTypes(v))
			}
		}
		if pageBody.Next == "" {
			break
		}
		// Fail rather than return a silently truncated result set.
		if page >= maxEXOPages {
			return nil, fmt.Errorf("%s returned more than %d pages; narrow the query (ResultSize, Filter, Identity)", cmdlet, maxEXOPages)
		}
		next, err := sameOrigin(req.URL, pageBody.Next)
		if err != nil {
			return nil, err
		}
		if next.RawQuery, err = cliutil.CanonicalQuery(next.RawQuery); err != nil {
			return nil, fmt.Errorf("invalid @odata.nextLink: %w", err)
		}
		key := pageKey(next)
		if seenLinks[key] {
			return nil, fmt.Errorf("%s paging did not advance (repeated @odata.nextLink): %w", cmdlet, cliutil.ErrUnreadableRequest)
		}
		seenLinks[key] = true
		u = *next
	}

	var merged []byte
	switch {
	case len(rows) > 0 || sawValue:
		if rows == nil {
			rows = []json.RawMessage{}
		}
		merged, err = json.Marshal(rows)
	case len(warnings) > 0:
		// Matches CIPP: a cmdlet that only warns returns the warnings as its rows.
		merged, err = json.Marshal(warnings)
	default:
		merged = []byte("[]")
	}
	if err != nil {
		return nil, fmt.Errorf("encoding %s rows: %w", cmdlet, err)
	}
	// Warnings that came with a value (even an empty one) would otherwise be lost.
	if sawValue {
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "warning: %s: %s\n", cmdlet, strings.Trim(string(w), `"`))
		}
	}
	return withBody(first, merged), nil
}

// normalizeParams fixes the two shapes a string CLI flag cannot express: the
// documented integer form of ResultSize, and the delegate list of
// GrantSendOnBehalfTo (comma-separated addresses, or the add/remove hash table,
// which Learn shows with "@odata.type": "#Exchange.GenericHashTable").
func normalizeParams(params map[string]any) {
	if s, ok := params["ResultSize"].(string); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
			params["ResultSize"] = n
		}
	}
	switch v := params["GrantSendOnBehalfTo"].(type) {
	case string:
		if strings.Contains(v, ",") {
			var list []string
			for _, p := range strings.Split(v, ",") {
				if p = strings.TrimSpace(p); p != "" {
					list = append(list, p)
				}
			}
			params["GrantSendOnBehalfTo"] = list
		}
	case map[string]any:
		if _, ok := v["@odata.type"]; !ok {
			v["@odata.type"] = "#Exchange.GenericHashTable"
		}
	}
}

// mailboxAnchor routes a mailbox-bound cmdlet to the mailbox it names when that
// value is a UPN or SMTP address (folder identities look like user@x:\Calendar).
func mailboxAnchor(cmdlet string, params map[string]any) string {
	key, ok := anchorParam[cmdlet]
	if !ok {
		return ""
	}
	v, _ := params[key].(string)
	if i := strings.Index(v, ":\\"); i >= 0 {
		v = v[:i]
	}
	v = strings.TrimSpace(v)
	if !strings.Contains(v, "@") || strings.ContainsAny(v, " \t\r\n") {
		return ""
	}
	return "UPN:" + v
}

// sameOrigin resolves a nextLink and refuses any host other than the one the
// bearer token was sent to, so a crafted link cannot exfiltrate it.
func sameOrigin(orig *url.URL, link string) (*url.URL, error) {
	next, err := orig.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("invalid @odata.nextLink: %w: %w", err, cliutil.ErrUnreadableRequest)
	}
	if !strings.EqualFold(next.Scheme, orig.Scheme) || !strings.EqualFold(next.Host, orig.Host) {
		return nil, fmt.Errorf("refusing @odata.nextLink to another origin (%s://%s): %w", next.Scheme, next.Host, cliutil.ErrUnreadableRequest)
	}
	// The kit holds a URL's user name and password on sight, so a link could choose
	// what every later unit may say.
	if next.User.String() != orig.User.String() {
		return nil, fmt.Errorf("refusing @odata.nextLink that carries a user name or password: %w", cliutil.ErrUnreadableRequest)
	}
	next.Fragment, next.RawFragment = "", ""
	return next, nil
}

// pageKey names the page a link with a canonical query asks for. Links that
// differ only by encoding (the host's case, an escaped path or query) are one
// page, so a server cannot defeat the repeat guard by respelling a link.
func pageKey(u *url.URL) string {
	q, _ := url.ParseQuery(u.RawQuery) // CanonicalQuery has read it
	return strings.ToLower(u.Scheme+"://"+u.Host) + u.Path + "?" + q.Encode()
}

func readEXOBody(resp *http.Response, limit int64) ([]byte, error) {
	defer resp.Body.Close()
	var src io.Reader = resp.Body
	// net/http only decompresses transparently when it chose Accept-Encoding itself.
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		zr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("decoding gzip response: %w", err)
		}
		defer zr.Close()
		src = zr
		resp.Header.Del("Content-Encoding")
		resp.Uncompressed = true
	}
	raw, err := io.ReadAll(io.LimitReader(src, limit+1))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("response exceeds %d MiB; narrow the query (ResultSize, Filter, --properties)", maxEXOBytes>>20)
	}
	return raw, nil
}

// exoErrorBody keeps the error status but surfaces the cmdlet's own message,
// which Exchange nests under error.details or error.innererror (CIPP reads the
// same three places).
func exoErrorBody(body []byte) []byte {
	var e struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details []struct {
				Message string `json:"message"`
			} `json:"details"`
			Inner struct {
				Internal struct {
					Message string `json:"message"`
				} `json:"internalException"`
			} `json:"innererror"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) != nil {
		return body
	}
	msg := e.Error.Message
	for _, d := range e.Error.Details {
		if d.Message != "" {
			msg = d.Message
			break
		}
	}
	if msg == e.Error.Message && e.Error.Inner.Internal.Message != "" {
		msg = e.Error.Inner.Internal.Message
	}
	if msg == "" {
		return body
	}
	out, _ := json.Marshal(map[string]any{"error": map[string]string{"code": e.Error.Code, "message": msg}})
	return out
}

// exoStatusBody describes an error response that carried no readable body.
func exoStatusBody(status int) []byte {
	msg := http.StatusText(status)
	if status == http.StatusForbidden {
		msg += ": the token is valid but has no Exchange role for this cmdlet (assign one such as Exchange Administrator)"
	}
	out, _ := json.Marshal(map[string]any{"error": map[string]string{"code": strconv.Itoa(status), "message": msg}})
	return out
}

// stripODataTypes drops the "<prop>@odata.type" companion keys Exchange adds
// next to typed properties; they carry no data.
func stripODataTypes(v json.RawMessage) json.RawMessage {
	var obj map[string]json.RawMessage
	if json.Unmarshal(v, &obj) != nil {
		return v
	}
	changed := false
	for k := range obj {
		if strings.HasSuffix(k, "@odata.type") {
			delete(obj, k)
			changed = true
		}
	}
	if !changed {
		return v
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return v
	}
	return out
}

func withBody(resp *http.Response, body []byte) *http.Response {
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Del("Content-Length")
	resp.Header.Del("Content-Encoding")
	return resp
}
