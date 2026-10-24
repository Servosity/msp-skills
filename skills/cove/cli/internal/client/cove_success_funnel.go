// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated), cove only: the generated endpoint commands
// send a visa in the JSON-RPC body (--visa) and print Cove's answer on the
// success path, which the shared funnel never sees. Every Cove answer carries
// the rotated session visa, and a JSON-RPC error rides an HTTP 200 answer, so
// both would reach stdout and MCP results unfunnelled. Hand-fix code:
// cove-short-credential-funnel.

package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"os"
	"strings"

	"cove-pp-cli/internal/cliutil"
	"cove-pp-cli/internal/coverpc"
	"cove-pp-cli/internal/platform"
)

// ErrCoveAnswerWithheld refuses a success answer that repeats a credential.
// The host did answer: doctor reports it reachable and still runs the Login.
var ErrCoveAnswerWithheld = errors.New("the answer repeated a credential this program holds, so it was withheld")

// coveHoldRequestVisas holds every visa a request body carries, so the error
// paths and the CLI and MCP backstops withhold an echo of it.
func coveHoldRequestVisas(body []byte) {
	cliutil.FunnelHold(coverpc.Visas(body)...)
}

// coveHoldResponseVisas holds every visa an answer carries the moment its
// bytes are read, before any status, content-type, size or cache branch, so
// every later masker (maskCredentialText masks every held value) and both
// backstops withhold it whatever path the answer then takes: a success, a
// binary download, an HTML page, an HTTP error, a retry.
func coveHoldResponseVisas(respBody []byte) {
	cliutil.FunnelHold(coverpc.Visas(respBody)...)
}

// coveCachedReplayable reports whether a stored answer may be replayed: it
// must pass the same exit check a live answer does (coveCheckReturned). A
// stored answer that fails is never replayed and its file is removed, so no
// visa stays on disk; the caller then fetches and checks the live answer.
func coveCachedReplayable(cacheFile string, data []byte) bool {
	if coveCheckReturned(data) != nil {
		_ = os.Remove(cacheFile)
		return false
	}
	return true
}

// coveExit is the one exit point of the generated client (doInternal returns
// through it): a value leaves only when it passes coveCheckReturned whole
// (a binary envelope included, not just the body it wraps), and an error whose
// text repeats a held credential is replaced by one that keeps its kind and
// HTTP status but not its text.
func coveExit(out json.RawMessage, err error) (json.RawMessage, error) {
	if len(out) > 0 {
		if cerr := coveCheckReturned(out); cerr != nil {
			out = nil
			if err == nil {
				err = cerr
			}
		}
	}
	if err != nil {
		err = coveExitError(err)
	}
	return out, err
}

// coveCheckReturned is the check every value the generated client returns or
// replays from its cache passes: the value as returned, and the payload of a
// binary envelope, each with its visas held, must come through the cove
// success check unchanged.
func coveCheckReturned(data []byte) error {
	views := [][]byte{data}
	if raw, _, ok := UnwrapBinaryResponse(data); ok {
		views = append(views, raw)
	}
	for _, view := range views {
		coveHoldResponseVisas(view)
		if shown, err := coveShownSuccess(nil, view); err != nil || !bytes.Equal(shown, view) {
			return ErrCoveAnswerWithheld
		}
	}
	return nil
}

// coveExitError is err when its text repeats no held credential; otherwise an
// error of the same kind (rate limit, HTTP status) whose text is withheld.
func coveExitError(err error) error {
	text := cliutil.FunnelHeldAll(err.Error())
	if !cliutil.FunnelWithheld(text) {
		return err
	}
	var kept error = errors.New(text)
	var rateLimited *platform.RateLimitedError
	var api *APIError
	switch {
	case errors.As(err, &rateLimited):
		kept = &platform.RateLimitedError{EndpointClass: rateLimited.EndpointClass, Attempts: rateLimited.Attempts, RetryAfter: rateLimited.RetryAfter}
	case errors.As(err, &api):
		kept = &APIError{Method: api.Method, Path: api.Path, StatusCode: api.StatusCode, Body: text}
	}
	if cliutil.FunnelWithheld(cliutil.FunnelHeldAll(kept.Error())) {
		return errors.New(text)
	}
	return kept
}

// coveHoldHeaderVisas holds every visa a response header carries (a header
// named like visa, or a value such as "visa=X"), each header read as one
// "Name: value" line by the one visa discovery.
func coveHoldHeaderVisas(h http.Header) {
	for name, values := range h {
		for _, v := range values {
			cliutil.FunnelHold(coverpc.Visas([]byte(name + ": " + v))...)
		}
	}
}

// coveMediaTypes are the content types a binary envelope may name. A response
// header value is never quoted into a result: any other type is reported as
// application/octet-stream.
var coveMediaTypes = map[string]bool{
	"application/octet-stream": true, "application/pdf": true, "application/zip": true,
	"application/gzip": true, "application/x-gzip": true, "text/csv": true,
	"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true,
}

func coveMediaType(ct string) string {
	if mt, _, err := mime.ParseMediaType(ct); err == nil && coveMediaTypes[mt] {
		return mt
	}
	return "application/octet-stream"
}

// coveTransportBlockHeader is what the 403 annotation may read from the
// response headers: cf-mitigated only with Cloudflare's documented value, so
// no other header value is ever quoted into an error.
func coveTransportBlockHeader(h http.Header) http.Header {
	out := http.Header{}
	if strings.EqualFold(strings.TrimSpace(h.Get("cf-mitigated")), "challenge") {
		out.Set("cf-mitigated", "challenge")
	}
	return out
}

// coveShownSuccess is a success answer as it may be shown: every top-level
// member whose decoded name is "visa" (the rotated session credential) is
// removed, and the rest is refused when it repeats a held credential: any
// value this process holds, a visa the request sent, or a visa the answer
// returned (one left anywhere else in it included).
func coveShownSuccess(reqBody, respBody []byte) (json.RawMessage, error) {
	held := append(append(cliutil.FunnelHeldValues(), coverpc.Visas(reqBody)...), coverpc.Visas(respBody)...)
	shown := coveDropVisaMembers(respBody)
	if cliutil.FunnelJSONEchoes(shown, held) {
		return nil, ErrCoveAnswerWithheld
	}
	return json.RawMessage(shown), nil
}

// coveDropVisaMembers removes every top-level member whose decoded name is
// "visa" (any case, escaped or not, duplicates included) from a JSON object.
// Anything that is not a single JSON object is returned unchanged.
func coveDropVisaMembers(body []byte) []byte {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return body
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &members); err != nil {
		return body
	}
	dropped := false
	for name := range members {
		if strings.EqualFold(strings.TrimSpace(name), "visa") {
			delete(members, name)
			dropped = true
		}
	}
	if !dropped {
		return body
	}
	// No HTML escaping: the members' text is kept as the server wrote it.
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(members); err != nil {
		return body
	}
	return bytes.TrimRight(out.Bytes(), "\n")
}
