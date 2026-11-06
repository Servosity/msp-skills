// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package jamfblackbox

import (
	"bytes"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

// Planted tenant credentials that only the one credential rule can find:
// none is held as a process credential, so withholding held values alone
// never removes them. Each must be redacted on every path that shows or
// stores data, in every output format.
const (
	orPIN          = "864209"          // an extension attribute labelled "Device PIN" (values array), and a numeric pin field
	orScript       = "ScriptOnly-7abc" // "password=..." inside a script body (an opaque document)
	orXMLNote      = "XmlNote-5def"    // an XML <password> element inside a JSON string value
	orPreviewJSON  = "RawPreview-7abc" // a JSON request body sent as body_base64 to jamf-pro_execute
	orPreviewXML   = "XmlPreview-3jkl" // a Classic XML request body sent as body_base64
	orXMLAttribute = "XmlAttr-9stu"    // an XML attribute named like a credential
	orBigPIN       = "553917"          // a Device PIN record cut by the MCP result budget (an oversized response)
	orBigPreview   = "XmlBigPw-4vwx"   // a <password> element cut by the MCP result budget (an oversized preview body)
)

func oneRuleValues() []string {
	return []string{orPIN, orScript, orXMLNote, orPreviewJSON, orPreviewXML, orXMLAttribute, orBigPIN, orBigPreview}
}

// oneRuleServer answers with documented Jamf Pro shapes that carry those
// values: the computer inventory's extension attributes (name + values
// array), a script's scriptContents, XML markup inside string values, and
// lists for sync and export. Every write is counted, never applied.
func oneRuleServer(t *testing.T) (*jamfServer, *atomic.Int32) {
	t.Helper()
	var writes atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case p == "/api/v1/oauth/token":
			fmt.Fprintf(w, `{"access_token":%q,"expires_in":1200,"token_type":"Bearer"}`, bbMinted)
		case r.Method != http.MethodGet:
			writes.Add(1)
			fmt.Fprint(w, `{"id":"1"}`)
		case p == "/api/v4/computers-inventory/1":
			fmt.Fprintf(w, `{"id":"1","general":{"name":"Mac-OneRule","extensionAttributes":[{"definitionId":"5","name":"Device PIN","enabled":true,"multiValue":false,"values":[%q],"dataType":"STRING","options":[],"inputType":"TEXT"}]}}`, orPIN)
		case p == "/api/v4/computers-inventory/2":
			// Over the MCP result budget (60000 bytes) and not one list, so
			// the MCP result is a preview of the first 4000 bytes, cut inside
			// the Device PIN record after its value.
			fmt.Fprintf(w, `{"id":"2","general":{"name":"Mac-Big","extensionAttributes":[{"name":"Device PIN","values":[%q],"description":"%s"}]},"a":[1],"b":[2],"z":"%s"}`, orBigPIN, strings.Repeat("x", 4000), strings.Repeat("y", 70000))
		case p == "/api/v1/scripts/1/download":
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprintf(w, "password=%s\necho done\n", orScript)
		case p == "/api/v1/scripts/2/download":
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "echo clean-download\n")
		case p == "/api/v1/scripts/1":
			fmt.Fprintf(w, `{"id":"1","name":"setup-onerule.sh","scriptContents":"password=%s","notes":"<account><password>%s</password></account>","description":"<account name=\"admin\" password=\"%s\"/>"}`, orScript, orXMLNote, orXMLAttribute)
		case page != "" && page != "0":
			fmt.Fprint(w, `{"totalCount":1,"results":[]}`)
		case p == "/api/v1/scripts":
			fmt.Fprintf(w, `{"totalCount":1,"results":[{"id":"1","name":"setup-onerule.sh","scriptContents":"#!/bin/sh\npassword=%s\n","notes":"<account><password>%s</password></account>"}]}`, orScript, orXMLNote)
		case p == "/api/v1/buildings":
			fmt.Fprintf(w, `{"totalCount":1,"results":[{"id":"1","name":"HQ-OneRule","pin":%s,"extensionAttributes":[{"name":"Device PIN","values":[%q]}]}]}`, orPIN, orPIN)
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
	return js, &writes
}

// assertNoneOf fails when text holds a planted value whole, in a standard
// encoding (base64, base64url, hex, base32, percent), or inside a longer
// text that a base64 run of the output decodes to (a request body echoed
// back base64-encoded).
func assertNoneOf(t *testing.T, where, text string, values []string) {
	t.Helper()
	views := []string{text}
	for _, run := range base64Runs.FindAllString(text, -1) {
		for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.RawURLEncoding} {
			if b, err := enc.DecodeString(run); err == nil {
				views = append(views, string(b))
			}
		}
	}
	for _, v := range values {
		for i, view := range views {
			for _, f := range echoForms(v) {
				if strings.Contains(view, f) {
					how := "as " + strconv.Quote(f)
					if i > 0 {
						how = "inside a base64 run"
					}
					t.Errorf("%s holds %q %s", where, v, how)
				}
			}
		}
	}
}

var base64Runs = regexp.MustCompile(`[A-Za-z0-9+/_-]{12,}={0,2}`)

