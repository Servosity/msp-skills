// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-fix (handfixes.json vault-secrets-withheld): Hudu answers in plaintext
// with two kinds of tenant secret. /api/v1/asset_passwords returns every vault
// entry's `password` and `otp_secret` (the TOTP seed), and /api/v1/assets
// returns the value of every asset layout field, Password and
// ConfidentialText included. Without this file those values reached the MCP
// caller, error text, the local SQLite mirror (and so `search` and MCP `sql`)
// and the five-minute HTTP response cache.
//
// The decision is made by RESOURCE, never by the shape of a body, using the
// shared rule in internal/huduprotect:
//
//   - do, doRead and doMutation put an answeredURLs holder in the context they
//     pass to doInternal, and doInternal records the URL that actually
//     answered (after redirects) into it. A response is protected when the
//     requested path OR any answered URL names the vault or assets.
//   - doInternal replaces the whole error body of a protected response with a
//     fixed message, so no vendor text (JSON, plain text, an HTML title)
//     reaches an error.
//   - withholdProtectedResponse filters every successful protected body
//     before it leaves this package: the vault keeps an exact metadata
//     allowlist, assets get their password-type field values replaced.
//     GetWithHeaders runs the same filter on every cached body it would serve.
//   - The response cache reads and writes only files named with
//     protectedCacheEntrySuffix. A body cached by an older binary, which
//     recorded no answered URL (a redirect from an unprotected path into the
//     vault was cached raw under the unprotected path), sits under a name this
//     client never reads.
//   - A body the filter cannot classify is withheld whole as an error.

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"hudu-pp-cli/internal/huduprotect"
)

// protectedCacheEntrySuffix names every response-cache file this client reads
// or writes. Older binaries named theirs "<key>.json" (no answered-URL rule)
// or "<key>.hudu-protect-1.json" (cached protected responses), so none of
// their bodies is ever served. Change it whenever the rule changes what a
// cached body may hold.
const protectedCacheEntrySuffix = ".hudu-protect-2.json"

// ErrVaultResponseWithheld is the sentinel every refusal from the
// password-vault filter wraps, so callers can tell it apart from an API error.
var ErrVaultResponseWithheld = errors.New("password-vault response withheld")

// ErrAssetResponseWithheld is the sentinel every refusal from the asset filter
// wraps.
var ErrAssetResponseWithheld = errors.New("asset response withheld")

// protectedErrorBodyWithheld replaces the whole server body of an error from a
// protected resource. The APIError keeps its method, masked path and status
// code, so exit codes and the rate-limit chain still work.
const protectedErrorBodyWithheld = "[response body withheld: Hudu password-vault and asset responses are never shown]"

// The client's own synthetic bodies never came from Hudu, carry no secret, and
// must survive the filter unchanged: the --dry-run envelope (client.dryRun)
// and the verify-mode short-circuit envelope (verifyShortCircuitEnvelope).
var vaultDryRunEnvelope = []byte(`{"dry_run": true}`)

// answeredURLs is the per-call holder do, doRead and doMutation hand to
// doInternal through the context. doInternal records the URL of every response
// it received (the final URL after redirects), so the filter can decide by the
// resource that actually answered as well as the one requested.
type answeredURLs struct {
	mu    sync.Mutex
	paths []string
}

type answeredURLsKey struct{}

// withAnsweredURLs returns ctx carrying a fresh holder, and the holder.
func withAnsweredURLs(ctx context.Context) (context.Context, *answeredURLs) {
	if ctx == nil {
		ctx = context.Background()
	}
	holder := &answeredURLs{}
	return context.WithValue(ctx, answeredURLsKey{}, holder), holder
}

// recordAnsweredURL stores the URL that answered resp in the call's holder.
// A context without a holder records nothing; answerIsProtected still reads
// resp itself.
func recordAnsweredURL(ctx context.Context, resp *http.Response) {
	if ctx == nil || resp == nil || resp.Request == nil || resp.Request.URL == nil {
		return
	}
	holder, _ := ctx.Value(answeredURLsKey{}).(*answeredURLs)
	if holder == nil {
		return
	}
	holder.mu.Lock()
	holder.paths = append(holder.paths, resp.Request.URL.EscapedPath())
	holder.mu.Unlock()
}

