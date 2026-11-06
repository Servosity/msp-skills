// pp:client-call
package awsx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/endpointcreds"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"

	"aws-billing-pp-cli/internal/cliutil"
)

// ceRegion is the only region Cost Explorer is served from. The CE client is
// pinned here regardless of the user's configured region.
const ceRegion = "us-east-1"

// Client bundles the AWS service clients the CLI uses, all sharing one
// resolved credential chain. CE is rate-paced through an AdaptiveLimiter
// because it bills $0.01 per request.
type Client struct {
	Profile string
	Region  string

	cfg aws.Config
	CE  *costexplorer.Client
	Org *organizations.Client
	EC2 *ec2.Client
	CW  *cloudwatch.Client
	S3  *s3.Client
	STS *sts.Client

	ceLimiter *cliutil.AdaptiveLimiter
}

// New resolves the AWS credential chain (env / shared profile / SSO /
// assume-role / credential_process / web identity / container; EC2 instance
// metadata is never used) and constructs the service clients. profile and
// region are optional; empty means "use the default resolution".
func New(ctx context.Context, profile, region string) (*Client, error) {
	cfg, err := loadConfig(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	// aws-billing handfix (minted-credentials-held): every resolved or refreshed
	// credential is held before it signs a request.
	if cfg.Credentials != nil {
		cfg.Credentials = holdingProvider{inner: cfg.Credentials}
	}
	c := &Client{
		Profile: profile,
		Region:  cfg.Region,
		cfg:     cfg,
		CE: costexplorer.NewFromConfig(cfg, func(o *costexplorer.Options) {
			o.Region = ceRegion // CE is global, served from us-east-1
		}),
		Org:       organizations.NewFromConfig(cfg),
		EC2:       ec2.NewFromConfig(cfg),
		CW:        cloudwatch.NewFromConfig(cfg),
		S3:        s3.NewFromConfig(cfg, func(o *s3.Options) { o.UsePathStyle = true }), // aws-billing handfix (s3-path-style)
		STS:       sts.NewFromConfig(cfg),
		ceLimiter: cliutil.NewAdaptiveLimiter(5),
	}
	return c, nil
}

// ResolveRegion is the region a client New(ctx, profile, region) builds sends
// its regional requests to (EC2, CloudWatch, S3: an inventory scan scans this
// one region): region when given, else the SDK's own resolution
// (AWS_REGION, AWS_DEFAULT_REGION, the named or default profile's region in
// the shared config), else us-east-1. aws-billing handfix
// (r1b-inventory-region-resolved): New and every stored inventory read
// resolve the region through the same loadConfig, so a stored scan answers
// only a read a live scan of that region would answer. It reads the
// environment and the shared config files only: nothing is sent and no
// credential is retrieved.
func ResolveRegion(ctx context.Context, profile, region string) (string, error) {
	cfg, err := loadConfig(ctx, profile, region)
	if err != nil {
		return "", err
	}
	return cfg.Region, nil
}

// loadConfig loads the AWS config every client of this CLI uses: the pinned
// HTTP client, instance metadata off, the profile and region asked for, and
// us-east-1 when nothing names a region.
func loadConfig(ctx context.Context, profile, region string) (aws.Config, error) {
	dropUnresolvedAWSPlaceholders()
	// aws-billing handfix (aws-endpoint-pinned): one pinned HTTP client, behind
	// the credential kit's outbound guard, for every service call and credential
	// provider (awsbilling_guard.go). The container credential provider (ECS,
	// EKS Pod Identity, AWS_CONTAINER_CREDENTIALS_FULL_URI) is the one the SDK
	// builds without the configured client (config's resolveHTTPCredProvider
	// sets no HTTPClient, so endpointcreds falls back to a client of its own):
	// it gets the pinned client by name (Codex r1 class
	// credential-sent-to-another-host).
	// aws-billing handfix (r1b-imds-off; Codex r1b class
	// credential-sent-over-plain-http): the SDK's EC2 instance-metadata client
	// is off for this config and every client and credential provider built
	// from it (the instance-role provider, defaults_mode auto): it mints an
	// IMDSv2 token and sends it, and reads the role's credentials back, over
	// plain http. This option is the config's first source, so it wins over
	// AWS_EC2_METADATA_DISABLED and the shared config.
	opts := []func(*config.LoadOptions) error{config.WithHTTPClient(pinnedHTTPClient()),
		config.WithEndpointCredentialOptions(func(o *endpointcreds.Options) { o.HTTPClient = pinnedHTTPClient() }),
		config.WithEC2IMDSClientEnableState(imds.ClientDisabled)}
	if profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(profile))
	}
	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		if strings.Contains(err.Error(), "unable to add custom RootCAs") {
			// The SDK adds a custom CA bundle only to its own HTTP client; this
			// CLI's client is the credential kit's guard (credkit-wiring).
			return aws.Config{}, errors.New("loading AWS config: a custom CA bundle (AWS_CA_BUNDLE or ca_bundle in the AWS config) is not supported by this CLI; unset it")
		}
		return aws.Config{}, fmt.Errorf("loading AWS config: %w", err)
	}
	if cfg.Region == "" {
		cfg.Region = ceRegion
	}
	return cfg, nil
}

