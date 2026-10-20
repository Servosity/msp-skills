// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: change seats, renewal, status, tier or friendly name on one subscription
// by building the documented UPDATE order (docs.streamone.cloud#update-order-*).
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"td-synnex-pp-cli/internal/cliutil"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		subsCmd, _, err := root.Find([]string{"subscriptions"})
		if err != nil || subsCmd == nil || subsCmd == root {
			return
		}
		set := newSubscriptionsSetCmd(flags)
		wrapAccountDefault(set, flags) // this hook runs after ion_account_default.go's walk
		addNovelCommandIfAbsent(subsCmd, set)
	})
}

type subSetChange struct {
	Seats        int
	SeatsSet     bool
	Renewal      string
	Status       string
	FriendlyName string
	UpgradeSKU   string
	UpgradePlan  string
}

var validRenewalSettings = map[string]bool{"auto_on": true, "cancel_at_renewal": true, "renew_to_est": true, "auto-on": true, "auto-off": true}
var validStatusUpdates = map[string]bool{"suspended": true, "active": true, "deleted": true}

// buildSubscriptionUpdate turns a requested change into the UPDATE order body
// ION documents for it. Status changes and tier changes are separate orders;
// seats, renewal setting and friendly name can be combined.
func buildSubscriptionUpdate(s ionSub, ch subSetChange) (map[string]any, error) {
	upgrade := ch.UpgradeSKU != "" || ch.UpgradePlan != ""
	kinds := 0
	if ch.SeatsSet || ch.Renewal != "" || ch.FriendlyName != "" {
		kinds++
	}
	if ch.Status != "" {
		kinds++
	}
	if upgrade {
		kinds++
	}
	switch {
	case kinds == 0:
		return nil, fmt.Errorf("nothing to change: pass --seats, --renewal, --friendly-name, --status, or --upgrade-sku/--upgrade-plan")
	case kinds > 1:
		return nil, fmt.Errorf("--status and --upgrade-sku/--upgrade-plan each need their own order; combine only --seats, --renewal and --friendly-name")
	}
	if ch.Renewal != "" && !validRenewalSettings[ch.Renewal] {
		return nil, fmt.Errorf("--renewal must be auto_on, cancel_at_renewal or renew_to_est (IONModule also uses auto-on/auto-off); got %q", ch.Renewal)
	}
	if ch.Status != "" && !validStatusUpdates[ch.Status] {
		return nil, fmt.Errorf("--status must be suspended, active or deleted; got %q", ch.Status)
	}
	if ch.SeatsSet && ch.Seats < 1 {
		return nil, fmt.Errorf("--seats must be at least 1 (use --status deleted to remove a subscription)")
	}
	sku, plan := s.CCPSkuID, s.CCPPlanID
	if ch.UpgradeSKU != "" {
		sku = ch.UpgradeSKU
	}
	if ch.UpgradePlan != "" {
		plan = ch.UpgradePlan
	}
	missing := make([]string, 0)
	for name, v := range map[string]string{"ccpProductId": s.CCPProductID, "ccpSkuId": sku, "ccpPlanId": plan, "subscriptionId": s.SubscriptionID} {
		if strings.TrimSpace(v) == "" || strings.Contains(v, redacted) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("subscription %s lacks %s, so an UPDATE order cannot be built from it", s.ID, strings.Join(missing, ", "))
	}
	quantity := int(math.Round(s.Seats.V))
	if ch.SeatsSet {
		quantity = ch.Seats
	} else if !s.Seats.Set || quantity < 1 || s.Seats.V != math.Trunc(s.Seats.V) {
		// A fractional count (1.5) would be rounded into a seat change nobody
		// asked for (adversarial round 28, Codex round 30, P1).
		// Renewal, name and status orders resend the current seat count; never send 0.
		return nil, fmt.Errorf("subscription %s reports %s seats, so the order would not carry the real quantity; pass --seats explicitly", s.ID, seatsText(s.Seats.V, s.Seats.Set))
	}
	op := "updateSubscription"
	if upgrade {
		op = "changeSubscription"
	}
	attrs := []map[string]string{{"name": "operations", "value": op}}
	if ch.Renewal != "" {
		attrs = append(attrs, map[string]string{"name": "renewalSetting", "value": ch.Renewal})
	}
	if ch.Status != "" {
		attrs = append(attrs, map[string]string{"name": "statusUpdate", "value": ch.Status})
	}
	if ch.FriendlyName != "" {
		attrs = append(attrs, map[string]string{"name": "friendlyName", "value": ch.FriendlyName})
	}
	item := map[string]any{
		"action":     "UPDATE",
		"productId":  s.CCPProductID,
		"skuId":      sku,
		"planId":     plan,
		"quantity":   quantity,
		"resourceId": s.SubscriptionID,
		"attributes": attrs,
	}
	if s.CloudProviderID != "" && ch.FriendlyName != "" {
		item["providerId"] = s.CloudProviderID // the documented friendlyName recipe carries providerId
	}
	return map[string]any{"orderItems": []any{item}}, nil
}

