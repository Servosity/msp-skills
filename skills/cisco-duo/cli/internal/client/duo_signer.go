// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (not generated): Duo Admin API v5 request signing and MSP
// subaccount routing. Survives reprints because the generator never emits
// this file; internal/cli/duo_hooks.go installs it via registerClientHook.
//
// Reference: https://duo.com/docs/adminapi#authentication (v5), and the
// official clients duosecurity/duo_api_golang (canonicalizeV5/signV5) and
// duosecurity/duo_client_python (canonicalize, sig_version 5).

package client

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"cisco-duo-pp-cli/internal/cliutil"
)

// DuoAccountEnv pins every Admin API call to one MSP subaccount unless the
// request already carries its own account_id.
const DuoAccountEnv = "CISCO_DUO_ACCOUNT_ID"

// PinnedAccountID is the subaccount pinned through DuoAccountEnv, or "".
// EnvOverride treats an unresolved MCPB "${user_config.*}" placeholder (left by
// a host for a blank optional field) as unset, so a one-click install never
// pins every call to a subaccount literally named after the template.
func PinnedAccountID() string { return strings.TrimSpace(cliutil.EnvOverride(DuoAccountEnv)) }

const duoAccountsListPath = "/accounts/v1/account/list"

// AccountResolvedHook, when set, is told the account ID the pinned subaccount
// (ID or name, as pinned) resolved to, before each request routed to it is
// sent; an error refuses the request.
var AccountResolvedHook func(requested, accountID string) error

// UnpinnedAccountGuard, when set, is consulted before a request that names
// its own subaccount (account_id in the query or JSON body) is sent while no
// subaccount is pinned; an error refuses the request. With a pin, a request
// may only name the pinned subaccount (see RoundTrip).
var UnpinnedAccountGuard func(requested string) error

// duoSigner replaces the static "Basic ikey:skey" header the generated config
// builds with a per-request Duo v5 HMAC-SHA512 signature, so the secret key
// itself never leaves the process.
type duoSigner struct {
	next http.RoundTripper
	now  func() time.Time
}

// duoAccounts caches each parent's subaccount list, keyed by parent host and
// ikey, for the process lifetime (the MCP server builds a new client, and so
// a new signer, per call). Concurrent callers share one in-flight load per
// key and wait for it with their own context; the map lock is never held
// across network I/O.
var duoAccounts = struct {
	sync.Mutex
	byKey map[string]*duoAccountEntry
}{byKey: map[string]*duoAccountEntry{}}

type duoAccountEntry struct {
	done chan struct{} // closed when set/err are final
	set  *duoAccountSet
	err  error
}

// storeDuoAccountSet replaces key's cached list with a newer one.
func storeDuoAccountSet(key string, set *duoAccountSet) {
	e := &duoAccountEntry{done: make(chan struct{}), set: set}
	close(e.done)
	duoAccounts.Lock()
	duoAccounts.byKey[key] = e
	duoAccounts.Unlock()
}

