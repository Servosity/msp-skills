// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): `webhooks register|unregister|status` and
// `events list`. Wired in novel_wiring.go. Pinned by this connector's
// handfixes.json under the "webhooks-listener" entry. The listener is in
// webhooks_listen.go.
//
// Zammad pushes a change instead of being polled: a Trigger fires a Webhook
// that POSTs to `webhooks listen`, signed with HMAC-SHA1 (X-Hub-Signature).
// One webhook and one trigger per event, so the event name is inside the
// signed body. The payload is IDs only; the listener fetches the record.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"zammad-pp-cli/internal/client"
	"zammad-pp-cli/internal/cliutil"
	"zammad-pp-cli/internal/store"
)

// webhookEvents is the event catalog: the names MCP Events (SEP-3415) would
// list, each with the Zammad trigger condition that fires it.
var webhookEvents = []struct {
	name      string
	condition map[string]any
}{
	{"zammad.ticket.created", map[string]any{"ticket.action": map[string]any{"operator": "is", "value": "create"}}},
	{"zammad.ticket.updated", map[string]any{"ticket.action": map[string]any{"operator": "is", "value": "update"}}},
	{"zammad.article.created", map[string]any{"article.action": map[string]any{"operator": "is", "value": "create"}}},
}

func isWebhookEvent(name string) bool {
	for _, e := range webhookEvents {
		if e.name == name {
			return true
		}
	}
	return false
}

// webhookPayload is the custom payload: the event name and IDs, nothing a
// customer wrote.
func webhookPayload(event string) string {
	return `{"event":"` + event + `","ticket_id":"#{ticket.id}","article_id":"#{article.id}"}`
}

func newWebhooksCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "webhooks",
		Short: "Receive Zammad changes as signed webhooks instead of polling",
		Long: `Zammad calls this machine when a ticket is created or updated or an
article is added. 'register' creates the Zammad webhooks and triggers,
'listen' receives, verifies and records them, 'events list' reads them.`,
	}
	cmd.AddCommand(newWebhooksRegisterCmd(flags), newWebhooksUnregisterCmd(flags),
		newWebhooksStatusCmd(flags), newWebhooksListenCmd(flags))
	return cmd
}

func newWebhooksRegisterCmd(flags *rootFlags) *cobra.Command {
	var endpoint, name string
	var sslVerify, confirm bool
	cmd := &cobra.Command{
		Use:   "register",
		Short: "Create the Zammad webhooks and triggers that call 'webhooks listen'",
		Long: `Creates one Zammad webhook and one trigger per event (ticket created,
ticket updated, article created), signed with a secret stored beside the API
token (or ZAMMAD_WEBHOOK_SECRET). Needs the admin.webhook and admin.trigger
permissions. Writes to Zammad only with --yes (CLI) or confirm: true (MCP).`,
		Example: `  # Zammad on this machine or the LAN posts to the listener
  zammad-cli webhooks register --endpoint http://192.168.1.20:8789/ --yes`,
		// pp:data-source live
		Annotations: map[string]string{"pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			if err := validateWebhookEndpoint(endpoint); err != nil {
				return err
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			prefix := name + ": "
			ctx := cmd.Context()
			hooks, triggers, err := listOwnedWebhooks(ctx, c, prefix)
			if err != nil {
				return err
			}
			if len(hooks)+len(triggers) > 0 {
				return fmt.Errorf("webhooks named %q already exist (%d webhooks, %d triggers); run 'zammad-cli webhooks unregister --yes' first", prefix+"*", len(hooks), len(triggers))
			}
			var plan []string
			for _, e := range webhookEvents {
				plan = append(plan, prefix+e.name)
			}
			if flags.dryRun || !(flags.yes || confirm) {
				if err := flags.printJSON(cmd, map[string]any{"written": false, "would_create": plan, "endpoint": endpoint}); err != nil {
					return err
				}
				if flags.dryRun {
					return nil
				}
				return fmt.Errorf("nothing was written: pass --yes (CLI) or confirm: true (MCP) to create %d webhooks and %d triggers", len(plan), len(plan))
			}
			secret, source, err := cliutil.LoadWebhookSecret(flags.configPath)
			if err != nil {
				return err
			}
			if secret == "" {
				if secret, err = cliutil.NewWebhookSecret(flags.configPath); err != nil {
					return fmt.Errorf("storing the webhook secret: %w", err)
				}
				source = "generated"
			}
			var created []map[string]any
			for _, e := range webhookEvents {
				hookID, err := postID(ctx, c, "/webhooks", map[string]any{
					"name": prefix + e.name, "endpoint": endpoint, "http_method": "post",
					"signature_token": secret, "ssl_verify": sslVerify, "customized_payload": true,
					"custom_payload": webhookPayload(e.name), "note": webhookNote, "active": true,
				})
				if err != nil {
					return fmt.Errorf("creating webhook %s: %w (created so far: %v; 'zammad-cli webhooks unregister --yes' removes them)", e.name, err, created)
				}
				triggerID, err := postID(ctx, c, "/triggers", map[string]any{
					"name": prefix + e.name, "condition": e.condition, "activator": "action",
					"execution_condition_mode": "always", "active": true,
					"perform": map[string]any{"notification.webhook": map[string]any{"webhook_id": hookID}},
				})
				created = append(created, map[string]any{"event": e.name, "webhook_id": hookID})
				if err != nil {
					return fmt.Errorf("creating trigger %s: %w (created so far: %v; 'zammad-cli webhooks unregister --yes' removes them)", e.name, err, created)
				}
				created[len(created)-1]["trigger_id"] = triggerID
			}
			return flags.printJSON(cmd, map[string]any{"written": true, "registered": created, "endpoint": endpoint, "secret_source": source})
		},
	}
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "URL Zammad posts to: where 'webhooks listen' is reachable from the Zammad server (required)")
	cmd.Flags().StringVar(&name, "name", "zammad-cli", "Name prefix for the webhooks and triggers")
	cmd.Flags().BoolVar(&sslVerify, "ssl-verify", true, "Have Zammad verify the endpoint's TLS certificate")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Create for real (the MCP form of --yes)")
	return cmd
}

