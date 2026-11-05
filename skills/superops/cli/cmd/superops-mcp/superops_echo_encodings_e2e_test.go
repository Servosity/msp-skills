// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the secret-echo class over TLS. class_harness.py's
// fake server is plain http, which the kit refuses before any request leaves, so
// its probe reports "not exercised" on a kit adopter; this drives the same
// encodings to the same surfaces (credkit-wiring).

package main

import (
	"encoding/base64"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func echoEncodings(v string) map[string]string {
	var u strings.Builder
	for _, r := range v {
		fmt.Fprintf(&u, `\u%04x`, r)
	}
	once := url.QueryEscape(v)
	return map[string]string{
		"raw":       v,
		"url":       once,
		"url2":      url.QueryEscape(once),
		"html":      strings.ReplaceAll(html.EscapeString(v), ".", "&#46;"),
		"json_u":    u.String(),
		"base64":    base64.StdEncoding.EncodeToString([]byte(v)),
		"base64url": base64.RawURLEncoding.EncodeToString([]byte(v)),
	}
}

func TestEchoedCredentialIsWithheldInEveryEncoding(t *testing.T) {
	t.Parallel()
	cliBin := buildHolderBinary(t, "superops-cli", "../superops-cli")
	mcpBin := buildHolderBinary(t, "superops-mcp", ".")
	const heldValue = "echo.Tok-8842+Zq/7xW"
	forms := echoEncodings(heldValue)
	for _, status := range []int{http.StatusForbidden, http.StatusOK} {
		for name, form := range forms {
			t.Run(fmt.Sprintf("%d %s", status, name), func(t *testing.T) {
				t.Parallel()
				var requests atomic.Int64
				host, ca := serveHolderTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("X-Echo", form)
					w.WriteHeader(status)
					// A JSON escape is written as its own bytes: Encode would escape the backslash.
					_, _ = fmt.Fprintf(w, `{"errors":[{"message":"credential %s invalid"}]}`, form)
				}))
				sandbox := t.TempDir()
				env := holderEnv(sandbox, map[string]string{
					"SUPEROPS_BASE_URL": "https://" + host, "SUPEROPS_CA_CERT": ca, "SUPEROPS_API_TOKEN": heldValue,
					"SUPEROPS_SUBDOMAIN": "acme-holder", "SUPEROPS_CLI_PATH": cliBin, "SUPEROPS_LEARN_NO_CAPTURE": "1",
				})
				out := runHolderCLI(t, cliBin, env, "", "tickets", "list", "--json", "--no-input") +
					runHolderCLI(t, cliBin, env, "", "tickets", "list", "--agent", "--no-input") +
					runHolderMCP(t, mcpBin, env, nil,
						map[string]any{"name": "tickets_list", "arguments": map[string]any{}},
						map[string]any{"name": "raw_query", "arguments": map[string]any{"args": holderRawQuery}})
				if requests.Load() == 0 {
					t.Fatalf("nothing reached the server, so nothing was proven:\n%s", out)
				}
				// A JSON envelope re-encodes what it carries ("&" as a unicode escape, a
				// backslash doubled), so a literal match alone would miss a
				// form that did get out.
				unwrapped := strings.NewReplacer(`\\`, `\`, `\u0026`, "&").Replace(out)
				for shown, f := range forms {
					if strings.Contains(out, f) || strings.Contains(unwrapped, f) {
						t.Errorf("output carries the %s form:\n%s", shown, out)
					}
				}
				assertRunWithheld(t, sandbox, out, []string{heldValue})
			})
		}
	}
}
