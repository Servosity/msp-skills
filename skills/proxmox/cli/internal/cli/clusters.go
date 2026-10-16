// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: multi-cluster profiles. One entry per client cluster in
// clusters.toml; `--cluster <name>` (or PROXMOX_CLUSTER) points every command
// at that cluster, and fleet commands loop over all of them.

// pp:data-source local

package cli

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"

	"proxmox-pp-cli/internal/client"
	"proxmox-pp-cli/internal/cliutil"
	"proxmox-pp-cli/internal/config"
)

// clusterProfile holds no secrets: tokens are referenced by the NAME of the
// environment variable that carries them.
type clusterProfile struct {
	Client         string `toml:"client,omitempty" json:"client,omitempty"`
	PVEHost        string `toml:"pve_host,omitempty" json:"pve_host,omitempty"`
	PVEBaseURL     string `toml:"pve_base_url,omitempty" json:"pve_base_url,omitempty"`
	PVETokenEnv    string `toml:"pve_token_env,omitempty" json:"pve_token_env,omitempty"`
	PVEFingerprint string `toml:"pve_fingerprint,omitempty" json:"pve_fingerprint,omitempty"`
	PBSHost        string `toml:"pbs_host,omitempty" json:"pbs_host,omitempty"`
	PBSTokenEnv    string `toml:"pbs_token_env,omitempty" json:"pbs_token_env,omitempty"`
	PBSFingerprint string `toml:"pbs_fingerprint,omitempty" json:"pbs_fingerprint,omitempty"`
	// PBSNamespace scopes this client's backups on a PBS shared by several
	// clients (vmids collide across clusters, namespaces do not).
	PBSNamespace string `toml:"pbs_namespace,omitempty" json:"pbs_namespace,omitempty"`
}

type clustersFile struct {
	Clusters map[string]clusterProfile `toml:"clusters"`
}

var (
	selectedCluster string
	// buildingProfileClient: newClusterClient is building a client that
	// applyClusterToClient will re-point, so the shell's routing is not checked.
	buildingProfileClient bool
	envNamePattern        = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	clusterNameRe         = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		if root.PersistentFlags().Lookup("cluster") == nil {
			root.PersistentFlags().StringVar(&selectedCluster, "cluster", cliutil.EnvOverride("PROXMOX_CLUSTER"),
				"Target the named cluster from clusters.toml (env: PROXMOX_CLUSTER)")
		}
		addNovelCommandIfAbsent(root, newClustersCmd(flags))
	})
	registerClientHook(func(c *client.Client) error {
		name := strings.TrimSpace(selectedCluster)
		if name == "" {
			dropConfiguredAuthorization(c)
			if buildingProfileClient {
				return nil // applyClusterToClient sets and checks this profile's routing
			}
			// Env-configured target: per-service pins from the environment.
			pvePins, err := client.EnvPins("PROXMOX_TLS_FINGERPRINT")
			if err != nil {
				return configErr(err)
			}
			pbsPins, err := client.EnvPins("PROXMOX_PBS_TLS_FINGERPRINT")
			if err != nil {
				return configErr(err)
			}
			if err := c.SetServicePins(pvePins, pbsPins); err != nil {
				return err
			}
			return c.CheckServiceOrigins()
		}
		cl, err := clusterByName(name)
		if err != nil {
			return configErr(err)
		}
		return applyClusterToClient(c, cl)
	})
}

// clusterStoreScope keys the local store by the selected clusters.toml entry:
// VMIDs collide across clusters, so one shared store would overwrite one
// client's rows with another's and answer for an unreachable cluster with
// another client's data.
//
// A name is not an identity: another clusters file, or `clusters add --force`,
// can reuse it for a different client. The key is the clusters file plus where
// the entry points (read even when the entry is invalid); the client label is
// display text, so renaming it keeps the store. An entry with no endpoint to
// bind to gets a store no other invocation can open.
func clusterStoreScope() string {
	name := strings.TrimSpace(selectedCluster)
	if name == "" {
		return ""
	}
	path, _ := clustersPath()
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	all, _, err := loadClusters()
	cl, ok := all[name]
	id := []string{cl.PVEHost, cl.PVEBaseURL, cl.PBSHost, cl.PBSNamespace}
	if err != nil || !ok || strings.Join(id, "") == "" {
		return strings.Join([]string{"proxmox-cluster-unresolved", name, unresolvedStoreNonce}, "\x00")
	}
	return strings.Join(append([]string{"proxmox-cluster", name, path}, id...), "\x00")
}

