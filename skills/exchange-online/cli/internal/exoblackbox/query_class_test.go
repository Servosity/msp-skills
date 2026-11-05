// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package exoblackbox

// Class unencoded-query-kills-the-run. The kit reads a query one way (canonical
// pairs) and answers any other query by marking every value unknown, after which
// every unit on every sink is dropped: the run exits with nothing on stdout or
// stderr, and an MCP server stops answering. A literal $ in @odata.nextLink is
// ordinary OData, so a real tenant sends that. Every test here runs the built
// binaries through the kit, each input seen failing first
// (.review/unencoded-query-seen-failing.log).

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

const (
	bbPageOne = "page-one@example.com"
	bbPageTwo = "page-two@example.com"
)

// queryAPI is a fake whose every Get-Mailbox answer names next's link as its
// @odata.nextLink, none when next returns "". Other cmdlets answer one clean row, so a test
// can ask whether the run is still alive.
type queryAPI struct {
	mu   sync.Mutex
	seen []*http.Request // the Get-Mailbox requests
	env  *bbEnv
	base string
}

func (a *queryAPI) requests() []*http.Request {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]*http.Request(nil), a.seen...)
}

// pageReq is what a queryAPI's next reads: which Get-Mailbox request this is (from 1) and
// where it went. An MCP session's calls arrive in any order, so a first page is told by
// its having no query.
type pageReq struct {
	n          int
	first      bool
	base, path string
}

func newQueryAPI(t *testing.T, next func(pageReq) string) *queryAPI {
	t.Helper()
	a := &queryAPI{}
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		if cmdletOf(body) != "Get-Mailbox" {
			fmt.Fprintf(w, `{"value":[{"Identity":"other-cmdlet@example.com"}]}`)
			return
		}
		a.mu.Lock()
		a.seen = append(a.seen, r)
		n := len(a.seen)
		a.mu.Unlock()
		link := next(pageReq{n: n, first: r.URL.RawQuery == "", base: "https://" + r.Host, path: r.URL.Path})
		if link == "" {
			fmt.Fprintf(w, `{"value":[{"Identity":%q}]}`, bbPageTwo)
			return
		}
		quoted, _ := json.Marshal(link)
		fmt.Fprintf(w, `{"value":[{"Identity":%q}],"@odata.nextLink":%s}`, bbPageOne, quoted)
	}))
	a.base = srv.URL
	a.env = newEnv(t, &server{url: srv.URL, caFile: pemFile(t, srv)}, true)
	return a
}

// oneLink answers the first page of a call with the link, and the next one as the end.
func oneLink(query string) func(pageReq) string {
	return func(p pageReq) string {
		if p.first {
			return p.base + p.path + "?" + query
		}
		return ""
	}
}

// noLink is a one-page listing.
func noLink(pageReq) string { return "" }

// runExit is one CLI run: stdout, stderr and the exit status.
func (e *bbEnv) runExit(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bbCLI, args...)
	cmd.Env, cmd.Dir = e.env, e.home
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return out.String(), errb.String(), 0
	case errors.As(err, &ee):
		return out.String(), errb.String(), ee.ExitCode()
	}
	t.Fatalf("%v: %v", args, err)
	return "", "", -1
}

func mailboxRead() map[string]any {
	return map[string]any{"name": "exchange-online_read", "arguments": map[string]any{
		"endpoint_id": "mailboxes.list", "params": map[string]any{"tenantId": bbTenant}}}
}

func recipientRead() map[string]any {
	return map[string]any{"name": "exchange-online_read", "arguments": map[string]any{
		"endpoint_id": "recipients.list", "params": map[string]any{"tenantId": bbTenant}}}
}

