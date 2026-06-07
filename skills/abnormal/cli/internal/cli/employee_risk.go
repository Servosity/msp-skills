// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature: employee account-takeover risk profile. Hand-authored; preserved across regenerations.

// pp:data-source live

package cli

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"abnormal-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

type loginsSummary struct {
	TotalEvents      int      `json:"total_events"`
	FailedEvents     int      `json:"failed_events"`
	DistinctIPs      int      `json:"distinct_ips"`
	Countries        []string `json:"countries"`
	LastLogin        string   `json:"last_login,omitempty"`
	WindowDays       int      `json:"window_days"`
	ParseLimitedNote string   `json:"note,omitempty"`
}

type riskCase struct {
	CaseID           string `json:"caseId"`
	Severity         string `json:"severity,omitempty"`
	AffectedEmployee string `json:"affectedEmployee,omitempty"`
	FirstObserved    string `json:"firstObserved,omitempty"`
	Status           string `json:"case_status,omitempty"`
}

type employeeRiskFailure struct {
	Source string `json:"source"`
	Error  string `json:"error"`
}

type employeeRiskView struct {
	Email         string                `json:"email"`
	Profile       json.RawMessage       `json:"profile,omitempty"`
	Identity      json.RawMessage       `json:"identity,omitempty"`
	Logins        *loginsSummary        `json:"logins,omitempty"`
	OpenCases     []riskCase            `json:"open_cases"`
	ScannedCases  int                   `json:"scanned_cases"`
	MaxScanPages  int                   `json:"max_scan_pages"`
	Note          string                `json:"note,omitempty"`
	FetchFailures []employeeRiskFailure `json:"fetch_failures,omitempty"`
}

// summarizeLoginsCSV reduces the 30-day login CSV to a compact, PII-light summary.
func summarizeLoginsCSV(raw []byte) *loginsSummary {
	sum := &loginsSummary{WindowDays: 30, Countries: make([]string, 0)}
	r := csv.NewReader(strings.NewReader(string(raw)))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil || len(rows) == 0 {
		sum.ParseLimitedNote = "login CSV could not be parsed; raw export available via the generated employee logins command"
		return sum
	}
	header := rows[0]
	col := map[string]int{}
	for i, h := range header {
		col[strings.ToLower(strings.TrimSpace(h))] = i
	}
	ips := map[string]bool{}
	countries := map[string]bool{}
	var lastTS time.Time
	get := func(row []string, name string) string {
		if i, ok := col[name]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}
	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		sum.TotalEvents++
		if s := strings.ToLower(get(row, "status")); s != "" && s != "success" && s != "0" {
			sum.FailedEvents++
		}
		if ip := get(row, "ip address"); ip != "" {
			ips[ip] = true
		}
		if cn := get(row, "country or region"); cn != "" {
			countries[cn] = true
		}
		if ts := get(row, "timestamp"); ts != "" {
			for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
				if t, err := time.Parse(layout, ts); err == nil {
					if t.After(lastTS) {
						lastTS = t
						sum.LastLogin = ts
					}
					break
				}
			}
		}
	}
	sum.DistinctIPs = len(ips)
	for cn := range countries {
		sum.Countries = append(sum.Countries, cn)
	}
	return sum
}