// unresolvedStoreNonce is fresh per process, so an unresolvable cluster never
// reopens a store an earlier invocation (possibly another tenant) wrote.
var unresolvedStoreNonce = func() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}()

func clustersPath() (string, error) {
	if p := strings.TrimSpace(cliutil.EnvOverride("PROXMOX_CLUSTERS_FILE")); p != "" {
		return p, nil
	}
	dir, err := cliutil.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "clusters.toml"), nil
}

// loadClusters returns an empty map (not an error) when no file exists yet.
// Entries are not validated here: callers validate the profiles they use, so
// one malformed entry never blocks the others.
func loadClusters() (map[string]clusterProfile, string, error) {
	path, err := clustersPath()
	if err != nil {
		return nil, "", err
	}
	// The file decides where tokens are sent, so it must not be editable by
	// others; readClustersFile checks the same descriptor it reads.
	data, err := readClustersFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]clusterProfile{}, path, nil
	}
	if err != nil {
		return nil, path, err
	}
	var f clustersFile
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, path, fmt.Errorf("parsing %s: %w", path, err)
	}
	if f.Clusters == nil {
		f.Clusters = map[string]clusterProfile{}
	}
	return f.Clusters, path, nil
}

func clusterByName(name string) (clusterProfile, error) {
	all, path, err := loadClusters()
	if err != nil {
		return clusterProfile{}, err
	}
	cl, ok := all[name]
	if !ok {
		return clusterProfile{}, fmt.Errorf("cluster %q is not defined in %s (run 'proxmox-cli clusters list')", name, path)
	}
	if err := cl.validate(name); err != nil {
		return clusterProfile{}, fmt.Errorf("%s: %w", path, err)
	}
	return cl, nil
}