// Every next-page form the review ran, and the ones around it. Each goes out as the kit
// reads it, reaches the server with the value the link wrote, and the listing holds both
// pages.
func TestNextLinkQueryFormsListEveryPage(t *testing.T) {
	skip := func(v string) url.Values { return url.Values{"$skiptoken": {v}} }
	for _, tc := range []struct {
		query string
		want  url.Values
	}{
		{"$skiptoken=abc", skip("abc")},
		{"$top=1000&$skip=1000", url.Values{"$top": {"1000"}, "$skip": {"1000"}}},
		{"$skiptoken=Id-1000", skip("Id-1000")},
		{"%24skiptoken=abc==", skip("abc==")},
		{"%24skiptoken='abc'", skip("'abc'")},
		{"%24skiptoken=a(b)", skip("a(b)")},
		{"$skiptoken=a%2Bb", skip("a+b")},
		{"$skiptoken=a%2bb", skip("a+b")},
		{"$skiptoken=a+b", skip("a b")},
		{"$skiptoken=a%20b", skip("a b")},
		{"$skiptoken=a%20b'c", skip("a b'c")},
		{"$skiptoken=a+b'c", skip("a b'c")},
		{"$filter=Name eq 'x,y'", url.Values{"$filter": {"Name eq 'x,y'"}}},
		{"$skiptoken=café", skip("café")},
		{"$skiptoken=%FF", skip("\xff")},
		{"$count", url.Values{"$count": {""}}},
		{"$skiptoken=abc&a=1&a=2", url.Values{"$skiptoken": {"abc"}, "a": {"1", "2"}}},
		{"%24skiptoken=abc", skip("abc")},
		{"plain=abc", url.Values{"plain": {"abc"}}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			t.Parallel()
			a := newQueryAPI(t, oneLink(tc.query))
			stdout, stderr, code := a.env.runExit(t, "mailboxes", "list", bbTenant, "--json")
			if code != 0 || !strings.Contains(stdout, bbPageOne) || !strings.Contains(stdout, bbPageTwo) {
				t.Fatalf("want both pages and exit 0, got exit %d\nstdout: %.800s\nstderr: %.800s", code, stdout, stderr)
			}
			got := a.requests()
			if len(got) != 2 {
				t.Fatalf("want 2 requests, the server saw %d", len(got))
			}
			if q := got[1].URL.Query(); fmt.Sprint(q) != fmt.Sprint(tc.want) {
				t.Errorf("page 2 reached the server as %q, decoded %q; want %q", got[1].URL.RawQuery, q, tc.want)
			}
		})
	}
}

// A value the server already wrote in a form the kit reads is sent byte for byte, so a
// server that reads "+" and "%20" its own way gets back what it wrote.
func TestNextLinkKeepsAReadableValueByteForByte(t *testing.T) {
	for _, value := range []string{"a+b", "a%20b", "a%2Bb", "abc-_.~"} {
		a := newQueryAPI(t, oneLink("%24skiptoken="+value))
		if _, stderr, code := a.env.runExit(t, "mailboxes", "list", bbTenant, "--json"); code != 0 {
			t.Fatalf("%q: exit %d: %s", value, code, stderr)
		}
		got := a.requests()
		if len(got) != 2 || got[1].URL.RawQuery != "%24skiptoken="+value {
			t.Errorf("%q reached the server changed: %q", value, got[len(got)-1].URL.RawQuery)
		}
	}
}

// Over MCP a paged read returns both pages, and the server answers the calls after it.
func TestNextLinkOverMCPReturnsBothPagesAndTheServerStaysUp(t *testing.T) {
	for _, query := range []string{"$skiptoken=abc", "%24skiptoken=abc==", "$top=1000&$skip=1000", "%24skiptoken='abc'"} {
		t.Run(query, func(t *testing.T) {
			t.Parallel()
			a := newQueryAPI(t, oneLink(query))
			out := a.env.mcpCalls(t, []map[string]any{mailboxRead(), recipientRead(), mailboxRead()})
			if !strings.Contains(out, `"id":3`) {
				t.Fatalf("the server did not answer every call:\n%.1500s", out)
			}
			if n := strings.Count(out, bbPageTwo); n < 2 || strings.Count(out, bbPageOne) < 2 {
				t.Errorf("a read lost a page (page two %d times):\n%.1500s", n, out)
			}
			if !strings.Contains(out, "other-cmdlet@example.com") {
				t.Errorf("a later call did not answer:\n%.1500s", out)
			}
			if strings.Contains(out, bbWithheld) {
				t.Errorf("a result was withheld:\n%.1500s", out)
			}
		})
	}
}