// dropUnresolvedAWSPlaceholders unsets every AWS_* variable that still holds an
// unresolved .mcpb `${user_config.*}` placeholder (aws-billing handfix
// mcpb-placeholder-not-a-credential): a host that skips an optional field
// (the session token, for long-term keys) leaves the template text, and the
// SDK would sign with it as a session token and fail every call.
func dropUnresolvedAWSPlaceholders() {
	for _, kv := range os.Environ() {
		name, v, ok := strings.Cut(kv, "=")
		if ok && strings.HasPrefix(name, "AWS_") && cliutil.UnresolvedUserConfigPlaceholder(v) {
			_ = os.Unsetenv(name)
		}
	}
}

// CallerIdentity returns the account ID and ARN of the resolved credentials.
// Used by doctor and account-context detection.
func (c *Client) CallerIdentity(ctx context.Context) (account, arn string, err error) {
	out, err := c.STS.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", "", err
	}
	return aws.ToString(out.Account), aws.ToString(out.Arn), nil
}

// IsAccessDenied reports whether err is an AWS access-denied / authorization
// failure (so callers can map it to a precise missing-permission hint).
func IsAccessDenied(err error) bool {
	if err == nil {
		return false
	}
	var ae smithy.APIError
	if errors.As(err, &ae) {
		code := ae.ErrorCode()
		switch code {
		case "AccessDenied", "AccessDeniedException", "UnauthorizedException",
			"AuthorizationError", "AuthFailure", "UnauthorizedOperation",
			"NotAuthorized", "DataUnavailableException":
			return true
		}
		// Some EC2/CE errors carry the word in the message but a generic code.
		if strings.Contains(strings.ToLower(ae.ErrorMessage()), "not authorized") ||
			strings.Contains(strings.ToLower(ae.ErrorMessage()), "access denied") {
			return true
		}
	}
	return false
}

// ErrorCode returns the AWS API error code, or "" if err is not an APIError.
func ErrorCode(err error) string {
	var ae smithy.APIError
	if errors.As(err, &ae) {
		return ae.ErrorCode()
	}
	return ""
}

// IsThrottle reports whether err is an AWS throttling / rate-limit error.
func IsThrottle(err error) bool {
	code := ErrorCode(err)
	switch code {
	case "Throttling", "ThrottlingException", "TooManyRequestsException",
		"RequestLimitExceeded", "LimitExceededException":
		return true
	}
	return false
}

// parseAmount converts a Cost Explorer amount string to float64. Empty/blank
// strings (no spend) parse to 0 without error.
func parseAmount(s *string) float64 {
	if s == nil {
		return 0
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return f
}
