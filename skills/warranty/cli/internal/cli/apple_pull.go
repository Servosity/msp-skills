// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (handfixes.json warranty-novels).
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"warranty-pp-cli/internal/client"
	"warranty-pp-cli/internal/cliutil"
	"warranty-pp-cli/internal/store"
	"warranty-pp-cli/internal/warranty"

	"github.com/spf13/cobra"
)

type appleOrgPage struct {
	Data []struct {
		ID         string `json:"id"`
		Attributes struct {
			SerialNumber            string `json:"serialNumber"`
			DeviceModel             string `json:"deviceModel"`
			ProductType             string `json:"productType"`
			OrderDateTime           string `json:"orderDateTime"`
			ReleasedFromOrgDateTime string `json:"releasedFromOrgDateTime"`
		} `json:"attributes"`
	} `json:"data"`
	Meta struct {
		Paging struct {
			NextCursor string `json:"nextCursor"`
		} `json:"paging"`
	} `json:"meta"`
}

type applePullView struct {
	Customer         string   `json:"customer"`
	ScannedDevices   int      `json:"scanned_devices"`
	Pages            int      `json:"pages"`
	MaxPages         int      `json:"max_pages"`
	Complete         bool     `json:"complete"`
	NewInOrg         []string `json:"new_in_org"`
	Released         []string `json:"released_from_org"`
	NotInImport      []string `json:"in_org_not_in_import"`
	ImportedNotInOrg []string `json:"imported_not_in_org"`
	MovedFrom        []string `json:"moved_from_other_customer"`
	Saved            bool     `json:"saved"`
	Note             string   `json:"note,omitempty"`
}

