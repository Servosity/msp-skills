// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"cisco-umbrella-pp-cli/internal/client"
)

type verdictListHit struct {
	ListID   int64  `json:"list_id"`
	ListName string `json:"list_name"`
	Access   string `json:"access"`
	IsGlobal bool   `json:"is_global"`
	Entry    string `json:"entry"`
	Match    string `json:"match"` // exact | parent | host
}

type verdictResolver struct {
	Identity string `json:"identity"`
	Type     string `json:"type"`
	Requests int64  `json:"requests"`
	Blocked  int64  `json:"blocked"`
}

type domainVerdict struct {
	Domain             string            `json:"domain"`
	Kind               string            `json:"kind"`
	Status             string            `json:"status,omitempty"`
	SecurityCategories []string          `json:"security_categories"`
	ContentCategories  []string          `json:"content_categories"`
	RiskScore          *float64          `json:"risk_score,omitempty"`
	ThreatType         string            `json:"threat_type,omitempty"`
	Attack             string            `json:"attack,omitempty"`
	SecureRank2        *float64          `json:"securerank2,omitempty"`
	Coverage           string            `json:"coverage"` // complete | partial | unknown
	ListsChecked       bool              `json:"lists_checked"`
	InvestigateChecked bool              `json:"investigate_checked"`
	ResolversChecked   bool              `json:"resolvers_checked"`
	ScoresChecked      bool              `json:"scores_checked"`
	BlockLists         int               `json:"on_block_lists"`
	AllowLists         int               `json:"on_allow_lists"`
	Lists              []verdictListHit  `json:"lists"`
	ResolvedBy         []verdictResolver `json:"resolved_by"`
	Notes              []string          `json:"notes,omitempty"`
}

