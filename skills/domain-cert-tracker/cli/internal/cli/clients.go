// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (handfixes.json tracker-novels).
// pp:data-source local

package cli

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"domain-cert-tracker-pp-cli/internal/store"
	"domain-cert-tracker-pp-cli/internal/tracker"

	"github.com/spf13/cobra"
)

func newNovelClientsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clients",
		Short: "Keep each client's domains and hosts in one local inventory that every sweep and report walks.",
		Long: `The local inventory: client name -> domains (checked over RDAP, Certificate
Transparency and DNS) and hosts (checked with a live TLS handshake). Set it by
hand with 'clients set' or import a CSV/JSON file with 'clients import'.`,
		Example: strings.Trim(`
  domain-cert-tracker-cli clients list --agent
  domain-cert-tracker-cli clients set "Acme Dental" --domain example.com --host www.example.com`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newClientsListCmd(flags))
	cmd.AddCommand(newClientsSetCmd(flags))
	cmd.AddCommand(newClientsRemoveCmd(flags))
	cmd.AddCommand(newClientsImportCmd(flags))
	return cmd
}

func newClientsListCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "List every client with its inventoried domains and hosts",
		Example:     "  domain-cert-tracker-cli clients list --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "clients list")
			}
			db, err := openTrackerRead(cmd.Context())
			if err != nil {
				return err
			}
			clients := []store.Client{}
			if db != nil {
				defer db.Close()
				if clients, err = db.ListClients(cmd.Context()); err != nil {
					return err
				}
			}
			return flags.printJSON(cmd, map[string]any{"clients": clients, "count": len(clients)})
		},
	}
}

func targetsFromFlags(client string, domains, hosts []string) ([]store.Target, error) {
	var out []store.Target
	for _, d := range splitList(domains) {
		norm, err := tracker.NormalizeDomain(d)
		if err != nil {
			return nil, usageErr(fmt.Errorf("--domain: %w", err))
		}
		out = append(out, store.Target{Client: client, Kind: "domain", Value: norm, Port: 443})
	}
	for _, h := range splitList(hosts) {
		host, port, err := parseHostTarget(h)
		if err != nil {
			return nil, usageErr(fmt.Errorf("--host %q: %w", h, err))
		}
		out = append(out, store.Target{Client: client, Kind: "host", Value: host, Port: port})
	}
	return out, nil
}

func newClientsSetCmd(flags *rootFlags) *cobra.Command {
	var domains, hosts []string
	var yes, confirm bool
	cmd := &cobra.Command{
		Use:   "set <client>",
		Short: "Add a client, or add domains and hosts to an existing one (previews and exits 2 unless --yes or MCP confirm is set)",
		Long: `Adds the client if it is new and adds each --domain and --host to it. Existing
entries are kept. A host may carry a port (host:8443), but sweep only probes it
when that run passes --allow-port 8443 on the command line. Changing the
inventory needs --yes on the command line (or confirm: true over MCP); without
it the command only shows what it would add and exits 2.`,
		Example:     `  domain-cert-tracker-cli clients set "Acme Dental" --domain example.com --host www.example.com --yes`,
		Annotations: map[string]string{"mcp:local-write": "true", "pp:data-source": "local", "pp:happy-args": "client=Example Client;--domain=example.com;--host=www.example.com;--yes", "pp:live-happy-path": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "clients set")
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				return usageErr(errors.New("clients set needs exactly one client name"))
			}
			name := strings.TrimSpace(args[0])
			targets, err := targetsFromFlags(name, domains, hosts)
			if err != nil {
				return err
			}
			if !yes && !confirm {
				if err := flags.printJSON(cmd, map[string]any{"client": name, "add": targets, "confirm_required": true}); err != nil {
					return err
				}
				return usageErr(errors.New("not saved: pass --yes (or confirm: true over MCP) to change the inventory"))
			}
			db, err := openTrackerWrite(cmd.Context())
			if err != nil {
				return err
			}
			defer db.Close()
			if err := db.AddTargets(cmd.Context(), name, targets); err != nil {
				return err
			}
			clients, err := db.ListClients(cmd.Context())
			if err != nil {
				return err
			}
			for _, c := range clients {
				if c.Name == name {
					return flags.printJSON(cmd, map[string]any{"client": c, "added": len(targets)})
				}
			}
			return fmt.Errorf("client %q was not saved", name)
		},
	}
	cmd.Flags().StringSliceVar(&domains, "domain", nil, "Registered domain to track (repeat or comma-separate)")
	cmd.Flags().StringSliceVar(&hosts, "host", nil, "Host to TLS-probe, host or host:port (repeat or comma-separate)")
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm the change (command line)")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Confirm the change (the MCP confirm: true argument)")
	return cmd
}

