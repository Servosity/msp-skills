// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: Active Directory / Azure hygiene per client.
// pp:data-source local

package cli

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type adHygieneRow struct {
	CompanyID            string   `json:"company_id"`
	CompanyName          string   `json:"company_name"`
	EnabledUsers         int      `json:"enabled_users"`
	PasswordNeverExpires int      `json:"password_never_expires"`
	PasswordNotRequired  int      `json:"password_not_required"`
	NoMFA                int      `json:"no_mfa"`
	StaleLogon           int      `json:"stale_logon"`
	PasswordPolicies     int      `json:"password_policies"`
	WeakPolicies         int      `json:"weak_policies"`
	WeakPolicyReasons    []string `json:"weak_policy_reasons"`
	AzureSecureScorePct  *float64 `json:"azure_secure_score_pct"`
	AzureSecureScoreRaw  string   `json:"azure_secure_score"`
	RiskRank             int      `json:"risk_rank_score"`
}

func buildADHygiene(data map[string][]csRow, staleLogon time.Duration, minLength int, now time.Time) []adHygieneRow {
	names := csCompanyNames(data[csResCompanies])
	byID := map[string]*adHygieneRow{}
	get := func(id string) *adHygieneRow {
		if id == "" {
			return nil
		}
		r, ok := byID[id]
		if !ok {
			r = &adHygieneRow{CompanyID: id, CompanyName: names[id], WeakPolicyReasons: make([]string, 0)}
			byID[id] = r
		}
		return r
	}
	for _, u := range data[csResADUsers] {
		if csBool(u["account_disabled"]) {
			continue
		}
		r := get(csString(u["company_id"]))
		if r == nil {
			continue
		}
		r.EnabledUsers++
		if csBool(u["password_never_expires"]) {
			r.PasswordNeverExpires++
		}
		if csBool(u["password_not_required"]) {
			r.PasswordNotRequired++
		}
		if v, present := u["mfa_enabled"]; present && v != nil && !csBool(v) {
			r.NoMFA++
		}
		if staleLogon > 0 {
			if t, ok := csTime(u["last_logon_timestamp"]); !ok || t.Before(now.Add(-staleLogon)) {
				r.StaleLogon++
			}
		}
	}
	for _, p := range data[csResADPolicies] {
		r := get(csString(p["company_id"]))
		if r == nil {
			continue
		}
		r.PasswordPolicies++
		reasons := make([]string, 0, 4)
		if v, present := p["complexity_enabled"]; present && v != nil && !csBool(v) {
			reasons = append(reasons, "complexity_disabled")
		}
		if csBool(p["reversible_encryption_enabled"]) {
			reasons = append(reasons, "reversible_encryption")
		}
		if v, ok := csFloat(p["lockout_threshold"]); ok && v == 0 {
			reasons = append(reasons, "no_lockout")
		}
		if l, ok := csFloat(p["min_password_length"]); ok && minLength > 0 && int(l) < minLength {
			reasons = append(reasons, fmt.Sprintf("min_length_%d", int(l)))
		}
		if len(reasons) > 0 {
			r.WeakPolicies++
			for _, reason := range reasons {
				if !slices.Contains(r.WeakPolicyReasons, reason) {
					r.WeakPolicyReasons = append(r.WeakPolicyReasons, reason)
				}
			}
		}
	}
	// Several snapshots per company are possible; keep the latest.
	azureLatest := map[string]time.Time{}
	for _, a := range data[csResAzureSecureScore] {
		r := get(csString(a["company_id"]))
		if r == nil {
			continue
		}
		at, _ := csTime(a["created_date_time"])
		if prev, seen := azureLatest[r.CompanyID]; seen && at.Before(prev) {
			continue
		}
		azureLatest[r.CompanyID] = at
		r.AzureSecureScorePct = nil // a newer snapshot replaces the old percentage even when it cannot compute one
		cur, okC := csFloat(a["current_score"])
		maxScore, okM := csFloat(a["max_score"])
		r.AzureSecureScoreRaw = fmt.Sprintf("%s/%s", csString(a["current_score"]), csString(a["max_score"]))
		if okC && okM && maxScore > 0 {
			pct := csRound(cur/maxScore*100, 1)
			r.AzureSecureScorePct = &pct
		}
	}
	out := make([]adHygieneRow, 0, len(byID))
	for _, r := range byID {
		// Rank: password-not-required weighs most, weak policies next, then
		// every other risky-account count, plus a point per 10 points of
		// Azure secure score below 100%.
		r.RiskRank = r.PasswordNotRequired*3 + r.WeakPolicies*2 + r.PasswordNeverExpires + r.NoMFA + r.StaleLogon
		if r.AzureSecureScorePct != nil {
			r.RiskRank += int((100 - *r.AzureSecureScorePct) / 10)
		}
		out = append(out, *r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RiskRank != out[j].RiskRank {
			return out[i].RiskRank > out[j].RiskRank
		}
		if out[i].CompanyName != out[j].CompanyName {
			return out[i].CompanyName < out[j].CompanyName
		}
		return out[i].CompanyID < out[j].CompanyID
	})
	return out
}

func newNovelAdHygieneCmd(flags *rootFlags) *cobra.Command {
	var staleFlag, company, dbPath string
	var minLength int
	cmd := &cobra.Command{
		Use:   "ad-hygiene",
		Short: "Per client: risky AD accounts, weak password policies and Azure secure score",
		Long: `Per-client identity hygiene rolled up from the synced Active Directory users,
password policies and Azure secure score reports. Enabled accounts are counted
for password-never-expires, password-not-required, no MFA and stale logon;
password policies are weak when complexity is off, reversible encryption is on,
there is no lockout threshold, or the minimum length is under --min-length.
Riskiest clients first. Reads the local store (run 'connectsecure-cli sync' first).`,
		Example: strings.Trim(`
  connectsecure-cli ad-hygiene --stale-logon 90d --min-length 12
  connectsecure-cli ad-hygiene --company "Acme Dental" --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "ad-hygiene")
			}
			stale, err := csDuration("stale-logon", staleFlag)
			if err != nil {
				return err
			}
			if minLength < 0 {
				return usageErr(fmt.Errorf("--min-length must be >= 0"))
			}
			data, unsynced, err := csLocalData(cmd, flags, dbPath, csResADUsers, csResADPolicies, csResAzureSecureScore, csResCompanies)
			if err != nil {
				return err
			}
			rows := buildADHygiene(data, stale, minLength, time.Now().UTC())
			rows = slices.DeleteFunc(rows, func(r adHygieneRow) bool { return !csCompanyMatch(company, r.CompanyID, r.CompanyName) })
			return csEmit(cmd, flags, rows, unsynced, "No Active Directory data in the local store.")
		},
	}
	cmd.Flags().StringVar(&staleFlag, "stale-logon", "90d", "Count enabled accounts with no logon within this window (0 disables)")
	cmd.Flags().IntVar(&minLength, "min-length", 12, "Password policies below this minimum length are weak (0 disables)")
	cmd.Flags().StringVar(&company, "company", "", "Only this client (name substring or company ID)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: the synced store)")
	return cmd
}