// A query that cannot be read fails that one call and says why; it never reaches the kit,
// whose answer would be silence, and the next call still works.
func TestUnreadableNextLinkQueryFailsOneCallAloud(t *testing.T) {
	for _, tc := range []struct{ query, word string }{
		{"$skiptoken=a;b", "semicolon"},
		{"%24skiptoken=a;b", "semicolon"},
		{"a=1;b=2", "semicolon"},
		{"%24skiptoken=%zz", "%zz"},
		{"$skiptoken=%2", "%2"},
		{"%zz=1", "%zz"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			t.Parallel()
			a := newQueryAPI(t, oneLink(tc.query))
			stdout, stderr, code := a.env.runExit(t, "mailboxes", "list", bbTenant, "--json")
			if code == 0 {
				t.Errorf("exit 0 on a truncated listing:\n%.800s", stdout)
			}
			if !strings.Contains(stderr+stdout, tc.word) || !strings.Contains(stderr+stdout, "nextLink") {
				t.Errorf("exit %d does not name the problem (%q):\nstdout: %.800s\nstderr: %.800s", code, tc.word, stdout, stderr)
			}
			if got := a.requests(); len(got) != 1 {
				t.Errorf("want the call to stop at the bad link, the server saw %d requests", len(got))
			}
			b := newQueryAPI(t, oneLink(tc.query))
			out := b.env.mcpCalls(t, []map[string]any{mailboxRead(), recipientRead()})
			if !strings.Contains(out, tc.word) {
				t.Errorf("the MCP error does not name the problem (%q):\n%.1500s", tc.word, out)
			}
			if !strings.Contains(out, "other-cmdlet@example.com") {
				t.Errorf("the next MCP call did not answer:\n%.1500s", out)
			}
		})
	}
}

// Class paging-did-not-advance, on the normalized link: links that differ only by
// encoding are one link. Each answer below is a different spelling of the same page.
func TestRepeatGuardSeesThroughEncoding(t *testing.T) {
	spell := func(base, path string) []string {
		host := strings.TrimPrefix(base, "https://")
		pathEsc := strings.Replace(path, "InvokeCommand", "Invoke%43ommand", 1)
		return []string{
			base + path + "?$skiptoken=same",
			base + path + "?%24skiptoken=same",
			base + path + "?%24skiptoken=s%61me",
			"https://" + strings.ToUpper(host) + path + "?$skiptoken=same",
			base + pathEsc + "?$skiptoken=same",
			path + "?%24skiptoken=same",
			"?$skiptoken=%73ame",
			base + path + "?$skiptoken=same#fragment",
		}
	}
	for i := 1; i < 8; i++ {
		t.Run(fmt.Sprintf("spelling-%d", i), func(t *testing.T) {
			t.Parallel()
			a := newQueryAPI(t, func(p pageReq) string {
				s := spell(p.base, p.path)
				if p.n == 1 {
					return s[0]
				}
				return s[i]
			})
			stdout, stderr, code := a.env.runExit(t, "mailboxes", "list", bbTenant, "--json")
			if code == 0 || !strings.Contains(stderr+stdout, "did not advance") {
				t.Fatalf("want the repeat refused, got exit %d\nstdout: %.600s\nstderr: %.600s", code, stdout, stderr)
			}
			if got := a.requests(); len(got) != 2 {
				t.Errorf("the guard let %d requests through, want 2", len(got))
			}
		})
	}
	// Advancing tokens still page.
	a := newQueryAPI(t, func(p pageReq) string {
		if p.n >= 5 {
			return ""
		}
		return p.base + p.path + fmt.Sprintf("?$skiptoken=%d", p.n)
	})
	if stdout, stderr, code := a.env.runExit(t, "mailboxes", "list", bbTenant, "--json"); code != 0 || !strings.Contains(stdout, bbPageTwo) || len(a.requests()) != 5 {
		t.Errorf("advancing tokens did not page: exit %d, %d requests\n%.500s\n%.500s", code, len(a.requests()), stdout, stderr)
	}
}