func newClientsRemoveCmd(flags *rootFlags) *cobra.Command {
	var domains, hosts []string
	var yes, confirm, whole bool
	cmd := &cobra.Command{
		Use:   "remove <client>",
		Short: "Remove a client (--whole-client), or only its named domains and hosts, from the inventory (previews and exits 2 unless --yes or MCP confirm is set)",
		Long: `With --domain or --host, removes only those entries. Removing the client and
everything it lists needs --whole-client instead; with neither, nothing is
removed. Deleting inventory needs --yes on the command line (or confirm: true over MCP);
without it the command only shows what it would remove and exits 2. Stored
sweep history and events are kept.`,
		Example:     `  domain-cert-tracker-cli clients remove "Acme Dental" --host www.example.com --yes`,
		Annotations: map[string]string{"pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "clients remove")
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				return usageErr(errors.New("clients remove needs exactly one client name"))
			}
			name := strings.TrimSpace(args[0])
			targets, err := targetsFromFlags(name, domains, hosts)
			if err != nil {
				return err
			}
			// Whole-client removal is opt-in by its own flag, so a selector that
			// parses to nothing, or that the MCP layer dropped as empty ([], "",
			// [null]), can never widen into deleting the client.
			if whole && (len(domains) > 0 || len(hosts) > 0) {
				return usageErr(errors.New("--whole-client cannot be combined with --domain or --host"))
			}
			if !whole && len(targets) == 0 {
				return usageErr(errors.New("--domain/--host named no valid entry; nothing removed (pass --whole-client to remove the client and everything it lists)"))
			}
			// A preview must not migrate the store: only a confirmed removal
			// opens it for writing.
			open := openTrackerRead
			if yes || confirm {
				open = openTrackerWrite
			}
			db, err := open(cmd.Context())
			if err != nil {
				return err
			}
			if db == nil { // no store yet, so no client
				return usageErr(fmt.Errorf("%w %q: run 'clients list'", tracker.ErrUnknownClient, name))
			}
			defer db.Close()
			clients, err := db.ListClients(cmd.Context())
			if err != nil {
				return err
			}
			var current *store.Client
			for i := range clients {
				if clients[i].Name == name {
					current = &clients[i]
				}
			}
			if current == nil {
				return usageErr(fmt.Errorf("%w %q: run 'clients list'", tracker.ErrUnknownClient, name))
			}
			preview := map[string]any{"client": name}
			if len(targets) == 0 {
				preview["remove"] = "the whole client"
				preview["domains"], preview["hosts"] = current.Domains, current.Hosts
			} else {
				preview["remove"] = targets
			}
			if !yes && !confirm {
				preview["confirm_required"] = true
				if err := flags.printJSON(cmd, preview); err != nil {
					return err
				}
				return usageErr(errors.New("not removed: pass --yes (or confirm: true over MCP) to delete inventory"))
			}
			if len(targets) == 0 {
				if _, err := db.RemoveClient(cmd.Context(), name); err != nil {
					return err
				}
				preview["removed"] = true
			} else {
				n, err := db.RemoveTargets(cmd.Context(), name, targets)
				if err != nil {
					return err
				}
				preview["removed"] = n
			}
			return flags.printJSON(cmd, preview)
		},
	}
	cmd.Flags().StringSliceVar(&domains, "domain", nil, "Domain to remove from the client")
	cmd.Flags().StringSliceVar(&hosts, "host", nil, "Host (host or host:port) to remove from the client")
	cmd.Flags().BoolVar(&whole, "whole-client", false, "Remove the client and every domain and host it lists")
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm the deletion (command line)")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Confirm the deletion (the MCP confirm: true argument)")
	return cmd
}

