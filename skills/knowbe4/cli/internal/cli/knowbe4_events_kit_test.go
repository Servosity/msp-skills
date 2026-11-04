// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Class credential-to-other-api-host under the credential kit (handfixes.json
// credkit-wiring): with the program's own Declarations, the User Event key
// goes to the User Event host, and the Reporting key does not, whole, as a
// piece of 8 or more characters split across fields, or base64 encoded.

import (
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"knowbe4-pp-cli/internal/cliutil"
	"knowbe4-pp-cli/internal/cliutil/testenv"
	"knowbe4-pp-cli/internal/credkit"
	"knowbe4-pp-cli/internal/credwire"
)

// wireRecorder stands in for the network under the kit's outbound guard: what
// reaches it would have been sent.
type wireRecorder struct {
	mu   sync.Mutex
	sent []string
}

func (r *wireRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	var b []byte
	if req.Body != nil {
		b, _ = io.ReadAll(req.Body)
	}
	r.mu.Lock()
	r.sent = append(r.sent, req.URL.Host+" "+string(b))
	r.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"data":{"id":"513f46ac-c3d7-4682-ad0d-0c149c0728a2"},"meta":{}}`)),
		Request:    req,
	}, nil
}

func (r *wireRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.sent)
}

func TestEventsRefuseAPieceOrEncodingOfTheReportingKey(t *testing.T) {
	const reportKey = "ReportingSecret-7c92f1ab3e"
	const eventKey = "EventSecret-8d31e6c40b"
	// credwire binds the User Event key at package init, so the environment
	// is set before the child starts (and again, harmlessly, in the child).
	testenv.Isolate(t)
	t.Setenv("KNOWBE4_USER_EVENT_BASE_URL", "https://events.kb4.test")
	t.Setenv("KNOWBE4_BASE_URL", "https://reporting.kb4.test/v1")
	t.Setenv("KNOWBE4_USER_EVENT_API_KEY", eventKey)
	t.Setenv("KNOWBE4_API_KEY", reportKey)
	if runInChild(t) {
		return
	}
	wire := &wireRecorder{}
	http.DefaultTransport = wire
	cliutil.OutboundGuard = credkit.Transport
	cliutil.HoldCredential = credkit.Hold
	credkit.Start(credwire.Declarations)

	create := func(extra ...string) []string {
		return append([]string{"events", "create", "--target-user", "alice@kb4-demo.com", "--event-type", "Training"}, extra...)
	}
	if out, err := runKnowBe4In(t, "", create("--description", "plain text")...); err != nil || wire.count() != 1 {
		t.Fatalf("control: a plain event under the kit: err %v, %d sent\n%s", err, wire.count(), out)
	}
	for name, args := range map[string][]string{
		"split":  create("--description", "ReportingSecret-", "--external-id", "7c92f1ab3e"),
		"base64": create("--description", "note "+base64.StdEncoding.EncodeToString([]byte(reportKey))),
	} {
		if _, err := runKnowBe4In(t, "", args...); err == nil {
			t.Errorf("%s: events create carrying the Reporting key succeeded", name)
		}
	}
	if n := wire.count(); n != 1 {
		t.Errorf("%d request(s) carrying the Reporting key reached the wire", n-1)
	}
}