func newNovelVerdictCmd(flags *rootFlags) *cobra.Command {
	var fromStdin, noActivity bool
	var from, to string
	cmd := &cobra.Command{
		Use:   "verdict [domain...]",
		Short: "One answer for a domain: category, risk score, security status, list membership, recent resolvers",
		Long: `Combine Investigate (categorization, security, risk score) with every destination list
that blocks or allows the domain, including parent-domain entries (an entry for
example.com covers login.example.com), and the identities that resolved it in a
time window. Sections the API key cannot reach (403) are skipped with a note.

Use this command for domains you supply (a ticket, an IOC, stdin). Do NOT use this
command to review the org's own allow lists; use 'allowlist audit' instead.`,
		Example: strings.Trim(`
  cisco-umbrella-cli verdict login.example.com --agent
  cisco-umbrella-cli verdict example.com 198.51.100.7 --from -7days
  cat iocs.txt | cisco-umbrella-cli verdict --stdin --json --select domain,status,on_block_lists,coverage`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "domain=example.com"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "verdict")
			}
			inputs := append([]string{}, args...)
			if fromStdin {
				lines, err := readLines(cmd.InOrStdin())
				if err != nil {
					return err
				}
				inputs = append(inputs, lines...)
			}
			type target struct{ value, kind string }
			targets := make([]target, 0, len(inputs))
			seen := map[string]bool{}
			for _, in := range inputs {
				v, k, ok := normalizeDestination(in)
				if !ok {
					return usageErr(fmt.Errorf("not a domain, URL, or IPv4 address: %q", in))
				}
				if !seen[v] {
					seen[v] = true
					targets = append(targets, target{v, k})
				}
			}
			if len(targets) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("give at least one domain, URL, or IP (or --stdin)"))
			}
			// ponytail: no whole-command --timeout here; this fans out across every list/customer
			// and each request is already bounded by the client's per-request timeout.
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			if err := requireCredentials(c); err != nil {
				return err
			}

			verdicts := make([]*domainVerdict, len(targets))
			for i, t := range targets {
				verdicts[i] = &domainVerdict{Domain: t.value, Kind: t.kind, SecurityCategories: []string{}, ContentCategories: []string{}, Lists: []verdictListHit{}, ResolvedBy: []verdictResolver{}}
			}

			// List membership: every list, every destination, matched suffix-aware.
			lists, err := fetchDestinationLists(ctx, c)
			switch {
			case isForbidden(err):
				noteAll(verdicts, "destination lists skipped: API key lacks the policies scope")
			case err != nil:
				return classifyAPIErrorOnly(err)
			default:
				for _, v := range verdicts {
					v.ListsChecked = true
				}
				for _, l := range lists {
					dests, err := fetchDestinations(ctx, c, l.ID)
					if err != nil {
						if isForbidden(err) {
							noteAll(verdicts, fmt.Sprintf("destinations of list %q skipped: forbidden", l.Name))
							for _, v := range verdicts {
								v.ListsChecked = false
							}
							continue
						}
						return classifyAPIErrorOnly(err)
					}
					for i, t := range targets {
						onList := false
						for _, d := range dests {
							if !destinationMatches(d, t.value, t.kind) {
								continue
							}
							onList = true
							hit := verdictListHit{ListID: l.ID, ListName: l.Name, Access: l.Access, IsGlobal: l.IsGlobal, Entry: d.Destination, Match: matchKind(d.Destination, t.value)}
							verdicts[i].Lists = append(verdicts[i].Lists, hit)
						}
						// Counts are lists, not entries: a list holding both example.com
						// and login.example.com still blocks the domain once.
						if onList && strings.EqualFold(l.Access, "block") {
							verdicts[i].BlockLists++
						} else if onList && strings.EqualFold(l.Access, "allow") {
							verdicts[i].AllowLists++
						}
					}
				}
			}

			domains := make([]string, 0, len(targets))
			for _, t := range targets {
				if hasDomainHost(t.value, t.kind) {
					domains = append(domains, hostOf(t.value))
				}
			}
			if err := applyCategorization(ctx, c, domains, verdicts); err != nil {
				return err
			}
			for _, v := range verdicts {
				if !hasDomainHost(v.Domain, v.Kind) {
					continue
				}
				if err := applyInvestigateScores(ctx, c, hostOf(v.Domain), v); err != nil {
					return err
				}
				if !noActivity {
					if err := applyResolvers(ctx, c, hostOf(v.Domain), from, to, v); err != nil {
						return err
					}
				}
			}

			for _, v := range verdicts {
				v.Coverage = verdictCoverage(v, !noActivity)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				// Keep sparse optional fields under --agent/--compact so one domain's score is not dropped.
				return printJSONFilteredKeep(cmd.OutOrStdout(), verdicts, flags, "risk_score", "securerank2", "threat_type", "attack", "status", "notes", "coverage")
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "DOMAIN\tSTATUS\tRISK\tBLOCK LISTS\tALLOW LISTS\tCOVERAGE\tTOP RESOLVER")
			for _, v := range verdicts {
				risk := "-"
				if v.RiskScore != nil {
					risk = strconv.FormatFloat(*v.RiskScore, 'f', -1, 64)
				}
				top := "-"
				if len(v.ResolvedBy) > 0 {
					top = v.ResolvedBy[0].Identity
				}
				status := v.Status
				if status == "" {
					status = "-"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%s\t%s\n", v.Domain, status, risk, v.BlockLists, v.AllowLists, v.Coverage, top)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			for _, v := range verdicts {
				for _, n := range v.Notes {
					fmt.Fprintf(cmd.ErrOrStderr(), "note (%s): %s\n", v.Domain, n)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&fromStdin, "stdin", false, "Also read domains, URLs, or IPs from stdin, one per line (# comments allowed)")
	cmd.Flags().StringVar(&from, "from", "-1days", "Start of the resolver window: epoch ms or relative (-1days, -7days)")
	cmd.Flags().StringVar(&to, "to", "now", "End of the resolver window: epoch ms or 'now'")
	cmd.Flags().BoolVar(&noActivity, "no-activity", false, "Skip the who-resolved-it report call")
	return cmd
}

// verdictCoverage says how much of the verdict was actually checked, so an
// all-zero result from skipped sections never reads as a clean domain.
func verdictCoverage(v *domainVerdict, wantResolvers bool) string {
	checks := []bool{v.ListsChecked}
	if hasDomainHost(v.Domain, v.Kind) {
		checks = append(checks, v.InvestigateChecked, v.ScoresChecked)
		if wantResolvers {
			checks = append(checks, v.ResolversChecked)
		}
	}
	done := 0
	for _, ok := range checks {
		if ok {
			done++
		}
	}
	switch done {
	case len(checks):
		return "complete"
	case 0:
		return "unknown"
	default:
		return "partial"
	}
}

func noteAll(vs []*domainVerdict, note string) {
	for _, v := range vs {
		v.Notes = append(v.Notes, note)
	}
}

func matchKind(entry, value string) string {
	e, kind, _ := normalizeDestination(entry)
	switch {
	case e == value:
		return "exact"
	case kind == "domain" && e == hostOf(value):
		return "host"
	default:
		return "parent"
	}
}

func readLines(r io.Reader) ([]string, error) {
	out := make([]string, 0)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" && !strings.HasPrefix(line, "#") {
			out = append(out, line)
		}
	}
	return out, sc.Err()
}

