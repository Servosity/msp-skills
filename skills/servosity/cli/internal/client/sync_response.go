// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/servosity/handfixes.json under
// the "sync-json-only-bounded" entry. See docs/reprint-survival.md.

package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ErrNonJSONResponse marks a successful response refused before its body was
// read, under WithJSONOnlyResponses.
var ErrNonJSONResponse = errors.New("response is not JSON")

// ErrOffOriginRedirect marks a redirect off the API's origin, not followed under
// WithJSONOnlyResponses.
var ErrOffOriginRedirect = errors.New("redirected off the API's origin; not followed")

type jsonOnlyResponsesKey struct{}

// WithJSONOnlyResponses makes every request under ctx refuse a successful
// response whose Content-Type is not JSON, before reading its body. Sync keeps
// rows, and a body that is not JSON is not rows: buffered and scanned whole, one
// large installer costs tens of gigabytes. Commands that read such a body on
// purpose do not set this.
//
// ponytail: the credential kit reads a body whole, to its own 64 MiB limit, inside
// its RoundTrip and so before this check, because the connector declares
// ResponseFields. Wrapping the transport inside the kit would give up its wire
// checks, so a binary from the API's own origin costs that one read. Upgrade path:
// a kit hook that stops at a non-JSON header. A redirect off the origin, the real
// installer's route, is stopped before any byte by jsonOnlyCheckRedirect.
func WithJSONOnlyResponses(ctx context.Context) context.Context {
	return context.WithValue(ctx, jsonOnlyResponsesKey{}, true)
}

func refuseNonJSONResponse(ctx context.Context, resp *http.Response) error {
	if ctx.Value(jsonOnlyResponsesKey{}) == nil {
		return nil
	}
	// An error status keeps its own handling, and an empty body has no type.
	if resp.StatusCode >= http.StatusBadRequest || resp.StatusCode == http.StatusNoContent || resp.ContentLength == 0 {
		return nil
	}
	mediaType, _, _ := strings.Cut(resp.Header.Get("Content-Type"), ";")
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if mediaType == "application/json" || mediaType == "text/json" || strings.HasSuffix(mediaType, "+json") {
		return nil
	}
	if len(mediaType) > 64 {
		mediaType = mediaType[:64]
	}
	return fmt.Errorf("%w (Content-Type %q); body not read", ErrNonJSONResponse, mediaType)
}

// jsonOnlyCheckRedirect stops a redirect off the API's origin under ctx. Sync's
// credentials do not follow one, so what is there cannot be this tenant's rows.
func jsonOnlyCheckRedirect(ctx context.Context, next func(*http.Request, []*http.Request) error) func(*http.Request, []*http.Request) error {
	if ctx.Value(jsonOnlyResponsesKey{}) == nil {
		return next
	}
	return func(req *http.Request, via []*http.Request) error {
		if redirectLeavesOrigin(req.URL, via) {
			return ErrOffOriginRedirect
		}
		if next != nil {
			return next(req, via)
		}
		return nil
	}
}
