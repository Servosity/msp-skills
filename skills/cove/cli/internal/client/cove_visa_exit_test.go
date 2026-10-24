// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated), cove only: the generated client has one exit
// point (coveExit), holds the visas of every header and of the raw body before
// any decoding, and never quotes a response header value into an error or a
// result (Codex round 2 and the round 2 adversarial review, press 4.34.0
// reprint). Hand-fix code: cove-short-credential-funnel.

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cove-pp-cli/internal/cliutil"
	"cove-pp-cli/internal/cliutil/testenv"
	"cove-pp-cli/internal/config"
)

type coveScripted struct {
	status  int
	headers map[string]string
	body    string
}

// coveScriptedClient answers the n-th request with answers[n] (the last one
// repeats), on loopback, with no retries.
func coveScriptedClient(t *testing.T, answers ...coveScripted) *Client {
	t.Helper()
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv(cliutil.DogfoodEnvVar, "1")
	var n int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(atomic.AddInt64(&n, 1)) - 1
		if i >= len(answers) {
			i = len(answers) - 1
		}
		a := answers[i]
		for k, v := range a.headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(a.status)
		_, _ = w.Write([]byte(a.body))
	}))
	t.Cleanup(srv.Close)
	return New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
}

func coveCall(c *Client, sentVisa string) (json.RawMessage, error) {
	data, _, err := c.PostQueryWithParams(context.Background(), "/jsonapi", nil, coveRawBody(sentVisa))
	return data, err
}

func coveShown(data json.RawMessage, err error) string {
	shown := string(data)
	if raw, _, ok := UnwrapBinaryResponse(data); ok {
		shown += "\n" + string(raw)
	}
	if err != nil {
		shown += "\n" + err.Error()
	}
	return shown
}

// Codex round 2, finding 1: the sent visa in a Content-Type parameter reached
// the binary envelope's content_type. The envelope names only a bare, known
// media type, and the whole returned value passes the exit check.
func TestCoveBinaryEnvelopeNeverQuotesTheContentType(t *testing.T) {
	const sent = "sent-visa-ct-0123456789"
	c := coveScriptedClient(t, coveScripted{200, map[string]string{"Content-Type": "application/octet-stream; name=" + sent}, "ok"})
	data, err := coveCall(c, sent)
	if coveLeaks(coveShown(data, err), sent) {
		t.Fatalf("the sent visa reached the result: %s", coveShown(data, err))
	}
	if err != nil {
		t.Fatalf("a clean binary answer was refused: %v", err)
	}
	if raw, ct, ok := UnwrapBinaryResponse(data); !ok || ct != "application/octet-stream" || string(raw) != "ok" {
		t.Fatalf("binary envelope = %s", data)
	}
}

// A content type outside the known set is never quoted, held value or not.
func TestCoveBinaryEnvelopeNamesOnlyAKnownMediaType(t *testing.T) {
	const quoted = "hdrquote-ct-0123456789"
	c := coveScriptedClient(t, coveScripted{200, map[string]string{"Content-Type": "application/" + quoted}, "ok"})
	data, err := coveCall(c, "sent-visa-0123456789")
	if strings.Contains(coveShown(data, err), quoted) {
		t.Fatalf("a Content-Type value was quoted: %s", coveShown(data, err))
	}
}

// Codex round 2, finding 2: Content-Encoding failed the decode before the
// body's visa was held, and the error quoted the header (the visa).
func TestCoveContentEncodingErrorShowsNoVisa(t *testing.T) {
	const fresh = "fresh-visa-ce-0123456789"
	c := coveScriptedClient(t,
		coveScripted{200, map[string]string{"Content-Encoding": fresh, "Content-Type": "application/json"}, `{"visa":"` + fresh + `","result":{"result":{}}}`},
		// The next answer repeats the visa with no "visa" name next to it:
		// only a visa held from the first answer's raw bytes withholds it.
		coveScripted{400, map[string]string{"Content-Type": "text/plain"}, "session " + fresh + " expired"},
	)
	data, err := coveCall(c, "sent-visa-0123456789")
	if err == nil || coveLeaks(coveShown(data, err), fresh) {
		t.Fatalf("decode failure shows the visa or succeeded: %s", coveShown(data, err))
	}
	data, err = coveCall(c, "sent-visa-0123456789")
	if coveLeaks(coveShown(data, err), fresh) {
		t.Fatalf("the visa from the undecodable answer was not held: %s", coveShown(data, err))
	}
}

// The decode error never quotes the header, whatever it holds.
func TestCoveContentEncodingErrorNeverQuotesTheHeader(t *testing.T) {
	const quoted = "hdrquote-enc-0123456789"
	c := coveScriptedClient(t, coveScripted{200, map[string]string{"Content-Encoding": quoted}, `{"result":{"result":{}}}`})
	data, err := coveCall(c, "sent-visa-0123456789")
	if err == nil || strings.Contains(coveShown(data, err), quoted) {
		t.Fatalf("Content-Encoding value quoted or accepted: %s", coveShown(data, err))
	}
}

