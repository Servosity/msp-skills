// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import "io"

// ReadBody reads a response body under the client's own size cap, and
// DecodeBody inflates it per Content-Encoding and strips a BOM or JSONP/XSSI
// prefix (sanitizeJSONResponse) exactly as the client does, so
// the transport guard (rewst handfix: response-credentials-held) sees the body
// a command will see.
func ReadBody(r io.Reader) ([]byte, error) { return readResponseBody(r) }

func DecodeBody(encoding string, body []byte) ([]byte, error) {
	b, err := decodeContentEncoding(encoding, body)
	if err != nil {
		return nil, err
	}
	return sanitizeJSONResponse(b), nil
}
