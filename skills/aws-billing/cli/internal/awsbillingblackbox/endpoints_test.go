// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package awsbillingblackbox

// aws-endpoint-pinned, on the built binaries: the SDK sends only to the AWS
// endpoints of the services this CLI calls (a test build dials the fake for
// each), so a custom endpoint and every redirect are refused before anything
// is sent; a production build never dials the fake; the generated client
// never leaves its base URL's origin.

import (
	"fmt"
	"strings"
	"testing"
)

const pinRefusal = "would leave the AWS service endpoints this CLI calls"

// A custom endpoint (AWS_ENDPOINT_URL and the service variables) that is not
// the admitted fake receives nothing: not the signed service calls, not the
// AssumeRole call that would carry the source profile's keys.
func TestCustomEndpointNeverReceivesARequest(t *testing.T) {
	t.Parallel()
	for _, src := range []source{srcEnv, srcRole} {
		e := newEnv(t, src)
		other := newFakeAWS(t, nil)
		c := e.with("AWS_ENDPOINT_URL="+other.url, "AWS_ENDPOINT_URL_STS="+other.url, "AWS_ENDPOINT_URL_SSO="+other.url)
		var out strings.Builder
		for _, args := range [][]string{{"doctor", "--json"}, {"sync", "--json"}, {"bill", "--data-source", "live"}} {
			out.WriteString(c.run(t, args...))
		}
		out.WriteString(c.mcpCalls(t, []map[string]any{{"name": "bill", "arguments": map[string]any{"data-source": "live"}}}))
		if got := other.seen(); len(got) != 0 {
			t.Errorf("%s: the custom endpoint received %v", src, got)
		}
		if got := e.aws.seen(); len(got) != 0 {
			t.Errorf("%s: the admitted fake received %v though no endpoint named it", src, got)
		}
		if !strings.Contains(out.String(), pinRefusal) {
			t.Errorf("%s: want the pin refusal, got:\n%.2000s", src, out.String())
		}
		// Class refusal-retried: a refusal can never succeed; the SDK must not
		// retry it with backoff (a sync took 73 seconds to fail).
		if strings.Contains(out.String(), "exceeded maximum number of attempts") {
			t.Errorf("%s: the SDK retried a refused request:\n%.2000s", src, out.String())
		}
		assertNoEcho(t, src.String()+" output", out.String())
		// Control: the same commands against the admitted fake reach it.
		e.run(t, "bill", "--data-source", "live")
		if e.aws.count("GetCostAndUsage") == 0 {
			t.Errorf("%s control: bill never reached the admitted fake: %v", src, e.aws.seen())
		}
	}
}