func sortedClusterNames(all map[string]clusterProfile) []string {
	names := make([]string, 0, len(all))
	for n := range all {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (cl clusterProfile) validate(name string) error {
	if !clusterNameRe.MatchString(name) {
		return fmt.Errorf("cluster name %q must be letters, digits, '.', '_' or '-'", name)
	}
	if cl.PVEHost == "" && cl.PVEBaseURL == "" && cl.PBSHost == "" {
		return fmt.Errorf("cluster %q needs pve_host, pve_base_url, or pbs_host", name)
	}
	for field, pins := range map[string]string{"pve_fingerprint": cl.PVEFingerprint, "pbs_fingerprint": cl.PBSFingerprint} {
		if strings.TrimSpace(pins) != "" && len(splitPins(pins)) == 0 {
			// A set pin that yields no pin must not fall back to system trust.
			return fmt.Errorf("cluster %q: %s is set but holds no SHA-256 fingerprint", name, field)
		}
		for _, p := range splitPins(pins) {
			if !client.ValidFingerprint(p) {
				return fmt.Errorf("cluster %q: %s %q is not a SHA-256 fingerprint (AB:CD:... 32 bytes)", name, field, config.RedactUserinfo(p))
			}
		}
	}
	for field, env := range map[string]string{"pve_token_env": cl.PVETokenEnv, "pbs_token_env": cl.PBSTokenEnv} {
		if env != "" && !envNamePattern.MatchString(env) {
			// Never echo it: a value that is not a variable name is most often the token itself.
			return fmt.Errorf("cluster %q: %s is not an environment variable name, so its value is not shown (it may be the token: store the variable NAME, never the token)", name, field)
		}
	}
	for field, v := range map[string]string{"pve_host": cl.PVEHost, "pbs_host": cl.PBSHost, "pve_base_url": cl.PVEBaseURL, "pbs_namespace": cl.PBSNamespace} {
		if config.TokenShaped(v) && (field != "pbs_namespace" || strings.ContainsAny(v, "!=")) { // a GUID-named namespace is a real one
			// Never echo or use it: a host holding '!' or '=' is a pasted API token.
			return fmt.Errorf("cluster %q: %s looks like an API token (it holds '!' or '=', or is a bare UUID), so it is not shown and not used: put the token in the environment and its variable NAME in pve_token_env/pbs_token_env", name, field)
		}
	}
	if cl.PVEBaseURL != "" {
		u, err := url.Parse(cl.PVEBaseURL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("cluster %q: pve_base_url must be an https:// URL", name)
		}
		if u.User != nil || strings.ContainsAny(cl.PVEBaseURL, "?#") {
			return fmt.Errorf("cluster %q: pve_base_url must not carry credentials, a query, or a fragment", name)
		}
	}
	for field, host := range map[string]string{"pve_host": cl.PVEHost, "pbs_host": cl.PBSHost} {
		if host == "" {
			continue
		}
		raw := host
		if !strings.Contains(raw, "://") {
			raw = "https://" + raw
		}
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" || u.User != nil || (u.Path != "" && u.Path != "/") || strings.ContainsAny(host, "?#") {
			return fmt.Errorf("cluster %q: %s %q must be a host, host:port, or https://host:port", name, field, config.RedactUserinfo(host))
		}
		if u.Scheme != "https" {
			return fmt.Errorf("cluster %q: %s %q must use https", name, field, config.RedactUserinfo(host))
		}
	}
	return nil
}

// A profile without a PVE host gets an empty base URL: requests fail before any
// network I/O (getJSON names the missing setting), so nothing can ever go to an
// inherited PVE URL.

// applyClusterToClient points a client at one cluster. Every field is reset,
// so a value from the shell environment or an earlier cluster never leaks into
// this one (a cluster without a PBS host gets no PBS host).
func applyClusterToClient(c *client.Client, cl clusterProfile) error {
	cfg := c.Config
	if cfg.TemplateVars == nil {
		cfg.TemplateVars = map[string]string{}
	}
	// Always set a concrete base: a profile must never inherit PROXMOX_BASE_URL
	// or PROXMOX_HOST from the shell, or its token would go to that host.
	switch {
	case cl.PVEBaseURL != "":
		c.BaseURL = strings.TrimRight(cl.PVEBaseURL, "/")
	case cl.PVEHost != "":
		// Parsed the way validate parses it (it accepts https://pve1/), never by
		// string surgery.
		raw := cl.PVEHost
		if !strings.Contains(raw, "://") {
			raw = "https://" + raw
		}
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			return fmt.Errorf("pve_host %q must be a host, host:port, or https://host:port", config.RedactUserinfo(cl.PVEHost))
		}
		port := u.Port()
		if port == "" {
			port = "8006"
		}
		c.BaseURL = "https://" + net.JoinHostPort(u.Hostname(), port) + "/api2/json"
	default:
		c.BaseURL = ""
		cfg.ProxmoxApiToken = ""
	}
	cfg.BaseURL = c.BaseURL
	cfg.EndpointRefusal, cfg.PBSRefusal = "", "" // this cluster's own endpoints, not config.toml's
	cfg.TemplateVars["host"] = ""
	cfg.TemplateVars["pbs_host"] = cl.PBSHost
	if cl.PBSHost == "" {
		cfg.TemplateVars["pbs_host"] = client.PBSHostNone
	}
	cfg.AuthHeaderVal = ""
	cfg.ProxmoxApiToken = envValue(cl.PVETokenEnv)
	cfg.ProxmoxPbsApiToken = envValue(cl.PBSTokenEnv)
	dropConfiguredAuthorization(c)
	if err := c.SetServicePins(splitPins(cl.PVEFingerprint), splitPins(cl.PBSFingerprint)); err != nil {
		return err
	}
	return c.CheckServiceOrigins()
}

// dropConfiguredAuthorization removes every credential-class entry from the
// config file's [headers] (Authorization, cookies, CSRF tokens, anything
// named like a token, key, secret or session): generic headers go to both
// services and every clusters.toml tenant, and custom ones follow cross-host
// redirects, so one would send one tenant's (or one service's) credential
// everywhere.
func dropConfiguredAuthorization(c *client.Client) {
	if c.Config == nil {
		return
	}
	for k := range c.Config.Headers {
		if credentialHeader.MatchString(strings.TrimSpace(k)) {
			delete(c.Config.Headers, k)
			fmt.Fprintf(os.Stderr, "warning: ignoring %s in the config file's [headers]; tokens come from PROXMOX_API_TOKEN / PROXMOX_PBS_API_TOKEN or clusters.toml\n", strings.TrimSpace(k))
		}
	}
}

