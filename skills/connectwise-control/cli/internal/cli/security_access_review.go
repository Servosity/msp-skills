// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type accessReviewUser struct {
	UserName       string   `json:"user_name"`
	DisplayName    string   `json:"display_name,omitempty"`
	Email          string   `json:"email,omitempty"`
	SecurityGroups []string `json:"security_groups"`
	Findings       []string `json:"findings"`
	Unknown        []string `json:"unknown,omitempty"`
}

type accessReviewReport struct {
	TotalUsers       int                `json:"total_users"`
	FlaggedUsers     int                `json:"flagged_users"`
	Disabled         int                `json:"disabled"`
	PasswordExpired  int                `json:"password_expired"`
	NoSecurityGroup  int                `json:"no_security_group"`
	UnknownFieldUser int                `json:"users_with_unknown_fields"`
	Users            []accessReviewUser `json:"users"`
}

// newNovelSecurityAccessReviewCmd flags instance users an access review
// should look at: disabled, password expired, or in no security group.
// pp:data-source live
func newNovelSecurityAccessReviewCmd(flags *rootFlags) *cobra.Command {
	var flaggedOnly bool

	cmd := &cobra.Command{
		Use:   "access-review",
		Short: "Flag instance users that are disabled, have an expired password, or belong to no security group",
		Long: `Read the instance security configuration and list every user with their
security groups and findings: disabled, password_expired, no_security_group.
A field the instance did not report is listed under unknown, never read as false.`,
		Example: strings.Trim(`
  connectwise-control-cli security access-review
  connectwise-control-cli security access-review --flagged-only --agent`, "\n"),
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return usageErr(err)
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "security access-review")
			}
			data, err := cwcPost(cmd, flags, "/Services/SecurityService.ashx/GetSecurityConfigurationInfo", []any{})
			if err != nil {
				return err
			}
			users, err := cwcSecurityUsers(data)
			if err != nil {
				return apiErr(err)
			}
			rep := buildAccessReview(users, flaggedOnly)
			rows := make([]map[string]any, 0, len(rep.Users))
			for _, u := range rep.Users {
				rows = append(rows, map[string]any{"user": u.UserName, "groups": strings.Join(u.SecurityGroups, ", "), "findings": strings.Join(u.Findings, ", "), "unknown": strings.Join(u.Unknown, ", ")})
			}
			summary := fmt.Sprintf("%d of %d users flagged: %d disabled, %d password expired, %d in no security group.",
				rep.FlaggedUsers, rep.TotalUsers, rep.Disabled, rep.PasswordExpired, rep.NoSecurityGroup)
			return cwcEmit(cmd, flags, rep, rows, summary)
		},
	}
	cmd.Flags().BoolVar(&flaggedOnly, "flagged-only", false, "List only users with at least one finding or unknown field")
	return cmd
}

// cwcSecurityUsers reads the users array from a GetSecurityConfigurationInfo
// answer. An answer with no users array is an error, never "no users".
func cwcSecurityUsers(data []byte) ([]map[string]any, error) {
	v, err := cwcDecode(data)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unrecognised security configuration answer: expected a JSON object")
	}
	users, found := cwcField(m, "users")
	if !found {
		return nil, fmt.Errorf("unrecognised security configuration answer: no users array")
	}
	return cwcObjects(users, "security configuration users")
}

func buildAccessReview(users []map[string]any, flaggedOnly bool) accessReviewReport {
	rep := accessReviewReport{TotalUsers: len(users), Users: []accessReviewUser{}}
	for _, m := range users {
		u := accessReviewUser{
			UserName:    cwcString(m, "userName"),
			DisplayName: cwcString(m, "displayName"),
			Email:       cwcString(m, "email"),
			Findings:    []string{},
		}
		if v, ok := cwcBool(m, "isDisabled"); !ok {
			u.Unknown = append(u.Unknown, "disabled")
		} else if v {
			u.Findings = append(u.Findings, "disabled")
			rep.Disabled++
		}
		if v, ok := cwcBool(m, "isPasswordExpired"); !ok {
			u.Unknown = append(u.Unknown, "password_expired")
		} else if v {
			u.Findings = append(u.Findings, "password_expired")
			rep.PasswordExpired++
		}
		groups, found := cwcStrings(m, "securityGroups")
		switch {
		case !found || groups == nil:
			u.SecurityGroups = []string{}
			u.Unknown = append(u.Unknown, "security_groups")
		case len(groups) == 0:
			u.SecurityGroups = groups
			u.Findings = append(u.Findings, "no_security_group")
			rep.NoSecurityGroup++
		default:
			u.SecurityGroups = groups
		}
		if len(u.Findings) > 0 {
			rep.FlaggedUsers++
		}
		if len(u.Unknown) > 0 {
			rep.UnknownFieldUser++
		}
		if flaggedOnly && len(u.Findings) == 0 && len(u.Unknown) == 0 {
			continue
		}
		rep.Users = append(rep.Users, u)
	}
	sort.SliceStable(rep.Users, func(i, j int) bool {
		if len(rep.Users[i].Findings) != len(rep.Users[j].Findings) {
			return len(rep.Users[i].Findings) > len(rep.Users[j].Findings)
		}
		return strings.ToLower(rep.Users[i].UserName) < strings.ToLower(rep.Users[j].UserName)
	})
	return rep
}
