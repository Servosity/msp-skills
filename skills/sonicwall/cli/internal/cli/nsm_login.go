// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"sonicwall-pp-cli/internal/config"
)

// nsmCloudTenant is one entry of content.arrTenants in MySonicWall
// GET /api/hgms/get-cloud-tenants.
type nsmCloudTenant struct {
	Name           string          `json:"name"`
	ProductGroupID json.RawMessage `json:"productGroupID"`
	CloudServices  []struct {
		ServiceName     string `json:"serviceName"`
		ServiceNameDisp string `json:"serviceNameDisp"`
		TenantSerial    string `json:"tenantSerial"`
	} `json:"cloudServices"`
}

type nsmLoginCandidate struct {
	Name           string `json:"tenant"`
	ProductGroupID string `json:"product_group_id"`
	TenantSerial   string `json:"tenant_serial"`
	// MatchedBy is "service-name" when the serial came from a cloud service named
	// NSM / Network Security Manager, "first-serial" when it is the tenant's only
	// fallback (the FortiSOAR connector's behaviour) and may not be NSM.
	MatchedBy string `json:"matched_by"`
}

// nsmLoginCandidates keeps tenants that carry an NSM tenantSerial, preferring a
// cloud service named NSM / Network Security Manager over any other serial.
func nsmLoginCandidates(tenants []nsmCloudTenant) []nsmLoginCandidate {
	out := make([]nsmLoginCandidate, 0)
	for _, t := range tenants {
		serial, matched := "", ""
		for _, s := range t.CloudServices {
			if s.TenantSerial == "" {
				continue
			}
			label := strings.ToLower(s.ServiceName + " " + s.ServiceNameDisp)
			if strings.Contains(label, "nsm") || strings.Contains(label, "network security manager") {
				serial, matched = s.TenantSerial, "service-name"
				break
			}
			if serial == "" {
				serial, matched = s.TenantSerial, "first-serial"
			}
		}
		if serial == "" {
			continue
		}
		out = append(out, nsmLoginCandidate{Name: t.Name, ProductGroupID: strings.Trim(string(t.ProductGroupID), `"`), TenantSerial: serial, MatchedBy: matched})
	}
	return out
}

func pickNSMTenant(cands []nsmLoginCandidate, want string) (nsmLoginCandidate, error) {
	// Serials from cloud services named NSM win. Fallback serials (a tenant with
	// no NSM-named service) are only offered when no tenant has one, and then
	// only by explicit --tenant, never picked automatically.
	named := make([]nsmLoginCandidate, 0, len(cands))
	for _, c := range cands {
		if c.MatchedBy == "service-name" {
			named = append(named, c)
		}
	}
	if len(named) > 0 {
		cands = named
	} else if len(cands) > 0 && want == "" {
		return nsmLoginCandidate{}, usageErr(fmt.Errorf("no cloud service named NSM was found; these tenants have other cloud-service serials, pick one explicitly with --tenant if it is your NSM subscription: %s", describeCandidates(cands)))
	}
	if want != "" {
		for _, c := range cands {
			if c.ProductGroupID == want || strings.EqualFold(c.Name, want) {
				return c, nil
			}
		}
		return nsmLoginCandidate{}, usageErr(fmt.Errorf("no NSM tenant matches --tenant %q; available: %s", want, describeCandidates(cands)))
	}
	switch len(cands) {
	case 0:
		return nsmLoginCandidate{}, authErr(fmt.Errorf("no MySonicWall cloud tenant with an NSM tenant serial was returned for this API key; check that the key's user can see your NSM subscription"))
	case 1:
		return cands[0], nil
	}
	return nsmLoginCandidate{}, usageErr(fmt.Errorf("this API key sees %d NSM tenants; pick one with --tenant: %s", len(cands), describeCandidates(cands)))
}

