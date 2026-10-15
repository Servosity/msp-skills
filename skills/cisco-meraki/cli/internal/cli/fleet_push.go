// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"cisco-meraki-pp-cli/internal/client"
	"cisco-meraki-pp-cli/internal/cliutil"
	"cisco-meraki-pp-cli/internal/store"
)

// actionBatchLimit is the Dashboard's per-batch action cap for asynchronous
// action batches.
const actionBatchLimit = 100

type pushRow struct {
	OrganizationID   string         `json:"organization_id"`
	OrganizationName string         `json:"organization_name"`
	NetworkID        string         `json:"network_id"`
	NetworkName      string         `json:"network_name"`
	Resource         string         `json:"resource"` // scrubbed for output
	path             string         // what the batch targets
	write            bool           // the real diff found a change (the shown one may be forced)
	Action           string         `json:"action"` // update | no-change | error
	ChangedKeys      []string       `json:"changed_keys"`
	Current          map[string]any `json:"current,omitempty"`
	Desired          map[string]any `json:"desired,omitempty"` // redacted copy for output
	body             map[string]any // what --apply sends
	Error            string         `json:"error,omitempty"`
	BatchID          string         `json:"batch_id,omitempty"`
	BatchStatus      string         `json:"batch_status,omitempty"`
}

type pushTargetFilter struct {
	networks    []string
	tag         string
	productType string
}

func (f pushTargetFilter) keep(id, name string, tags, productTypes []string) bool {
	if len(f.networks) > 0 && !containsFold(f.networks, id) && !containsFold(f.networks, name) {
		return false
	}
	if f.tag != "" && !containsFold(tags, f.tag) {
		return false
	}
	if f.productType != "" && !containsFold(productTypes, f.productType) {
		return false
	}
	return true
}

// pushFilterValues splits comma-separated filter values and drops blanks,
// the same normalization fleetOrgFilter applies.
func pushFilterValues(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

func containsFold(list []string, v string) bool {
	for _, s := range list {
		if strings.EqualFold(strings.TrimSpace(s), v) {
			return true
		}
	}
	return false
}

// diffSettings compares each top-level key of the desired body with the
// current settings. Values are compared as canonical JSON.
func diffSettings(current, desired map[string]any) (changed []string, cur map[string]any) {
	cur = map[string]any{}
	keys := make([]string, 0, len(desired))
	for k := range desired {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		a, _ := json.Marshal(current[k])
		b, _ := json.Marshal(desired[k])
		cur[k] = current[k]
		if !bytes.Equal(a, b) {
			changed = append(changed, k)
		}
	}
	return changed, cur
}

func validatePushPath(p string) (string, error) {
	p = "/" + strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(p), "/"), "api/v1/")
	if !strings.HasPrefix(p, "/networks/{networkId}/") {
		return "", fmt.Errorf("--path must be a network settings path like /networks/{networkId}/syslogServers")
	}
	if strings.Contains(strings.TrimPrefix(p, "/networks/{networkId}"), "{") || strings.ContainsAny(p, "?#") {
		return "", fmt.Errorf("--path may only contain the {networkId} placeholder and no query string")
	}
	return p, nil
}