// class is the strongest protected class among the requested path and every
// URL that answered this call.
func (h *answeredURLs) class(requested string) huduprotect.Class {
	class := huduprotect.ClassifyPath(requested)
	if h == nil {
		return class
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, p := range h.paths {
		class = huduprotect.Strongest(class, huduprotect.ClassifyPath(p))
	}
	return class
}

// answerIsProtected reports whether a response doInternal is about to turn
// into a result or an error is on a protected resource: the requested path,
// any URL already recorded for this call, or the URL that answered resp.
func answerIsProtected(ctx context.Context, requested string, resp *http.Response) bool {
	var holder *answeredURLs
	if ctx != nil {
		holder, _ = ctx.Value(answeredURLsKey{}).(*answeredURLs)
	}
	class := holder.class(requested)
	if resp != nil && resp.Request != nil && resp.Request.URL != nil {
		class = huduprotect.Strongest(class, huduprotect.ClassifyPath(resp.Request.URL.EscapedPath()))
	}
	return class != huduprotect.None
}

// protectedSeen is set when a call's requested path or any URL that answered
// it is protected. A cached body keeps no answered URL, so GetWithHeaders and
// GetWithHeadersNoCache write the response cache only when it is unset: a
// vault or asset answer, redirected or not, is never cached.
type protectedSeen struct{ seen atomic.Bool }

type protectedSeenKey struct{}

// withProtectedSeen returns ctx carrying a fresh flag, and the flag.
func withProtectedSeen(ctx context.Context) (context.Context, *protectedSeen) {
	if ctx == nil {
		ctx = context.Background()
	}
	flag := &protectedSeen{}
	return context.WithValue(ctx, protectedSeenKey{}, flag), flag
}

// isVaultPath reports whether a request path names Hudu's password-vault
// resource (see huduprotect.ClassifyPath).
func isVaultPath(path string) bool {
	return huduprotect.ClassifyPath(path) == huduprotect.Vault
}

// withholdProtectedResponse is the choke point every response the do, doRead
// and doMutation wrappers return passes, and every cached body GetWithHeaders
// would serve (with a nil holder: the requested path decides there). A
// response outside the protected resources passes through byte-identical. An
// error from a protected resource already carries no vendor text (doInternal
// withheld it); any body beside it is dropped. A successful vault body keeps
// only the metadata allowlist; a successful asset body has its password-type
// field values replaced; a body either filter cannot classify is withheld
// whole and an error is returned instead. The status code is always kept.
func (c *Client) withholdProtectedResponse(ctx context.Context, method, path string, answered *answeredURLs, data json.RawMessage, status int, err error) (json.RawMessage, int, error) {
	class := answered.class(path)
	if class == huduprotect.None {
		return data, status, err
	}
	if ctx != nil {
		if flag, _ := ctx.Value(protectedSeenKey{}).(*protectedSeen); flag != nil {
			flag.seen.Store(true)
		}
	}
	if err != nil {
		return nil, status, err
	}
	if isSyntheticEnvelope(method, path, data) {
		return data, status, nil
	}
	// Only a 2xx body is filtered and shown; any other status (a 3xx Hudu did
	// not redirect, a 1xx) carries a body the filters were not written for.
	if status < 200 || status > 299 {
		sentinel, returns := ErrAssetResponseWithheld, "assets with password-type field values withheld"
		if class == huduprotect.Vault {
			sentinel, returns = ErrVaultResponseWithheld, "password-vault metadata"
		}
		return nil, status, c.protectedWithheldError(method, path, sentinel, returns, fmt.Errorf("HTTP %d is not a success status", status))
	}
	switch class {
	case huduprotect.Vault:
		if _, _, ok := UnwrapBinaryResponse(bytes.TrimSpace(data)); ok {
			return nil, status, c.protectedWithheldError(method, path, ErrVaultResponseWithheld, "password-vault metadata", errors.New("the response is a file download"))
		}
		shown, ferr := huduprotect.FilterVaultBody(data)
		if ferr != nil {
			return nil, status, c.protectedWithheldError(method, path, ErrVaultResponseWithheld, "password-vault metadata", ferr)
		}
		return shown, status, nil
	default:
		if _, _, ok := UnwrapBinaryResponse(bytes.TrimSpace(data)); ok {
			return nil, status, c.protectedWithheldError(method, path, ErrAssetResponseWithheld, "assets with password-type field values withheld", errors.New("the response is a file download"))
		}
		shown, ferr := huduprotect.FilterAssetBody(data, func(layoutID string) (map[string]string, bool) {
			return c.assetLayoutFieldTypes(ctx, layoutID)
		})
		if ferr != nil {
			return nil, status, c.protectedWithheldError(method, path, ErrAssetResponseWithheld, "assets with password-type field values withheld", ferr)
		}
		return shown, status, nil
	}
}

// isSyntheticEnvelope reports the --dry-run and verify-mode envelopes.
func isSyntheticEnvelope(method, path string, data json.RawMessage) bool {
	trimmed := bytes.TrimSpace(data)
	return bytes.Equal(trimmed, vaultDryRunEnvelope) || bytes.Equal(trimmed, bytes.TrimSpace(verifyShortCircuitEnvelope(method, path)))
}

// protectedWithheldError names the request (credentials masked) and the
// reason, never the body.
func (c *Client) protectedWithheldError(method, path string, sentinel error, returns string, reason error) error {
	shownPath := path
	if i := strings.IndexByte(shownPath, '?'); i >= 0 {
		shownPath = shownPath[:i]
	}
	if c != nil {
		shownPath = c.displayURL(shownPath)
	}
	return fmt.Errorf("%s %s: %w: this CLI returns only %s and %v, so the whole response was withheld; open it in the Hudu portal", method, shownPath, sentinel, returns, reason)
}

// assetLayoutFieldTypes returns the field types of one asset layout, keyed by
// normalized label, as Hudu answers for it now. It is asked once per layout
// per filtered body (huduprotect memoizes per body), never remembered across
// bodies: a layout whose field became Password between two pages of a sync is
// read again for the second page. The fetch goes straight to doInternal and
// skips the response cache: it never passes the asset rule, so it cannot
// recurse, and a lookup that a redirect turned into a protected resource
// counts as a failure. ok is false on any failure, and the caller then treats
// the asset's fields as secret.
func (c *Client) assetLayoutFieldTypes(ctx context.Context, layoutID string) (map[string]string, bool) {
	if c == nil || layoutID == "" {
		return nil, false
	}
	return c.fetchAssetLayoutFieldTypes(ctx, layoutID)
}

func (c *Client) fetchAssetLayoutFieldTypes(ctx context.Context, layoutID string) (map[string]string, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	path := "/asset_layouts/" + layoutID
	lookupCtx, answered := withAnsweredURLs(ctx)
	data, status, err := c.doInternal(lookupCtx, http.MethodGet, path, nil, nil, nil, false, false)
	if err != nil || status < 200 || status > 299 || answered.class(path) != huduprotect.None {
		return nil, false
	}
	return huduprotect.ParseLayoutFieldTypes(data)
}

// ErrWithheldMarkerInRequest is the sentinel for a request body that carries
// the placeholder this client shows in place of a password-type asset field
// value.
var ErrWithheldMarkerInRequest = errors.New("request carries a withheld-field placeholder")

// refuseWithheldMarker stops a write that would replace a real password-type
// field value in Hudu with the placeholder: an asset read through this CLI and
// sent back (an `assets update --stdin` round trip, or custom_fields copied
// from `fields`) still carries it. It runs before the dry-run branch, so a
// preview shows the refusal too.
func (c *Client) refuseWithheldMarker(method, path string, body []byte) error {
	if len(body) == 0 || !bytes.Contains(body, []byte(huduprotect.WithheldFieldMarker)) {
		return nil
	}
	shownPath := path
	if i := strings.IndexByte(shownPath, '?'); i >= 0 {
		shownPath = shownPath[:i]
	}
	if c != nil {
		shownPath = c.displayURL(shownPath)
	}
	return fmt.Errorf("%s %s: %w: the body holds %q, which this CLI shows in place of a password field value; sending it would overwrite the real value in Hudu, so the request was not sent. Remove that field from the body, or set its real value", method, shownPath, ErrWithheldMarkerInRequest, huduprotect.WithheldFieldMarker)
}
