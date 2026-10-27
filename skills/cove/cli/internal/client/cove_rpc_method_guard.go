// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated), cove only: the one guard every request body of
// the generated client passes before anything is sent (doInternal calls
// coveRefuseNonReadRPC right after marshalling the body, ahead of auth, the
// dry-run preview and the transport). Cove's API is JSON-RPC: one POST
// endpoint, and the "method" member of the body decides whether the call
// reads or changes the tenant. The generated endpoint commands, their --stdin
// bodies and their MCP tools are read-only surfaces, so this client sends only
// read methods (coverpc.IsReadMethod, the rule `cove-cli call` applies too);
// writes go only through `cove-cli call <Method> --yes`, which uses the
// coverpc client and needs the operator's confirmation. Hand-fix code:
// cove-rpc-read-method-guard.

package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"cove-pp-cli/internal/coverpc"
)

// ErrRPCRequestRefused marks a request the generated client refused to send
// because its body is not a well-formed JSON-RPC read.
var ErrRPCRequestRefused = errors.New("refused to send this Cove JSON-RPC request")

const coveRPCWriteHint = "the generated endpoint commands, their --stdin bodies and their MCP tools only send Get... and Enumerate... read methods; to run any other method, use `cove-cli call <Method> --yes` from a terminal"

// coveRPCRequestMembers are the only members a Cove JSON-RPC request object
// carries. Any other name (a case or whitespace variant of "method", a name
// with a NUL or a replacement character, ...) could be read as the method by
// a server that decodes or folds names differently, so it is refused.
var coveRPCRequestMembers = map[string]bool{"jsonrpc": true, "id": true, "method": true, "params": true, "visa": true}

func coveRPCRefusal(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrRPCRequestRefused}, args...)...)
}

// coveRefuseNonReadRPC refuses a request unless every JSON-RPC method it
// carries is a read. A body sent to the JSON-RPC endpoint, and a body sent
// anywhere that names a method member, must be valid UTF-8 JSON holding one
// request object or a non-empty batch array of them, with nothing after it;
// each object may carry only the JSON-RPC members, each at most once, and
// exactly one "method" member whose decoded string value is a read method. A
// body-less request (the doctor reachability probe) and a body sent to another
// path that names no method call no method; a query parameter that looks like
// a method member must still name a read.
func coveRefuseNonReadRPC(path string, params map[string]string, body []byte) error {
	for k, v := range params {
		if coveMethodLikeName(k) && !coverpc.IsReadMethod(v) {
			return coveRPCRefusal("query parameter %q names the JSON-RPC method %q, which is not a read; %s", k, v, coveRPCWriteHint)
		}
	}
	if body == nil {
		return nil
	}
	if !coveIsRPCPath(path) && !coveBodyNamesAMethod(body) {
		return nil
	}
	if !utf8.Valid(body) {
		return coveRPCRefusal("the request body is not valid UTF-8")
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return coveRPCRefusal("the request body is not valid JSON")
	}
	switch tok {
	case json.Delim('{'):
		if err := coveCheckRPCRequest(dec); err != nil {
			return err
		}
	case json.Delim('['):
		n := 0
		for dec.More() {
			member, err := dec.Token()
			if err != nil {
				return coveRPCRefusal("the request body is not valid JSON")
			}
			if member != json.Delim('{') {
				return coveRPCRefusal("a batch member is not a JSON-RPC request object")
			}
			if err := coveCheckRPCRequest(dec); err != nil {
				return err
			}
			n++
		}
		if _, err := dec.Token(); err != nil {
			return coveRPCRefusal("the request body is not valid JSON")
		}
		if n == 0 {
			return coveRPCRefusal("the request body is an empty batch")
		}
	default:
		return coveRPCRefusal("the request body is not a JSON-RPC request object or batch")
	}
	if _, err := dec.Token(); err != io.EOF {
		return coveRPCRefusal("the request body carries data after the JSON-RPC request")
	}
	return nil
}

// coveCheckRPCRequest checks one request object; dec has consumed its '{'.
func coveCheckRPCRequest(dec *json.Decoder) error {
	seen := map[string]bool{}
	method, hasMethod := "", false
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return coveRPCRefusal("the request body is not valid JSON")
		}
		name, ok := tok.(string)
		if !ok {
			return coveRPCRefusal("the request body is not valid JSON")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return coveRPCRefusal("the request body is not valid JSON")
		}
		if !coveRPCRequestMembers[name] {
			if coveMethodLikeName(name) {
				return coveRPCRefusal("member %q is not the JSON-RPC \"method\" member; spell it exactly \"method\", once", name)
			}
			return coveRPCRefusal("member %q is not part of a Cove JSON-RPC request (allowed: jsonrpc, id, method, params, visa)", name)
		}
		if seen[name] {
			return coveRPCRefusal("member %q appears more than once in one JSON-RPC request", name)
		}
		seen[name] = true
		if name != "method" {
			continue
		}
		if len(value) == 0 || value[0] != '"' || json.Unmarshal(value, &method) != nil {
			return coveRPCRefusal("the JSON-RPC \"method\" member is not a string")
		}
		hasMethod = true
	}
	if _, err := dec.Token(); err != nil {
		return coveRPCRefusal("the request body is not valid JSON")
	}
	if !hasMethod {
		return coveRPCRefusal("the request names no JSON-RPC method (a \"method\" member naming a Get... or Enumerate... read is required)")
	}
	if !coverpc.IsReadMethod(method) {
		return coveRPCRefusal("%q is not a Get... or Enumerate... read method, so it may change your Cove tenant; %s", method, coveRPCWriteHint)
	}
	return nil
}

// coveIsRPCPath reports whether path addresses the JSON-RPC endpoint, in any
// spelling a server might route there (case, a trailing slash, a query).
func coveIsRPCPath(path string) bool {
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	return strings.EqualFold(strings.Trim(path, "/"), strings.Trim(coverpc.RPCPath, "/"))
}

// coveBodyNamesAMethod reports whether a body sent to another path still
// carries a member that could be read as a JSON-RPC method: a method-like
// member of the top-level object or of an object in a top-level array. Text
// it cannot scan counts as naming one, so the full check judges it.
func coveBodyNamesAMethod(body []byte) bool {
	if !utf8.Valid(body) {
		return true
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	tok, err := dec.Token()
	if err != nil {
		return true
	}
	switch tok {
	case json.Delim('{'):
		return coveObjectNamesAMethod(dec)
	case json.Delim('['):
		for dec.More() {
			member, err := dec.Token()
			if err != nil {
				return true
			}
			switch member {
			case json.Delim('{'):
				if coveObjectNamesAMethod(dec) {
					return true
				}
			case json.Delim('['):
				return true
			}
		}
	}
	return false
}

// coveObjectNamesAMethod scans one object; dec has consumed its '{'.
func coveObjectNamesAMethod(dec *json.Decoder) bool {
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return true
		}
		if name, ok := tok.(string); !ok || coveMethodLikeName(name) {
			return true
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return true
		}
	}
	_, err := dec.Token()
	return err != nil
}

// coveMethodLikeName reports whether a decoded member or parameter name could
// be read as "method" by a server that folds case, trims spaces or stops at a
// NUL.
func coveMethodLikeName(name string) bool {
	if i := strings.IndexByte(name, 0); i >= 0 {
		name = name[:i]
	}
	return strings.EqualFold(strings.TrimSpace(name), "method")
}
