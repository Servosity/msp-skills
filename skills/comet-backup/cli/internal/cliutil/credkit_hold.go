// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring).
// What the connector hands the credential kit's own readers (credkit.HoldBody)
// that the kit's sources do not reach: a request body or params before a
// preview or a send, an MCP tool's arguments, an import record, a --stdin body,
// and a config file read again after startup (the MCP server reloads it on
// every tool call). No reader of the connector's own: free text is not a
// credential source (credential leak contract section 1).

package cliutil

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"

	"comet-backup-pp-cli/internal/credkit"
)

// NotCredentials are the top-level keys whose values are not credentials.
// internal/credwire declares each as a credkit Identifier (config:<key>), so
// the kit exempts them; a credential inside base_url (its password and query
// values) is still held, as userinfo and query:<name>. template_vars names the
// Comet Server host, which every API URL prints.
var NotCredentials = []string{"base_url", "ca_cert", "tls_server_name", "token_expiry", "backup_username", "server"}

// maxNestedDocuments bounds how deep a document inside a member is read.
const maxNestedDocuments = 8

// HoldDocument holds a document's credentials with the kit's reader, and those
// of every member whose value is itself a JSON object or array (Comet's
// ProfileData, a form field carrying JSON). A value that is not valid JSON is
// left to the kit's reading of the outer document: it is never prose here.
func HoldDocument(b []byte) {
	holdDocument(b, 0)
}

func holdDocument(b []byte, depth int) {
	if len(bytes.TrimSpace(b)) == 0 {
		return
	}
	credkit.HoldBody(b)
	if depth >= maxNestedDocuments {
		return
	}
	for _, inner := range nestedDocuments(b) {
		holdDocument([]byte(inner), depth+1)
	}
}

// nestedDocuments are the string values in b (JSON documents, or a form) that
// are themselves valid JSON objects or arrays.
func nestedDocuments(b []byte) []string {
	var out []string
	add := func(s string) {
		t := strings.TrimSpace(s)
		if len(t) > 1 && (t[0] == '{' || t[0] == '[') && json.Valid([]byte(t)) {
			out = append(out, t)
		}
	}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for _, e := range x {
				walk(e)
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		case string:
			add(x)
		}
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	parsed := false
	for {
		var v any
		if dec.Decode(&v) != nil {
			break
		}
		parsed = true
		walk(v)
	}
	if !parsed {
		if form, err := url.ParseQuery(strings.TrimSpace(string(b))); err == nil {
			for _, vs := range form {
				for _, v := range vs {
					add(v)
				}
			}
		}
	}
	return out
}

// HoldRequestCredentials holds an outgoing request's credentials (its params
// read as a form, and its body) before it is previewed or sent.
func HoldRequestCredentials(params map[string]string, body []byte) {
	if len(params) > 0 {
		form := url.Values{}
		for k, v := range params {
			form.Add(k, v)
		}
		HoldDocument([]byte(form.Encode()))
	}
	HoldDocument(body)
}

// HoldArguments holds an MCP tool call's arguments as one document, before any
// handler can preview, echo or pass them on.
func HoldArguments(args map[string]any) {
	if len(args) == 0 {
		return
	}
	if b, err := json.Marshal(args); err == nil {
		HoldDocument(b)
	}
}