// duoAccountSetFor returns the cached list for key, loading it with fetch
// when absent. loadedHere reports whether this call performed the load (so
// the list is at least as new as the caller's request). Failed loads are not
// cached.
func duoAccountSetFor(ctx context.Context, key string, fetch func(context.Context) ([]DuoSubaccount, error)) (set *duoAccountSet, loadedHere bool, err error) {
	duoAccounts.Lock()
	e := duoAccounts.byKey[key]
	start := e == nil
	if start {
		e = &duoAccountEntry{done: make(chan struct{})}
		duoAccounts.byKey[key] = e
	}
	duoAccounts.Unlock()
	if start {
		accounts, ferr := fetch(ctx)
		if ferr != nil {
			e.err = ferr
			duoAccounts.Lock()
			if duoAccounts.byKey[key] == e {
				delete(duoAccounts.byKey, key)
			}
			duoAccounts.Unlock()
		} else {
			e.set = newDuoAccountSet(accounts)
		}
		close(e.done)
		return e.set, true, e.err
	}
	select {
	case <-e.done:
		return e.set, false, e.err
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
}

type duoAccountSet struct {
	byID   map[string]DuoSubaccount   // upper(account_id) -> row
	byName map[string][]DuoSubaccount // lower(name) -> rows; >1 means ambiguous
}

func newDuoAccountSet(accounts []DuoSubaccount) *duoAccountSet {
	set := &duoAccountSet{byID: map[string]DuoSubaccount{}, byName: map[string][]DuoSubaccount{}}
	for _, a := range accounts {
		if a.AccountID == "" || a.APIHostname == "" {
			continue
		}
		set.byID[strings.ToUpper(a.AccountID)] = a
		if n := strings.ToLower(strings.TrimSpace(a.Name)); n != "" {
			set.byName[n] = append(set.byName[n], a)
		}
	}
	return set
}

// lookup finds a subaccount by ID (case-insensitive) or exact name; found is
// false only when nothing matches (an ambiguous name is found with an error).
func (set *duoAccountSet) lookup(idOrName string) (acct DuoSubaccount, found bool, err error) {
	if a, ok := set.byID[strings.ToUpper(strings.TrimSpace(idOrName))]; ok {
		return a, true, nil
	}
	switch matches := set.byName[strings.ToLower(strings.TrimSpace(idOrName))]; len(matches) {
	case 0:
		return DuoSubaccount{}, false, &accountError{status: http.StatusBadRequest, code: 40002,
			msg: fmt.Sprintf("subaccount %q not found under this parent account (run 'accounts list')", idOrName)}
	case 1:
		return matches[0], true, nil
	default:
		ids := make([]string, 0, len(matches))
		for _, m := range matches {
			ids = append(ids, m.AccountID)
		}
		return DuoSubaccount{}, true, &accountError{status: http.StatusBadRequest, code: 40002,
			msg: fmt.Sprintf("subaccount name %q matches %d accounts (%s); pass the account ID", idOrName, len(matches), strings.Join(ids, ", "))}
	}
}

// NewDuoSigner wraps next with Duo v5 signing; the CLI installs it through
// registerClientHook so generated and MCP clients both sign every request.
func NewDuoSigner(next http.RoundTripper) http.RoundTripper { return newDuoSigner(next) }

func newDuoSigner(next http.RoundTripper) *duoSigner {
	if next == nil {
		next = http.DefaultTransport
	}
	return &duoSigner{next: next, now: time.Now}
}

func (s *duoSigner) RoundTrip(req *http.Request) (*http.Response, error) {
	// The generated config sends "Basic ikey:skey"; the secret only signs.
	ikey, skey, ok := duoCredentials(req.Header.Get("Authorization"))
	if !ok {
		// Nothing signable: never forward an Authorization header we could not
		// parse (it may still carry the secret); Duo answers 401 itself.
		unsigned := req.Clone(req.Context())
		unsigned.Header.Del("Authorization")
		return s.next.RoundTrip(unsigned)
	}
	out := req.Clone(req.Context())
	body, err := readBody(req)
	if err != nil {
		return nil, err
	}

	query := out.URL.Query()
	hasBody := out.Method == http.MethodPost || out.Method == http.MethodPut || out.Method == http.MethodPatch

	// Subaccount routing: account_id travels as a normal parameter (query for
	// GET/DELETE, JSON body for POST/PUT/PATCH); the request must also go to
	// that subaccount's api_hostname. /accounts/v1/* always targets the parent.
	//
	// The pin (--account-id / CISCO_DUO_ACCOUNT_ID) also selects the local
	// store the response is written to and read from, so a request may not
	// name a different subaccount of its own (a --stdin body, an import line,
	// an MCP argument): it would reach that customer while its response landed
	// in the pinned customer's mirror. Unpinned, the store is the parent's, and
	// UnpinnedAccountGuard decides whether a request may name a subaccount.
	if !strings.HasPrefix(out.URL.Path, "/accounts/") {
		pinned := PinnedAccountID()
		accountID, fromRequest := "", false
		if hasBody {
			if body, accountID, fromRequest, err = bodyAccountID(body, pinned); err != nil {
				return accountFailure(out, err)
			}
		} else {
			if vals := query["account_id"]; len(vals) > 1 {
				return refuseRequest(out, fmt.Sprintf("the request names account_id %d times; pin one subaccount with --account-id (or %s) instead", len(vals), DuoAccountEnv)), nil
			}
			accountID = strings.TrimSpace(query.Get("account_id"))
			fromRequest = accountID != ""
			if !fromRequest && pinned != "" {
				accountID = pinned
			}
		}
		if fromRequest && pinned == "" {
			if guard := UnpinnedAccountGuard; guard != nil {
				if err := guard(accountID); err != nil {
					return refuseRequest(out, err.Error()), nil
				}
			}
		}
		if accountID != "" {
			acct, err := s.resolveAccount(out.Context(), out.URL, ikey, skey, accountID)
			if err != nil {
				return accountFailure(out, err)
			}
			if pinned != "" {
				pinAcct := acct
				if fromRequest && !strings.EqualFold(accountID, pinned) {
					if pinAcct, err = s.resolveAccount(out.Context(), out.URL, ikey, skey, pinned); err != nil {
						return accountFailure(out, err)
					}
				}
				if hook := AccountResolvedHook; hook != nil {
					if err := hook(pinned, pinAcct.AccountID); err != nil {
						return refuseRequest(out, err.Error()), nil
					}
				}
				if !strings.EqualFold(pinAcct.AccountID, acct.AccountID) {
					return refuseRequest(out, fmt.Sprintf("the request names subaccount %q (%s), but this run is pinned to %q (%s) and would store the response as that customer's data; pin %s instead (--account-id, or %s for the MCP server) and leave account_id out of the request", accountID, acct.AccountID, pinned, pinAcct.AccountID, acct.AccountID, DuoAccountEnv)), nil
				}
			}
			// Always send the resolved ID (a name becomes its ID, and a body
			// that repeats the key is re-encoded with exactly one account_id).
			if hasBody {
				if body, err = setJSONField(body, "account_id", acct.AccountID); err != nil {
					return nil, err
				}
			} else {
				query.Set("account_id", acct.AccountID)
			}
			out.URL.Host = acct.APIHostname
			out.Host = acct.APIHostname
		}
	}

	if hasBody {
		if len(bytes.TrimSpace(body)) == 0 {
			body = []byte("{}")
		}
		out.Header.Set("Content-Type", "application/json")
	} else {
		body = nil
	}
	canon := duoCanonParams(query)
	out.URL.RawQuery = canon
	setBody(out, body)
	s.sign(out, ikey, skey, canon, body)
	return s.next.RoundTrip(out)
}

// sign stamps the Date header and the v5 Authorization header on req.
func (s *duoSigner) sign(req *http.Request, ikey, skey, canonQuery string, body []byte) {
	date := s.now().UTC().Format(time.RFC1123Z)
	req.Header.Set("Date", date)
	req.Header.Set("Authorization", duoSignature(ikey, skey, req.Method, req.URL.Host, req.URL.Path, canonQuery, body, req.Header, date))
}

// resolveAccount maps a subaccount ID (case-insensitive) or its exact name
// to the subaccount row from the parent's /accounts/v1/account/list. Only an
// ID that matches a cached account's ID is answered from the cache (an ID
// names one tenant for good). Anything else, names included, is resolved
// against a list fetched for this request, so a subaccount created since the
// cached load is found and a newly shared name is reported as ambiguous.
// ponytail: one account-list call per request routed by name; pass the ID
// (as fleet fan-out does) to avoid it.
func (s *duoSigner) resolveAccount(ctx context.Context, parent *url.URL, ikey, skey, idOrName string) (DuoSubaccount, error) {
	key := strings.ToLower(parent.Host) + "|" + ikey
	fetch := func(ctx context.Context) ([]DuoSubaccount, error) { return s.listAccounts(ctx, parent, ikey, skey) }
	set, loadedHere, err := duoAccountSetFor(ctx, key, fetch)
	if err != nil {
		return DuoSubaccount{}, resolveErr(idOrName, err)
	}
	if acct, ok := set.byID[strings.ToUpper(strings.TrimSpace(idOrName))]; ok {
		return acct, nil
	}
	if !loadedHere {
		accounts, err := fetch(ctx)
		if err != nil {
			return DuoSubaccount{}, resolveErr(idOrName, err)
		}
		set = newDuoAccountSet(accounts)
		storeDuoAccountSet(key, set)
	}
	acct, _, err := set.lookup(idOrName)
	return acct, err
}

func resolveErr(idOrName string, err error) error {
	var ae *accountError
	if errors.As(err, &ae) {
		return &accountError{status: ae.status, code: ae.code, msg: fmt.Sprintf("resolving subaccount %q: %s", idOrName, ae.msg)}
	}
	return fmt.Errorf("resolving subaccount %q: %w", idOrName, err)
}

// DuoSubaccount is one row of /accounts/v1/account/list.
type DuoSubaccount struct {
	AccountID   string `json:"account_id"`
	Name        string `json:"name"`
	APIHostname string `json:"api_hostname"`
}

func (s *duoSigner) listAccounts(ctx context.Context, parent *url.URL, ikey, skey string) ([]DuoSubaccount, error) {
	u := url.URL{Scheme: parent.Scheme, Host: parent.Host, Path: duoAccountsListPath}
	body := []byte("{}")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	s.sign(req, ikey, skey, "", body)
	resp, err := s.next.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	var env struct {
		Stat     string          `json:"stat"`
		Code     int             `json:"code"`
		Message  string          `json:"message"`
		Response []DuoSubaccount `json:"response"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || resp.StatusCode != http.StatusOK || env.Stat != "OK" {
		status := resp.StatusCode
		if status == http.StatusOK {
			status = http.StatusBadGateway // 200 without a usable OK envelope
		}
		return nil, &accountError{status: status, code: env.Code,
			msg: fmt.Sprintf("HTTP %d from %s: %s", resp.StatusCode, duoAccountsListPath, env.Message)}
	}
	return env.Response, nil
}

// accountError is a subaccount-resolution failure that the signer reports as
// a Duo-shaped HTTP error response rather than a transport error.
type accountError struct {
	status, code int
	msg          string
}

func (e *accountError) Error() string { return e.msg }

func (e *accountError) response(req *http.Request) *http.Response {
	code := e.code
	if code == 0 {
		code = e.status * 100
	}
	body, _ := json.Marshal(map[string]any{"stat": "FAIL", "code": code, "message": e.msg})
	return &http.Response{
		StatusCode:    e.status,
		Status:        fmt.Sprintf("%d %s", e.status, http.StatusText(e.status)),
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": {"application/json"}},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}
}

// duoCredentials recovers ikey/skey from "Basic base64(ikey:skey)", tolerating
// surrounding whitespace (Go's header writer trims it on the wire).
func duoCredentials(header string) (string, string, bool) {
	scheme, encoded, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "Basic") {
		return "", "", false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", "", false
	}
	ikey, skey, ok := strings.Cut(string(raw), ":")
	if !ok || ikey == "" || skey == "" {
		return "", "", false
	}
	return ikey, skey, true
}

// refuseRequest answers a refused request the way Duo answers a bad
// parameter (HTTP 400, code 40002), so the client reports it instead of
// retrying it as a network failure.
func refuseRequest(req *http.Request, msg string) *http.Response {
	return (&accountError{status: http.StatusBadRequest, code: 40002, msg: msg}).response(req)
}

// accountFailure turns a subaccount-resolution error into the Duo-shaped
// response it carries, or passes a transport error through.
func accountFailure(req *http.Request, err error) (*http.Response, error) {
	var ae *accountError
	if errors.As(err, &ae) {
		// Answer like Duo would, so the client classifies it (401, 400)
		// instead of retrying it as a network failure.
		return ae.response(req), nil
	}
	return nil, err
}

// bodyAccountID returns the JSON body with account_id set from the pin when
// the caller did not supply one, the effective account_id, and whether the
// request named that account itself. A null or empty account_id counts as
// absent. An account_id that is not a string, and (with a pin) a body that is
// not a JSON object, are refused: neither can be compared with the pin or
// routed, and the request would otherwise reach the parent account while its
// response was stored as the pinned customer's.
func bodyAccountID(body []byte, pinned string) ([]byte, string, bool, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		if pinned == "" {
			return body, "", false, nil
		}
		b, err := json.Marshal(map[string]string{"account_id": pinned})
		return b, pinned, false, err
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		if pinned != "" {
			return nil, "", false, &accountError{status: http.StatusBadRequest, code: 40002,
				msg: fmt.Sprintf("a request body that is not a JSON object cannot carry the pinned subaccount %q", pinned)}
		}
		return body, "", false, nil // not an object: leave untouched
	}
	if obj == nil { // a JSON null body
		obj = map[string]json.RawMessage{}
	}
	if raw, ok := obj["account_id"]; ok {
		var id *string
		if err := json.Unmarshal(raw, &id); err != nil {
			return nil, "", false, &accountError{status: http.StatusBadRequest, code: 40002,
				msg: "account_id in the request body must be a string (a subaccount ID or exact name)"}
		}
		if id != nil && strings.TrimSpace(*id) != "" {
			return body, strings.TrimSpace(*id), true, nil
		}
	}
	if pinned == "" {
		return body, "", false, nil
	}
	obj["account_id"], _ = json.Marshal(pinned)
	b, err := json.Marshal(obj)
	return b, pinned, false, err
}

func setJSONField(body []byte, key, value string) ([]byte, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, err
	}
	if obj == nil {
		obj = map[string]json.RawMessage{}
	}
	obj[key], _ = json.Marshal(value)
	return json.Marshal(obj)
}

// duoCanonParams is Duo's canonical query string: keys and values sorted,
// RFC 3986 escaping (space -> %20, upper-case hex, ~ unescaped). Go's
// url.Values.Encode sorts keys and escapes like Duo except spaces.
func duoCanonParams(params url.Values) string {
	sorted := url.Values{}
	for k, v := range params {
		vals := append([]string(nil), v...)
		sort.Strings(vals)
		sorted[k] = vals
	}
	return cliutil.EncodeQuery(sorted)
}

// duoSignV5 signs with no X-Duo-* headers (used by the golden-vector tests).
func duoSignV5(ikey, skey, method, host, path string, params url.Values, body []byte, date string) string {
	return duoSignature(ikey, skey, method, host, path, duoCanonParams(params), body, nil, date)
}

// duoCanonHeaders hashes every X-Duo-* header per the v5 spec: lower-cased
// names, sorted, trimmed values, joined name\x00value\x00...; SHA-512 of
// the empty string when there are none.
func duoCanonHeaders(h http.Header) string {
	names := make([]string, 0)
	vals := map[string]string{}
	for name, v := range h {
		lower := strings.ToLower(name)
		if !strings.HasPrefix(lower, "x-duo-") || len(v) == 0 {
			continue
		}
		names = append(names, lower)
		vals[lower] = strings.TrimSpace(strings.Join(v, ","))
	}
	sort.Strings(names)
	parts := make([]string, 0, 2*len(names))
	for _, n := range names {
		parts = append(parts, n, vals[n])
	}
	return sha512Hex([]byte(strings.Join(parts, "\x00")))
}

// duoSignature is the v5 Authorization value for an already-canonical query.
func duoSignature(ikey, skey, method, host, path, canonQuery string, body []byte, headers http.Header, date string) string {
	canon := strings.Join([]string{
		date,
		strings.ToUpper(method),
		strings.ToLower(host),
		path,
		canonQuery,
		sha512Hex(body),
		duoCanonHeaders(headers),
	}, "\n")
	mac := hmac.New(sha512.New, []byte(skey))
	mac.Write([]byte(canon))
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(ikey+":"+hex.EncodeToString(mac.Sum(nil))))
}

func sha512Hex(b []byte) string {
	sum := sha512.Sum512(b)
	return hex.EncodeToString(sum[:])
}

func readBody(req *http.Request) ([]byte, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, nil
	}
	if req.GetBody != nil {
		rc, err := req.GetBody()
		if err == nil {
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	defer req.Body.Close()
	return io.ReadAll(req.Body)
}

func setBody(req *http.Request, body []byte) {
	if body == nil {
		req.Body = http.NoBody
		req.GetBody = nil
		req.ContentLength = 0
		req.Header.Del("Content-Type")
		return
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	req.ContentLength = int64(len(body))
}
