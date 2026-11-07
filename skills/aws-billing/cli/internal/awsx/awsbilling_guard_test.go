// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package awsx

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"

	"aws-billing-pp-cli/internal/credkit"
)

// kitWithholds reports whether the kit withholds s on a stdout writer.
func kitWithholds(s string) bool {
	var buf bytes.Buffer
	_, err := credkit.NewWriter(&buf, credkit.SinkStdout).Write([]byte(s))
	return err != nil || buf.String() != s
}

// inFreshProcess reruns the calling test alone in a child process: a held
// value lasts for the process.
func inFreshProcess(t *testing.T) bool {
	t.Helper()
	const marker = "AWS_BILLING_TEST_FRESH_PROCESS"
	if os.Getenv(marker) == t.Name() {
		return false
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.count=1") // #nosec G204 -- this test binary
	cmd.Env = append(os.Environ(), marker+"="+t.Name())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	return true
}

// aws-endpoint-pinned: only the AWS endpoints of the services this connector
// calls, over https on 443, and the SDK's link-local container credential
// sources over plain http.
func TestAWSEndpointAllowlist(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://ce.us-east-1.amazonaws.com/":                   true,
		"https://ce.cn-northwest-1.amazonaws.com.cn/":           true,
		"https://organizations.us-east-1.amazonaws.com/":        true,
		"https://sts.amazonaws.com/":                            true,
		"https://sts.eu-west-2.amazonaws.com/":                  true,
		"https://ec2.ap-southeast-4.amazonaws.com/":             true,
		"https://ec2-fips.us-gov-west-1.amazonaws.com/":         true,
		"https://ec2.us-east-1.api.aws/":                        true,
		"https://monitoring.eu-central-1.amazonaws.com/":        true,
		"https://s3.us-east-1.amazonaws.com/":                   true,
		"https://s3.dualstack.us-west-2.amazonaws.com/":         true,
		"https://portal.sso.us-east-1.amazonaws.com/federation": true,
		"https://oidc.us-east-1.amazonaws.com/token":            true,
		"https://signin.us-east-1.amazonaws.com/":               true,
		"https://CE.US-EAST-1.AMAZONAWS.COM/":                   true,
		"http://169.254.169.254/latest/api/token":               false, // instance metadata is never used (r1b-imds-off)
		"http://[fd00:ec2::254]/latest/meta-data/":              false,
		"http://169.254.170.2/v2/credentials/abc":               true,
		"http://ce.us-east-1.amazonaws.com/":                    false, // plain http
		"https://ce.us-east-1.amazonaws.com:8443/":              false,
		"https://mybucket.s3.us-east-1.amazonaws.com/":          false, // virtual-hosted S3 is never used
		"https://iam.amazonaws.com/":                            false, // not a service this connector calls
		"https://ce.us-east-1.amazonaws.com.evil.example/":      false,
		"https://evil.example/":                                 false,
		"https://127.0.0.1:4566/":                               false, // a local emulator endpoint
		"https://user:pw@ce.us-east-1.amazonaws.com/":           false,
		"https://ce.us-east-1.amazоnaws.com/":                   false, // Cyrillic o
		"http://169.254.169.254:8080/":                          false,
		"http://10.0.0.1/latest/meta-data/":                     false,
		"ftp://ce.us-east-1.amazonaws.com/":                     false,
	} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := awsEndpointAllowed(u); got != want {
			t.Errorf("%s: allowed=%v, want %v", raw, got, want)
		}
	}
}

// The loopback container-credential endpoint is allowed only when the
// environment names it (AWS_CONTAINER_CREDENTIALS_FULL_URI), port included.
func TestContainerFullURIOnlyAsConfigured(t *testing.T) {
	u, _ := url.Parse("http://127.0.0.1:51679/creds")
	if awsEndpointAllowed(u) {
		t.Fatal("loopback allowed without AWS_CONTAINER_CREDENTIALS_FULL_URI")
	}
	t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", "http://127.0.0.1:51679/creds")
	if !awsEndpointAllowed(u) {
		t.Fatal("the configured container endpoint is refused")
	}
	other, _ := url.Parse("http://127.0.0.1:9999/creds")
	if awsEndpointAllowed(other) {
		t.Fatal("another loopback port is allowed")
	}
}

