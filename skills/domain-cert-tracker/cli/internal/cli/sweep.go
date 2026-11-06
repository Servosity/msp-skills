// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (handfixes.json tracker-novels).
// pp:data-source live

package cli

import (
	"fmt"
	"strings"
	"time"

	"domain-cert-tracker-pp-cli/internal/cliutil"
	"domain-cert-tracker-pp-cli/internal/tracker"

	"github.com/spf13/cobra"
)

func newNovelSweepCmd(flags *rootFlags) *cobra.Command {
	var client, days, only string
	var allowPorts []int
	var concurrency int
	cmd := &cobra.Command{
		Use:   "sweep",
		Short: "Check every inventoried domain's registration, new CT certificates, live TLS and DNS in one run, then store and diff against the last run.",
		Long: `Use this command to fetch fresh RDAP, CT, TLS and DNS observations for the
inventory and write the diff as events. Do NOT use this command to read what
is expiring; use 'report' instead, which reads the last sweep without network
calls.

RDAP goes straight to each registry through the IANA bootstrap file (rdap.org
allows only 10 lookups in 10 seconds). A check that fails is stored as a
failure and listed in fetch_failures: it never replaces the last good result,
never moves the CT cursor and never raises a change event. crt.sh's search
lists only the names on each certificate that matched the domain, so the CT
check also downloads each certificate it has not read before from crt.sh
(https://crt.sh/?d=<id>, newest first, at most 50 per sweep and half of each
CT check's time, at crt.sh's rate limit) and keeps its full SAN list once the
download matches the crt.sh row's issuer, serial and validity; a verified
certificate is never downloaded again. Certificates past the cap, or whose
download fails, keep crt.sh's partial names until a later sweep (issuers audit
reports them as names_partial), and cert_names counts them (complete,
downloaded, partial, download_failed, deferred). The CT check then records the
CAA set that applies to every name on those certificates, for issuers audit. A
crt.sh answer truncated at its 10,000-identity limit is a failed CT check,
never a partial history. Hosts on ports other than 443 are probed only when
this run passes --allow-port on the command line. The command exits non-zero
when every check failed.`,
		Example: strings.Trim(`
  domain-cert-tracker-cli sweep --agent
  domain-cert-tracker-cli sweep --client "Acme Dental" --only rdap,tls`, "\n"),
		Annotations: map[string]string{"mcp:local-write": "true", "pp:data-source": "live", "mcp:read-flags": "allow-port", "pp:live-happy-path": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			d, err := parseDaysFlag("days", days)
			if err != nil {
				return err
			}
			kinds := map[string]bool{}
			for _, k := range splitList([]string{only}) {
				switch k {
				case tracker.KindRDAP, tracker.KindCT, tracker.KindTLS, tracker.KindDNS:
					kinds[k] = true
				default:
					return usageErr(fmt.Errorf("--only %q: use rdap, ct, tls or dns", k))
				}
			}
			if concurrency < 1 || concurrency > 4 {
				return usageErr(fmt.Errorf("--concurrency must be between 1 and 4"))
			}
			g, err := trackerGuard(allowPorts)
			if err != nil {
				return err
			}
			if dryRunOK(flags) || cliutil.IsVerifyEnv() {
				return writeDryRun(cmd.OutOrStdout(), flags, "sweep")
			}
			db, err := openTrackerWrite(cmd.Context())
			if err != nil {
				return err
			}
			defer db.Close()
			opt := tracker.SweepOptions{Client: client, Days: d, Kinds: kinds, Concurrency: concurrency}
			if flags.timeout > 0 {
				opt.CheckTime = flags.timeout
			}
			if cliutil.IsDogfoodEnv() {
				// The live matrix gives each command 30 seconds: check a few
				// subjects for real instead of the whole inventory.
				opt.MaxSubjects, opt.CheckTime = 4, 6*time.Second
			}
			res, err := tracker.RunSweep(cmd.Context(), db, trackerSources(g), opt)
			if err != nil && res.SweepID == 0 {
				return trackerErr(err)
			}
			if len(res.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d checks failed; they are listed in fetch_failures and as unverified in report\n",
					len(res.FetchFailures), totalChecks(res))
			}
			if perr := flags.printJSON(cmd, res); perr != nil {
				return perr
			}
			return err
		},
	}
	cmd.Flags().StringVar(&client, "client", "", "Only this inventoried client")
	cmd.Flags().StringVar(&days, "days", "30", "Raise expiring events for domains and certificates within this many days")
	cmd.Flags().StringVar(&only, "only", "", "Comma-separated checks to run: rdap, ct, tls, dns (default all)")
	cmd.Flags().IntVar(&concurrency, "concurrency", 3, "Parallel checks (1 to 4)")
	// mcp:read-flags drops --allow-port from the MCP schema; trackerGuard
	// also refuses it at run time under the MCP server.
	cmd.Flags().IntSliceVar(&allowPorts, "allow-port", nil, "Also allow this TCP port for TLS probes in this run (command line only)")
	return cmd
}

func totalChecks(r tracker.SweepResult) int {
	n := 0
	for _, k := range r.Checks {
		n += k["ok"] + k["failed"]
	}
	return n
}
