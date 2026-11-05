// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// The credential kit reads an outgoing request one way (internal/credkit:
// holdRequest and requestWhole). It answers any other request by marking every
// value unknown, and from then on every unit on every sink is dropped: a run
// exits with nothing on stdout or stderr, and an MCP server stops answering. So
// a request that goes wrong here must never get that far. Every request this
// connector sends passes KitReadableRequest first (handfixes.json
// request-read-as-the-kit-reads-it): it rewrites what can be written the kit's
// way with the same meaning, and fails that one call, aloud, for anything else.

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ErrUnreadableRequest marks a request refused before it was sent. Sending the
// same request again cannot help, so the client does not retry it.
var ErrUnreadableRequest = errors.New("request not sent")

// kitKept is the characters the kit takes escaped or as written (the fleet's
// hand-built queries leave them as written).
var kitKept = strings.NewReplacer("%2C", ",", "%3A", ":", "%2F", "/", "%40", "@")

// CanonicalQuery is raw in the form the kit reads: pairs in the order written,
// each name and value the re-escape of its decoding, with the same decoded
// values. A piece the kit already reads goes as written, byte for byte. A query
// that cannot be decoded (url.ParseQuery refuses it: a semicolon, a bad percent
// escape) is an error naming the problem.
func CanonicalQuery(raw string) (string, error) {
	if _, err := url.ParseQuery(raw); err != nil {
		return "", fmt.Errorf("%w: %w", err, ErrUnreadableRequest)
	}
	pairs := strings.Split(raw, "&")
	for i, pair := range pairs {
		name, value, hasValue := strings.Cut(pair, "=")
		name, err := canonicalPiece(name)
		if err != nil {
			return "", fmt.Errorf("%w: %w", err, ErrUnreadableRequest)
		}
		if !hasValue {
			pairs[i] = name // a key-only flag, "?upload", stays one
			continue
		}
		if value, err = canonicalPiece(value); err != nil {
			return "", fmt.Errorf("%w: %w", err, ErrUnreadableRequest)
		}
		pairs[i] = name + "=" + value
	}
	return strings.Join(pairs, "&"), nil
}

// canonicalPiece is one name or value as canonicalQuery in internal/credkit
// reads it: QueryEscape, with a space as "+" or "%20", the kept characters
// escaped or not. A "+" stays a "+" and a "%20" a "%20", because a server may
// read them differently; a space with neither becomes "%20", which every reader
// reads as a space.
func canonicalPiece(x string) (string, error) {
	decoded, err := url.QueryUnescape(x)
	if err != nil {
		return "", err
	}
	escaped := url.QueryEscape(decoded)
	kept := kitKept.Replace(escaped)
	for _, form := range []string{escaped, kept, strings.ReplaceAll(escaped, "+", "%20"), strings.ReplaceAll(kept, "+", "%20")} {
		if x == form {
			return x, nil
		}
	}
	if !strings.Contains(x, "+") || strings.Contains(x, "%20") {
		return strings.ReplaceAll(escaped, "+", "%20"), nil
	}
	return escaped, nil
}

// KitReadableRequest is req as the kit reads it, or an error that names what the
// kit could not read. It changes nothing in req: a request it has to rewrite
// comes back as a clone. The caller still owns req.Body when it returns an error.
// Bodies are not its concern: this client's are JSON it marshalled itself, and a
// --stdin body that is not JSON is refused by the command.
func KitReadableRequest(req *http.Request) (*http.Request, error) {
	if req.URL.Opaque != "" {
		return nil, fmt.Errorf("the request URL has no host and path to send to: %w", ErrUnreadableRequest)
	}
	raw, err := CanonicalQuery(req.URL.RawQuery)
	if err != nil {
		return nil, fmt.Errorf("the request URL's query cannot be read: %w", err)
	}
	if err := checkKitHeaders(req.Header); err != nil {
		return nil, fmt.Errorf("%w: %w", err, ErrUnreadableRequest)
	}
	if raw == req.URL.RawQuery {
		return req, nil
	}
	out := req.Clone(req.Context())
	out.URL.RawQuery = raw
	return out, nil
}

// checkKitHeaders refuses the headers that make the kit read a body some other
// way. This client sends JSON and codes nothing, so these can only come from a
// header the operator set in the config file.
func checkKitHeaders(h http.Header) error {
	for _, name := range []string{"Content-Encoding", "Transfer-Encoding", "Content-Transfer-Encoding"} {
		for _, coding := range strings.Split(h.Get(name), ",") {
			switch strings.ToLower(strings.TrimSpace(coding)) {
			case "", "identity", "chunked", "7bit", "8bit", "binary":
			default:
				return fmt.Errorf("the %s header says the body is coded, and this client never codes a body", name)
			}
		}
	}
	contentType := h.Get("Content-Type")
	if contentType == "" {
		return nil
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("the Content-Type header cannot be read: %w", err)
	}
	if charset := strings.ToLower(params["charset"]); charset != "" && charset != "utf-8" && charset != "us-ascii" {
		return fmt.Errorf("the Content-Type header names charset %s, and this client sends UTF-8", strconv.Quote(charset))
	}
	if mediaType == "application/x-www-form-urlencoded" || strings.HasPrefix(mediaType, "multipart/") {
		return fmt.Errorf("the Content-Type header says %s, and this client sends JSON", mediaType)
	}
	return nil
}