// importRow is one client in a JSON import file.
type importRow struct {
	Name    string   `json:"name"`
	Client  string   `json:"client"`
	Domains []string `json:"domains"`
	Hosts   []string `json:"hosts"`
}

const maxImportBytes = 5 << 20

func newClientsImportCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import <file.csv|file.json>",
		Short: "Import clients, domains and hosts from a CSV or JSON file (command line only)",
		Long: `CSV rows are client,domain[,host] (a header row is skipped; either column may
be empty). JSON is an array of {"name": "...", "domains": [...], "hosts": [...]}.
Entries are added to the inventory; nothing is removed. The file path is read
only when you run this on the command line: the MCP server does not offer it.`,
		Example: "  domain-cert-tracker-cli clients import clients.csv",
		// A host-file read: hidden from the MCP surface, and refused at run
		// time if an MCP child process ever reaches it.
		Annotations: map[string]string{"mcp:hidden": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "clients import")
			}
			if err := refuseUnderMCP("clients import"); err != nil {
				return err
			}
			if len(args) != 1 {
				return usageErr(errors.New("clients import needs exactly one file path"))
			}
			f, err := os.Open(args[0])
			if err != nil {
				return err
			}
			defer f.Close()
			raw, err := io.ReadAll(io.LimitReader(f, maxImportBytes+1))
			if err != nil {
				return err
			}
			if len(raw) > maxImportBytes {
				return usageErr(fmt.Errorf("%s is larger than %d bytes", args[0], maxImportBytes))
			}
			byClient, err := parseImport(raw)
			if err != nil {
				return usageErr(fmt.Errorf("%s: %w", args[0], err))
			}
			db, err := openTrackerWrite(cmd.Context())
			if err != nil {
				return err
			}
			defer db.Close()
			names := make([]string, 0, len(byClient))
			for n := range byClient {
				names = append(names, n)
			}
			sort.Strings(names)
			total := 0
			for _, n := range names {
				if err := db.AddTargets(cmd.Context(), n, byClient[n]); err != nil {
					return err
				}
				total += len(byClient[n])
			}
			return flags.printJSON(cmd, map[string]any{"clients": len(names), "entries": total})
		},
	}
	return cmd
}

// parseImport reads a JSON array or CSV rows into targets per client.
func parseImport(raw []byte) (map[string][]store.Target, error) {
	out := map[string][]store.Target{}
	add := func(client string, domains, hosts []string) error {
		client = strings.TrimSpace(client)
		if client == "" {
			return errors.New("a row has no client name")
		}
		t, err := targetsFromFlags(client, domains, hosts)
		if err != nil {
			return err
		}
		if _, ok := out[client]; !ok {
			out[client] = []store.Target{}
		}
		out[client] = append(out[client], t...)
		return nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "[") {
		var rows []importRow
		if err := json.Unmarshal([]byte(trimmed), &rows); err != nil {
			return nil, fmt.Errorf("parse JSON: %w", err)
		}
		for _, r := range rows {
			name := r.Name
			if name == "" {
				name = r.Client
			}
			if err := add(name, r.Domains, r.Hosts); err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	rd := csv.NewReader(strings.NewReader(trimmed))
	rd.FieldsPerRecord = -1
	rd.TrimLeadingSpace = true
	records, err := rd.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse CSV: %w", err)
	}
	for i, rec := range records {
		if len(rec) < 2 {
			return nil, fmt.Errorf("CSV row %d: want client,domain[,host]", i+1)
		}
		if i == 0 && strings.EqualFold(strings.TrimSpace(rec[0]), "client") {
			continue
		}
		var domains, hosts []string
		if strings.TrimSpace(rec[1]) != "" {
			domains = []string{rec[1]}
		}
		if len(rec) > 2 && strings.TrimSpace(rec[2]) != "" {
			hosts = []string{rec[2]}
		}
		if err := add(rec[0], domains, hosts); err != nil {
			return nil, fmt.Errorf("CSV row %d: %w", i+1, err)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no clients found")
	}
	return out, nil
}
