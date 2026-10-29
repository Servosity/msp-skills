// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type seenReq struct {
	path, query, anchor, prefer string
	body                        map[string]any
}

func exoServer(t *testing.T, handle func(n int, r seenReq, w http.ResponseWriter, base string)) (*httptest.Server, *[]seenReq) {
	t.Helper()
	var seen []seenReq
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		s := seenReq{path: r.URL.Path, query: r.URL.RawQuery, anchor: r.Header.Get("X-AnchorMailbox"), prefer: r.Header.Get("Prefer")}
		_ = json.Unmarshal(raw, &s.body)
		seen = append(seen, s)
		handle(len(seen), s, w, srv.URL)
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func exoDo(t *testing.T, url, body string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	resp, err := (&exoTransport{base: http.DefaultTransport}).RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	out, _ := io.ReadAll(resp.Body)
	return resp, string(out)
}

func TestEXOEnvelopePagingAndAnchor(t *testing.T) {
	srv, seen := exoServer(t, func(n int, r seenReq, w http.ResponseWriter, base string) {
		if n == 1 {
			io.WriteString(w, `{"value":[{"Identity":"a","Identity@odata.type":"#String"}],"@odata.nextLink":"`+base+`/adminapi/beta/t1/InvokeCommand?$skiptoken=2"}`)
			return
		}
		io.WriteString(w, `{"value":[{"Identity":"b"}]}`)
	})
	_, out := exoDo(t, srv.URL+"/adminapi/beta/t1/InvokeCommand?properties=Identity#Get-Mailbox", `{"ResultSize":"Unlimited"}`)
	if out != `[{"Identity":"a"},{"Identity":"b"}]` {
		t.Fatalf("merged rows = %s", out)
	}
	s := (*seen)[0]
	in := s.body["CmdletInput"].(map[string]any)
	if in["CmdletName"] != "Get-Mailbox" || in["Parameters"].(map[string]any)["ResultSize"] != "Unlimited" {
		t.Fatalf("envelope = %v", s.body)
	}
	if s.query != "%24select=Identity" || s.prefer != "odata.maxpagesize=1000" {
		t.Fatalf("query=%q prefer=%q", s.query, s.prefer)
	}
	if s.anchor != "APP:SystemMailbox{bb558c35-97f1-4cb9-8ff7-d53741dc928c}@t1" {
		t.Fatalf("anchor = %q", s.anchor)
	}
	if p2 := (*seen)[1]; p2.query != "%24skiptoken=2" || p2.body["CmdletInput"] == nil {
		t.Fatalf("page 2 = %+v", p2)
	}
}

func TestEXOMailboxAnchorAndV2EmptyParams(t *testing.T) {
	srv, seen := exoServer(t, func(n int, r seenReq, w http.ResponseWriter, base string) { io.WriteString(w, `{"value":[]}`) })
	_, out := exoDo(t, srv.URL+"/adminapi/beta/t1/InvokeCommand#Get-InboxRule", `{"Mailbox":"alex@contoso.com"}`)
	if out != "[]" || (*seen)[0].anchor != "UPN:alex@contoso.com" {
		t.Fatalf("out=%s anchor=%q", out, (*seen)[0].anchor)
	}
	exoDo(t, srv.URL+"/adminapi/v2.0/t1/MailboxFolderPermission#Get-MailboxFolderPermission", `{"Identity":"alex@contoso.com:\\Calendar"}`)
	if a := (*seen)[1].anchor; a != "UPN:alex@contoso.com" {
		t.Fatalf("folder anchor = %q", a)
	}
	exoDo(t, srv.URL+"/adminapi/v2.0/t1/OrganizationConfig#Get-OrganizationConfig", ``)
	if _, has := (*seen)[2].body["CmdletInput"].(map[string]any)["Parameters"]; has {
		t.Fatalf("v2 empty call sent Parameters: %v", (*seen)[2].body)
	}
}

func TestEXOErrorsWarningsAndOrigin(t *testing.T) {
	srv, _ := exoServer(t, func(n int, r seenReq, w http.ResponseWriter, base string) {
		switch r.body["CmdletInput"].(map[string]any)["CmdletName"] {
		case "Get-TransportRule":
			w.WriteHeader(400)
			io.WriteString(w, `{"error":{"code":"BadRequest","message":"outer","details":[{"message":"A parameter cannot be found"}]}}`)
		case "Set-Mailbox":
			io.WriteString(w, `{"@adminapi.warnings":["The command completed successfully but no settings have been modified."]}`)
		default:
			io.WriteString(w, `{"value":[{"x":1}],"@odata.nextLink":"https://evil.example/steal"}`)
		}
	})
	resp, out := exoDo(t, srv.URL+"/adminapi/beta/t1/InvokeCommand#Get-TransportRule", `{}`)
	if resp.StatusCode != 400 || !strings.Contains(out, "A parameter cannot be found") {
		t.Fatalf("error = %d %s", resp.StatusCode, out)
	}
	if _, out = exoDo(t, srv.URL+"/adminapi/beta/t1/InvokeCommand#Set-Mailbox", `{"Identity":"a@b.c"}`); !strings.Contains(out, "no settings have been modified") {
		t.Fatalf("warnings = %s", out)
	}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/adminapi/beta/t1/InvokeCommand#Get-Mailbox", strings.NewReader(`{}`))
	if _, err := (&exoTransport{base: http.DefaultTransport}).RoundTrip(req); err == nil || !strings.Contains(err.Error(), "another origin") {
		t.Fatalf("cross-origin nextLink not refused: %v", err)
	}
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/adminapi/beta/t1/InvokeCommand#Get-Mailbox", strings.NewReader(`[1]`))
	if _, err := (&exoTransport{base: http.DefaultTransport}).RoundTrip(req); err == nil {
		t.Fatal("non-object parameters accepted")
	}
}

func TestEXONormalizesFlagShapes(t *testing.T) {
	p := map[string]any{"ResultSize": "50", "GrantSendOnBehalfTo": "a@x.com, b@x.com"}
	normalizeParams(p)
	if p["ResultSize"] != 50 || len(p["GrantSendOnBehalfTo"].([]string)) != 2 {
		t.Fatalf("normalized = %#v", p)
	}
	p = map[string]any{"ResultSize": "Unlimited", "GrantSendOnBehalfTo": map[string]any{"add": []any{"a@x.com"}}}
	normalizeParams(p)
	if p["ResultSize"] != "Unlimited" || p["GrantSendOnBehalfTo"].(map[string]any)["@odata.type"] != "#Exchange.GenericHashTable" {
		t.Fatalf("normalized = %#v", p)
	}
}

func TestEXOKeepsStatusOnUnreadableErrorBody(t *testing.T) {
	srv, _ := exoServer(t, func(n int, r seenReq, w http.ResponseWriter, base string) {
		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(403)
		w.Write(make([]byte, 16)) // NUL bytes labelled gzip
	})
	resp, out := exoDo(t, srv.URL+"/adminapi/beta/t1/InvokeCommand#Get-Mailbox", `{}`)
	if resp.StatusCode != 403 || !strings.Contains(out, "no Exchange role") {
		t.Fatalf("got %d %s", resp.StatusCode, out)
	}
}

type failRT struct{ t *testing.T }

func (f failRT) RoundTrip(r *http.Request) (*http.Response, error) {
	f.t.Fatalf("request reached the network: %s", r.URL)
	return nil, nil
}

func TestEXORejectsBadPathsBeforeSending(t *testing.T) {
	for _, u := range []string{
		"https://outlook.office365.com/adminapi/beta/a%2Fb/InvokeCommand#Get-Mailbox",
		"https://outlook.office365.com/adminapi/beta/a/b/InvokeCommand#Get-Mailbox",
		"https://outlook.office365.com/adminapi/v9/t1/InvokeCommand#Get-Mailbox",
		"https://outlook.office365.com/adminapi/beta/t1/InvokeCommand#not a cmdlet",
	} {
		req, _ := http.NewRequest(http.MethodPost, u, strings.NewReader(`{}`))
		if _, err := (&exoTransport{base: failRT{t}}).RoundTrip(req); err == nil {
			t.Fatalf("%s accepted", u)
		}
	}
}

func TestEXOPagingFailsClosed(t *testing.T) {
	srv, _ := exoServer(t, func(n int, r seenReq, w http.ResponseWriter, base string) {
		// Always the same next link: a cursor that never advances.
		io.WriteString(w, `{"value":[{"x":1}],"@odata.nextLink":"`+base+`/adminapi/beta/t1/InvokeCommand?$skiptoken=same"}`)
	})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/adminapi/beta/t1/InvokeCommand#Get-Mailbox", strings.NewReader(`{}`))
	if _, err := (&exoTransport{base: http.DefaultTransport}).RoundTrip(req); err == nil || !strings.Contains(err.Error(), "did not advance") {
		t.Fatalf("repeated nextLink: %v", err)
	}
	old := maxEXOPages
	maxEXOPages = 1
	defer func() { maxEXOPages = old }()
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/adminapi/beta/t1/InvokeCommand#Get-Mailbox", strings.NewReader(`{}`))
	if _, err := (&exoTransport{base: http.DefaultTransport}).RoundTrip(req); err == nil || !strings.Contains(err.Error(), "more than 1 pages") {
		t.Fatalf("page cap: %v", err)
	}
}

// Class read-properties-after-the-fragment: the shared client appends a query after
// the path, which for a cmdlet endpoint is after the fragment. It belongs on the query
// string, and the fragment stays the cmdlet's name.
func TestEXOMovesAQueryOffTheFragment(t *testing.T) {
	srv, seen := exoServer(t, func(n int, r seenReq, w http.ResponseWriter, base string) {
		io.WriteString(w, `{"value":[{"Identity":"a"}]}`)
	})
	if _, out := exoDo(t, srv.URL+"/adminapi/beta/t1/InvokeCommand?a=1#Get-Mailbox?%24select=Identity%2CName", `{}`); out != `[{"Identity":"a"}]` {
		t.Fatalf("rows = %s", out)
	}
	s := (*seen)[0]
	if name := s.body["CmdletInput"].(map[string]any)["CmdletName"]; name != "Get-Mailbox" {
		t.Errorf("cmdlet = %v", name)
	}
	if s.query != "a=1&%24select=Identity%2CName" {
		t.Errorf("query = %q", s.query)
	}
}