func readPushBody(src string, stdin io.Reader) (map[string]any, error) {
	var data []byte
	var err error
	switch {
	case src == "-":
		data, err = io.ReadAll(stdin)
	case strings.HasPrefix(strings.TrimSpace(src), "{"):
		data = []byte(src)
	default:
		// --body names a local JSON file the caller chose to send; reading it is
		// the flag's purpose, and its content must parse as a JSON object.
		data, err = os.ReadFile(src)
	}
	if err != nil {
		return nil, fmt.Errorf("reading --body: %w", err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, fmt.Errorf("--body must be a JSON object: %w", err)
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("--body is empty; nothing to change")
	}
	return body, nil
}

func planOrgPush(ctx context.Context, c *client.Client, o fleetOrg, path string, body map[string]any, f pushTargetFilter) ([]pushRow, error) {
	if !o.APIEnabled {
		return nil, errOrgAPIDisabled
	}
	res, err := merakiGetAll(ctx, c, orgPath("/organizations/{organizationId}/networks", o.ID), nil, 0)
	if err != nil {
		return nil, fmt.Errorf("networks: %w", err)
	}
	// The preview prints a redacted copy of --body: a PSK the caller loaded
	// from a file never reaches Read-tier output.
	var desired map[string]any
	b, _ := json.Marshal(body)
	_ = json.Unmarshal(b, &desired)
	store.RedactMerakiObject(desired, path)
	rows := make([]pushRow, 0)
	for _, raw := range res.Items {
		var n struct {
			ID           string   `json:"id"`
			Name         string   `json:"name"`
			Tags         []string `json:"tags"`
			ProductTypes []string `json:"productTypes"`
		}
		if json.Unmarshal(raw, &n) != nil || !f.keep(n.ID, n.Name, n.Tags, n.ProductTypes) {
			continue
		}
		resource := strings.ReplaceAll(path, "{networkId}", url.PathEscape(n.ID))
		row := pushRow{OrganizationID: o.ID, OrganizationName: o.Name, NetworkID: n.ID, NetworkName: n.Name, Resource: store.ScrubMerakiURLSecrets(resource), path: resource, Desired: desired, body: body, ChangedKeys: []string{}}
		data, err := c.GetNoCache(ctx, resource, nil)
		if err != nil {
			row.Action, row.Error = "error", "reading current settings: "+pushErrText(path, err)
			rows = append(rows, row)
			continue
		}
		var current map[string]any
		if err := json.Unmarshal(data, &current); err != nil {
			row.Action, row.Error = "error", "current settings are not a JSON object; this path cannot be diffed"
			rows = append(rows, row)
			continue
		}
		changed, cur := diffSettings(current, body)
		row.write = len(changed) > 0
		store.RedactMerakiObject(cur, path) // a preview never prints a live PSK or RADIUS secret
		// Whether a secret matches is itself a secret: a key whose value
		// redaction touched always reports as changed, so the preview cannot
		// confirm a guessed PSK.
		for k := range body {
			if !reflect.DeepEqual(desired[k], body[k]) && !slices.Contains(changed, k) {
				changed = append(changed, k)
			}
		}
		sort.Strings(changed)
		row.Current = cur
		if len(changed) == 0 {
			row.Action = "no-change"
		} else {
			row.Action, row.ChangedKeys = "update", changed
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// pushErrText renders a push error. Under a webhook path the API's error body
// can echo the HTTP server ID (the receiver URL) in any encoding (JSON "\/",
// percent), so only the HTTP status survives; elsewhere the text is scrubbed.
func pushErrText(path string, err error) string {
	if !store.MerakiWebhookPath(path) {
		return store.ScrubMerakiURLSecrets(err.Error())
	}
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		return fmt.Sprintf("HTTP %d (details withheld under a webhook path)", apiErr.StatusCode)
	}
	return "request failed (details withheld under a webhook path)"
}

// pushStatusText keeps only the state word of a batch status under a webhook
// path ("failed: <errors>" becomes "failed"), for the same reason.
func pushStatusText(path, status string) string {
	if !store.MerakiWebhookPath(path) {
		return store.ScrubMerakiURLSecrets(status)
	}
	if i := strings.IndexAny(status, ":("); i > 0 {
		return strings.TrimSpace(status[:i]) + " (details withheld under a webhook path)"
	}
	return status
}

// applyOrgPush sends one asynchronous action batch per actionBatchLimit
// updates and polls each until it completes, fails, or wait runs out.
func applyOrgPush(ctx context.Context, c *client.Client, orgID string, rows []*pushRow, wait time.Duration) {
	for start := 0; start < len(rows); start += actionBatchLimit {
		end := min(start+actionBatchLimit, len(rows))
		chunk := rows[start:end]
		actions := make([]map[string]any, 0, len(chunk))
		for _, r := range chunk {
			actions = append(actions, map[string]any{"resource": r.path, "operation": "update", "body": r.body})
		}
		batchPath := orgPath("/organizations/{organizationId}/actionBatches", orgID)
		data, _, err := c.Post(ctx, batchPath, map[string]any{"confirmed": true, "synchronous": false, "actions": actions})
		if err != nil {
			for _, r := range chunk {
				r.BatchStatus = "rejected: " + pushErrText(r.path, err)
			}
			continue
		}
		var batch struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(data, &batch)
		status := pollActionBatch(ctx, c, batchPath+"/"+url.PathEscape(batch.ID), wait)
		for _, r := range chunk {
			r.BatchID, r.BatchStatus = batch.ID, pushStatusText(r.path, status)
		}
	}
}

func pollActionBatch(ctx context.Context, c *client.Client, path string, wait time.Duration) string {
	deadline := time.Now().Add(wait)
	var lastErr error
	for {
		data, err := c.GetNoCache(ctx, path, nil)
		if err != nil {
			lastErr = err
			// The batch was accepted; a 4xx on its status read will not heal.
			var apiErr *client.APIError
			if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 && apiErr.StatusCode != 429 {
				return "unknown: status read failed: " + err.Error()
			}
		}
		if err == nil {
			var b struct {
				Status struct {
					Completed bool     `json:"completed"`
					Failed    bool     `json:"failed"`
					Errors    []string `json:"errors"`
				} `json:"status"`
			}
			if derr := json.Unmarshal(data, &b); derr != nil {
				lastErr = fmt.Errorf("decoding batch status: %w", derr)
			} else {
				switch {
				case b.Status.Failed:
					return "failed: " + strings.Join(b.Status.Errors, "; ")
				case b.Status.Completed:
					return "completed"
				}
			}
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				return "pending (last status read failed: " + lastErr.Error() + ")"
			}
			return "pending (check: organizations action-batches get)"
		}
		select {
		case <-ctx.Done():
			return "pending: " + ctx.Err().Error()
		case <-time.After(2 * time.Second):
		}
	}
}

func newNovelFleetPushCmd(flags *rootFlags) *cobra.Command {
	var orgFilter, networkFilter []string
	var path, bodySrc, tag, productType, waitFlag string
	var apply, allNetworks bool
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Apply one network settings change to many networks across orgs via action batches; previews a per-network diff, sends only with --apply",
		Long: strings.TrimSpace(`
Applies one network settings change (a PUT body such as syslog servers, SNMP,
alert settings or an SSID field) to every matching network across the orgs the
API key can see. Without --apply it only previews: for each target network it
reads the current settings and shows which top-level keys would change, and
networks already matching are reported as no-change (except a credential such as a
PSK or community string, which always shows as update so the preview cannot confirm
a guess; --apply sends only real changes). With --apply it sends one
asynchronous action batch per org (up to 100 actions each) for the networks
that differ and waits for each batch to finish. Target networks with --org,
--network, --network-tag and --product-type, or pass --all-networks.

Use this command to apply one settings change to many networks across orgs through action batches, with a per-network diff before anything is sent.
Do NOT use this command to read data across orgs; use 'fleet fanout' instead.`),
		Example: strings.Trim(`
  cisco-meraki-cli fleet push --path /networks/{networkId}/syslogServers --body syslog.json --network-tag managed
  cisco-meraki-cli fleet push --path /networks/{networkId}/snmp --body '{"access":"none"}' --org "Acme Dental" --json
  cisco-meraki-cli fleet push --path /networks/{networkId}/syslogServers --body syslog.json --network-tag managed --apply`, "\n"),
		Annotations: map[string]string{
			"pp:data-source":             "live",
			"mcp:read-only-unless-flags": "apply",
			// handfixes.json: mcp-filesystem-source-gate. Over MCP --body must be
			// inline JSON; a path would read a caller-chosen local file.
			"mcp:inline-only-flags": "body",
			"pp:preview-happy-path": "true",
			"pp:happy-args":         `--path=/networks/{networkId}/syslogServers;--body={"servers":[]};--network-tag=managed`,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet push")
			}
			p, err := validatePushPath(path)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(err)
			}
			if bodySrc == "" {
				return usageErr(fmt.Errorf("--body is required: a JSON file, '-' for stdin, or inline JSON"))
			}
			// Judge targeting on the values the filters will actually use: a
			// blank --org " " is dropped by the org filter, so it must not count
			// as a target (handfixes.json: meraki-fleet-push-exit).
			orgFilter, networkFilter = pushFilterValues(orgFilter), pushFilterValues(networkFilter)
			tag, productType = strings.TrimSpace(tag), strings.TrimSpace(productType)
			targeted := len(orgFilter) > 0 || len(networkFilter) > 0 || tag != "" || productType != ""
			if !targeted && !allNetworks {
				return usageErr(fmt.Errorf("choose targets with --org, --network, --network-tag or --product-type, or pass --all-networks"))
			}
			wait, err := parseWindow("wait", waitFlag)
			if err != nil {
				return err
			}
			if apply && cliutil.IsAnyHarness() {
				return writeHarnessRefusal(cmd.OutOrStdout(), flags, "send action batches")
			}
			body, err := readPushBody(bodySrc, cmd.InOrStdin())
			if err != nil {
				return usageErr(err)
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, orgs, err := fleetClientOrgs(ctx, cmd, flags, orgFilter)
			if err != nil {
				return err
			}
			filter := pushTargetFilter{networks: networkFilter, tag: tag, productType: productType}
			outcomes := fanOrgs(ctx, orgs, func(ctx context.Context, o fleetOrg) ([]pushRow, error) {
				return planOrgPush(ctx, c, o, p, body, filter)
			})
			reportFleetFailures(cmd.ErrOrStderr(), outcomes)
			rows := make([]pushRow, 0)
			for _, oc := range outcomes {
				rows = append(rows, oc.Value...)
			}
			if apply {
				// --apply writes only real changes, so a matching secret is not
				// re-sent; the forced "update" stays in the Read-tier preview.
				for i := range rows {
					if rows[i].Action == "update" && !rows[i].write {
						rows[i].Action, rows[i].ChangedKeys = "no-change", []string{}
					}
				}
			}
			updates := 0
			for _, r := range rows {
				if r.Action == "update" {
					updates++
				}
			}
			if apply && updates > 0 {
				byOrg := map[string][]*pushRow{}
				for i := range rows {
					if rows[i].Action == "update" {
						byOrg[rows[i].OrganizationID] = append(byOrg[rows[i].OrganizationID], &rows[i])
					}
				}
				for orgID, orgRows := range byOrg {
					applyOrgPush(ctx, c, orgID, orgRows, wait)
				}
			} else if !apply && updates > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "preview only: %d network(s) would change; rerun with --apply to send action batches\n", updates)
			}
			if err := printFleetRows(cmd, flags, rows, []string{"organization_name", "network_name", "action", "changed_keys", "batch_status", "error"}, "No networks match the targets."); err != nil {
				return err
			}
			if apply {
				if failed, n := pushNotCompleted(rows, outcomes); n > 0 {
					// Exit 6 so automation and the MCP tool never read a change
					// that did not land as success. The MCP tool returns only
					// stderr on a non-zero exit, so the error names each target.
					return partialFailureErr(fmt.Errorf("%d target(s) did not complete (a pending batch may still finish in the Dashboard): %s", n, strings.Join(failed, "; ")))
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "Network settings path with {networkId}, e.g. /networks/{networkId}/syslogServers")
	cmd.Flags().StringVar(&bodySrc, "body", "", "Desired settings: inline JSON, a JSON file path, or '-' for stdin (MCP accepts inline JSON only, as a string)")
	addOrgFlag(cmd, &orgFilter)
	cmd.Flags().StringSliceVar(&networkFilter, "network", nil, "Limit to networks by ID or exact name (repeatable or comma-separated)")
	cmd.Flags().StringVar(&tag, "network-tag", "", "Only networks carrying this tag")
	cmd.Flags().StringVar(&productType, "product-type", "", "Only networks with this product type (appliance, switch, wireless, ...)")
	cmd.Flags().BoolVar(&allNetworks, "all-networks", false, "Target every network in every visible org (no other filter)")
	cmd.Flags().BoolVar(&apply, "apply", false, "Send the action batches; without it the command only previews")
	cmd.Flags().StringVar(&waitFlag, "wait", "2m", "How long to wait for each action batch to finish, e.g. 30s, 2m")
	return cmd
}

// pushNotCompleted names each target whose change did not land: an update
// whose action batch was rejected, failed, is still pending after --wait or
// could not be read; a network whose current settings could not be read, so
// no change was planned for it; and an organization whose network list could
// not be read, so none of its networks were evaluated. An org with API access
// disabled is not counted: that is a standing Dashboard setting, already
// reported as a warning. At most 20 are named; n is the true total.
func pushNotCompleted(rows []pushRow, outcomes []orgOutcome[[]pushRow]) (named []string, n int) {
	add := func(s string) {
		n++
		if len(named) < 20 {
			named = append(named, s)
		} else if len(named) == 20 {
			named = append(named, "...")
		}
	}
	for _, oc := range outcomes {
		if oc.Err != nil && !errors.Is(oc.Err, errOrgAPIDisabled) {
			add(fmt.Sprintf("%s: %s", oc.Org.Name, oc.Err.Error()))
		}
	}
	for _, r := range rows {
		switch {
		case r.Action == "update" && r.BatchStatus != "completed":
			add(fmt.Sprintf("%s / %s: %s", r.OrganizationName, r.NetworkName, r.BatchStatus))
		case r.Action == "error":
			add(fmt.Sprintf("%s / %s: %s", r.OrganizationName, r.NetworkName, r.Error))
		}
	}
	return named, n
}
