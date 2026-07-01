package awsx

import (
	"errors"
	"testing"

	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
)

func apiErr(code, msg string) error {
	return &smithy.GenericAPIError{Code: code, Message: msg}
}

func TestParseAmount(t *testing.T) {
	str := func(s string) *string { return &s }
	tests := []struct {
		name string
		in   *string
		want float64
	}{
		{"nil", nil, 0},
		{"empty", str(""), 0},
		{"blank", str("   "), 0},
		{"whole", str("42"), 42},
		{"decimal", str("12.34"), 12.34},
		{"padded", str("  9.5 "), 9.5},
		{"garbage", str("not-a-number"), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseAmount(tt.in))
		})
	}
}

func TestIsAccessDenied(t *testing.T) {
	assert.False(t, IsAccessDenied(nil))
	assert.False(t, IsAccessDenied(errors.New("plain error")))

	for _, code := range []string{
		"AccessDenied", "AccessDeniedException", "UnauthorizedException",
		"AuthorizationError", "AuthFailure", "UnauthorizedOperation",
		"NotAuthorized", "DataUnavailableException",
	} {
		assert.Truef(t, IsAccessDenied(apiErr(code, "")), "code %s should be access-denied", code)
	}

	// Generic code but message carries the denial wording.
	assert.True(t, IsAccessDenied(apiErr("ClientError", "You are not authorized to perform ce:GetCostAndUsage")))
	assert.True(t, IsAccessDenied(apiErr("ClientError", "Access Denied")))
	// Unrelated API error stays false.
	assert.False(t, IsAccessDenied(apiErr("ValidationException", "bad date range")))
}

func TestErrorCode(t *testing.T) {
	assert.Equal(t, "", ErrorCode(nil))
	assert.Equal(t, "", ErrorCode(errors.New("plain")))
	assert.Equal(t, "Throttling", ErrorCode(apiErr("Throttling", "slow down")))
}

func TestIsThrottle(t *testing.T) {
	for _, code := range []string{
		"Throttling", "ThrottlingException", "TooManyRequestsException",
		"RequestLimitExceeded", "LimitExceededException",
	} {
		assert.Truef(t, IsThrottle(apiErr(code, "")), "code %s should be throttle", code)
	}
	assert.False(t, IsThrottle(nil))
	assert.False(t, IsThrottle(apiErr("AccessDenied", "")))
}

func TestEBSPerGBMonth(t *testing.T) {
	tests := []struct {
		volType string
		want    float64
	}{
		{"gp2", ebsGP2PerGBMonth},
		{"GP2", ebsGP2PerGBMonth}, // case-insensitive
		{"gp3", ebsGP3PerGBMonth},
		{"io1", ebsIO1PerGBMonth},
		{"io2", ebsIO1PerGBMonth},
		{"st1", ebsST1PerGBMonth},
		{"sc1", ebsSC1PerGBMonth},
		{"standard", ebsStdPerGBMonth},
		{"unknown", ebsStdPerGBMonth}, // fallback
	}
	for _, tt := range tests {
		t.Run(tt.volType, func(t *testing.T) {
			assert.Equal(t, tt.want, ebsPerGBMonth(tt.volType))
		})
	}
}

func TestEC2OnDemandMonthly(t *testing.T) {
	price, ok := ec2OnDemandMonthly("t3.micro")
	assert.True(t, ok)
	assert.InDelta(t, 0.0104*hoursPerMonth, price, 0.0001)

	// Case-insensitive lookup.
	price2, ok2 := ec2OnDemandMonthly("M5.LARGE")
	assert.True(t, ok2)
	assert.InDelta(t, 0.096*hoursPerMonth, price2, 0.0001)

	// Unknown type returns (0, false) so the caller avoids a fabricated figure.
	price3, ok3 := ec2OnDemandMonthly("zz.gigantic")
	assert.False(t, ok3)
	assert.Equal(t, float64(0), price3)
}

func TestAssignGroupKeys(t *testing.T) {
	var line CostLine
	assignGroupKeys(&line, []string{"LINKED_ACCOUNT", "SERVICE"}, []string{"123456789012", "Amazon EC2"})
	assert.Equal(t, "123456789012", line.AccountID)
	assert.Equal(t, "Amazon EC2", line.Service)

	var line2 CostLine
	assignGroupKeys(&line2, []string{"SERVICE", "USAGE_TYPE", "REGION"}, []string{"S3", "DataTransfer-Out", "us-east-1"})
	assert.Equal(t, "S3", line2.Service)
	assert.Equal(t, "DataTransfer-Out", line2.UsageType)
	assert.Equal(t, "us-east-1", line2.Region)

	// More keys than values: the extra key is skipped, no panic.
	var line3 CostLine
	assignGroupKeys(&line3, []string{"LINKED_ACCOUNT", "SERVICE"}, []string{"999999999999"})
	assert.Equal(t, "999999999999", line3.AccountID)
	assert.Equal(t, "", line3.Service)
}

func TestTagsToJSON(t *testing.T) {
	assert.Equal(t, "{}", tagsToJSON(nil))
	assert.Equal(t, "{}", tagsToJSON(map[string]string{}))
	assert.JSONEq(t, `{"env":"prod","team":"billing"}`, tagsToJSON(map[string]string{"env": "prod", "team": "billing"}))
}

func TestEC2TagMap(t *testing.T) {
	k1, v1 := "Name", "web-1"
	k2, v2 := "env", "prod"
	tags := []ec2types.Tag{
		{Key: &k1, Value: &v1},
		{Key: &k2, Value: &v2},
	}
	m := ec2TagMap(tags)
	assert.Equal(t, map[string]string{"Name": "web-1", "env": "prod"}, m)
	assert.Equal(t, map[string]string{}, ec2TagMap(nil))
}

func TestMergeAttrs(t *testing.T) {
	// nil base is allocated.
	got := mergeAttrs(nil, map[string]string{"a": "1"})
	assert.Equal(t, map[string]string{"a": "1"}, got)

	// extra overrides base on key conflict.
	got2 := mergeAttrs(map[string]string{"a": "1", "b": "2"}, map[string]string{"b": "override", "c": "3"})
	assert.Equal(t, map[string]string{"a": "1", "b": "override", "c": "3"}, got2)
}

func TestRound2(t *testing.T) {
	assert.Equal(t, 1.23, round2(1.234))
	assert.Equal(t, 1.24, round2(1.235)) // rounds half up
	assert.Equal(t, 0.0, round2(0))
	assert.Equal(t, 100.0, round2(99.999))
}