// The same-origin check still runs first: a link to another host is refused for that,
// whatever its query says, and the other host is never contacted.
func TestSameOriginGuardRunsOnTheLinkWhateverItsQuery(t *testing.T) {
	var offHost atomic.Int32
	other := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { offHost.Add(1) }))
	for _, query := range []string{"$skiptoken=1", "a=b;c", "%zz=1"} {
		a := newQueryAPI(t, func(p pageReq) string { return other.URL + p.path + "?" + query })
		stdout, stderr, code := a.env.runExit(t, "mailboxes", "list", bbTenant, "--json")
		if code == 0 || !strings.Contains(stderr+stdout, "another origin") {
			t.Errorf("%q: want the origin refusal, got exit %d\nstdout: %.600s\nstderr: %.600s", query, code, stdout, stderr)
		}
	}
	if offHost.Load() != 0 {
		t.Errorf("the other host was contacted %d time(s)", offHost.Load())
	}
}

// Everything else a person or a server can put in a query or a URL on the way out. None of
// it may reach the kit in a form the kit shuts down on: the run prints, and an MCP session
// goes on answering.
func TestInputsInTheURLNeverSilenceTheRun(t *testing.T) {
	selected := func(t *testing.T, a *queryAPI) string {
		t.Helper()
		got := a.requests()
		if len(got) == 0 {
			t.Fatal("no request reached the server")
		}
		return got[0].URL.Query().Get("$select")
	}

	for _, props := range []string{"a;b", "a b", "a%zz", "a&b=c", "$x", "'x'", "a(b)", "a+b", "café", "a#b", "a?b"} {
		t.Run("properties "+props, func(t *testing.T) {
			t.Parallel()
			a := newQueryAPI(t, noLink)
			stdout, stderr, code := a.env.runExit(t, "mailboxes", "list", bbTenant, "--properties", props, "--json")
			if code != 0 || !strings.Contains(stdout, bbPageTwo) {
				t.Fatalf("exit %d\nstdout: %.600s\nstderr: %.600s", code, stdout, stderr)
			}
			if got := selected(t, a); got != props {
				t.Errorf("$select reached the server as %q, want %q", got, props)
			}
			b := newQueryAPI(t, noLink)
			out := b.env.mcpCalls(t, []map[string]any{{"name": "exchange-online_read", "arguments": map[string]any{
				"endpoint_id": "mailboxes.list", "params": map[string]any{"tenantId": bbTenant, "properties": props}}}, recipientRead()})
			if !strings.Contains(out, bbPageTwo) || !strings.Contains(out, "other-cmdlet@example.com") {
				t.Errorf("MCP lost a result:\n%.1500s", out)
			}
			if got := selected(t, b); got != props {
				t.Errorf("MCP: $select reached the server as %q, want %q", got, props)
			}
		})
	}

	for _, tenant := range []string{"a;b", "a?b=c;d", "a%zz", "a#b;c", "x/y", "a%3Bb", "a b", "..", "a?$x=y", "a?x=%zz"} {
		t.Run("tenant "+tenant, func(t *testing.T) {
			t.Parallel()
			a := newQueryAPI(t, noLink)
			stdout, stderr, code := a.env.runExit(t, "mailboxes", "list", tenant, "--json")
			if strings.TrimSpace(stdout+stderr) == "" {
				t.Errorf("exit %d with nothing printed", code)
			}
			b := newQueryAPI(t, noLink)
			out := b.env.mcpCalls(t, []map[string]any{{"name": "exchange-online_read", "arguments": map[string]any{
				"endpoint_id": "mailboxes.list", "params": map[string]any{"tenantId": tenant}}}, recipientRead()})
			if !strings.Contains(out, "other-cmdlet@example.com") {
				t.Errorf("the MCP call after it did not answer:\n%.1500s", out)
			}
		})
	}

	for _, tc := range []struct{ name, base, word string }{
		// The shared client rebuilds a query it adds to, so its first two reach the
		// server without the pair it cannot read; what counts here is that they print.
		{"base url semicolon", "/?first=firstvalue;second=secondvalue", "@example.com"},
		{"base url escape", "/?first=firstvalue%zz", "@example.com"},
		{"base url dollar", "/?$xparam=dollarvalue", "@example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := newQueryAPI(t, noLink)
			env := a.env.with("EXCHANGE_ONLINE_BASE_URL=" + a.base + tc.base)
			stdout, stderr, code := env.runExit(t, "mailboxes", "list", bbTenant, "--json")
			if !strings.Contains(stdout+stderr, tc.word) {
				t.Errorf("exit %d does not say %q\nstdout: %.500s\nstderr: %.500s", code, tc.word, stdout, stderr)
			}
			out := env.mcpCalls(t, []map[string]any{mailboxRead(), mailboxRead()})
			if strings.Count(out, `"id":`) < 3 {
				t.Errorf("the MCP server stopped answering:\n%.1500s", out)
			}
			doctor, doctorErr, doctorCode := env.runExit(t, "doctor", "--json")
			if strings.TrimSpace(doctor+doctorErr) == "" {
				t.Errorf("doctor exit %d with nothing printed", doctorCode)
			}
		})
	}
}

