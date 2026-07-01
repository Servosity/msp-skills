// Static AWS price reference for waste estimation. Approximate us-east-1
// on-demand list prices; used only to rank/estimate waste, never billed.
// Refresh against the AWS pricing pages periodically.
//
// pp:novel-static-reference
package awsx

import "strings"

const (
	// EBS storage $/GB-month, us-east-1.
	ebsGP2PerGBMonth = 0.10
	ebsGP3PerGBMonth = 0.08
	ebsIO1PerGBMonth = 0.125
	ebsST1PerGBMonth = 0.045
	ebsSC1PerGBMonth = 0.015
	ebsStdPerGBMonth = 0.05 // magnetic / default fallback

	// EBS snapshot $/GB-month.
	snapshotPerGBMonth = 0.05

	// Unassociated Elastic IP / idle public IPv4 $/hour -> month.
	eipPerHour    = 0.005
	hoursPerMonth = 730.0
	eipPerMonth   = eipPerHour * hoursPerMonth // ~3.65

	// S3 Standard storage $/GB-month (first tier), us-east-1.
	s3StandardPerGBMonth = 0.023
)

// ebsPerGBMonth returns the $/GB-month for an EBS volume type string.
func ebsPerGBMonth(volType string) float64 {
	switch strings.ToLower(volType) {
	case "gp2":
		return ebsGP2PerGBMonth
	case "gp3":
		return ebsGP3PerGBMonth
	case "io1", "io2":
		return ebsIO1PerGBMonth
	case "st1":
		return ebsST1PerGBMonth
	case "sc1":
		return ebsSC1PerGBMonth
	default:
		return ebsStdPerGBMonth
	}
}

// ec2OnDemandMonthly returns an approximate $/month for common instance types.
// Returns (price, true) when known; (0, false) for unknown types so the caller
// can flag the resource without a fabricated dollar figure.
func ec2OnDemandMonthly(instanceType string) (float64, bool) {
	// $/hour for the most common general-purpose / compute families.
	perHour := map[string]float64{
		"t2.nano": 0.0058, "t2.micro": 0.0116, "t2.small": 0.023, "t2.medium": 0.0464, "t2.large": 0.0928, "t2.xlarge": 0.1856,
		"t3.nano": 0.0052, "t3.micro": 0.0104, "t3.small": 0.0208, "t3.medium": 0.0416, "t3.large": 0.0832, "t3.xlarge": 0.1664, "t3.2xlarge": 0.3328,
		"t3a.micro": 0.0094, "t3a.small": 0.0188, "t3a.medium": 0.0376, "t3a.large": 0.0752,
		"t4g.micro": 0.0084, "t4g.small": 0.0168, "t4g.medium": 0.0336, "t4g.large": 0.0672,
		"m5.large": 0.096, "m5.xlarge": 0.192, "m5.2xlarge": 0.384, "m5.4xlarge": 0.768,
		"m6i.large": 0.096, "m6i.xlarge": 0.192, "m6i.2xlarge": 0.384,
		"c5.large": 0.085, "c5.xlarge": 0.17, "c5.2xlarge": 0.34, "c5.4xlarge": 0.68,
		"c6i.large": 0.085, "c6i.xlarge": 0.17,
		"r5.large": 0.126, "r5.xlarge": 0.252, "r5.2xlarge": 0.504,
	}
	if h, ok := perHour[strings.ToLower(instanceType)]; ok {
		return h * hoursPerMonth, true
	}
	return 0, false
}