func newNovelApplePullCmd(flags *rootFlags) *cobra.Command {
	var customer string
	var maxPages int
	var confirm bool
	cmd := &cobra.Command{
		Use:   "apple-pull",
		Short: "Load one client's Apple devices from its Apple Business organization and see what is new, gone",
		Long: `Use this command to load or refresh one client's device list from its Apple
Business organization. Do NOT use this command to load an RMM CSV or JSON
export; use 'import' instead. Do NOT use it to fetch AppleCare coverage for a
serial; use 'lookup' instead.

Pages GET /orgDevices (1000 devices a page, up to --max-pages) with the Apple
Business API account in WARRANTY_APPLE_CLIENT_ID, WARRANTY_APPLE_KEY_ID and
WARRANTY_APPLE_PRIVATE_KEY, which belongs to one organization: name its
customer with --customer. Each device is saved under that customer with its
model and order date. Reports new_in_org, released_from_org,
in_org_not_in_import (Macs the RMM export never listed) and
imported_not_in_org (Apple serials imported for this customer that the
organization does not hold). Only a complete pull reports imported_not_in_org.
Over MCP the pull is a preview unless --confirm is passed. Run recheck
--vendor apple --customer <name> afterwards for coverage. Exits 10
when the Apple Business variables are not set.`,
		Example: strings.Trim(`
  warranty-cli apple-pull --customer "Acme Dental" --agent
  warranty-cli apple-pull --customer "Acme Dental" --max-pages 2 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "mcp:read-only-unless-flags": "confirm", "pp:data-source": "live", "pp:typed-exit-codes": "0,10", "pp:happy-args": "--customer=Acme Dental;--max-pages=1"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "page the Apple Business organization's devices into the mirror")
			}
			customer = normalizeCustomer(customer)
			if customer == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--customer is required: the Apple Business account belongs to one customer's organization"))
			}
			if maxPages < 1 || maxPages > 100 {
				return usageErr(fmt.Errorf("--max-pages must be between 1 and 100"))
			}
			if cliutil.IsDogfoodEnv() || cliutil.IsMCPChildEnv() {
				maxPages = 1
			}
			view := applePullView{Customer: customer, MaxPages: maxPages, NewInOrg: []string{}, Released: []string{},
				NotInImport: []string{}, ImportedNotInOrg: []string{}, MovedFrom: []string{}}
			if _, unset := client.AppleCredentialsFromEnv(); len(unset) > 0 {
				view.Note = "Apple Business is not configured: set " + strings.Join(unset, ", ")
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					if err := printJSONFiltered(cmd.OutOrStdout(), view, flags); err != nil {
						return err
					}
				}
				return configErr(fmt.Errorf("%s", view.Note))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ac := c.WithTier("apple")
			type orgDevice struct{ serial, model, ordered string }
			pulled := []orgDevice{}
			released := map[string]bool{}
			cursor := ""
			for view.Pages < maxPages {
				params := map[string]string{"limit": "1000"}
				if cursor != "" {
					params["cursor"] = cursor
				}
				body, err := ac.GetNoCache(ctx, "/orgDevices", params)
				if err != nil {
					return classifyAPIErrorOnly(err)
				}
				var page appleOrgPage
				if err := json.Unmarshal(body, &page); err != nil || page.Data == nil {
					return apiErr(fmt.Errorf("Apple orgDevices: unexpected response (no data array)"))
				}
				view.Pages++
				for _, d := range page.Data {
					s := warranty.NormalizeSerial(d.Attributes.SerialNumber)
					if s == "" {
						s = warranty.NormalizeSerial(d.ID)
					}
					if !warranty.ValidSerial(s) {
						continue
					}
					view.ScannedDevices++
					if d.Attributes.ReleasedFromOrgDateTime != "" {
						released[s] = true
						view.Released = append(view.Released, s)
						continue
					}
					pulled = append(pulled, orgDevice{s, firstNonBlank(d.Attributes.DeviceModel, d.Attributes.ProductType), warranty.Day(d.Attributes.OrderDateTime)})
				}
				cursor = page.Meta.Paging.NextCursor
				if cursor == "" {
					view.Complete = true
					break
				}
			}
			if !view.Complete {
				view.Note = fmt.Sprintf("stopped after %d page(s) (--max-pages): raise it for the rest; imported_not_in_org needs a complete pull", view.Pages)
			}
			save := mirrorWriteAllowed()
			var db *store.Store
			if save {
				db, err = openWarrantyWrite(ctx)
			} else {
				db, err = openWarrantyRead(ctx)
			}
			if err != nil {
				return fmt.Errorf("opening the warranty mirror: %w", err)
			}
			before := []store.Device{}
			if db != nil {
				defer db.Close()
				if before, err = db.ListDevices(ctx, customer); err != nil {
					return err
				}
			}
			inOrg := map[string]bool{}
			for _, d := range pulled {
				inOrg[d.serial] = true
				var prev store.Device
				existed := false
				if db != nil {
					if prev, existed, err = db.GetDevice(ctx, d.serial); err != nil {
						return err
					}
				}
				up := store.UpsertResult{Created: !existed}
				if existed && prev.Customer != "" && !strings.EqualFold(prev.Customer, customer) {
					up.PrevCustomer = prev.Customer
				}
				if save {
					if up, err = db.UpsertDevice(ctx, store.DeviceUpsert{Serial: d.serial, Vendor: warranty.Apple, VendorSource: warranty.SourceDeclared,
						Customer: customer, Source: store.DeviceSourceAppleOrg, Model: d.model, PurchaseDate: d.ordered}); err != nil {
						return fmt.Errorf("saving %s: %w", d.serial, err)
					}
				}
				switch {
				case up.Created:
					view.NewInOrg = append(view.NewInOrg, d.serial)
				case up.PrevCustomer != "":
					view.MovedFrom = append(view.MovedFrom, d.serial+" (from "+up.PrevCustomer+")")
				}
				if !existed || prev.Source != store.DeviceSourceImport {
					view.NotInImport = append(view.NotInImport, d.serial)
				}
			}
			if view.Complete {
				for _, d := range before {
					if d.Vendor == warranty.Apple && d.Source == store.DeviceSourceImport && !inOrg[d.Serial] && !released[d.Serial] {
						view.ImportedNotInOrg = append(view.ImportedNotInOrg, d.Serial)
					}
				}
			}
			for _, l := range [][]string{view.NewInOrg, view.Released, view.NotInImport, view.ImportedNotInOrg, view.MovedFrom} {
				sort.Strings(l)
			}
			view.Saved = save
			if !save {
				view.Note = joinNote(view.Note, previewNote)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %d Apple device(s) in the organization (%d page(s)%s).\n", customer, len(pulled), view.Pages,
				map[bool]string{true: "", false: ", incomplete"}[view.Complete])
			for _, row := range []struct {
				label string
				list  []string
			}{{"new in org", view.NewInOrg}, {"released from org", view.Released}, {"in org, not in the RMM import", view.NotInImport},
				{"imported, not in org", view.ImportedNotInOrg}, {"moved from another customer", view.MovedFrom}} {
				fmt.Fprintf(cmd.OutOrStdout(), "  %-30s %s\n", row.label+" ("+strconv.Itoa(len(row.list))+"):", strings.Join(row.list, ", "))
			}
			if view.Note != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), view.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&customer, "customer", "", "Customer whose Apple Business organization the configured API account belongs to (required)")
	cmd.Flags().IntVar(&maxPages, "max-pages", 20, "Maximum pages of 1000 devices to read (1 to 100)")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Save the devices to the mirror when run over MCP (the CLI saves without it)")
	return cmd
}

func firstNonBlank(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
