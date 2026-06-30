// pp:client-call
package awsx

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
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
// assume-role / IMDS) and constructs the service clients. profile and region
// are optional; empty means "use the default resolution".
func New(ctx context.Context, profile, region string) (*Client, error) {
	opts := []func(*config.LoadOptions) error{}
	if profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(profile))
	}
	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}
	resolvedRegion := cfg.Region
	if resolvedRegion == "" {
		resolvedRegion = ceRegion
		cfg.Region = ceRegion
	}
	c := &Client{
		Profile: profile,
		Region:  resolvedRegion,
		cfg:     cfg,
		CE: costexplorer.NewFromConfig(cfg, func(o *costexplorer.Options) {
			o.Region = ceRegion // CE is global, served from us-east-1
		}),
		Org:       organizations.NewFromConfig(cfg),
		EC2:       ec2.NewFromConfig(cfg),
		CW:        cloudwatch.NewFromConfig(cfg),
		S3:        s3.NewFromConfig(cfg),
		STS:       sts.NewFromConfig(cfg),
		ceLimiter: cliutil.NewAdaptiveLimiter(5),
	}
	return c, nil
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