// caseMatchesEmployee reports whether a case's affectedEmployee plausibly
// refers to the given email (full address, or name matching the local part).
func caseMatchesEmployee(affected, email string) bool {
	a := strings.ToLower(strings.TrimSpace(affected))
	e := strings.ToLower(strings.TrimSpace(email))
	if a == "" || e == "" {
		return false
	}
	if strings.Contains(a, e) {
		return true
	}
	local := e
	if i := strings.IndexByte(e, '@'); i > 0 {
		local = e[:i]
	}
	// Match "first.last" local parts against "First Last" display names on
	// whole-token boundaries — a raw substring match would attribute
	// "Albert Jones" to al@corp.com.
	norm := strings.NewReplacer(".", " ", "_", " ", "-", " ")
	localTokens := strings.Fields(norm.Replace(local))
	affectedTokens := strings.Fields(norm.Replace(a))
	if len(localTokens) == 0 {
		return false
	}
	for _, lt := range localTokens {
		found := false
		for _, at := range affectedTokens {
			if lt == at {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	// Single short tokens ("al", "jo") are too ambiguous to claim a match.
	if len(localTokens) == 1 && len(localTokens[0]) < 4 {
		return false
	}
	return true
}

func newNovelEmployeeRiskCmd(flags *rootFlags) *cobra.Command {
	var maxScanPages int

	cmd := &cobra.Command{
		Use:   "employee-risk <email>",
		Short: "One account-takeover risk picture per employee: profile, Genome identity, logins, open cases",
		Long: strings.Trim(`
Use this command to assemble one account-takeover risk picture for an employee:
profile, Genome identity analysis, a 30-day login summary, and recent cases
naming them. Do NOT use it for vendor compromise; use 'vendor-risk'.
Do NOT use it for the raw login CSV alone; the generated employee logins
endpoint returns that.

Case matching scans recent cases and matches affectedEmployee against the
email or its local part; scanned_cases reports how many were examined.`, "\n"),
		Example: strings.Trim(`
  abnormal-cli employee-risk vip@example.com
  abnormal-cli employee-risk vip@example.com --agent
  abnormal-cli employee-risk first.last@example.com --max-scan-pages 5`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "email=vip@example.com",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if len(args) < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("employee-risk requires an employee email address"))
			}
			email := strings.TrimSpace(args[0])
			if !strings.Contains(email, "@") {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("%q does not look like an email address", email))
			}
			if flags.dataSource == "local" {
				return usageErr(fmt.Errorf("employee-risk queries live employee endpoints; no local data source"))
			}
			if dryRunOK(flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "would join profile, identity, logins, and recent cases for %s\n", email)
				return nil
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			esc := url.PathEscape(email)
			view := employeeRiskView{Email: email, OpenCases: make([]riskCase, 0), MaxScanPages: maxScanPages, FetchFailures: make([]employeeRiskFailure, 0)}
			var mu sync.Mutex
			var wg sync.WaitGroup
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()
			fail := func(source string, err error) {
				mu.Lock()
				view.FetchFailures = append(view.FetchFailures, employeeRiskFailure{Source: source, Error: err.Error()})
				mu.Unlock()
			}
			wg.Add(3)
			go func() {
				defer wg.Done()
				data, err := c.Get(ctx, "/employee/"+esc, nil)
				if err != nil {
					fail("profile", err)
					return
				}
				mu.Lock()
				view.Profile = data
				mu.Unlock()
			}()
			go func() {
				defer wg.Done()
				data, err := c.Get(ctx, "/employee/"+esc+"/identity", nil)
				if err != nil {
					fail("identity", err)
					return
				}
				mu.Lock()
				view.Identity = data
				mu.Unlock()
			}()
			go func() {
				defer wg.Done()
				data, err := c.GetWithHeaders(ctx, "/employee/"+esc+"/logins", nil, map[string]string{"Accept": "text/csv"})
				if err != nil {
					fail("logins", err)
					return
				}
				sum := summarizeLoginsCSV(data)
				mu.Lock()
				view.Logins = sum
				mu.Unlock()
			}()
			wg.Wait()

			// Scan recent cases for this employee (the API has no employee filter).
			pages := maxScanPages
			if cliutil.IsDogfoodEnv() && pages > 1 {
				pages = 1
			}
			now := time.Now().UTC()
			filter := fmt.Sprintf("customerVisibleTime gte %s lte %s", now.AddDate(0, 0, -30).Format("2006-01-02T15:04:05Z"), now.Format("2006-01-02T15:04:05Z"))
			scanCapHit := true
			for page := 1; page <= pages; page++ {
				data, err := c.Get(ctx, "/cases", map[string]string{
					"filter":     filter,
					"pageSize":   "100",
					"pageNumber": strconv.Itoa(page),
				})
				if err != nil {
					fail("cases", err)
					scanCapHit = false // failure, not a cap — fetch_failures carries the cause
					break
				}
				var pageDoc struct {
					Cases          []riskCase `json:"cases"`
					NextPageNumber *int       `json:"nextPageNumber"`
				}
				if err := json.Unmarshal(data, &pageDoc); err != nil {
					fail("cases", fmt.Errorf("parsing cases page %d: %w", page, err))
					scanCapHit = false
					break
				}
				for _, cs := range pageDoc.Cases {
					view.ScannedCases++
					if caseMatchesEmployee(cs.AffectedEmployee, email) {
						view.OpenCases = append(view.OpenCases, cs)
					}
				}
				if pageDoc.NextPageNumber == nil || len(pageDoc.Cases) == 0 {
					scanCapHit = false
					break
				}
			}
			if len(view.OpenCases) == 0 && scanCapHit {
				view.Note = fmt.Sprintf("scanned %d recent cases without a match for %s; raise --max-scan-pages to widen the search", view.ScannedCases, email)
			}
			if len(view.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of 4 sources failed; profile assembled from the remaining sources\n", len(view.FetchFailures))
			}
			if view.Profile == nil && view.Identity == nil && view.Logins == nil {
				return classifyAPIError(fmt.Errorf("all employee sources failed for %s; first error: %s", email, view.FetchFailures[0].Error), flags)
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 3, "Maximum recent-case pages (100 cases each) to scan for this employee")
	return cmd
}
