// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// AWS-aware doctor checks: resolve the credential chain, detect member-vs-
// management account, and probe Cost Explorer / Organizations / EC2 access,
// mapping any AccessDenied to the exact missing permission + iam-setup tier.
//
// pp:client-call
package cli

import (
	"context"
	"fmt"

	"aws-billing-pp-cli/internal/awsx"
	"aws-billing-pp-cli/internal/cliutil"
)

// awsDoctorReport owns the api and credentials rows for this connector: the
// AWS SDK resolves the credential chain and calls AWS directly, so base_url is
// never probed. credentials is positive only when STS answered for the
// resolved credentials.
func awsDoctorReport(ctx context.Context, profile, region string, report map[string]any) {
	if cliutil.IsVerifyEnv() {
		report["aws_credentials"] = "INFO skipped under verify mode (no AWS calls)"
		report["api"] = "INFO not probed under verify mode (no AWS calls)"
		report["credentials"] = "INFO not verified under verify mode (no AWS calls)"
		return
	}

	client, err := awsx.New(ctx, profile, region)
	if err != nil {
		report["aws_credentials"] = fmt.Sprintf("ERROR loading AWS config: %s", err)
		report["api"] = "ERROR not probed: the AWS configuration did not load"
		report["credentials"] = "ERROR not verified: the AWS configuration did not load"
		return
	}

	account, arn, err := client.CallerIdentity(ctx)
	if err != nil {
		report["aws_credentials"] = "ERROR no AWS credentials resolved — run 'aws sso login --profile <p>' or set AWS_PROFILE; pass --profile-aws to pick one"
		report["api"] = fmt.Sprintf("ERROR AWS STS GetCallerIdentity failed: %s", err)
		report["credentials"] = "ERROR not verified: sts:GetCallerIdentity failed (no AWS credential resolved, or AWS refused it)"
		return
	}
	report["aws_credentials"] = "valid"
	report["api"] = "ok (AWS STS answered for the resolved credentials)"
	report["credentials"] = "valid (sts:GetCallerIdentity succeeded)"
	report["aws_account"] = fmt.Sprintf("%s (region %s) %s", account, client.Region, arn)

	// Management vs member account.
	mgmt, mErr := client.ManagementAccountID(ctx)
	isManagement := mErr == nil && mgmt != "" && mgmt == account
	switch {
	case mErr != nil && awsx.IsAccessDenied(mErr):
		report["aws_org_access"] = "scope-limited: organizations:ListAccounts/DescribeOrganization denied — likely a member account; account names won't resolve (grant via iam-setup --tier core in the management account)"
	case isManagement:
		report["aws_org_access"] = fmt.Sprintf("ok — this IS the management (payer) account %s; org-wide cost data available", mgmt)
	case mErr == nil && mgmt != "":
		report["aws_org_access"] = fmt.Sprintf("ok, but this is a MEMBER account; the payer is %s — org-wide cost needs a management-account profile", mgmt)
	default:
		report["aws_org_access"] = "unknown (DescribeOrganization unavailable; standalone account or no org)"
	}

	// Cost Explorer access (cheap single-month probe).
	start, end := resolveProbeMonth()
	if _, cerr := client.GetCostAndUsageGrouped(ctx, start, end, "MONTHLY", []string{"SERVICE"}); cerr != nil {
		switch {
		case awsx.IsAccessDenied(cerr):
			if isManagement {
				report["aws_cost_access"] = "ERROR denied — grant ce:GetCostAndUsage (run 'iam-setup --tier core')"
			} else {
				report["aws_cost_access"] = "scope-limited: Cost Explorer denied from this member account — use a management-account profile, or grant ce:GetCostAndUsage (iam-setup --tier core)"
			}
		default:
			report["aws_cost_access"] = fmt.Sprintf("WARN reachable but errored: %s", cerr)
		}
	} else {
		report["aws_cost_access"] = "ok"
	}

	// Inventory access (cheap probe).
	if _, ierr := client.CollectInventory(ctx, awsx.InventoryOptions{SkipCPU: true}); ierr != nil {
		if awsx.IsAccessDenied(ierr) {
			report["aws_inventory_access"] = "scope-limited: some ec2/cloudwatch/s3 describes denied — grant the waste tier (iam-setup --tier waste) for full waste detection"
		} else {
			report["aws_inventory_access"] = fmt.Sprintf("WARN errored: %s", ierr)
		}
	} else {
		report["aws_inventory_access"] = "ok"
	}
}

// resolveProbeMonth returns the current month range for a cheap doctor probe.
func resolveProbeMonth() (string, string) {
	start, end, _ := resolveSyncRange("", "", 1)
	return start, end
}
