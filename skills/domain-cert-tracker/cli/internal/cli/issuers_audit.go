// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (handfixes.json tracker-novels).
// pp:data-source local

package cli

import (
	"time"

	"domain-cert-tracker-pp-cli/internal/tracker"

	"github.com/spf13/cobra"
)

func newNovelIssuersAuditCmd(flags *rootFlags) *cobra.Command {
	var client, days string
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Separate certificates from issuers a domain's CAA does not permit, or a first-ever issuer, from routine renewals.",
		Long: `Use this command to separate unexpected certificates (an issuer the domain's
CAA does not permit, or a first-ever issuer for that domain) from routine
renewals. Do NOT use this command to find hostnames that have certificates but
are not in the inventory; use 'hosts discover' instead.

Each name on a certificate is judged against the CAA set that applies to that
name (its own, else its closest parent's; issuewild only for wildcard names),
as the last sweep recorded it. The names judged are the certificate's own SAN
list, which sweep downloads from crt.sh: crt.sh's search lists only the names
that matched the domain, so a certificate found for shop.example.com that also
names *.example.com shows shop.example.com alone. A certificate is routine only
when its full name list came from the certificate itself and the CAA set for
every one of those names was read.

Statuses: not_in_caa (the CAA for at least one name on the certificate does not
permit the issuer; CAA may have changed since issuance; it wins over every other
status), names_partial (only crt.sh's matched names are known because the
certificate has not been downloaded yet, its download failed, or it did not
match the crt.sh row; a later sweep retries), caa_unverified (the CAA set for a
name on it was not read), unmapped (issuer not in the built-in CA table;
compare by hand), new_issuer. names_source on each finding is "certificate" or
"crtsh". totals counts every certificate in the window by status, routine
included. A name whose CAA could not be read is also listed under unverified.
Reads the local store only.`,
		Example:     "  domain-cert-tracker-cli issuers audit --days 7 --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			d, err := parseDaysFlag("days", days)
			if err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "issuers audit")
			}
			since := time.Now().UTC().AddDate(0, 0, -d)
			db, err := openTrackerRead(cmd.Context())
			if err != nil {
				return err
			}
			res := tracker.EmptyIssuerAudit(since)
			if db != nil {
				defer db.Close()
				if res, err = tracker.AuditIssuers(cmd.Context(), db, client, since); err != nil {
					return trackerErr(err)
				}
			}
			return printJSONFilteredKeep(cmd.OutOrStdout(), res, flags, "caa_issuers")
		},
	}
	cmd.Flags().StringVar(&client, "client", "", "Only this inventoried client")
	cmd.Flags().StringVar(&days, "days", "7", "Look at certificates issued within this many days")
	return cmd
}
