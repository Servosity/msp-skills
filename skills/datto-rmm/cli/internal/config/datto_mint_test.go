// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// recordingTransport answers nothing and records every request it is asked
// to send.
type recordingTransport struct{ sent []string }

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.sent = append(r.sent, req.Method+" "+req.URL.String())
	if req.Body != nil {
		_ = req.Body.Close()
	}
	return nil, errors.New("recording transport: not sent")
}

// mint-refuses-plaintext (tests-helper finding): the password grant posts
// the API key and secret, so it is refused over plaintext http to any host
// but a loopback one, exactly as the API client refuses to send a bearer
// token there ("refusing to send credentials over plaintext http"). The mint
// ran in the client hook before any API request, through a transport that
// does not apply that check, so DATTO_RMM_BASE_URL=http://<host>/api posted
// grant_type=password&username=<key>&password=<secret> in the clear. The
// refusal happens before anything reaches the transport, names no credential,
// and a loopback http host (a local double) still mints.
func TestMintRefusesPlaintextOffLoopback(t *testing.T) {
	const mintUser, mintPass = "plainMintKey-4c1e", "plainMintSecret-8a3d"
	for _, base := range []string{"http://datto-plain.example/api", "HTTP://merlot-api.centrastage.net/api", "http://10.0.0.5:8080/api"} {
		rt := &recordingTransport{}
		_, _, err := MintDattoToken(context.Background(), &http.Client{Transport: rt}, base, mintUser, mintPass)
		if err == nil || !strings.Contains(err.Error(), "plaintext") {
			t.Errorf("%s: want the plaintext refusal, got %v", base, err)
		}
		if len(rt.sent) != 0 {
			t.Errorf("%s: the API key and secret were sent in the clear: %v", base, rt.sent)
		}
		if err != nil && (strings.Contains(err.Error(), mintUser) || strings.Contains(err.Error(), mintPass)) {
			t.Errorf("%s: the refusal repeats a credential: %v", base, err)
		}
	}
	for _, base := range []string{"http://127.0.0.1:9/api", "http://localhost:9/api", "http://[::1]:9/api", "https://merlot-api.centrastage.net/api"} {
		rt := &recordingTransport{}
		_, _, _ = MintDattoToken(context.Background(), &http.Client{Transport: rt}, base, mintUser, mintPass)
		if len(rt.sent) != 1 {
			t.Errorf("%s: want the mint sent (a loopback double or https), got %v", base, rt.sent)
		}
	}
}
