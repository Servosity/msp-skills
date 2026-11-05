// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package awsbillingblackbox

// Codex r1b class credential-sent-over-plain-http (r1b-imds-off), on the
// built binaries: with no other credential source, the AWS SDK's default
// chain ends at the EC2 instance-metadata service, which mints an IMDSv2
// token and sends it in x-aws-ec2-metadata-token to http://169.254.169.254.
// The production build is run with its http proxy pointed at a fake
// instance-metadata service (a request for 169.254.169.254 reaches the
// proxy, never the network): it must receive nothing. The container
// credential variants that need an authorization token stay refused.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	r1bIMDSToken  = "bbImdsV2Token-AQAEAKl9c3e7f1a5b"
	r1bIMDSKey    = "ASIA" + "BBIMDSKEY0000001"
	r1bIMDSSecret = "bbImdsRoleSecret/4kT7wQ1zX9cV3bN6"
	r1bIMDSSessn  = "bbImdsRoleSession+Zm9vYmFyYmF6cXV4"
)

// fakeIMDS is an http proxy that answers as the instance-metadata service:
// an IMDSv2 token, a role name, and the role's credentials.
type fakeIMDS struct {
	url    string
	mu     sync.Mutex
	reqs   []string // "METHOD host path"
	tokens []string // x-aws-ec2-metadata-token values received
}

func newFakeIMDS(t *testing.T) *fakeIMDS {
	t.Helper()
	f := &fakeIMDS{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.reqs = append(f.reqs, r.Method+" "+r.Host+" "+r.URL.Path)
		if tok := r.Header.Get("X-Aws-Ec2-Metadata-Token"); tok != "" {
			f.tokens = append(f.tokens, tok)
		}
		f.mu.Unlock()
		switch p := r.URL.Path; {
		case p == "/latest/api/token":
			w.Header().Set("X-Aws-Ec2-Metadata-Token-Ttl-Seconds", r.Header.Get("X-Aws-Ec2-Metadata-Token-Ttl-Seconds"))
			_, _ = w.Write([]byte(r1bIMDSToken))
		case p == "/latest/meta-data/iam/security-credentials/" || p == "/latest/meta-data/iam/security-credentials-extended/":
			_, _ = w.Write([]byte("bb-instance-role"))
		case strings.HasPrefix(p, "/latest/meta-data/iam/security-credentials"):
			v, _ := json.Marshal(map[string]any{"Code": "Success", "Type": "AWS-HMAC", "AccessKeyId": r1bIMDSKey,
				"SecretAccessKey": r1bIMDSSecret, "Token": r1bIMDSSessn,
				"Expiration": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "LastUpdated": time.Now().UTC().Format(time.RFC3339)})
			_, _ = w.Write(v)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f
}

func (f *fakeIMDS) seen() ([]string, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.reqs...), append([]string(nil), f.tokens...)
}

func TestR1bInstanceMetadataNeverContacted(t *testing.T) {
	t.Parallel()
	e := newEnv(t, srcEnv)
	imds := newFakeIMDS(t)
	empty := filepath.Join(e.home, "empty")
	e.input(t, empty, "")
	env := e.env
	for _, k := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_EC2_METADATA_DISABLED"} {
		env = withoutEnv(env, k)
	}
	c := *e
	c.env = append(env, "AWS_SHARED_CREDENTIALS_FILE="+empty, "AWS_CONFIG_FILE="+empty,
		"HTTP_PROXY="+imds.url, "http_proxy="+imds.url)
	out, code := c.runCode(t, "", bbProdCLI, "bill", "--data-source", "live")
	reqs, tokens := imds.seen()
	if len(reqs) != 0 {
		t.Errorf("the instance-metadata service was contacted over plain http (requests %v; IMDSv2 tokens sent: %v)", reqs, tokens)
	}
	if code == 0 {
		t.Errorf("bill resolved credentials with no source but instance metadata:\n%.1200s", out)
	}
	if !strings.Contains(out, "instance-metadata (instance role) credentials are not used") {
		t.Errorf("the error does not say instance-metadata credentials are not used:\n%.1200s", out)
	}
	for _, s := range []string{r1bIMDSToken, r1bIMDSKey, r1bIMDSSecret, r1bIMDSSessn} {
		if strings.Contains(out, s) {
			t.Errorf("the output carries an instance-metadata credential: %s", around(out, s))
		}
	}
}

// The container credential endpoint that needs an authorization token over
// plain http (AWS_CONTAINER_AUTHORIZATION_TOKEN, or its _FILE) is never sent
// the token: the token is held, and the kit refuses a plain-http request that
// carries a held value.
func TestR1bContainerTokenNeverSentOverPlainHTTP(t *testing.T) {
	t.Parallel()
	for _, viaFile := range []bool{false, true} {
		e := newEnv(t, srcContainer)
		if viaFile {
			tok := filepath.Join(e.home, "ecs-token")
			e.input(t, tok, bbECSAuth)
			e = e.with("AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE=" + tok)
			e.env = withoutEnv(e.env, "AWS_CONTAINER_AUTHORIZATION_TOKEN")
		}
		out := e.run(t, "bill", "--data-source", "live")
		for _, r := range e.aws.seen() {
			if strings.HasPrefix(r, "ecs ") && strings.Contains(r, bbECSAuth) {
				t.Errorf("viaFile=%v: the container endpoint received the authorization token over plain http", viaFile)
			}
		}
		if e.aws.mintedSigned() != 0 {
			t.Errorf("viaFile=%v: a request was signed with container credentials fetched over plain http with the token", viaFile)
		}
		assertNoEcho(t, "bill output", out)
	}
}