// Class: the one credential rule on every path that shows or stores data.
// Fleet output applies the whole rule (credential-named fields of any JSON
// type, name/value records, opaque documents, credentials inside strings
// and inside encoded strings); every other output, file and the local store
// must apply the same rule, not a subset of it.
func TestOneRuleOnEveryOutputPath(t *testing.T) {
	t.Parallel()
	js, writes := oneRuleServer(t)
	e := newEnv(t, js, true)
	exportScripts := filepath.Join(e.home, "scripts.jsonl")
	exportBuildings := filepath.Join(e.home, "buildings.jsonl")
	deliverFile := filepath.Join(e.home, "deliver.json")
	runs := [][]string{
		{"computer-inventory", "get", "1", "--json", "--data-source", "live"},
		{"computer-inventory", "get", "1", "--agent", "--data-source", "live"},
		{"computer-inventory", "get", "1", "--csv", "--data-source", "live"},
		{"buildings", "list", "--json", "--data-source", "live"},
		{"buildings", "list", "--csv", "--data-source", "live"},
		{"buildings", "list", "--plain", "--data-source", "live"},
		{"buildings", "list", "--select", "pin,extensionAttributes", "--json", "--data-source", "live"},
		{"scripts", "get", "1", "--json", "--data-source", "live"},
		{"scripts", "get", "1", "--plain", "--data-source", "live"},
		{"scripts", "list", "--csv", "--data-source", "live"},
		{"scripts", "list", "--json", "--data-source", "live", "--deliver", "file:" + deliverFile},
		{"scripts", "get-download", "1"},
		{"scripts", "get-download", "2"},
		{"scripts", "get-download", "1", "--deliver", "file:" + filepath.Join(e.home, "script.sh")},
		{"export", "scripts", "-o", exportScripts},
		{"export", "buildings", "-o", exportBuildings},
		{"sync", "--resources", "buildings,scripts"},
		{"scripts", "get", "1", "--json", "--data-source", "local"},
		{"buildings", "list", "--json", "--data-source", "local"},
	}
	var all strings.Builder
	for _, args := range runs {
		out := e.run(t, args...)
		all.WriteString(out)
		assertNoneOf(t, fmt.Sprintf("output of %v", args), out, oneRuleValues())
	}
	jsonBody := base64.StdEncoding.EncodeToString([]byte(`{"password":"` + orPreviewJSON + `"}`))
	xmlBody := base64.StdEncoding.EncodeToString([]byte(`<policy><general><name>p</name></general><account_maintenance><accounts><account><name>admin</name><password>` + orPreviewXML + `</password></account></accounts></account_maintenance></policy>`))
	mcp := e.mcpCalls(t, []map[string]any{
		{"name": "jamf-pro_read", "arguments": map[string]any{"endpoint_id": "computer_inventory.get", "params": map[string]any{"id": "1"}}},
		{"name": "jamf-pro_read", "arguments": map[string]any{"endpoint_id": "scripts.get", "params": map[string]any{"id": "1"}}},
		{"name": "jamf-pro_read", "arguments": map[string]any{"endpoint_id": "buildings.list"}},
		{"name": "jamf-pro_read", "arguments": map[string]any{"endpoint_id": "scripts.get-download", "params": map[string]any{"id": "1"}}},
		{"name": "jamf-pro_execute", "arguments": map[string]any{"endpoint_id": "policies.update", "params": map[string]any{"id": "1", "body_base64": jsonBody}}},
		{"name": "jamf-pro_execute", "arguments": map[string]any{"endpoint_id": "policies.update", "params": map[string]any{"id": "1", "body_base64": xmlBody, "content_type": "application/xml"}}},
		{"name": "jamf-pro_read", "arguments": map[string]any{"endpoint_id": "computer_inventory.get", "params": map[string]any{"id": "2"}}},
		{"name": "jamf-pro_execute", "arguments": map[string]any{"endpoint_id": "policies.update", "params": map[string]any{"id": "1", "body_base64": bigPreviewBody(), "content_type": "application/xml"}}},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT data FROM resources"}},
	})
	all.WriteString(mcp)
	assertNoneOf(t, "MCP session", mcp, oneRuleValues())
	if writes.Load() != 0 {
		t.Errorf("previews sent %d write(s)", writes.Load())
	}
	// Not vacuous: the records themselves still print.
	for _, want := range []string{"Mac-OneRule", "setup-onerule.sh", "HQ-OneRule", "confirmation_required", "original_bytes", "the response was withheld", "clean-download"} {
		if !strings.Contains(all.String(), want) {
			t.Errorf("no output shows %q: the data itself was lost", want)
		}
	}
	var sawStore bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path == js.caFile {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		rel, _ := filepath.Rel(e.home, path)
		if strings.Contains(d.Name(), ".db") && bytes.Contains(b, []byte("HQ-OneRule")) {
			sawStore = true
		}
		assertNoneOf(t, "file "+rel, string(b), oneRuleValues())
		return nil
	})
	if !sawStore {
		t.Errorf("no local store holds the synced record")
	}
	for _, path := range []string{exportScripts, exportBuildings} {
		if b, err := os.ReadFile(path); err != nil || len(bytes.TrimSpace(b)) == 0 {
			t.Errorf("%s: want exported records (err %v)", filepath.Base(path), err)
		}
	}
}

// bigPreviewBody is a Classic XML policy body over the MCP result budget
// whose <password> element sits where the preview (the first 4000 bytes of
// the encoded preview, about 2993 body bytes) ends: the value is inside the
// preview, its closing tag is cut.
func bigPreviewBody() string {
	const at = 2965 // where <password> starts in the body
	head := "<policy><general><name>"
	tail := "</name></general><account_maintenance><accounts><account>"
	body := head + strings.Repeat("p", at-len(head)-len(tail)) + tail +
		"<password>" + orBigPreview + "</password></account></accounts></account_maintenance><notes>" +
		strings.Repeat("z", 50000) + "</notes></policy>"
	return base64.StdEncoding.EncodeToString([]byte(body))
}