var credentialHeader = regexp.MustCompile(`(?i)auth|cookie|csrf|token|secret|key|session|ticket|passw`)

func envValue(name string) string {
	if name == "" {
		return ""
	}
	// EnvOverride: an unfilled .mcpb prompt arrives as the literal
	// ${user_config.x}; that is unset, not a token.
	return strings.TrimSpace(cliutil.EnvOverride(name))
}

// newClusterClient builds a client for one cluster without consulting
// --cluster, for fleet commands that visit every cluster.
func newClusterClient(flags *rootFlags, cl clusterProfile) (*client.Client, error) {
	saved := selectedCluster
	selectedCluster, buildingProfileClient = "", true
	c, err := flags.newClient()
	selectedCluster, buildingProfileClient = saved, false
	if err != nil {
		return nil, err
	}
	if err := applyClusterToClient(c, cl); err != nil {
		return nil, err
	}
	return c, nil
}

type clusterRow struct {
	Name           string `json:"name"`
	Client         string `json:"client,omitempty"`
	PVE            string `json:"pve,omitempty"`
	PVETokenEnv    string `json:"pve_token_env,omitempty"`
	PVETokenSet    bool   `json:"pve_token_set"`
	PVEPinned      bool   `json:"pve_pinned"`
	PBS            string `json:"pbs,omitempty"`
	PBSTokenEnv    string `json:"pbs_token_env,omitempty"`
	PBSTokenSet    bool   `json:"pbs_token_set"`
	PBSPinned      bool   `json:"pbs_pinned"`
	PBSNamespace   string `json:"pbs_namespace,omitempty"`
	Invalid        string `json:"invalid,omitempty"`
	ClustersSource string `json:"clusters_file"`
}

func newClustersCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clusters",
		Short: "Configure and list client clusters (one entry per PVE cluster + its PBS)",
		Long: "Clusters are kept in clusters.toml (PROXMOX_CLUSTERS_FILE overrides the path). " +
			"Each entry names a PVE host, an optional PBS host, TLS fingerprints, and the NAMES of the " +
			"environment variables that hold the API tokens. Tokens are never written to the file.",
	}
	cmd.AddCommand(newClustersListCmd(flags), newClustersAddCmd(flags), newClustersCheckCmd(flags))
	return cmd
}

func newClustersListCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "List configured clusters and whether each token env var is set (values never shown)",
		Example:     "  proxmox-cli clusters list --json",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "list configured clusters")
			}
			all, path, err := loadClusters()
			if err != nil {
				return configErr(err)
			}
			rows := make([]clusterRow, 0, len(all))
			for _, name := range sortedClusterNames(all) {
				cl := all[name]
				pve := cl.PVEBaseURL
				if pve == "" {
					pve = cl.PVEHost
				}
				pve = config.RedactUserinfo(pve)
				rows = append(rows, clusterRow{
					Name: name, Client: cl.Client, PVE: pve, PVETokenEnv: shownEnvName(cl.PVETokenEnv),
					PVETokenSet: envValue(cl.PVETokenEnv) != "", PVEPinned: cl.PVEFingerprint != "",
					PBS: config.RedactUserinfo(cl.PBSHost), PBSTokenEnv: shownEnvName(cl.PBSTokenEnv), PBSTokenSet: envValue(cl.PBSTokenEnv) != "",
					PBSPinned: cl.PBSFingerprint != "", PBSNamespace: shownNamespace(cl.PBSNamespace), ClustersSource: path,
				})
				if err := cl.validate(name); err != nil {
					rows[len(rows)-1].Invalid = err.Error()
				}
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No clusters configured in %s.\nAdd one: proxmox-cli clusters add acme --pve-host pve1.acme.lan --pve-token-env ACME_PVE_TOKEN\n", path)
				return nil
			}
			tableRows := make([][]string, 0, len(rows))
			for _, r := range rows {
				name := r.Name
				if r.Invalid != "" {
					name += " (INVALID: " + r.Invalid + ")"
				}
				tableRows = append(tableRows, []string{name, r.Client, r.PVE, tokenState(r.PVETokenEnv, r.PVETokenSet), r.PBS, tokenState(r.PBSTokenEnv, r.PBSTokenSet)})
			}
			return flags.printTable(cmd, []string{"NAME", "CLIENT", "PVE", "PVE TOKEN", "PBS", "PBS TOKEN"}, tableRows)
		},
	}
}

