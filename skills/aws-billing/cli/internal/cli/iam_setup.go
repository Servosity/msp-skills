// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// `iam-setup` — the low-friction onboarding feature. Emits a tiered
// least-privilege IAM policy, a one-click CloudFormation template, or an admin
// bootstrap script granting exactly the read-only permissions this CLI uses.
// Curated static reference; emits text, makes no API call.
//
// pp:novel-static-reference
package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// tierActions maps a tier name to the IAM actions it grants. All read-only.
var tierActions = map[string][]string{
	"core": {
		"ce:GetCostAndUsage",
		"ce:GetCostAndUsageWithResources",
		"ce:GetCostForecast",
		"ce:GetDimensionValues",
		"ce:GetTags",
		"ce:GetCostCategories",
		"ce:GetAnomalies",
		"organizations:ListAccounts",
		"organizations:DescribeOrganization",
	},
	"waste": {
		"ec2:DescribeInstances",
		"ec2:DescribeVolumes",
		"ec2:DescribeSnapshots",
		"ec2:DescribeAddresses",
		"cloudwatch:GetMetricStatistics",
		"s3:ListAllMyBuckets",
		"s3:GetBucketLocation",
	},
	"budgets": {
		"budgets:ViewBudget",
		"budgets:DescribeBudgetActionsForBudget",
	},
}

// tierOrder is the canonical emission order.
var tierOrder = []string{"core", "waste", "budgets"}

func actionsForTier(tier string) ([]string, error) {
	switch tier {
	case "", "core":
		return tierActions["core"], nil
	case "waste":
		// waste is additive on top of core for a usable scan.
		return append(append([]string{}, tierActions["core"]...), tierActions["waste"]...), nil
	case "budgets":
		return append(append([]string{}, tierActions["core"]...), tierActions["budgets"]...), nil
	case "all":
		var all []string
		for _, t := range tierOrder {
			all = append(all, tierActions[t]...)
		}
		return all, nil
	}
	return nil, fmt.Errorf("unknown --tier %q (core, waste, budgets, all)", tier)
}

func newNovelIamSetupCmd(flags *rootFlags) *cobra.Command {
	var tier, format string

	cmd := &cobra.Command{
		Use:   "iam-setup",
		Short: "Emit a least-privilege IAM policy, CloudFormation template, or bootstrap script for billing read access",
		Long: `The lowest-friction way to give this CLI (and a colleague you share it with)
the read-only AWS permissions it needs — nothing more.

Tiers (additive):
  core      Cost Explorer + Organizations: the bill, breakdowns, compare, forecast
  waste     adds EC2/CloudWatch/S3 describes for the waste hunters
  budgets   adds Budgets read (optional)
  all       every tier

Formats:
  policy          the IAM policy JSON to paste into a role/user (default)
  cloudformation  a one-click CloudFormation template that mints a BillingReadOnly role
  bootstrap       a bash script an admin runs once to create the policy + user

Org-wide cost data lives in the management (payer) account, so attach the policy
there. The waste tier works in any account.`,
		Example: `  # Copy-paste IAM policy for the core (billing) tier
  aws-billing-cli iam-setup --tier core

  # One-click CloudFormation template for billing + waste
  aws-billing-cli iam-setup --tier waste --format cloudformation > billing-readonly.yaml

  # Admin bootstrap script
  aws-billing-cli iam-setup --tier all --format bootstrap`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			actions, err := actionsForTier(strings.ToLower(strings.TrimSpace(tier)))
			if err != nil {
				return usageErr(err)
			}
			policy := iamPolicyJSON(actions)

			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{
					"tier":           orDefault(tier, "core"),
					"format":         orDefault(format, "policy"),
					"actions":        actions,
					"policy":         json.RawMessage(policy),
					"cloudformation": cloudFormationTemplate(actions),
				})
			}
			w := cmd.OutOrStdout()
			switch strings.ToLower(strings.TrimSpace(format)) {
			case "", "policy":
				fmt.Fprintln(w, "# Least-privilege IAM policy — attach to a role or user in the MANAGEMENT (payer) account")
				fmt.Fprintln(w, "# (the waste tier also works in any member account).")
				fmt.Fprintln(w, policy)
			case "cloudformation", "cfn":
				fmt.Fprintln(w, cloudFormationTemplate(actions))
				fmt.Fprintln(w, "\n# Deploy:  aws cloudformation deploy --template-file <this>.yaml --stack-name aws-billing-readonly --capabilities CAPABILITY_NAMED_IAM")
			case "bootstrap", "script":
				fmt.Fprintln(w, bootstrapScript(actions))
			default:
				return usageErr(fmt.Errorf("unknown --format %q (policy, cloudformation, bootstrap)", format))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&tier, "tier", "core", "Permission tier: core, waste, budgets, all")
	cmd.Flags().StringVar(&format, "format", "policy", "Output format: policy, cloudformation, bootstrap")
	return cmd
}

