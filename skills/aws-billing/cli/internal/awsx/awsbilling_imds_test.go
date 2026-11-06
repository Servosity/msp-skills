// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package awsx

// Codex r1b class credential-sent-over-plain-http (r1b-imds-off): the SDK's
// EC2 instance-metadata client mints an IMDSv2 token and sends it, in
// x-aws-ec2-metadata-token, to http://169.254.169.254 (or
// http://[fd00:ec2::254]), and reads instance-role credentials back over the
// same plain http. Every config this connector loads turns that client off,
// and the endpoint pin refuses the IMDS addresses as well.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
)

func TestR1bIMDSAddressesRefusedByThePin(t *testing.T) {
	for _, raw := range []string{
		"http://169.254.169.254/latest/api/token",
		"http://169.254.169.254/latest/meta-data/iam/security-credentials/",
		"http://[fd00:ec2::254]/",
		"http://[fd00:ec2::254]/latest/api/token",
	} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if awsEndpointAllowed(u) {
			t.Errorf("%s: the pin allows the instance-metadata address", raw)
		}
		req, _ := http.NewRequest(http.MethodPut, raw, nil)
		if err := checkAWSRequest(req); !errors.Is(err, ErrEndpointRefused) {
			t.Errorf("%s: want ErrEndpointRefused from the transport hook, got %v", raw, err)
		}
	}
	// The container credential addresses stay as they are: an ECS task role
	// without an authorization token is read over plain http (a token is a
	// held credential, so the kit refuses a plain-http request carrying one).
	for _, raw := range []string{"http://169.254.170.2/v2/credentials/abc", "http://169.254.170.23/v1/credentials", "http://[fd00:ec2::23]/v1/credentials"} {
		u, _ := url.Parse(raw)
		if !awsEndpointAllowed(u) {
			t.Errorf("%s: the container credential address is refused", raw)
		}
	}
}

// isolateAWSCredentialSources leaves the SDK no credential source but the
// instance-metadata one: no credential variable, no profile, no container
// endpoint, and shared files that do not exist.
func isolateAWSCredentialSources(t *testing.T) {
	t.Helper()
	for _, name := range []string{"AWS_ACCESS_KEY_ID", "AWS_ACCESS_KEY", "AWS_SECRET_ACCESS_KEY", "AWS_SECRET_KEY", "AWS_SESSION_TOKEN",
		"AWS_SECURITY_TOKEN", "AWS_PROFILE", "AWS_DEFAULT_PROFILE", "AWS_ROLE_ARN", "AWS_WEB_IDENTITY_TOKEN_FILE",
		"AWS_CONTAINER_CREDENTIALS_FULL_URI", "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_CONTAINER_AUTHORIZATION_TOKEN",
		"AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE", "AWS_ENDPOINT_URL", "AWS_CA_BUNDLE", "AWS_DEFAULTS_MODE"} {
		t.Setenv(name, "")
	}
	none := filepath.Join(t.TempDir(), "none")
	t.Setenv("AWS_CONFIG_FILE", none)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", none)
	t.Setenv("AWS_REGION", "us-east-1")
}

// A config loaded the way New loads it has the instance-metadata client off,
// even when the environment asks for it (AWS_EC2_METADATA_DISABLED=false):
// the IMDS client every SDK credential and defaults-mode lookup builds from
// it sends nothing, and New's credential chain with no other source never
// reaches IMDS.
func TestR1bIMDSClientOffInEveryLoadedConfig(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("imdsUnitToken-7c1e9a5d3b"))
	}))
	t.Cleanup(srv.Close)
	isolateAWSCredentialSources(t)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "false")
	t.Setenv("AWS_EC2_METADATA_SERVICE_ENDPOINT", srv.URL)

	c, err := New(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	// The SDK's own IMDS client, built from this config, given a transport
	// that would reach the fake: the config's state alone keeps it silent.
	client := imds.NewFromConfig(c.cfg, func(o *imds.Options) { o.HTTPClient = srv.Client(); o.Endpoint = srv.URL })
	if _, err := client.GetMetadata(context.Background(), &imds.GetMetadataInput{Path: "instance-id"}); err == nil {
		t.Error("the IMDS client of a loaded config answered: it is on")
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("the IMDS client of a loaded config sent %d request(s) to the instance-metadata endpoint", n)
	}

	_, err = c.cfg.Credentials.Retrieve(context.Background())
	if err == nil {
		t.Fatal("credentials resolved with no source configured")
	}
	if !strings.Contains(err.Error(), "instance-metadata (instance role) credentials are not used") {
		t.Errorf("the credential chain did not stop at a disabled instance-metadata client: %v", err)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("the credential chain sent %d request(s) to the instance-metadata endpoint", n)
	}
}