// A same-origin redirect is a URL the server wrote too. It goes through the same read.
func TestRedirectQueryNeverSilencesTheRun(t *testing.T) {
	for _, tc := range []struct{ location, word string }{
		{"?$x=y", "after-the-redirect@example.com"},
		{"?a=b;c", "semicolon"},
		{"?a=%zz", `invalid URL escape`},
	} {
		t.Run(tc.location, func(t *testing.T) {
			t.Parallel()
			srv := tlsServer(t, redirectOnce(tc.location))
			e := newEnv(t, &server{url: srv.URL, caFile: pemFile(t, srv)}, true)
			stdout, stderr, code := e.runExit(t, "mailboxes", "list", bbTenant, "--json")
			if !strings.Contains(stdout+stderr, tc.word) {
				t.Errorf("exit %d does not say %q\nstdout: %.500s\nstderr: %.500s", code, tc.word, stdout, stderr)
			}
			read := e.mcpCalls(t, []map[string]any{mailboxRead(), mailboxRead()})
			if strings.Count(read, `"id":`) < 3 {
				t.Errorf("the MCP server stopped answering:\n%.1500s", read)
			}
		})
	}
}

// redirectOnce sends a request with no query to location, on the same origin, and answers
// the request that follows.
func redirectOnce(location string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.RawQuery == "" {
			w.Header().Set("Location", r.URL.Path+location)
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		fmt.Fprint(w, `{"value":[{"Identity":"after-the-redirect@example.com"}]}`)
	})
}

