// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package awsx

// aws-billing handfixes on the AWS SDK client, which every AWS call of the CLI,
// its novel commands and its MCP server goes through (New). The shared
// credential kit (credkit.Transport) is the outbound guard around every SDK
// request and every credential provider's request, and it reads the declared
// credential fields of every answer (credwire ResponseFields). What stays here
// is what the kit does not do:
//
//   - aws-endpoint-pinned: a request (signed or not, the SDK's credential
//     providers included) goes only to an AWS endpoint of a service this
//     connector calls, over https on port 443 (awsEndpointAllowed), or over
//     plain http to the container-credential addresses the SDK reads task
//     credentials from. The kit allows a credential to every
//     *.amazonaws.com host; this narrows the hosts to the services this
//     connector calls, refuses a custom endpoint (AWS_ENDPOINT_URL*, an
//     endpoint_url in the shared config) and refuses every redirect.
//   - minted-credentials-held: every credential the SDK resolves or refreshes
//     (static keys, SSO, assume-role, credential_process, task role) is held
//     with the kit the moment Retrieve returns it, before it signs anything.
//   - r1b-signers-held (Codex r1b class credential-used-but-not-held): the
//     provider wrapper above sees only the outermost provider, and in an
//     assume-role chain the SDK signs the STS call with the source profile's
//     credentials (a credential_process, static keys, SSO or a nested role),
//     which it never returns. So the transport's per-request hook holds every
//     credential that signs any request, whatever produced it, before the
//     request goes out (holdSigners): the access key ID in the SigV4
//     Authorization header or presigned X-Amz-Credential, and the session
//     token in X-Amz-Security-Token. A request to an AWS endpoint whose
//     Authorization is not SigV4 is refused.
//   - r1b-imds-off (Codex r1b class credential-sent-over-plain-http): the
//     SDK's EC2 instance-metadata client is off in every config New loads
//     (it mints an IMDSv2 token and sends it, and reads instance-role
//     credentials back, over plain http), and the pin refuses the IMDS
//     addresses too.
//   - dry-run-sends-nothing: under --dry-run every AWS request is refused at
//     the transport (DryRun, set by the CLI).
//   - s3-path-style: S3 is addressed path-style, so its host never carries a
//     bucket name.
//   - credential-process-output-withheld (Codex r1 class
//     secret-in-output-before-holder): a credential_process prints to this
//     process's stderr (the SDK hands it os.Stderr) while it runs, before its
//     answer is parsed and held, so nothing could withhold what it printed; and
//     an answer that does not parse is quoted whole in the SDK's error. While a
//     credential retrieval runs, os.Stderr is the null device (every
//     credential_process the chain starts, at any depth, inherits it), and a
//     process-provider error is replaced by a fixed text.

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/processcreds"
	"github.com/aws/smithy-go"

	"aws-billing-pp-cli/internal/credkit"
)

// ErrEndpointRefused marks a request refused because it would leave AWS.
var ErrEndpointRefused = errors.New("request refused: it would leave the AWS service endpoints this CLI calls (custom endpoints and redirects are not followed)")

// ErrDryRunSend marks an AWS request refused under --dry-run.
var ErrDryRunSend = errors.New("--dry-run: AWS request not sent")

// DryRun reports whether the running command is a --dry-run preview. The CLI
// sets it at startup; it is read on every request.
var DryRun = func() bool { return false }

// OutboundGuard and HoldMinted are the kit's. In-process tests run no
// credkit.Start, so the guard has no declarations and a held fixture would
// stay held for every later test: internal/cli's tests replace both
// (credkit_seam_test.go). The built binaries prove the real ones.
var (
	OutboundGuard = credkit.Transport
	HoldMinted    = credkit.Hold
)

// pinnedHTTPClient is the one HTTP client the SDK uses (config.WithHTTPClient
// and the container credential provider), for service calls and credential
// providers alike: the credential kit's outbound guard around a transport
// with the SDK's own defaults. The pin runs in the transport's per-request
// proxy hook, which every request and every redirect hop passes before it
// dials. Hand-fix (handfixes.json credkit-wiring).
func pinnedHTTPClient() *http.Client {
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}
	pinTransport(tr)
	if blackboxTransport != nil {
		blackboxTransport(tr) // the black-box test build only (awsbilling_testendpoint.go); nil in production
	}
	return &http.Client{Transport: OutboundGuard(tr)}
}