// A custom endpoint (AWS_ENDPOINT_URL) is refused before anything is sent:
// the signed request, its access key ID and session token never reach the
// host. The same for a redirect hop and under --dry-run.
func TestCustomEndpointNeverReceivesASignedRequest(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("AWS_ENDPOINT_URL", srv.URL)
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIA"+"UNITPINTEST00001")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "pinUnitSecret/1c9e7a5b3d")
	t.Setenv("AWS_SESSION_TOKEN", "pinUnitSessionToken-7e3a9c")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_CONFIG_FILE", "/nonexistent")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/nonexistent")
	c, err := New(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = c.CallerIdentity(context.Background())
	if !errors.Is(err, ErrEndpointRefused) {
		t.Fatalf("custom endpoint: want ErrEndpointRefused, got %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("the custom endpoint received %d request(s)", hits.Load())
	}
}

func TestRedirectAndDryRunRefused(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://sts.amazonaws.com/", nil)
	req.Response = &http.Response{StatusCode: http.StatusTemporaryRedirect}
	if err := checkAWSRequest(req); !errors.Is(err, ErrEndpointRefused) {
		t.Fatalf("redirect hop: want ErrEndpointRefused, got %v", err)
	}
	prev := DryRun
	t.Cleanup(func() { DryRun = prev })
	DryRun = func() bool { return true }
	req, _ = http.NewRequest(http.MethodPost, "https://ce.us-east-1.amazonaws.com/", nil)
	if err := checkAWSRequest(req); !errors.Is(err, ErrDryRunSend) {
		t.Fatalf("dry run: want ErrDryRunSend, got %v", err)
	}
	// The transport hook refuses through the real transport too.
	tr := &http.Transport{}
	pinTransport(tr)
	if _, err := (&http.Client{Transport: tr}).Get("https://ce.us-east-1.amazonaws.com/"); !errors.Is(err, ErrDryRunSend) {
		t.Fatalf("transport under --dry-run: want ErrDryRunSend, got %v", err)
	}
}

// minted-credentials-held: a refreshed credential is held when Retrieve
// returns it, before the signer uses it.
func TestRetrievedCredentialsHeld(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	n := 0
	p := holdingProvider{inner: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
		n++
		return aws.Credentials{AccessKeyID: "ASIA" + "UNITREFRESH00" + strings.Repeat("0", n), SecretAccessKey: "refreshSecretUnit-" + strings.Repeat("a", n),
			SessionToken: "refreshSessionUnit-" + strings.Repeat("b", n)}, nil
	})}
	for i := 0; i < 2; i++ {
		v, err := p.Retrieve(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range []string{v.AccessKeyID, v.SecretAccessKey, v.SessionToken} {
			if !kitWithholds("x " + s + " y") {
				t.Errorf("retrieve %d: %s not held", i+1, s)
			}
		}
	}
}

// mcpb-placeholder-not-a-credential: a .mcpb host that skips the optional
// session token leaves "${user_config.aws_session_token}" in the environment;
// New drops it, so long-term keys sign without a bogus session token.
const unitPlaceholderKey = "AKIA" + "UNITPLACEHOLDER1"

func TestUnresolvedMCPBPlaceholderIsNotASessionToken(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", unitPlaceholderKey)
	t.Setenv("AWS_SECRET_ACCESS_KEY", "placeholderUnitSecret-1c9e")
	t.Setenv("AWS_SESSION_TOKEN", "${user_config.aws_session_token}")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_CONFIG_FILE", "/nonexistent")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/nonexistent")
	c, err := New(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	v, err := c.cfg.Credentials.Retrieve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v.SessionToken != "" || v.AccessKeyID != unitPlaceholderKey {
		t.Fatalf("credentials = %q / session %q, want the keys and no session token", v.AccessKeyID, v.SessionToken)
	}
}
