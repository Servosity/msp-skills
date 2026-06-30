// pp:client-call
package awsx

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
)

// ListAccounts returns all member accounts in the organization. Returns an
// access-denied error (check with IsAccessDenied) when called from a member
// account that lacks organizations:ListAccounts — the caller degrades to
// account-ID-only display in that case.
func (c *Client) ListAccounts(ctx context.Context) ([]Account, error) {
	var accounts []Account
	p := organizations.NewListAccountsPaginator(c.Org, &organizations.ListAccountsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return accounts, err
		}
		for _, a := range page.Accounts {
			accounts = append(accounts, Account{
				AccountID: aws.ToString(a.Id),
				Name:      aws.ToString(a.Name),
				Status:    string(a.Status),
				Email:     aws.ToString(a.Email),
			})
		}
	}
	return accounts, nil
}

// ManagementAccountID returns the organization's management (payer) account
// ID, or "" on error. Used by doctor to tell the user whether their current
// credentials are in the payer account (where org-wide Cost Explorer lives).
func (c *Client) ManagementAccountID(ctx context.Context) (string, error) {
	out, err := c.Org.DescribeOrganization(ctx, &organizations.DescribeOrganizationInput{})
	if err != nil {
		return "", err
	}
	if out.Organization == nil {
		return "", nil
	}
	return aws.ToString(out.Organization.MasterAccountId), nil
}
