// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package jamfblackbox

import (
	"bytes"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// classServer answers with the exact responses the review's failing inputs name.
func classServer(t *testing.T) *jamfServer {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		page := r.URL.Query().Get("page")
		switch {
		case p == "/api/v1/oauth/token":
			fmt.Fprintf(w, `{"access_token":%q,"expires_in":1200,"token_type":"Bearer"}`, bbMinted)
		case page != "" && page != "0":
			fmt.Fprint(w, `{"totalCount":2,"results":[]}`)
		case p == "/api/v3/computers-inventory", p == "/api/v2/computers-inventory", p == "/api/v1/computers-inventory", p == "/api/v4/computers-inventory":
			fmt.Fprint(w, `{"totalCount":2,"results":[{`+"\n"+`"id":"1","general":{"extensionAttributes":[{`+"\n"+`"name":"Device PIN",`+"\n"+`"values":["864209"]`+"\n"+`}]}},{"id":"2","general":{"name":"Clean-Mac"}}]}`)
		case p == "/api/v1/scripts/1/download":
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, `"password=DownloadPw-7abc"`)
		case p == "/api/v1/scripts/2":
			fmt.Fprint(w, `{"id":"2","name":"setup-nested.sh","notes":"<password><value>NestedPw-7abc</value></password>"}`)
		case p == "/api/v1/scripts/3":
			fmt.Fprint(w, `{"id":"3","name":"setup-held.sh","notes":"<password>XmlOnly-7abc</password>","info":"XmlOnly-7abc"}`)
		case p == "/api/v1/computer-extension-attributes/1/download":
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<extensionAttribute><displayName>Uptime</displayName><dataType>string</dataType><scriptContentsMac>#!/bin/sh
uptime</scriptContentsMac></extensionAttribute>`)
		default:
			fmt.Fprint(w, `{"totalCount":0,"results":[]}`)
		}
	}))
	t.Cleanup(srv.Close)
	js := &jamfServer{host: strings.TrimPrefix(srv.URL, "https://")}
	js.caFile = filepath.Join(t.TempDir(), "ca.pem")
	var pemBuf bytes.Buffer
	_ = pem.Encode(&pemBuf, &pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(js.caFile, pemBuf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return js
}

// A credential label and its value on different lines of one response: the rule
// reads the whole body before any command splits it by line.
func TestCredentialRecordAcrossLinesInAnExport(t *testing.T) {
	t.Parallel()
	e := newEnv(t, classServer(t), true)
	out := filepath.Join(e.home, "ea.jsonl")
	printed := e.run(t, "export", "computer_inventory", "--format", "jsonl", "--limit", "2", "-o", out)
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("no export file: %v\n%s", err, printed)
	}
	if bytes.Contains(b, []byte("864209")) {
		t.Fatalf("the export file holds the PIN:\n%s", b)
	}
	if !bytes.Contains(b, []byte("Clean-Mac")) {
		t.Fatalf("the export lost the clean record:\n%s\n%s", b, printed)
	}
}

// A download whose whole body is one JSON string has no fields to redact: it is
// judged as a document.
func TestScalarDownloadIsJudgedWhole(t *testing.T) {
	t.Parallel()
	e := newEnv(t, classServer(t), true)
	out := filepath.Join(e.home, "script.txt")
	printed := e.run(t, "scripts", "get-download", "1", "--deliver", "file:"+out)
	assertNoneOf(t, "output", printed, []string{"DownloadPw-7abc"})
	if b, err := os.ReadFile(out); err == nil {
		assertNoneOf(t, "file", string(b), []string{"DownloadPw-7abc"})
	}
}

// A credential-named XML element protects everything inside it.
func TestXMLContainerProtectsItsDescendants(t *testing.T) {
	t.Parallel()
	e := newEnv(t, classServer(t), true)
	printed := e.run(t, "scripts", "get", "2", "--json", "--data-source", "live")
	assertNoneOf(t, "output", printed, []string{"NestedPw-7abc"})
	if !strings.Contains(printed, "setup-nested.sh") {
		t.Fatalf("the record was lost:\n%s", printed)
	}
}

// What the rule finds in XML is held, so a repeat under an innocent name goes too.
func TestCredentialFoundInXMLIsHeld(t *testing.T) {
	t.Parallel()
	e := newEnv(t, classServer(t), true)
	printed := e.run(t, "scripts", "get", "3", "--json", "--data-source", "live")
	assertNoneOf(t, "output", printed, []string{"XmlOnly-7abc"})
}

// A documented XML template whose element name starts with "script" is not an
// HTML page, and it still comes back.
func TestXMLTemplateIsNotAnHTMLPage(t *testing.T) {
	t.Parallel()
	e := newEnv(t, classServer(t), true)
	printed := e.run(t, "computer-extension-attributes", "get-download", "1", "--data-source", "live")
	if strings.Contains(printed, "HTML page") || !strings.Contains(printed, "Uptime") {
		t.Fatalf("want the template, got:\n%s", printed)
	}
}