func pinTransport(tr *http.Transport) {
	// HTTP/1.1 only (black-box suite class pin-skipped-on-reused-connection):
	// the SDK's transport attempts HTTP/2, and a request for a host with a
	// cached HTTP/2 connection goes to the HTTP/2 round tripper without the
	// proxy hook, so the redirect and dry-run refusals below never saw it (a
	// same-origin redirect hop was followed). An empty non-nil TLSNextProto
	// keeps every request on the path that calls the hook, and the TLS
	// handshake offers HTTP/1.1 only (an inherited "h2"
	// offer would have the server answer in a protocol the client no longer
	// speaks). No API this connector calls needs HTTP/2.
	tr.ForceAttemptHTTP2 = false
	tr.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	if tr.TLSClientConfig == nil {
		tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	tr.TLSClientConfig.NextProtos = []string{"http/1.1"}
	next := tr.Proxy
	tr.Proxy = func(req *http.Request) (*url.URL, error) {
		// r1b-signers-held: the kit's guard runs this hook on every request
		// before it judges the request (and outside its locks), so a signer
		// held here is held for that judgment and for every later sink.
		if err := holdSigners(req); err != nil {
			return nil, err
		}
		if err := checkAWSRequest(req); err != nil {
			return nil, err
		}
		if next != nil {
			return next(req)
		}
		return nil, nil
	}
}

// checkAWSRequest refuses a redirect hop, a request to anything but an AWS
// endpoint of a service this connector calls, and any request under --dry-run.
func checkAWSRequest(req *http.Request) error {
	if req.Response != nil {
		return refusal{fmt.Errorf("%w (a redirect to %s)", ErrEndpointRefused, displayHost(req.URL))}
	}
	if !awsEndpointAllowed(req.URL) {
		return refusal{fmt.Errorf("%w (%s)", ErrEndpointRefused, displayHost(req.URL))}
	}
	if DryRun() {
		return refusal{fmt.Errorf("%w (%s %s)", ErrDryRunSend, req.Method, displayHost(req.URL))}
	}
	return nil
}

// ErrUnreadableSignature marks a request to an AWS endpoint whose
// Authorization header is not a SigV4 signature this CLI can read: the
// credential inside it could not be held before it is sent.
var ErrUnreadableSignature = errors.New("request refused: its Authorization header is not a SigV4 signature this CLI can read, so the credential in it cannot be held")

// holdSigners holds every credential that signs req (r1b-signers-held),
// before req is sent: the access key ID of each SigV4 Authorization header
// (AWS4-HMAC-SHA256 Credential=<key>/<date>/<region>/<service>/aws4_request,
// SignedHeaders=..., Signature=...) and of a presigned X-Amz-Credential
// query value, and each X-Amz-Security-Token header or query value. Header
// and query names are matched in any case. Fail closed: an https request
// (every one the pin allows goes to an AWS endpoint) that carries an
// Authorization header which is not SigV4 is refused. A plain-http request
// goes only to a container credential address, whose Authorization is the
// container's token: the kit holds that token from its source and refuses
// it over plain http.
func holdSigners(req *http.Request) error {
	var held []string
	strict := strings.EqualFold(req.URL.Scheme, "https")
	for name, vals := range req.Header {
		switch {
		case strings.EqualFold(name, "Authorization"):
			for _, v := range vals {
				key, ok := sigV4AccessKey(v)
				if !ok {
					if strict {
						return refusal{fmt.Errorf("%w (%s)", ErrUnreadableSignature, displayHost(req.URL))}
					}
					continue
				}
				held = append(held, key)
			}
		case strings.EqualFold(name, "X-Amz-Security-Token"):
			held = append(held, vals...)
		}
	}
	for name, vals := range req.URL.Query() {
		switch {
		case strings.EqualFold(name, "X-Amz-Credential"):
			for _, v := range vals {
				key, ok := scopedAccessKey(v)
				if !ok {
					return refusal{fmt.Errorf("%w (%s)", ErrUnreadableSignature, displayHost(req.URL))}
				}
				held = append(held, key)
			}
		case strings.EqualFold(name, "X-Amz-Security-Token"):
			held = append(held, vals...)
		}
	}
	if len(held) > 0 {
		HoldMinted("minted:aws-credentials", held...)
	}
	return nil
}

// sigV4AccessKey reads the access key ID of a SigV4 Authorization value as
// the SDK's signer writes it (aws/signer/v4 buildAuthorizationHeader):
// "AWS4-HMAC-SHA256 Credential=<key>/<scope>, SignedHeaders=<names>,
// Signature=<hex>". Anything else is not SigV4.
func sigV4AccessKey(v string) (string, bool) {
	const prefix = "AWS4-HMAC-SHA256 Credential="
	if !strings.HasPrefix(v, prefix) {
		return "", false
	}
	cred, rest, ok := strings.Cut(v[len(prefix):], ", SignedHeaders=")
	if !ok {
		return "", false
	}
	headers, sig, ok := strings.Cut(rest, ", Signature=")
	if !ok || headers == "" || sig == "" {
		return "", false
	}
	return scopedAccessKey(cred)
}

// scopedAccessKey is the access key ID in front of a SigV4 credential scope:
// "<key>/<yyyymmdd>/<region>/<service>/aws4_request". The scope is the last
// four parts, so a key is read whole whatever it holds.
func scopedAccessKey(cred string) (string, bool) {
	parts := strings.Split(cred, "/")
	if len(parts) < 5 || parts[len(parts)-1] != "aws4_request" {
		return "", false
	}
	date := parts[len(parts)-4]
	if len(date) != 8 || strings.Trim(date, "0123456789") != "" || parts[len(parts)-3] == "" || parts[len(parts)-2] == "" {
		return "", false
	}
	key := strings.Join(parts[:len(parts)-4], "/")
	if key == "" {
		return "", false
	}
	return key, true
}

// refusal is a request checkAWSRequest refused (black-box suite class
// refusal-retried): it can never succeed, yet the SDK read it as a failed send
// (smithyhttp.RequestSendError is a connection error) and retried it with
// backoff, three attempts per operation and three per credential refresh
// inside each, so a sync against a refused endpoint took 73 seconds to fail.
// RetryableError is the aws/retry.RetryableError contract, which the standard
// retryer reads before the connection-error rule: never retry it.
type refusal struct{ err error }

func (r refusal) Error() string      { return r.err.Error() }
func (r refusal) Unwrap() error      { return r.err }
func (refusal) RetryableError() bool { return false }

// awsServiceHost matches the endpoint hosts of the AWS services this connector
// calls, in every partition DNS suffix the SDK uses for them, with an optional
// FIPS or dual-stack form and an optional region:
// ce, organizations, sts, ec2, monitoring (CloudWatch), s3, and the credential
// services portal.sso, oidc (SSO OIDC) and signin.
var awsServiceHost = regexp.MustCompile(`^(ce|organizations|sts|ec2|monitoring|s3|portal\.sso|oidc|signin)(-fips)?(\.dualstack)?(\.[a-z]{2,4}(-[a-z]+)+-[0-9]+)?\.(amazonaws\.com|amazonaws\.com\.cn|amazonaws\.eu|api\.aws|api\.amazonwebservices\.com\.cn)$`)

// credentialSourceHosts are the link-local addresses the SDK reads task
// credentials from (ECS, EKS Pod Identity), over plain http. A request there
// carrying an authorization token (EKS Pod Identity, or ECS with
// AWS_CONTAINER_AUTHORIZATION_TOKEN) is refused by the kit, which holds the
// token and refuses every held value over plain http. The EC2
// instance-metadata addresses (169.254.169.254, fd00:ec2::254) are not here:
// that client is off (r1b-imds-off), and the pin refuses them as well.
var credentialSourceHosts = map[string]bool{
	"169.254.170.2": true, "169.254.170.23": true, "fd00:ec2::23": true,
}

// blackboxTransport is set only in a build with the awsbilling_blackbox tag
// (awsbilling_testendpoint.go), which internal/awsbillingblackbox builds to
// dial one loopback fake for every AWS host. It is nil in every production
// build.
var blackboxTransport func(*http.Transport)

// awsEndpointAllowed reports whether a request to u may be sent. Hosts are
// compared as written (ASCII only): IDNA or Unicode folding could make another
// name compare equal while the transport dials it.
func awsEndpointAllowed(u *url.URL) bool {
	if u == nil || u.User != nil || !isASCII(u.Host) {
		return false
	}
	host, port := strings.ToLower(u.Hostname()), u.Port()
	switch strings.ToLower(u.Scheme) {
	case "https":
		return (port == "" || port == "443") && awsServiceHost.MatchString(host)
	case "http":
		if port != "" && port != "80" && !containerFullURIHost(host, port) {
			return false
		}
		return credentialSourceHosts[host] || containerFullURIHost(host, port)
	}
	return false
}

// containerFullURIHost allows the loopback container-credential endpoint the
// operator's environment names (AWS_CONTAINER_CREDENTIALS_FULL_URI, which the
// SDK accepts over http only for loopback hosts), and nothing else on loopback.
func containerFullURIHost(host, port string) bool {
	raw := os.Getenv("AWS_CONTAINER_CREDENTIALS_FULL_URI")
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "http") || !strings.EqualFold(u.Hostname(), host) || u.Port() != port {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func displayHost(u *url.URL) string {
	if u == nil {
		return "no URL"
	}
	if !isASCII(u.Host) {
		return "a non-ASCII host"
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
}

// holdingProvider holds every credential the wrapped provider returns, a
// refreshed one included, with the credential kit before the signer uses it. Every credential source
// the chain runs (a credential_process included, as the source of an
// assume-role profile too) runs inside Retrieve, so what it prints is withheld
// there (quietCredentialProcesses) and its error text is checked
// (credentialSourceError) before anything can print either.
type holdingProvider struct{ inner aws.CredentialsProvider }

func (p holdingProvider) Retrieve(ctx context.Context) (aws.Credentials, error) {
	restore, err := quietCredentialProcesses()
	if err != nil {
		return aws.Credentials{}, err
	}
	v, err := func() (aws.Credentials, error) {
		defer restore()
		return p.inner.Retrieve(ctx)
	}()
	if err != nil {
		return v, credentialSourceError(err)
	}
	HoldMinted("minted:aws-credentials", v.AccessKeyID, v.SecretAccessKey, v.SessionToken) // Hand-fix (handfixes.json credkit-wiring)
	return v, nil
}

// ErrInstanceMetadataNotUsed marks a credential chain that ended at the EC2
// instance-metadata service, which this CLI never contacts (r1b-imds-off).
var ErrInstanceMetadataNotUsed = errors.New("no AWS credentials found: EC2 instance-metadata (instance role) credentials are not used by this CLI; on EC2 use a profile, SSO or environment keys")

// ErrCredentialProcessOutput replaces a credential_process failure: the SDK
// quotes an answer that does not parse whole in its error (processcreds:
// "parse failed of process output: <output>"), and nothing of it was held.
var ErrCredentialProcessOutput = errors.New("the AWS credential_process failed; its output is not shown because it may carry credentials (run the credential_process command yourself to see why)")

// credentialSourceError keeps a credential source's error unless it may quote
// what a credential_process printed: any process-provider error, whatever it
// says, found in the wrapped chain or, should a layer not wrap it, in the
// text. That one is the fixed ErrCredentialProcessOutput.
func credentialSourceError(err error) error {
	var pe *processcreds.ProviderError
	if errors.As(err, &pe) || strings.Contains(err.Error(), "process provider error") {
		return ErrCredentialProcessOutput
	}
	// The chain's last source is the instance-metadata one, whose client is
	// off: say so instead of the SDK's "access disabled to EC2 IMDS".
	var oe *smithy.OperationError
	if errors.As(err, &oe) && oe.Service() == "ec2imds" {
		return fmt.Errorf("%w (%v)", ErrInstanceMetadataNotUsed, err)
	}
	return err
}

// retrievalStderr serializes the stderr swap of concurrent retrievals.
var retrievalStderr sync.Mutex

// quietCredentialProcesses points os.Stderr at the null device until restore:
// the SDK's credential_process command builder (processcreds
// DefaultNewCommandBuilder) gives the process os.Stderr as it starts it, so a
// credential_process started during a retrieval writes nowhere this program's
// output reaches. A prompt it writes there is not shown (run the command
// yourself, or use one that prompts on its own terminal). Fail closed: when
// the null device cannot be opened, the retrieval does not run.
func quietCredentialProcesses() (func(), error) {
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("resolving AWS credentials: cannot open %s to keep a credential_process's output off this program's stderr: %w", os.DevNull, err)
	}
	retrievalStderr.Lock()
	prev := os.Stderr
	os.Stderr = null
	return func() {
		os.Stderr = prev
		retrievalStderr.Unlock()
		_ = null.Close()
	}, nil
}