// applyCategorization marks a domain investigate_checked only when Investigate
// actually returned a categorization for it.
func applyCategorization(ctx context.Context, c *client.Client, domains []string, vs []*domainVerdict) error {
	cats, missing, forbidden, err := categorizeDomains(ctx, c, domains)
	if err != nil {
		return classifyAPIErrorOnly(err)
	}
	if forbidden {
		for _, v := range vs {
			if hasDomainHost(v.Domain, v.Kind) {
				v.Notes = append(v.Notes, "categorization skipped: API key lacks the investigate scope")
			}
		}
		return nil
	}
	missingSet := map[string]bool{}
	for _, d := range missing {
		missingSet[strings.ToLower(d)] = true
	}
	for _, v := range vs {
		if !hasDomainHost(v.Domain, v.Kind) {
			continue
		}
		host := strings.ToLower(hostOf(v.Domain))
		cat, ok := cats[host]
		if !ok || missingSet[host] {
			v.Notes = append(v.Notes, "Investigate returned no categorization for "+host)
			continue
		}
		v.InvestigateChecked = true
		v.Status = statusLabel(cat.Status)
		if cat.SecurityCategories != nil {
			v.SecurityCategories = cat.SecurityCategories
		}
		if cat.ContentCategories != nil {
			v.ContentCategories = cat.ContentCategories
		}
	}
	return nil
}

func applyInvestigateScores(ctx context.Context, c *client.Client, domain string, v *domainVerdict) error {
	riskOK, secOK := false, false
	raw, err := c.Get(ctx, "/investigate/v2/domains/risk-score/"+url.PathEscape(domain), nil)
	switch {
	case isForbidden(err):
		v.Notes = append(v.Notes, "risk score skipped: API key lacks the investigate scope")
	case err != nil:
		return classifyAPIErrorOnly(err)
	default:
		var rs struct {
			RiskScore *float64 `json:"risk_score"`
		}
		if err := decodeObject(raw, &rs); err != nil {
			return fmt.Errorf("parsing risk score for %s: %w", domain, err)
		}
		v.RiskScore, riskOK = rs.RiskScore, rs.RiskScore != nil
		if !riskOK {
			v.Notes = append(v.Notes, "Investigate returned no risk score for "+domain)
		}
	}
	raw, err = c.Get(ctx, "/investigate/v2/security/name/"+url.PathEscape(domain), nil)
	switch {
	case isForbidden(err):
		v.Notes = append(v.Notes, "security scores skipped: API key lacks the investigate scope")
	case err != nil:
		return classifyAPIErrorOnly(err)
	default:
		var sec struct {
			ThreatType  string   `json:"threat_type"`
			Attack      string   `json:"attack"`
			SecureRank2 *float64 `json:"securerank2"`
			Found       *bool    `json:"found"`
		}
		if err := decodeObject(raw, &sec); err != nil {
			return fmt.Errorf("parsing security scores for %s: %w", domain, err)
		}
		v.ThreatType, v.Attack, v.SecureRank2 = sec.ThreatType, sec.Attack, sec.SecureRank2
		// found:false (documented) or an empty body means Investigate has no security data.
		secOK = sec.Found != nil && *sec.Found
		if !secOK {
			v.Notes = append(v.Notes, "Investigate has no security scores for "+domain)
		}
	}
	v.ScoresChecked = riskOK && secOK
	return nil
}

// applyResolvers lists the identities that queried the domain in the window
// (reports/v2/top-identities filtered by domains).
func applyResolvers(ctx context.Context, c *client.Client, domain, from, to string, v *domainVerdict) error {
	raw, err := c.Get(ctx, "/reports/v2/top-identities", map[string]string{"from": from, "to": to, "domains": domain, "limit": "10", "offset": "0"})
	if isForbidden(err) {
		v.Notes = append(v.Notes, "resolvers skipped: API key lacks the reports scope")
		return nil
	}
	if err != nil {
		return classifyAPIErrorOnly(err)
	}
	var rows []*struct {
		Requests *int64 `json:"requests"`
		Identity *struct {
			Label string `json:"label"`
			Type  struct {
				Type string `json:"type"`
			} `json:"type"`
		} `json:"identity"`
		Counts *struct {
			Blocked *int64 `json:"blockedrequests"`
		} `json:"counts"`
	}
	if err := decodeArray(unwrapData(raw), &rows); err != nil {
		v.Notes = append(v.Notes, "resolvers not checked: unreadable top-identities response")
		return nil
	}
	resolved := make([]verdictResolver, 0, len(rows))
	for _, r := range rows {
		if r == nil || r.Requests == nil || r.Identity == nil || strings.TrimSpace(r.Identity.Label) == "" || r.Counts == nil || r.Counts.Blocked == nil {
			v.Notes = append(v.Notes, "resolvers not checked: a top-identities row lacks identity or counts")
			return nil
		}
		resolved = append(resolved, verdictResolver{Identity: r.Identity.Label, Type: r.Identity.Type.Type, Requests: *r.Requests, Blocked: *r.Counts.Blocked})
	}
	v.ResolvedBy = resolved
	v.ResolversChecked = true
	return nil
}