// shownNamespace hides a pbs_namespace holding '!' or '=' (a pasted token).
func shownNamespace(ns string) string {
	if strings.ContainsAny(ns, "!=") {
		return "(not shown: it looks like an API token)"
	}
	return ns
}

// shownEnvName: a *_token_env value that is not a variable name may be the
// token pasted in its place, so it is never shown.
func shownEnvName(env string) string {
	if env == "" || envNamePattern.MatchString(env) {
		return env
	}
	return "(not a variable name; not shown)"
}

func tokenState(env string, set bool) string {
	switch {
	case env == "":
		return "-"
	case set:
		return env + " (set)"
	default:
		return env + " (MISSING)"
	}
}

func newClustersAddCmd(flags *rootFlags) *cobra.Command {
	var cl clusterProfile
	var force bool
	cmd := &cobra.Command{
		Use:   "add [name]",
		Short: "Add or replace a cluster entry in clusters.toml (local file only; never touches Proxmox)",
		// Never an MCP tool: an entry chooses where a token is sent, which TLS
		// pin vouches for that host, and which env var is sent as the token.
		// An agent able to write one could send any credential to its own host.
		Annotations: map[string]string{"mcp:hidden": "true"},
		Example: "  proxmox-cli clusters add acme --client \"Acme Dental\" --pve-host pve1.acme.lan --pve-token-env ACME_PVE_TOKEN \\\n" +
			"    --pbs-host pbs.acme.lan --pbs-token-env ACME_PBS_TOKEN --pbs-fingerprint AB:CD:...",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "add cluster to clusters.toml")
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(errors.New("cluster name is required"))
			}
			name := args[0]
			if err := cl.validate(name); err != nil {
				return usageErr(err)
			}
			all, path, err := loadClusters()
			if err != nil {
				return configErr(err)
			}
			if _, exists := all[name]; exists && !force {
				return usageErr(fmt.Errorf("cluster %q already exists in %s; pass --force to replace it", name, path))
			}
			all[name] = cl
			data, err := toml.Marshal(clustersFile{Clusters: all})
			if err != nil {
				return err
			}
			if err := cliutil.AtomicWritePrivateFile(path, data, 0o600, 0o700); err != nil {
				return err
			}
			return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"cluster": name, "clusters_file": path, "saved": true}, flags)
		},
	}
	f := cmd.Flags()
	f.StringVar(&cl.Client, "client", "", "Client display name used in fleet reports")
	f.StringVar(&cl.PVEHost, "pve-host", "", "PVE host or host:port (default port 8006)")
	f.StringVar(&cl.PVEBaseURL, "pve-base-url", "", "Full PVE API base URL, e.g. https://pve1:8006/api2/json")
	f.StringVar(&cl.PVETokenEnv, "pve-token-env", "", "NAME of the env var holding USER@REALM!TOKENID=SECRET")
	f.StringVar(&cl.PVEFingerprint, "pve-fingerprint", "", "PVE TLS certificate SHA-256 fingerprint to pin")
	f.StringVar(&cl.PBSHost, "pbs-host", "", "PBS host or host:port (default port 8007)")
	f.StringVar(&cl.PBSTokenEnv, "pbs-token-env", "", "NAME of the env var holding USER@REALM!TOKENID:SECRET")
	f.StringVar(&cl.PBSFingerprint, "pbs-fingerprint", "", "PBS TLS certificate SHA-256 fingerprint to pin")
	f.StringVar(&cl.PBSNamespace, "pbs-namespace", "", "PBS namespace holding this client's backups on a shared PBS, with the namespaces below it (read only on the datastores this cluster's PVE storages write to)")
	f.BoolVar(&force, "force", false, "Replace an existing entry with the same name")
	return cmd
}

func splitPins(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