// decimalInt is an int flag read in decimal only: pflag's int reads 012 as
// octal 10, a seat change nobody typed (adversarial round 30). Its type stays
// "int", so the MCP tool schema is unchanged.
type decimalInt struct {
	v   *int
	set bool
}

func (d *decimalInt) String() string { return strconv.Itoa(*d.v) }
func (d *decimalInt) Type() string   { return "int" }
func (d *decimalInt) Set(s string) error {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return fmt.Errorf("must be a whole number in decimal, got %q", s)
	}
	*d.v, d.set = n, true
	return nil
}

func newSubscriptionsSetCmd(flags *rootFlags) *cobra.Command {
	var ch subSetChange
	seatsFlag := &decimalInt{v: &ch.Seats}
	var confirm bool

	cmd := &cobra.Command{
		Use:   "set <accountId> <customerId> <subscriptionId>",
		Short: "Change seats, renewal setting, status, tier or friendly name on a subscription (preview by default)",
		Long: "Change one subscription by placing the UPDATE order ION documents for it. The command reads the subscription " +
			"live (subscriptions get), fills productId, skuId, planId and the vendor resourceId from it, and prints the exact " +
			"order body. Nothing is placed until you add --confirm.\n\n" +
			"--seats sets the licence count (also during renewal). --renewal sets the renewal setting (auto_on, " +
			"cancel_at_renewal, renew_to_est). --friendly-name sets the subscription name. These three can be combined. " +
			"--status suspended|active|deleted suspends, reactivates or deletes, and --upgrade-sku/--upgrade-plan moves to " +
			"another tier; each of those is its own order. Omit <accountId> when TD_SYNNEX_ACCOUNT_ID is set. subscriptionId " +
			"is the ION subscription id from subscriptions list " +
			"(the id field), not the vendor GUID. Vendor rules still apply: NCE seat decreases and cancellations are only " +
			"allowed inside Microsoft's window.",
		Example: strings.Trim(`
  td-synnex-cli subscriptions set 10505 108536 1608355 --seats 12
  td-synnex-cli subscriptions set 10505 108536 1608355 --seats 12 --confirm
  td-synnex-cli subscriptions set 10505 108536 1608355 --renewal cancel_at_renewal
  td-synnex-cli subscriptions set 10505 108536 1608355 --status suspended --confirm`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "pp:preview-happy-path": "true",
			"pp:happy-args": "accountId=10505;customerId=108536;subscriptionId=1608355;--seats=12", "td:account-positional": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "subscriptions set (build an UPDATE order from the subscription)")
			}
			if len(args) < 3 || strings.TrimSpace(args[0]) == "" || strings.TrimSpace(args[1]) == "" || strings.TrimSpace(args[2]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("accountId (or TD_SYNNEX_ACCOUNT_ID), customerId and subscriptionId are required"))
			}
			// A --profile sets the value without marking Changed (Codex round 31).
			// seatsFlag records any value set, from the command line or a
			// --profile (which does not mark Changed), so a saved 0 is refused
			// like a typed one (Codex rounds 31 and 32).
			ch.SeatsSet = cmd.Flags().Changed("seats") || seatsFlag.set
			ch.Renewal = strings.ToLower(strings.TrimSpace(ch.Renewal))
			ch.Status = strings.ToLower(strings.TrimSpace(ch.Status))
			if _, err := buildSubscriptionUpdate(ionSub{ID: args[2], CCPProductID: "x", CCPSkuID: "x", CCPPlanID: "x", SubscriptionID: "x", Seats: optFloat{V: 1, Set: true}}, ch); err != nil {
				return usageErr(err) // validate flags before any network call
			}
			if confirm && cliutil.IsAnyHarness() {
				return writeHarnessRefusal(cmd.OutOrStdout(), flags, "place a subscription UPDATE order")
			}
			c, err := flags.newClient()
			if err != nil {
				return configErr(err)
			}
			accountID, customerID, subID := url.PathEscape(args[0]), url.PathEscape(args[1]), url.PathEscape(args[2])
			// Read fresh: a cached copy could carry an old seat count into the order and revert a change.
			data, err := c.GetNoCache(cmd.Context(), "/accounts/"+accountID+"/customers/"+customerID+"/subscriptions/"+subID, nil)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			// The write path decodes strictly: a field of an unexpected type
			// fails closed instead of reading as absent (adversarial round 20:
			// an unreadable customerId skipped the ownership check).
			var sub ionSub
			type strictSub ionSub
			var check ionSub // the licence book's own rules too: a null status or trial flag is unreadable (Codex round 26, P1)
			if err := json.Unmarshal(data, &check); err != nil {
				return apiErr(fmt.Errorf("decoding subscription %s: %w", args[2], err))
			}
			if err := json.Unmarshal(data, (*strictSub)(&sub)); err != nil {
				return apiErr(fmt.Errorf("decoding subscription %s: %w", args[2], err))
			}
			if sub.ID == "" {
				sub.ID = args[2]
			}
			// The order is posted under args[1]; a subscription that says it belongs
			// to another customer must not be ordered there.
			if sub.CustomerID != "" && sub.CustomerID != args[1] {
				return apiErr(fmt.Errorf("subscription %s belongs to customer %s, not %s; nothing was ordered", args[2], sub.CustomerID, args[1]))
			}
			body, err := buildSubscriptionUpdate(sub, ch)
			if err != nil {
				return apiErr(err) // the flags were validated above; this is the API's subscription missing fields
			}
			orderPath := "/accounts/" + accountID + "/customers/" + customerID + "/orders"
			current := map[string]any{"subscription": sub.Label(), "seats": seatsJSON(sub.Seats), "status": sub.Status,
				"auto_renew": sub.AutoRenewState(), "product_id": sub.CCPProductID, "sku_id": sub.CCPSkuID, "plan_id": sub.CCPPlanID}
			if !confirm {
				// The order body passes through the response redactor like --dry-run,
				// so a friendly name holding a password prints as [redacted]
				// (Codex round 21, P1); current came through the transport redactor
				// already. The order itself is sent as given.
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"preview": true, "would_post": orderPath, "current": current, "body": redactedValue(body),
					"next": "re-run with --confirm to place this order",
				}, flags)
			}
			resp, _, err := c.Post(cmd.Context(), orderPath, body)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			var order any
			if err := json.Unmarshal(redactSecretsJSON(resp), &order); err != nil {
				order = string(resp)
			}
			return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"placed": true, "order": order}, flags)
		},
	}
	cmd.Flags().Var(seatsFlag, "seats", "New licence count (a whole number)")
	cmd.Flags().StringVar(&ch.Renewal, "renewal", "", "Renewal setting: auto_on, cancel_at_renewal or renew_to_est (auto-on and auto-off are accepted aliases; auto-off cancels at renewal)")
	cmd.Flags().StringVar(&ch.Status, "status", "", "Status change: suspended, active or deleted")
	cmd.Flags().StringVar(&ch.FriendlyName, "friendly-name", "", "New friendly name for the subscription")
	cmd.Flags().StringVar(&ch.UpgradeSKU, "upgrade-sku", "", "Target ION SKU ID for a tier change")
	cmd.Flags().StringVar(&ch.UpgradePlan, "upgrade-plan", "", "Target ION plan ID for a tier change")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Place the order (without it the command only previews)")
	return cmd
}