// The two senders that are given a URL: an operator's feedback endpoint and a --deliver
// webhook. A query that can be read arrives with its value; one that cannot fails that
// send, aloud, and sends nothing.
func TestSendersGivenAURLReadItsQueryOrSayWhy(t *testing.T) {
	var mu sync.Mutex
	var got []*url.URL
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.URL)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(sink.Close)
	take := func() []*url.URL {
		mu.Lock()
		defer mu.Unlock()
		out := got
		got = nil
		return out
	}
	e, _ := listingAPI(t)
	unreadable := []struct{ query, word string }{
		{"first=firstvalue;second=secondvalue", "semicolon"},
		{"first=firstvalue%zz", `invalid URL escape "%zz"`},
	}

	deliver := func(u string) (string, string, int) {
		return e.runExit(t, "mailboxes", "list", bbTenant, "--json", "--deliver", "webhook:"+u)
	}
	for _, tc := range []struct{ query, key, want string }{
		{"$x='y'", "$x", "'y'"},
		{"%24x=a(b)==", "$x", "a(b)=="},
		{"plain=1", "plain", "1"},
	} {
		stdout, stderr, code := deliver(sink.URL + "/hook?" + tc.query)
		arrived := take()
		if code != 0 || len(arrived) != 1 {
			t.Errorf("deliver %q: exit %d, %d arrived\nstdout: %.500s\nstderr: %.500s", tc.query, code, len(arrived), stdout, stderr)
			continue
		}
		if v := arrived[0].Query().Get(tc.key); v != tc.want {
			t.Errorf("deliver %q: the sink read %q, want %q", tc.query, v, tc.want)
		}
	}
	for _, tc := range unreadable {
		stdout, stderr, code := deliver(sink.URL + "/hook?" + tc.query)
		if code == 0 || !strings.Contains(stdout+stderr, tc.word) {
			t.Errorf("deliver %q: exit %d does not name the problem\nstdout: %.500s\nstderr: %.500s", tc.query, code, stdout, stderr)
		}
		if arrived := take(); len(arrived) != 0 {
			t.Errorf("deliver %q: %d request(s) arrived", tc.query, len(arrived))
		}
	}

	// A value in this variable's query is held as a credential of that endpoint, and plain
	// http carries no held value (the kit's contract 2c), so what arrives here has a name
	// and no value. feedback's own status is the local record: a failed send is its warning.
	feedback := func(u string) (string, string, int) {
		return e.with("EXCHANGE_ONLINE_FEEDBACK_ENDPOINT="+u).runExit(t, "feedback", "a note", "--send")
	}
	for _, query := range []string{"$xparam", "%24xparam="} {
		stdout, stderr, code := feedback(sink.URL + "/hook?" + query)
		arrived := take()
		if code != 0 || len(arrived) != 1 {
			t.Errorf("feedback %q: exit %d, %d arrived\nstdout: %.500s\nstderr: %.500s", query, code, len(arrived), stdout, stderr)
			continue
		}
		if !arrived[0].Query().Has("$xparam") {
			t.Errorf("feedback %q: the sink read %q", query, arrived[0].RawQuery)
		}
	}
	for _, query := range []string{"$xparam=heldvalue1", "%24xparam=he(ld)value=="} {
		stdout, stderr, code := feedback(sink.URL + "/hook?" + query)
		if code != 0 || !strings.Contains(stdout, "recorded") {
			t.Errorf("feedback %q went silent: exit %d\nstdout: %.500s\nstderr: %.500s", query, code, stdout, stderr)
		}
		take()
	}
	for _, tc := range unreadable {
		stdout, stderr, code := feedback(sink.URL + "/hook?" + tc.query)
		if !strings.Contains(stdout+stderr, tc.word) {
			t.Errorf("feedback %q: exit %d does not name the problem\nstdout: %.500s\nstderr: %.500s", tc.query, code, stdout, stderr)
		}
		if arrived := take(); len(arrived) != 0 {
			t.Errorf("feedback %q: %d request(s) arrived", tc.query, len(arrived))
		}
	}
}