func newWebhooksUnregisterCmd(flags *rootFlags) *cobra.Command {
	var name string
	var confirm bool
	cmd := &cobra.Command{
		Use:   "unregister",
		Short: "Remove the Zammad webhooks and triggers 'webhooks register' created",
		Long: `Deletes every trigger and webhook whose name starts with the prefix
(triggers first: Zammad refuses to delete a webhook a trigger still uses).
Writes to Zammad only with --yes (CLI) or confirm: true (MCP). The stored
secret is kept.`,
		Example: `  zammad-cli webhooks unregister --yes`,
		// pp:data-source live
		Annotations: map[string]string{"pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			hooks, triggers, err := listOwnedWebhooks(ctx, c, name+": ")
			if err != nil {
				return err
			}
			plan := map[string]any{"triggers": idsOf(triggers), "webhooks": idsOf(hooks)}
			if len(hooks)+len(triggers) == 0 || flags.dryRun || !(flags.yes || confirm) {
				plan["written"] = false
				if err := flags.printJSON(cmd, plan); err != nil {
					return err
				}
				if len(hooks)+len(triggers) == 0 || flags.dryRun {
					return nil
				}
				return fmt.Errorf("nothing was written: pass --yes (CLI) or confirm: true (MCP) to delete %d triggers and %d webhooks", len(triggers), len(hooks))
			}
			for _, group := range []struct {
				path, kind string
				ids        []int64
			}{{"/triggers/", "trigger", idsOf(triggers)}, {"/webhooks/", "webhook", idsOf(hooks)}} {
				for _, id := range group.ids {
					if _, _, err := c.Delete(ctx, group.path+strconv.FormatInt(id, 10)); err != nil {
						return fmt.Errorf("deleting %s %d: %w", group.kind, id, err)
					}
				}
			}
			plan["written"] = true
			return flags.printJSON(cmd, plan)
		},
	}
	cmd.Flags().StringVar(&name, "name", "zammad-cli", "Name prefix of the webhooks and triggers to remove")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Delete for real (the MCP form of --yes)")
	return cmd
}

func newWebhooksStatusCmd(flags *rootFlags) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:     "status",
		Short:   "Show the registered webhooks and triggers and where the secret comes from (never the secret)",
		Example: `  zammad-cli webhooks status`,
		// pp:data-source live
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "webhooks status")
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			hooks, triggers, err := listOwnedWebhooks(cmd.Context(), c, name+": ")
			if err != nil {
				return err
			}
			var hookRows, triggerRows []map[string]any
			for _, h := range hooks {
				token, _ := h["signature_token"].(string)
				hookRows = append(hookRows, map[string]any{"id": h["id"], "name": h["name"], "active": h["active"],
					"endpoint": h["endpoint"], "ssl_verify": h["ssl_verify"], "signed": token != ""})
			}
			for _, t := range triggers {
				triggerRows = append(triggerRows, map[string]any{"id": t["id"], "name": t["name"], "active": t["active"]})
			}
			_, source, secretErr := cliutil.LoadWebhookSecret(flags.configPath)
			out := map[string]any{"webhooks": hookRows, "triggers": triggerRows, "secret_source": source,
				"registered": len(hooks) == len(webhookEvents) && len(triggers) == len(webhookEvents)}
			if secretErr != nil {
				out["secret_error"] = secretErr.Error()
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().StringVar(&name, "name", "zammad-cli", "Name prefix of the webhooks and triggers")
	return cmd
}

func newEventsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "events", Short: "Read the Zammad events 'webhooks listen' recorded"}
	var since int64
	var eventType, dbPath string
	var limit int
	list := &cobra.Command{
		Use:   "list",
		Short: "List recorded Zammad events after a cursor, oldest first",
		Long: `Reads the local event table that 'webhooks listen' fills (no Zammad call).
Pass the returned cursor as --since next time to get only newer events.
Each event is {eventId, name, timestamp, data, cursor}; data holds IDs and
ticket fields, never customer text: run 'tickets get <id>' for the rest.`,
		Example: `  zammad-cli events list --since 0
  zammad-cli events list --since 41 --type zammad.ticket.created --json`,
		// pp:data-source local
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if eventType != "" && !isWebhookEvent(eventType) {
				return fmt.Errorf("unknown --type %q", eventType)
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "events list")
			}
			if limit < 1 || limit > 1000 {
				limit = 100
			}
			db, err := store.OpenReadOnlyContext(cmd.Context(), defaultNovelDBPath(dbPath))
			if err != nil {
				return fmt.Errorf("%w\nNo events recorded yet: run 'zammad-cli webhooks listen'.", err)
			}
			defer db.Close()
			rows, more, err := db.VendorEventsSince(cmd.Context(), since, eventType, limit)
			if err != nil {
				return err
			}
			events := make([]map[string]any, 0, len(rows))
			cursor := since
			for _, e := range rows {
				cursor = e.Seq
				events = append(events, map[string]any{"eventId": e.EventID, "name": e.Type, "timestamp": e.OccurredAt,
					"data": e.Data, "cursor": strconv.FormatInt(e.Seq, 10), "source": e.Source})
			}
			return flags.printJSON(cmd, map[string]any{"events": events, "cursor": strconv.FormatInt(cursor, 10), "hasMore": more})
		},
	}
	list.Flags().Int64Var(&since, "since", 0, "Cursor: return events after this one (0 for all)")
	list.Flags().StringVar(&eventType, "type", "", "Only this event: zammad.ticket.created, zammad.ticket.updated or zammad.article.created")
	list.Flags().IntVar(&limit, "limit", 100, "Maximum events (1 to 1000)")
	list.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	cmd.AddCommand(list)
	return cmd
}

// validateWebhookEndpoint accepts an http(s) URL with a host and no
// userinfo: Zammad stores the endpoint and shows it to every admin.
func validateWebhookEndpoint(endpoint string) error {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if endpoint == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("--endpoint must be the http(s) URL Zammad can reach 'webhooks listen' at, for example http://192.168.1.20:8789/")
	}
	if u.User != nil {
		return fmt.Errorf("--endpoint must not carry a user or password; the signature secret authenticates Zammad")
	}
	return nil
}

// webhookNote marks the webhooks 'register' created; nothing else counts as
// ours, whatever its name.
const webhookNote = "Created by zammad-cli webhooks register"

// listOwnedWebhooks returns the webhooks 'register' created under prefix
// (one of the names it gives, and webhookNote) and the triggers with one of
// those names that fire them. An admin's own webhook or trigger that only
// shares the name prefix is never listed, so unregister cannot delete it.
func listOwnedWebhooks(ctx context.Context, c *client.Client, prefix string) (hooks, triggers []map[string]any, err error) {
	read := func(path string) ([]map[string]any, error) {
		raw, err := c.GetNoCache(ctx, path, nil)
		if err != nil {
			return nil, err
		}
		var items []map[string]any
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		return items, nil
	}
	// Exactly the names register gives (Codex nb-r1 [P2]): an admin trigger
	// named "<prefix>escalation" that fires one of our webhooks is not ours.
	names := map[string]bool{}
	for _, e := range webhookEvents {
		names[prefix+e.name] = true
	}
	named := func(it map[string]any) bool {
		n, _ := it["name"].(string)
		return names[n]
	}
	all, err := read("/webhooks")
	if err != nil {
		return nil, nil, err
	}
	ours := map[int64]bool{}
	for _, h := range all {
		if note, _ := h["note"].(string); named(h) && note == webhookNote {
			hooks = append(hooks, h)
			ours[numericID(h["id"])] = true
		}
	}
	if all, err = read("/triggers"); err != nil {
		return nil, nil, err
	}
	for _, t := range all {
		perform, _ := t["perform"].(map[string]any)
		fire, _ := perform["notification.webhook"].(map[string]any)
		if id := numericID(fire["webhook_id"]); named(t) && id != 0 && ours[id] {
			triggers = append(triggers, t)
		}
	}
	return hooks, triggers, nil
}

func idsOf(items []map[string]any) []int64 {
	ids := []int64{}
	for _, it := range items {
		if id := numericID(it["id"]); id != 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

// numericID reads a JSON id Zammad sends as a number or a numeric string.
func numericID(v any) int64 {
	switch v := v.(type) {
	case float64:
		return int64(v)
	case string:
		id, _ := strconv.ParseInt(v, 10, 64)
		return id
	}
	return 0
}

func postID(ctx context.Context, c *client.Client, path string, body map[string]any) (int64, error) {
	raw, _, err := c.Post(ctx, path, body)
	if err != nil {
		return 0, err
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.ID == 0 {
		return 0, fmt.Errorf("%s returned no id", path)
	}
	return out.ID, nil
}