func orDefault(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}

func iamPolicyJSON(actions []string) string {
	sorted := append([]string{}, actions...)
	sort.Strings(sorted)
	doc := map[string]any{
		"Version": "2012-10-17",
		"Statement": []map[string]any{
			{
				"Sid":      "AWSBillingIntelligenceReadOnly",
				"Effect":   "Allow",
				"Action":   sorted,
				"Resource": "*",
			},
		},
	}
	b, _ := json.MarshalIndent(doc, "", "  ")
	return string(b)
}

func cloudFormationTemplate(actions []string) string {
	sorted := append([]string{}, actions...)
	sort.Strings(sorted)
	var actionLines strings.Builder
	for _, a := range sorted {
		actionLines.WriteString("                  - " + a + "\n")
	}
	return fmt.Sprintf(`AWSTemplateFormatVersion: '2010-09-09'
Description: Read-only IAM role for AWS Billing Intelligence (aws-billing-cli). Least privilege.
Parameters:
  TrustedPrincipalArn:
    Type: String
    Description: ARN allowed to assume this role (e.g. arn:aws:iam::<account-id>:root or a specific user/role).
Resources:
  BillingReadOnlyRole:
    Type: AWS::IAM::Role
    Properties:
      RoleName: BillingIntelligenceReadOnly
      AssumeRolePolicyDocument:
        Version: '2012-10-17'
        Statement:
          - Effect: Allow
            Principal:
              AWS: !Ref TrustedPrincipalArn
            Action: sts:AssumeRole
      Policies:
        - PolicyName: BillingIntelligenceReadOnly
          PolicyDocument:
            Version: '2012-10-17'
            Statement:
              - Effect: Allow
                Action:
%s                Resource: '*'
Outputs:
  RoleArn:
    Description: Assume this role with the CLI's --profile-aws (configure a profile with role_arn=<this>).
    Value: !GetAtt BillingReadOnlyRole.Arn
`, actionLines.String())
}

func bootstrapScript(actions []string) string {
	sorted := append([]string{}, actions...)
	sort.Strings(sorted)
	policy := iamPolicyJSON(sorted)
	return fmt.Sprintf(`#!/usr/bin/env bash
# Run ONCE with AWS admin credentials in the MANAGEMENT (payer) account.
# Creates a least-privilege IAM user + policy for AWS Billing Intelligence.
# Review before running. Read-only permissions only.
set -euo pipefail

USER_NAME="billing-intelligence-readonly"
POLICY_NAME="BillingIntelligenceReadOnly"

cat > /tmp/billing-readonly-policy.json <<'POLICY'
%s
POLICY

ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
POLICY_ARN="arn:aws:iam::${ACCOUNT_ID}:policy/${POLICY_NAME}"

aws iam create-policy --policy-name "$POLICY_NAME" \
  --policy-document file:///tmp/billing-readonly-policy.json 2>/dev/null || echo "policy exists, continuing"
aws iam create-user --user-name "$USER_NAME" 2>/dev/null || echo "user exists, continuing"
aws iam attach-user-policy --user-name "$USER_NAME" --policy-arn "$POLICY_ARN"

echo "Creating access key (store it in 'aws configure --profile billing'):"
aws iam create-access-key --user-name "$USER_NAME" --output table

echo "Done. Configure a profile, then: aws-billing-cli doctor --profile-aws billing"
`, policy)
}