func describeCandidates(cands []nsmLoginCandidate) string {
	parts := make([]string, 0, len(cands))
	for _, c := range cands {
		parts = append(parts, fmt.Sprintf("%q (product group %s)", c.Name, c.ProductGroupID))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func newNsmLoginCmd(flags *rootFlags) *cobra.Command {
	var tenant string
	cmd := &cobra.Command{
		Use:   "nsm-login",
		Short: "Mint an NSM session token from SONICWALL_MSW_API_KEY and save it; run before NSM commands and again when they exit 4 (tokens last 8 hours, 15 minutes idle)",
		Long: "Mint an NSM session token from your MySonicWall API key and save it for every later NSM call.\n\n" +
			"Runs SonicWall's documented exchange: list your MySonicWall cloud tenants, request an access code\n" +
			"for the NSM tenant (tileName ISNSMSAFEENABLED), then trade it at NSM /api/manager/auth/sso for a\n" +
			"bearer token. The token lasts 8 hours with a 15-minute idle timeout; rerun this command when NSM\n" +
			"calls start exiting with code 4. Needs SONICWALL_MSW_API_KEY. EU portals: export SONICWALL_REGION=eucentral.\n" +
			"The token is never printed.",
		Example: strings.Trim(`
  sonicwall-cli nsm-login
  sonicwall-cli nsm-login --tenant "Acme Corp" --json`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "nsm-login")
			}
			if !mswKeyConfigured(flags) {
				return authErr(fmt.Errorf("SONICWALL_MSW_API_KEY is not set: generate a key in MySonicWall (My Workspace > User Groups > User list > Generate My API Key) and export it"))
			}
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}

			raw, err := c.Get(ctx, mswBase+"/api/hgms/get-cloud-tenants", nil)
			if err != nil {
				return classifyReadError(cmd.OutOrStdout(), fmt.Errorf("step 1/3 get-cloud-tenants: %w", err), flags)
			}
			var tenants struct {
				Content struct {
					ArrTenants []nsmCloudTenant `json:"arrTenants"`
				} `json:"content"`
			}
			if err := json.Unmarshal(raw, &tenants); err != nil {
				return apiErr(fmt.Errorf("step 1/3 get-cloud-tenants: unexpected response: %w", err))
			}
			chosen, err := pickNSMTenant(nsmLoginCandidates(tenants.Content.ArrTenants), tenant)
			if err != nil {
				return err
			}

			codeRaw, _, err := c.Post(ctx, mswBase+"/api/generate-cscAccessCode", map[string]string{
				"tenantId": chosen.ProductGroupID,
				"tileName": "ISNSMSAFEENABLED",
			})
			if err != nil {
				return classifyReadError(cmd.OutOrStdout(), fmt.Errorf("step 2/3 generate-cscAccessCode: %w", err), flags)
			}
			var code struct {
				Content struct {
					AccessCode json.RawMessage `json:"accessCode"`
				} `json:"content"`
			}
			_ = json.Unmarshal(codeRaw, &code)
			accessCode := strings.Trim(string(code.Content.AccessCode), `"`)
			if accessCode == "" || accessCode == "null" {
				return apiErr(fmt.Errorf("step 2/3 generate-cscAccessCode: no content.accessCode in the response"))
			}

			ssoRaw, _, err := c.Post(ctx, "/auth/sso", map[string]string{
				"tenantserial": chosen.TenantSerial,
				"code":         accessCode,
				"client":       "nsm-api",
			})
			if err != nil {
				return classifyReadError(cmd.OutOrStdout(), fmt.Errorf("step 3/3 NSM /auth/sso: %w", err), flags)
			}
			var sso struct {
				Status struct {
					Success *bool `json:"success"`
					Info    []struct {
						Message string `json:"message"`
					} `json:"info"`
				} `json:"status"`
			}
			if err := json.Unmarshal(ssoRaw, &sso); err != nil {
				return apiErr(fmt.Errorf("step 3/3 NSM /auth/sso: unexpected response: %w", err))
			}
			// Only an explicit success envelope carries a token; anything else is not saved.
			if sso.Status.Success == nil || !*sso.Status.Success || len(sso.Status.Info) == 0 || strings.TrimSpace(sso.Status.Info[0].Message) == "" {
				return authErr(fmt.Errorf("step 3/3 NSM /auth/sso did not return a token (check SONICWALL_REGION matches your NSM portal)"))
			}
			token := strings.TrimSpace(sso.Status.Info[0].Message)
			if strings.ContainsAny(token, " \t\r\n") {
				return apiErr(fmt.Errorf("step 3/3 NSM /auth/sso returned a message that is not a token; nothing saved"))
			}

			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			shadowed, err := cfg.SaveNSMToken(token)
			if err != nil {
				return configErr(fmt.Errorf("saving NSM token: %w", err))
			}
			if shadowed {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: SONICWALL_NSM_TOKEN is set in the environment and overrides the saved token; unset it to use this one")
			}
			if lerr := cfg.SaveNSMLogin(config.NSMLogin{Tenant: chosen.Name, ProductGroupID: chosen.ProductGroupID,
				TenantSerial: chosen.TenantSerial, Region: cfg.TemplateVars["region"]}, token); lerr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not record the NSM tenant for history scoping: %v\n", lerr)
			}
			result := map[string]any{
				"saved":            true,
				"tenant":           chosen.Name,
				"product_group_id": chosen.ProductGroupID,
				"tenant_serial":    chosen.TenantSerial,
				"region":           cfg.TemplateVars["region"],
				"credentials_path": credentialSavePath(cfg),
				"valid_for":        "8h, 15m idle timeout",
				"matched_by":       chosen.MatchedBy,
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), result, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "NSM token saved for tenant %q (product group %s, region %s). Valid 8h, 15m idle timeout.\n",
				chosen.Name, chosen.ProductGroupID, cfg.TemplateVars["region"])
			return nil
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "MySonicWall tenant name or product group ID to log in to, when the key sees more than one NSM tenant")
	return cmd
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		addNovelCommandIfAbsent(root, newNsmLoginCmd(flags))
	})
}
