// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package awsx

// Codex r1b class credential-used-but-not-held (r1b-signers-held): every
// credential that signs an outgoing request is held before the request is
// sent, whatever provider produced it and at any depth of an assume-role
// chain. The transport's per-request hook reads the SigV4 Authorization
// header, X-Amz-Security-Token and the presigned query forms, holds the
// access key ID and the session token, and refuses a request to an AWS
// endpoint whose Authorization is not SigV4.

import (
	"net/http"
	"strings"
	"testing"
)

const (
	r1bSignerKey   = "ASIA" + "UNITSIGNERKEY001"
	r1bSignerToken = "unitSignerSessionToken+Q2hhaW4"
	r1bPresignKey  = "ASIA" + "UNITPRESIGNKEY02"
	r1bPresignTok  = "unitPresignSessionTok/Zm9vYmFy"
)

// hookOf is the per-request hook the pinned transport runs on every request
// (pinTransport).
func hookOf(t *testing.T) func(*http.Request) error {
	t.Helper()
	tr := &http.Transport{}
	pinTransport(tr)
	return func(req *http.Request) error {
		_, err := tr.Proxy(req)
		return err
	}
}

func TestR1bSignersHeldAtTheWire(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	hook := hookOf(t)

	req, _ := http.NewRequest(http.MethodPost, "https://sts.amazonaws.com/", nil)
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+r1bSignerKey+"/20261105/us-east-1/sts/aws4_request, "+
		"SignedHeaders=content-type;host;x-amz-date;x-amz-security-token, Signature=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	req.Header.Set("X-Amz-Security-Token", r1bSignerToken)
	if err := hook(req); err != nil {
		t.Fatalf("a SigV4-signed request to STS was refused: %v", err)
	}
	for _, s := range []string{r1bSignerKey, r1bSignerToken} {
		if !kitWithholds("AccessDenied for " + s + " here") {
			t.Errorf("%s signed a request and is not held", s)
		}
	}

	// The presigned query forms.
	req, _ = http.NewRequest(http.MethodGet, "https://s3.us-east-1.amazonaws.com/?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential="+
		r1bPresignKey+"%2F20261105%2Fus-east-1%2Fs3%2Faws4_request&X-Amz-Security-Token="+r1bPresignTok+"&X-Amz-Signature=00ff", nil)
	if err := hook(req); err != nil {
		t.Fatalf("a presigned request to S3 was refused: %v", err)
	}
	for _, s := range []string{r1bPresignKey, r1bPresignTok} {
		if !kitWithholds("x " + s + " y") {
			t.Errorf("%s (presigned query) signed a request and is not held", s)
		}
	}

	// Fail closed: an Authorization on a request to an AWS endpoint that is
	// not SigV4 is refused before it is sent.
	for _, auth := range []string{
		"Bearer unitBearerNotSigV4-1a2b3c",
		"AWS4-HMAC-SHA256 SignedHeaders=host, Signature=00",
		"AWS4-HMAC-SHA256 Credential=/20261105/us-east-1/sts/aws4_request, SignedHeaders=host, Signature=00",
		"AWS4-HMAC-SHA256 Credential=unitNoScopeKey1, SignedHeaders=host, Signature=00",
		"AWS unitV2Key:sig",
	} {
		req, _ = http.NewRequest(http.MethodPost, "https://ce.us-east-1.amazonaws.com/", nil)
		req.Header.Set("Authorization", auth)
		if err := hook(req); err == nil || !strings.Contains(err.Error(), "not a SigV4 signature") {
			t.Errorf("Authorization %q: want the unreadable-signature refusal, got %v", auth, err)
		}
	}
}