// Round 2 adversarial P1: an earlier "name:" pair swallowed "visa=X", so the
// fresh visa in a text answer was never held and was shown.
func TestCoveTextAnswerVisaAfterAnotherNameIsWithheld(t *testing.T) {
	const fresh = "FRESHVISAq8W3e5R7t9Y1u2I4K"
	for _, a := range []coveScripted{
		{400, map[string]string{"Content-Type": "text/plain"}, "error: visa=" + fresh},
		{200, map[string]string{"Content-Type": "text/plain"}, "result: visa=" + fresh},
		{500, map[string]string{"Content-Type": "text/plain"}, "upstream: visa = " + fresh + " (rotated)"},
	} {
		c := coveScriptedClient(t, a)
		data, err := coveCall(c, "sent-visa-0123456789")
		if coveLeaks(coveShown(data, err), fresh) {
			t.Fatalf("HTTP %d %q: the visa was shown: %s", a.status, a.body, coveShown(data, err))
		}
	}
}

// A visa carried in a response header is held before anything is shown, so
// a body that repeats it without naming it is refused.
func TestCoveHeaderVisaIsHeld(t *testing.T) {
	const fresh = "hdr-visa-0123456789abcdef"
	c := coveScriptedClient(t, coveScripted{200, map[string]string{"Content-Type": "application/json", "X-Cove-Visa": fresh}, `{"result":{"result":{"Note":"` + fresh + `"}}}`})
	data, err := coveCall(c, "sent-visa-0123456789")
	if coveLeaks(coveShown(data, err), fresh) {
		t.Fatalf("a body repeating the header visa was shown: %s", coveShown(data, err))
	}
	if !coveHolds(fresh) {
		t.Fatal("the header visa is not held")
	}
}

// The 403 annotation quotes no header value; Cloudflare's documented
// challenge marker is kept.
func TestCoveTransportBlockQuotesNoHeaderValue(t *testing.T) {
	const quoted = "hdrquote-cf-0123456789"
	c := coveScriptedClient(t, coveScripted{403, map[string]string{"Content-Type": "text/plain", "cf-mitigated": quoted}, "forbidden"})
	data, err := coveCall(c, "sent-visa-0123456789")
	if strings.Contains(coveShown(data, err), quoted) {
		t.Fatalf("cf-mitigated value quoted: %s", coveShown(data, err))
	}
	c = coveScriptedClient(t, coveScripted{403, map[string]string{"Content-Type": "text/plain", "cf-mitigated": "challenge"}, "forbidden"})
	_, err = coveCall(c, "sent-visa-0123456789")
	if err == nil || !strings.Contains(err.Error(), "cf-mitigated: challenge") {
		t.Fatalf("the Cloudflare challenge marker was lost: %v", err)
	}
}

// The false hold: a word next to "visa" ("visa: required"), a number or null
// is never held, so later ordinary output is neither masked nor withheld.
func TestCoveWordNextToVisaIsNotHeld(t *testing.T) {
	c := coveScriptedClient(t,
		coveScripted{401, map[string]string{"Content-Type": "text/plain"}, "visa: required"},
		coveScripted{200, map[string]string{"Content-Type": "application/json"}, `{"visa":0,"result":{"result":{}}}`},
		coveScripted{200, map[string]string{"Content-Type": "application/json"}, `{"result":{"result":{"Name":"required-nas-01","Count":0}}}`},
	)
	_, _ = coveCall(c, "sent-visa-0123456789")
	_, _ = coveCall(c, "sent-visa-0123456789")
	data, err := coveCall(c, "sent-visa-0123456789")
	if err != nil || !strings.Contains(string(data), "required-nas-01") {
		t.Fatalf("ordinary output was withheld after a word next to visa: %s %v", data, err)
	}
	if coveHolds("required") || coveHolds("0") {
		t.Fatal("a word or number next to visa was held")
	}
}

// The exit point checks the whole returned value: a binary envelope whose
// content type repeats a held value is refused even though its body is clean.
func TestCoveExitChecksTheWholeReturnedValue(t *testing.T) {
	const held = "exit-check-visa-0123456789"
	cliutil.FunnelHold(held)
	env, _ := wrapBinaryResponse("application/octet-stream; name="+held, []byte("ok"))
	if out, err := coveExit(env, nil); err == nil || out != nil {
		t.Fatalf("an envelope repeating a held value left the client: %s", out)
	}
	clean, _ := wrapBinaryResponse("application/octet-stream", []byte("ok"))
	if out, err := coveExit(clean, nil); err != nil || string(out) != string(clean) {
		t.Fatalf("a clean envelope was refused: %v", err)
	}
}

// An error whose text repeats a held value keeps its kind and HTTP status.
func TestCoveExitKeepsTheErrorKind(t *testing.T) {
	const held = "exit-err-visa-0123456789"
	cliutil.FunnelHold(held)
	err := coveExitError(&APIError{Method: "POST", Path: "/jsonapi", StatusCode: 401, Body: "bad " + held})
	var api *APIError
	if strings.Contains(err.Error(), held) || !strings.Contains(err.Error(), "HTTP 401") || !asAPIError(err, &api) {
		t.Fatalf("exit error = %v", err)
	}
}

func asAPIError(err error, target **APIError) bool {
	e, ok := err.(*APIError)
	if ok {
		*target = e
	}
	return ok
}
