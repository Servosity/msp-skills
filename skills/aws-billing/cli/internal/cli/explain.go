// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// `explain` — decode a confusing AWS usage-type or service line item into plain English.
// Curated static glossary; no API call.
//
// pp:novel-static-reference
package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// glossaryEntry maps a case-insensitive substring to a plain-English decode.
type glossaryEntry struct {
	match string
	desc  string
}

// awsGlossary decodes the most common confusing AWS line items. Ordered most
// specific first.
var awsGlossary = []glossaryEntry{
	{"NatGateway-Bytes", "Data processed by a NAT Gateway ($0.045/GB on top of the hourly charge). Often the single most surprising line on a bill — every byte your private subnet sends to the internet goes through it."},
	{"NatGateway-Hours", "Hourly charge for running a NAT Gateway (~$0.045/hr ≈ $32/mo each), separate from data processing. Idle NAT gateways still bill."},
	{"DataTransfer-Out-Bytes", "Data leaving AWS to the public internet. The first GB/month is free, then ~$0.09/GB. Egress is where 'free to put data in, costly to get it out' bites."},
	{"DataTransfer-In-Bytes", "Data coming into AWS from the internet. Almost always free — if this is a big number, double-check what's being measured."},
	{"DataTransfer-Regional-Bytes", "Traffic between Availability Zones in the same region (~$0.01/GB each way). Chatty cross-AZ services quietly rack this up."},
	{"AWS-Out-Bytes", "Data transferred out to another AWS region. Cross-region replication and multi-region setups drive this."},
	{"DataTransfer", "Generic data-transfer charge. Check the prefix (Out/In/Regional) to see which direction and price tier."},
	{"BoxUsage", "EC2 on-demand instance running time, billed per second. The suffix is the instance type (e.g. BoxUsage:t3.medium)."},
	{"SpotUsage", "EC2 Spot instance running time — discounted but interruptible capacity."},
	{"EBS:VolumeUsage", "EBS volume storage, billed per GB-month. The suffix is the volume type (gp2/gp3/io1/st1)."},
	{"EBS:VolumeIOUsage", "Provisioned IOPS charge on io1/io2 volumes — separate from the storage charge."},
	{"EBS:SnapshotUsage", "EBS snapshot storage, billed per GB-month of changed blocks. Old snapshots accumulate silently."},
	{"TimedStorage", "S3 storage, billed per GB-month. The suffix indicates the storage class (Standard, IA, Glacier)."},
	{"Requests-Tier1", "S3 PUT/COPY/POST/LIST requests, billed per 1,000."},
	{"Requests-Tier2", "S3 GET and other read requests, billed per 10,000."},
	{"LoadBalancerUsage", "Hourly charge for an Elastic Load Balancer."},
	{"LCUUsage", "Load Balancer Capacity Units — usage-based ALB/NLB charge on top of the hourly fee."},
	{"ElasticIP:IdleAddress", "An Elastic IP that isn't attached to a running instance, billed ~$0.005/hr (~$3.65/mo)."},
	{"PublicIPv4", "Charge for allocated public IPv4 addresses (AWS now bills all public IPv4, ~$0.005/hr each)."},
	{"InstanceUsage", "RDS / managed-service instance running time."},
	{"CW:Requests", "CloudWatch API requests / custom metrics charge."},
	{"Lambda-GB-Second", "AWS Lambda compute, billed per GB-second of memory × duration."},
}

// serviceGlossary decodes the more cryptic Cost Explorer service names.
var serviceGlossary = map[string]string{
	"Amazon Elastic Compute Cloud - Compute": "EC2 virtual machines (the compute portion — storage and data transfer bill separately).",
	"EC2 - Other":                            "EC2-adjacent charges that aren't instance hours: EBS volumes, snapshots, Elastic IPs, NAT gateways, data transfer. Often a mystery bucket worth drilling into.",
	"Amazon Simple Storage Service":          "S3 object storage — storage + requests + data transfer.",
	"Amazon Relational Database Service":     "RDS managed databases — instance hours + storage + I/O + backups.",
	"AmazonCloudWatch":                       "Metrics, logs, alarms, and dashboards. Log ingestion/retention is the usual cost driver.",
	"AWS Cost Explorer":                      "The Cost Explorer API itself — yes, asking what you spent costs $0.01 per request. (This CLI caches to avoid that.)",
}

func newExplainCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "explain <usage-type-or-service>",
		Short: "Decode a confusing AWS usage type or service into plain English",
		Long: `Decode an opaque AWS line item (usage type like EUC1-DataTransfer-Out-Bytes,
or a service name) into plain English, including why it's billed and what
usually drives it.`,
		Example: `  aws-billing-cli explain EUC1-DataTransfer-Out-Bytes
  aws-billing-cli explain NatGateway-Bytes
  aws-billing-cli explain "EC2 - Other"`,
		// explain decodes any usage-type/service string (unknown -> generic
		// decode, exit 0), so there is no invalid-argument error path to probe.
		Annotations: map[string]string{"mcp:read-only": "true", "pp:no-error-path-probe": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			term := strings.Join(args, " ")
			desc, matched := lookupGlossary(term)
			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{"term": term, "matched": matched, "explanation": desc})
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%s\n\n%s\n", term, desc)
			return nil
		},
	}
	return cmd
}

func lookupGlossary(term string) (string, bool) {
	// Service exact (case-insensitive) match first.
	for svc, d := range serviceGlossary {
		if strings.EqualFold(svc, term) {
			return d, true
		}
	}
	// Usage-type substring match (handles region prefixes like EUC1-, USE1-).
	for _, e := range awsGlossary {
		if strings.Contains(strings.ToLower(term), strings.ToLower(e.match)) {
			return e.desc, true
		}
	}
	// Generic decode of the AWS usage-type naming convention.
	return genericUsageTypeDecode(term), false
}

func genericUsageTypeDecode(term string) string {
	parts := strings.SplitN(term, "-", 2)
	var b strings.Builder
	b.WriteString("No exact glossary entry. AWS usage types follow <REGION-CODE>-<Resource><Action>-<Unit>.\n")
	if len(parts) == 2 && len(parts[0]) <= 5 {
		b.WriteString(fmt.Sprintf("  • %q looks like a region code (e.g. USE1=us-east-1, EUC1=eu-central-1, APN1=ap-northeast-1).\n", parts[0]))
		b.WriteString(fmt.Sprintf("  • The rest, %q, names the resource and unit being billed.\n", parts[1]))
	}
	b.WriteString("\nTry 'aws-billing-cli explain' with a known fragment like DataTransfer, NatGateway, BoxUsage, EBS:VolumeUsage, or TimedStorage.")
	return b.String()
}