// A setting in the operator's own config file: a header that says the body is coded, or is
// not the JSON this client sends, makes the kit shut down. A redirect hop sends the
// config's headers as they are, with no envelope around them. A config header's value is
// held, so the kit may withhold what repeats it, with its notice, or refuse a value that
// is public text, which it names. What it may not do is go silent.
func TestConfigHeaderNeverSilencesTheRun(t *testing.T) {
	for _, tc := range []struct{ name, toml, word string }{
		{"transfer coding", "[headers]\nContent-Transfer-Encoding = \"base64\"\n", "Content-Transfer-Encoding"},
		{"content coding", "[headers]\nContent-Encoding = \"customcoding\"\n", "Content-Encoding"},
		{"public text", "[headers]\nContent-Encoding = \"br\"\n", "refusing to start"},
		{"charset", "[headers]\nContent-Type = \"application/json; charset=latin1\"\n", "charset"},
		{"unparseable type", "[headers]\nContent-Type = \"a;;b\"\n", "Content-Type"},
		{"form", "[headers]\nContent-Type = \"application/x-www-form-urlencoded\"\n", "x-www-form-urlencoded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := tlsServer(t, redirectOnce("?next=1"))
			e := newEnv(t, &server{url: srv.URL, caFile: pemFile(t, srv)}, true)
			file := filepath.Join(e.home, "custom.toml")
			if err := os.WriteFile(file, []byte(tc.toml), 0o600); err != nil {
				t.Fatal(err)
			}
			stdout, stderr, code := e.runExit(t, "mailboxes", "list", bbTenant, "--json", "--config", file)
			if !strings.Contains(stdout+stderr, tc.word) && !strings.Contains(stdout+stderr, bbWithheld) {
				t.Errorf("exit %d says neither %q nor the kit's notice\nstdout: %.500s\nstderr: %.500s", code, tc.word, stdout, stderr)
			}
			if code == 0 {
				t.Errorf("exit 0 with a header the request could not carry\nstdout: %.500s\nstderr: %.500s", stdout, stderr)
			}
		})
	}
}

// A user name or password in a URL the server wrote. The kit holds both on sight, so the
// server would choose what every later unit may say: this exited 74 with nothing printed.
func TestServerWrittenUserinfoIsRefusedAloud(t *testing.T) {
	a := newQueryAPI(t, func(p pageReq) string {
		return "https://aa:bb@" + strings.TrimPrefix(p.base, "https://") + p.path + "?$skiptoken=abc"
	})
	stdout, stderr, code := a.env.runExit(t, "mailboxes", "list", bbTenant, "--json")
	if code == 0 || !strings.Contains(stdout+stderr, "user name or password") {
		t.Errorf("nextLink: exit %d\nstdout: %.500s\nstderr: %.500s", code, stdout, stderr)
	}
	if got := a.requests(); len(got) != 1 {
		t.Errorf("nextLink: the server saw %d requests, want 1", len(got))
	}
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://aa:bb@"+r.Host+r.URL.Path+"?next=1")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	e := newEnv(t, &server{url: srv.URL, caFile: pemFile(t, srv)}, true)
	stdout, stderr, code = e.runExit(t, "mailboxes", "list", bbTenant, "--json")
	if code == 0 || !strings.Contains(stdout+stderr, "user name or password") {
		t.Errorf("redirect: exit %d\nstdout: %.500s\nstderr: %.500s", code, stdout, stderr)
	}
}

// A URL with no host and path to send to: everything after the scheme is opaque, its query
// included, and the kit reads that query from the request line all the same. --deliver
// refuses such a URL when it parses the flag; the feedback variable reaches the send.
func TestOpaqueURLIsRefusedAloud(t *testing.T) {
	e, _ := listingAPI(t)
	const target = "https:hook?$xparam=dollarvalue;second=secondvalue"
	stdout, stderr, code := e.with("EXCHANGE_ONLINE_FEEDBACK_ENDPOINT="+target).runExit(t, "feedback", "a note", "--send")
	if !strings.Contains(stdout+stderr, "no host") {
		t.Errorf("feedback: exit %d\nstdout: %.500s\nstderr: %.500s", code, stdout, stderr)
	}
	stdout, stderr, code = e.runExit(t, "mailboxes", "list", bbTenant, "--json", "--deliver", "webhook:"+target)
	if code == 0 || !strings.Contains(stdout+stderr, "https:// URL") {
		t.Errorf("deliver: exit %d\nstdout: %.500s\nstderr: %.500s", code, stdout, stderr)
	}
}