// Every redirect is refused before it is followed: one to another host, and
// one to the same origin.
func TestRedirectIsNeverFollowed(t *testing.T) {
	t.Parallel()
	e := newEnv(t, srcEnv)
	other := newFakeAWS(t, nil)
	e.aws.set(func(f *fakeAWS) {
		f.quiet = true // the refusals below are read from the output
		f.redirect["GetCallerIdentity"] = other.url + "/"
	})
	var out strings.Builder
	for _, args := range [][]string{{"doctor", "--json"}, {"sync", "--json"}} {
		out.WriteString(e.run(t, args...))
	}
	// A POST's redirect is answered as the error it is (the HTTP client does
	// not replay a body it cannot rewind).
	if !strings.Contains(out.String(), "StatusCode: 307") {
		t.Errorf("want the redirect answered as an error, got:\n%.2000s", out.String())
	}
	before := out.Len()
	// sync stores every layer under the credentials' account, so it resolves
	// that first (sts:GetCallerIdentity) and stops when it cannot: the other
	// operations meet their redirects with STS answering (Codex r1 class
	// cache-answer-not-the-request).
	e.aws.set(func(f *fakeAWS) {
		delete(f.redirect, "GetCallerIdentity")
		f.redirect["ListAccounts"] = other.url + "/"
		f.redirect["ListBuckets"] = e.aws.url + "/redirected" // a GET: the one the HTTP client would follow
	})
	for _, args := range [][]string{{"sync", "--json"}, {"accounts", "list", "--data-source", "live", "--json"}} {
		out.WriteString(e.run(t, args...))
	}
	out.WriteString(e.mcpCalls(t, []map[string]any{{"name": "sync", "arguments": map[string]any{"json": true}}}))
	if got := other.seen(); len(got) != 0 {
		t.Errorf("the redirect target host received %v", got)
	}
	e.aws.mu.Lock()
	hops := e.aws.hops
	e.aws.mu.Unlock()
	if hops != 0 {
		t.Errorf("%d same-origin redirect hop(s) were followed", hops)
	}
	for _, op := range []string{"GetCallerIdentity", "ListBuckets", "ListAccounts"} {
		if e.aws.count(op) < 2 {
			t.Errorf("%s met its redirect fewer than twice (the CLI and the MCP child): %v", op, e.aws.seen())
		}
	}
	// A GET's redirect hop is refused by the pin before it is sent.
	if !strings.Contains(out.String()[before:], pinRefusal) {
		t.Errorf("want the pin's redirect refusal, got:\n%.3000s", out.String()[before:])
	}
	// Class pin-skipped-on-reused-connection: a request on a cached HTTP/2
	// connection skips the transport's proxy hook, where the pin, the
	// redirect refusal and the dry-run refusal run (the same-origin hop above
	// was followed whenever the connection was cached). Every request goes
	// over HTTP/1.1.
	e.aws.mu.Lock()
	protos := fmt.Sprint(e.aws.protos)
	h2 := e.aws.protos["HTTP/2.0"]
	e.aws.mu.Unlock()
	if h2 != 0 {
		t.Errorf("%d request(s) reached AWS over HTTP/2, where the pin is skipped on a cached connection: %s", h2, protos)
	}
	assertNoEcho(t, "output", out.String())
}

// The production build has no test endpoint: given the black-box variables,
// it dials the real AWS hosts (here, the tripwire proxy answers instead) and
// the fake receives nothing.
func TestProductionBuildIgnoresTheTestEndpoint(t *testing.T) {
	t.Parallel()
	e := newEnv(t, srcEnv)
	e.run(t, "bill", "--data-source", "live") // control: the test build reaches the fake
	if e.aws.count("GetCostAndUsage") == 0 {
		t.Fatalf("control: the test build never reached the fake: %v", e.aws.seen())
	}
	e.aws.reset()
	out, _ := e.runCode(t, "", bbProdCLI, "bill", "--data-source", "live")
	if got := e.aws.seen(); len(got) != 0 {
		t.Errorf("the production build sent %v to the loopback fake", got)
	}
	e.trip.mu.Lock()
	hits := e.trip.hits
	e.trip.hits = nil // expected here: the production build asked for real AWS
	e.trip.mu.Unlock()
	if len(hits) == 0 || !strings.Contains(strings.Join(hits, " "), ".amazonaws.com") {
		t.Errorf("the production build asked for no AWS host (proxy saw %v):\n%.2000s", hits, out)
	}
	assertNoEcho(t, "production build output", out)
}

// The generated client (config.toml's auth_header) refuses a redirect off its
// base URL's origin, from the CLI and from the MCP server's endpoint tools.
func TestGeneratedClientNeverLeavesItsOrigin(t *testing.T) {
	t.Parallel()
	e := newEnv(t, srcEnv)
	e.api.mu.Lock()
	e.api.redirect = true
	e.api.mu.Unlock()
	out := e.run(t, "export", "accounts", "--format", "json")
	out += e.mcpCalls(t, []map[string]any{{"name": "accounts_list", "arguments": map[string]any{}}})
	e.api.mu.Lock()
	off := *e.api.offHost
	e.api.mu.Unlock()
	if off != 0 {
		t.Errorf("the other host was contacted %d time(s)", off)
	}
	var redirected int
	for _, p := range e.api.seen() {
		if p == "GET /accounts" {
			redirected++
		}
	}
	if redirected < 2 {
		t.Errorf("only %d request(s) met the redirect; the CLI and MCP calls did not both run: %v", redirected, e.api.seen())
	}
	if !strings.Contains(out, "refusing redirect") {
		t.Errorf("want the origin-pin refusal, got:\n%.2000s", out)
	}
	assertNoEcho(t, "output", out)
}
