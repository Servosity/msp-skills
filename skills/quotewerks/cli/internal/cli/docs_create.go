// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"quotewerks-pp-cli/internal/qw"
)

// joinedPairRe spots "A=1,B=2" in one --set value: the MCP mirror passes a
// repeatable flag as a single string, which would otherwise set A to "1,B=2".
var joinedPairRe = regexp.MustCompile(`,\s*[A-Za-z_][A-Za-z0-9_]*=`)

// parseSetPairs turns repeated Column=Value flags into attributes. --set
// values are sent verbatim as text (a postal code like 02134 must keep its
// leading zero); --set-number values must parse as finite decimal numbers.
func parseSetPairs(texts, numbers []string, owned map[string]string) (map[string]any, error) {
	out := map[string]any{}
	add := func(flag, p string, numeric bool) error {
		k, v, ok := strings.Cut(p, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			return usageErr(fmt.Errorf("%s %q: use Column=Value", flag, p))
		}
		if joinedPairRe.MatchString(v) {
			return usageErr(fmt.Errorf("%s %q looks like several pairs in one value; pass %s once per column", flag, p, flag))
		}
		if f, isOwned := owned[k]; isOwned {
			return usageErr(fmt.Errorf("%s %s: use %s for this column", flag, k, f))
		}
		if !numeric {
			out[k] = v
			return nil
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			return usageErr(fmt.Errorf("%s %q: value is not a finite number", flag, p))
		}
		out[k] = f
		return nil
	}
	for _, p := range texts {
		if err := add("--set", p, false); err != nil {
			return nil, err
		}
	}
	for _, p := range numbers {
		if err := add("--set-number", p, true); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func newNovelDocsCreateCmd(flags *rootFlags) *cobra.Command {
	var customer, contact, name, docType, status, rep, date string
	var sets, setNumbers []string
	var confirm bool

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a quote, order or invoice header; previews unless --confirm",
		Long: `Create a DocumentHeaders row from typed flags. By default the command only prints
the JSON:API request it would send; add --confirm to create it. The result carries the
new RecGUID and server-assigned DocNo; add lines next with 'items add'.

QuoteWerks does not fill SoldTo fields from the CRM or prices from the product database
on create. SalesRep must be an existing QuoteWerks Web user or the API returns 422.

Use this command to start a new empty QUOTE/ORDER header. Do NOT use it to copy an
existing document; use 'requote' instead.`,
		Example: strings.Trim(`
  quotewerks-cli docs create --customer "Acme Dental" --name "Q4 laptop refresh" --json
  quotewerks-cli docs create --customer "Acme Dental" --contact "Dana Lee" --rep "Derek Bryan" --confirm --json`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "mcp:read-only-unless-flags": "confirm", "pp:preview-happy-path": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "docs create")
			}
			if len([]rune(customer)) > qw.SoldToCompanyMax {
				return usageErr(fmt.Errorf("--customer is %d characters; QuoteWerks Web caps SoldToCompany at %d", len([]rune(customer)), qw.SoldToCompanyMax))
			}
			if date == "" {
				date = time.Now().Format("2006-01-02")
			}
			if _, err := time.Parse("2006-01-02", date); err != nil {
				return usageErr(fmt.Errorf("--date %q: use YYYY-MM-DD", date))
			}
			owned := map[string]string{"DocType": "--type", "DocDate": "--date", "SoldToCompany": "--customer", "SoldToContact": "--contact", "DocName": "--name", "DocStatus": "--status", "SalesRep": "--rep"}
			attrs, err := parseSetPairs(sets, setNumbers, owned)
			if err != nil {
				return err
			}
			if t := strings.ToUpper(docType); t != "QUOTE" && t != "ORDER" && t != "INVOICE" {
				return usageErr(fmt.Errorf("--type %q: use QUOTE, ORDER or INVOICE", docType))
			}
			attrs["DocType"] = strings.ToUpper(docType)
			attrs["DocDate"] = date + "T00:00:00"
			for col, v := range map[string]string{"SoldToCompany": customer, "SoldToContact": contact, "DocName": name, "DocStatus": status, "SalesRep": rep} {
				if v != "" {
					attrs[col] = v
				}
			}
			if !confirm {
				view := writeModeEnvelope(false, map[string]any{"would": []any{jsonapiBody("POST", qw.TableHeaders, "", attrs)}})
				return printWriteResult(cmd.OutOrStdout(), view, flags)
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			created, err := qwWrite(cmd.Context(), c, "POST", qw.TableHeaders, "", attrs)
			if err != nil {
				return err
			}
			doc := project(created.Flat(), qwHeaderFields)
			view := writeModeEnvelope(true, map[string]any{
				"created": doc,
				"next":    fmt.Sprintf("%s items add %s --part <part-number> --qty 1", qwCLIName, created.ID),
			})
			return printWriteResult(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&customer, "customer", "", "Customer company name (SoldToCompany, max 50 characters)")
	cmd.Flags().StringVar(&contact, "contact", "", "Customer contact name (SoldToContact)")
	cmd.Flags().StringVar(&name, "name", "", "Document name shown in QuoteWerks (DocName)")
	cmd.Flags().StringVar(&docType, "type", "QUOTE", "Document type: QUOTE, ORDER or INVOICE (DocType)")
	cmd.Flags().StringVar(&status, "status", "", "Document status as your tenant names it, e.g. Open (DocStatus)")
	cmd.Flags().StringVar(&rep, "rep", "", "Sales rep; must be an existing QuoteWerks Web user (SalesRep)")
	cmd.Flags().StringVar(&date, "date", "", "Document date YYYY-MM-DD (DocDate, default today)")
	cmd.Flags().StringArrayVar(&sets, "set", nil, "Extra header column as Column=Value, sent as text; repeat once per column (see 'tables schema DocumentHeaders')")
	cmd.Flags().StringArrayVar(&setNumbers, "set-number", nil, "Extra numeric header column as Column=Number; repeat once per column")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Create the document; without it the command only previews the request")
	return cmd
}
