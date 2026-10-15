// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Fleet plumbing shared by the fleet, audit and policies hygiene commands:
// the devices file, per-device clients and the fan-out. Plus `fleet devices`
// and `fleet backup`.
// pp:data-source live

package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"

	"fortinet-pp-cli/internal/client"
	"fortinet-pp-cli/internal/cliutil"
	"fortinet-pp-cli/internal/config"
	"fortinet-pp-cli/internal/fortios"
)

// fleetDevice is one [[device]] entry of devices.toml. Tokens are never
// stored in the file: token_file names a 0600 file holding the token.
// token_env is parsed only to reject it: an env var named at run time cannot
// be declared in the MCP manifest, so Claude Desktop would never pass it.
type fleetDevice struct {
	Name      string   `toml:"name" json:"name"`
	URL       string   `toml:"url" json:"url"`
	TokenEnv  string   `toml:"token_env" json:"token_env,omitempty"`
	TokenFile string   `toml:"token_file" json:"token_file,omitempty"`
	Insecure  bool     `toml:"insecure" json:"insecure,omitempty"`
	VDOMs     []string `toml:"vdoms" json:"vdoms,omitempty"`
	Client    string   `toml:"client" json:"client,omitempty"`
}

type fleetFile struct {
	Device []fleetDevice `toml:"device"`
}

type fleetOpts struct {
	devicesPath string
	only        string
	vdoms       string
	concurrency int
}

// fleetTarget is a resolved device. single=true means "no devices file":
// the configured FORTINET_BASE_URL / FORTINET_API_TOKEN device.
type fleetTarget struct {
	fleetDevice
	single bool
	token  string
	apiURL string
}

func addFleetFlags(cmd *cobra.Command, o *fleetOpts) {
	cmd.Flags().StringVar(&o.devicesPath, "devices", "", "Devices file (default $FORTINET_DEVICES or <config dir>/devices.toml; without one, the configured FORTINET_BASE_URL device is used)")
	cmd.Flags().StringVar(&o.only, "device", "", "Only these devices (comma-separated names from the devices file)")
	cmd.Flags().StringVar(&o.vdoms, "vdoms", "", "VDOMs to check on every device (comma-separated; overrides each device's vdoms; default every VDOM the device lists)")
	cmd.Flags().IntVar(&o.concurrency, "concurrency", 4, "Devices queried in parallel")
}

// devicesEnv is FORTINET_DEVICES, with an unedited "<your-...>" placeholder
// from the mcp-install.md JSON templates read as unset (the same way
// EnvOverride treats an unresolved ${user_config...}): a pasted template with
// only the base URL and token filled in must not break every fleet command.
func devicesEnv() string {
	v := strings.TrimSpace(cliutil.EnvOverride("FORTINET_DEVICES"))
	if strings.HasPrefix(v, "<your-") && strings.HasSuffix(v, ">") {
		return ""
	}
	return v
}

func defaultDevicesPath() string {
	if v := devicesEnv(); v != "" {
		return v
	}
	dir, err := cliutil.ConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "devices.toml")
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// checkAPIURL refuses URLs that would leak credentials: plain http (except
// to a loopback test host) and embedded user:password.
func checkAPIURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("invalid url (want https://<fortigate>[:port])")
	}
	if u.User != nil {
		return nil, fmt.Errorf("url for %s must not embed a user or password; use token_file", u.Hostname())
	}
	host := u.Hostname()
	loopback := host == "localhost" || net.ParseIP(host).IsLoopback()
	if !strings.EqualFold(u.Scheme, "https") && !(loopback && strings.EqualFold(u.Scheme, "http")) {
		return nil, fmt.Errorf("url %s must use https; the API token would otherwise be sent in clear text", u.Redacted())
	}
	return u, nil
}

// normalizeAPIURL turns https://host[:port][/api/v2] into the API root.
func normalizeAPIURL(raw string) (string, error) {
	u, err := checkAPIURL(raw)
	if err != nil {
		return "", err
	}
	p := strings.TrimRight(u.Path, "/")
	if !strings.HasSuffix(p, "/api/v2") {
		p += "/api/v2"
	}
	u.Path, u.RawQuery, u.Fragment = p, "", ""
	return u.String(), nil
}

func readDevicesFile(path string) ([]fleetDevice, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- the operator's own devices file (--devices / config path), read as data only
	if err != nil {
		return nil, err
	}
	var f fleetFile
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	seen := map[string]bool{}
	for i, d := range f.Device {
		if strings.TrimSpace(d.Name) == "" || strings.TrimSpace(d.URL) == "" {
			return nil, fmt.Errorf("%s: device #%d needs name and url", path, i+1)
		}
		if seen[d.Name] {
			return nil, fmt.Errorf("%s: duplicate device name %q", path, d.Name)
		}
		seen[d.Name] = true
	}
	return f.Device, nil
}

func resolveToken(d fleetDevice) (string, error) {
	switch {
	case d.TokenEnv != "":
		return "", fmt.Errorf("device %s: token_env is not supported; put the token in a chmod 600 file and set token_file", d.Name)
	case d.TokenFile != "":
		p := expandHome(d.TokenFile)
		// ponytail: Windows has no POSIX mode bits (Go reports 0666/0444), so the
		// check would refuse every file there; NTFS ACLs are the operator's job.
		if info, err := os.Stat(p); err == nil && runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
			return "", fmt.Errorf("device %s: token file %s is readable by other users; chmod 600 it", d.Name, p)
		}
		b, err := os.ReadFile(p) // #nosec G304 -- token_file named in the operator's devices file; mode checked above
		if err != nil {
			return "", fmt.Errorf("device %s: %w", d.Name, err)
		}
		if v := strings.TrimSpace(string(b)); v != "" {
			return v, nil
		}
		return "", fmt.Errorf("device %s: token file %s is empty", d.Name, p)
	}
	return "", fmt.Errorf("device %s: set token_file to a chmod 600 file holding its REST API token", d.Name)
}

// loadFleet resolves the devices to query. Devices whose token cannot be
// resolved are returned in failures so they show as UNKNOWN, not vanish.
func loadFleet(flags *rootFlags, o fleetOpts) ([]fleetTarget, []fetchFailure, error) {
	// A path the operator named (--devices or FORTINET_DEVICES) must exist:
	// silently falling back to the one FORTINET_BASE_URL device would audit
	// one box while the operator believes the whole fleet was checked.
	path := o.devicesPath
	explicit := path != "" || devicesEnv() != ""
	if path == "" {
		path = defaultDevicesPath()
	}
	path = expandHome(path)
	devices, err := readDevicesFile(path)
	if err != nil {
		if explicit || !errors.Is(err, os.ErrNotExist) {
			return nil, nil, configErr(err)
		}
		if strings.TrimSpace(o.only) != "" {
			return nil, nil, usageErr(fmt.Errorf("--device needs a devices file; %s does not exist (see 'fleet devices --help')", path))
		}
		cfg, cerr := config.Load(flags.configPath)
		if cerr != nil {
			return nil, nil, configErr(cerr)
		}
		if _, uerr := checkAPIURL(cfg.BaseURL); uerr != nil {
			return nil, nil, configErr(fmt.Errorf("FORTINET_BASE_URL: %w", uerr))
		}
		name := "default"
		if u, perr := url.Parse(cfg.BaseURL); perr == nil && u.Hostname() != "" {
			name = u.Hostname()
		}
		t := fleetTarget{fleetDevice: fleetDevice{Name: name, URL: cfg.BaseURL}, single: true, apiURL: cfg.BaseURL}
		t.VDOMs = splitCSV(o.vdoms)
		return []fleetTarget{t}, nil, nil
	}
	if len(devices) == 0 {
		return nil, nil, configErr(fmt.Errorf("%s lists no [[device]] entries", path))
	}
	want := map[string]bool{}
	for _, n := range splitCSV(o.only) {
		want[n] = true
	}
	var out []fleetTarget
	failures := []fetchFailure{}
	seen := map[string]bool{}
	for _, d := range devices {
		if len(want) > 0 && !want[d.Name] {
			continue
		}
		seen[d.Name] = true
		if v := splitCSV(o.vdoms); len(v) > 0 {
			d.VDOMs = v
		}
		api, err := normalizeAPIURL(d.URL)
		if err != nil {
			failures = append(failures, fetchFailure{ID: d.Name, Error: err.Error(), vdoms: d.VDOMs, url: d.URL})
			continue
		}
		tok, err := resolveToken(d)
		if err != nil {
			failures = append(failures, fetchFailure{ID: d.Name, Error: err.Error(), vdoms: d.VDOMs, url: api})
			continue
		}
		out = append(out, fleetTarget{fleetDevice: d, token: tok, apiURL: api})
	}
	var missing []string
	for n := range want {
		if !seen[n] {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		names := missing
		sort.Strings(names)
		return nil, nil, usageErr(fmt.Errorf("--device %s not found in %s", strings.Join(names, ","), path))
	}
	return out, failures, nil
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// vdomList is the configured VDOM selection, "(all)" when none is set; it
// names the selection (fleet devices, audit scope), not what a device holds.
func (t fleetTarget) vdomList() []string {
	if len(t.VDOMs) == 0 {
		return []string{"(all)"}
	}
	return t.VDOMs
}

// vdoms resolves the VDOMs to check: the configured ones, else every VDOM the
// device lists (cmdb system/vdom), so a multi-VDOM or split-VDOM FortiGate is
// never graded on root alone. When the list is unreadable it falls back to
// root and returns the error, which callers report so the device cannot PASS.
func (t fleetTarget) vdoms(ctx context.Context, c *client.Client) ([]string, error) {
	if len(t.VDOMs) > 0 {
		return t.VDOMs, nil
	}
	_, rows, err := fortiGetList(ctx, c, "/cmdb/system/vdom", map[string]string{"format": "name"})
	var names []string
	for _, r := range rows {
		if n := fortios.Str(r, "name"); n != "" {
			names = append(names, n)
		}
	}
	if err == nil && len(names) == 0 {
		err = fmt.Errorf("system/vdom listed no VDOMs")
	}
	if err != nil {
		return []string{"root"}, fmt.Errorf("could not list VDOMs (%s), so only root was checked; set --vdoms or vdoms in the devices file", shortErr(err))
	}
	return names, nil
}

// client builds a per-device client on the generated transport (retries,
// rate limiting, TLS, verify-mode handling). The response cache is off so
// one device's answer can never be served for another.
func (t fleetTarget) client(flags *rootFlags) (*client.Client, error) {
	if t.single {
		c, err := flags.newClient()
		if err != nil {
			return nil, err
		}
		c.NoCache = true
		return c, nil
	}
	cfg, err := config.Load(flags.configPath)
	if err != nil {
		return nil, configErr(err)
	}
	cfg.BaseURL = t.apiURL
	cfg.AuthHeaderVal = ""
	cfg.AccessToken = ""
	cfg.FortinetApiToken = t.token
	// Config-file headers apply to every request; never let a configured
	// credential header follow the per-device token to another device.
	for k := range cfg.Headers {
		switch strings.ToLower(k) {
		case "authorization", "cookie", "x-api-key", "x-auth-token":
			delete(cfg.Headers, k)
		}
	}
	c := client.New(cfg, flags.timeout, flags.rateLimit)
	if flags.timeoutExplicit {
		c.SetTimeoutExplicit(true)
	}
	c.DryRun = flags.dryRun
	c.NoCache = true
	if err := bindPlatformClient(c, flags); err != nil {
		return nil, err
	}
	if err := ApplyClientHooks(c); err != nil {
		return nil, err
	}
	if t.Insecure {
		skipTLSVerify(c)
	}
	return c, nil
}

// fortiEnvelope is the wrapper every FortiOS REST response carries.
type fortiEnvelope struct {
	Results json.RawMessage `json:"results"`
	Serial  string          `json:"serial"`
	Version string          `json:"version"`
	VDOM    string          `json:"vdom"`
	Status  string          `json:"status"`
}

func fortiGet(ctx context.Context, c *client.Client, path string, params map[string]string) (fortiEnvelope, error) {
	raw, err := c.Get(ctx, path, params)
	if err != nil {
		return fortiEnvelope{}, err
	}
	var env fortiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fortiEnvelope{}, fmt.Errorf("GET %s: decode response: %w", path, err)
	}
	if env.Status != "" && !strings.EqualFold(env.Status, "success") {
		return env, fmt.Errorf("GET %s: FortiOS status %q", path, env.Status)
	}
	if len(env.Results) == 0 || string(env.Results) == "null" {
		return env, fmt.Errorf("GET %s: response has no results", path)
	}
	return env, nil
}

// fortiGetObj requires `results` to be a JSON object.
func fortiGetObj(ctx context.Context, c *client.Client, path string, params map[string]string) (fortiEnvelope, fortios.Obj, error) {
	env, err := fortiGet(ctx, c, path, params)
	if err != nil {
		return env, nil, err
	}
	var o fortios.Obj
	if err := json.Unmarshal(env.Results, &o); err != nil || o == nil {
		return env, nil, fmt.Errorf("GET %s: results is not an object", path)
	}
	return env, o, nil
}

// fortiGetList requires `results` to be a JSON array.
func fortiGetList(ctx context.Context, c *client.Client, path string, params map[string]string) (fortiEnvelope, []fortios.Obj, error) {
	env, err := fortiGet(ctx, c, path, params)
	if err != nil {
		return env, nil, err
	}
	var arr []any
	if err := json.Unmarshal(env.Results, &arr); err != nil || arr == nil {
		return env, nil, fmt.Errorf("GET %s: results is not a list", path)
	}
	rows := fortios.Objs(arr)
	if len(rows) != len(arr) {
		return env, nil, fmt.Errorf("GET %s: results contains %d non-object entries", path, len(arr)-len(rows))
	}
	return env, rows, nil
}

func (e fortiEnvelope) any() any {
	var v any
	_ = json.Unmarshal(e.Results, &v)
	return v
}

func isHTTPStatus(err error, codes ...int) bool {
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	for _, c := range codes {
		if apiErr.StatusCode == c {
			return true
		}
	}
	return false
}

func vdomParams(vdom string) map[string]string {
	if vdom == "" {
		return nil
	}
	return map[string]string{"vdom": vdom}
}

// fanOut runs fn for every target with bounded concurrency. Failures stay
// attached to their device; results keep inventory order. Callers pass a
// context without a deadline: --timeout is the client's per-request timeout,
// and a fleet-wide deadline ran out at about 45 devices on default flags,
// taking the fleet audit history write down with it.
func fanOut[T any](ctx context.Context, targets []fleetTarget, concurrency int, fn func(context.Context, fleetTarget) (T, error)) ([]T, []fetchFailure) {
	if concurrency < 1 {
		concurrency = 1
	}
	results, errs := cliutil.FanoutRun(ctx, targets, func(t fleetTarget) string { return t.Name }, fn, cliutil.WithConcurrency(concurrency))
	out := make([]T, 0, len(results))
	for _, r := range results {
		out = append(out, r.Value)
	}
	failures := make([]fetchFailure, 0, len(errs))
	for _, e := range errs {
		failures = append(failures, fetchFailure{ID: e.Source, Error: e.Err.Error()})
	}
	return out, failures
}

type fetchFailure struct {
	ID    string `json:"device"`
	Error string `json:"error"`
	// vdoms and url are the device's configured VDOM selection and API URL,
	// kept so audit history scope does not change when only its credential
	// fails to resolve.
	vdoms []string
	url   string
}

func warnFailures(cmd *cobra.Command, failures []fetchFailure, total int) {
	if len(failures) > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d device(s) could not be read; they are reported as UNKNOWN and excluded from totals\n", len(failures), total)
	}
}

// ---------- fleet devices ----------

type fleetDeviceView struct {
	Name     string   `json:"name"`
	Client   string   `json:"client,omitempty"`
	URL      string   `json:"url"`
	Token    string   `json:"token"`
	Insecure bool     `json:"insecure,omitempty"`
	VDOMs    []string `json:"vdoms"`
	Ready    bool     `json:"ready"`
	Problem  string   `json:"problem,omitempty"`
}

func newFleetDevicesCmd(flags *rootFlags) *cobra.Command {
	var o fleetOpts
	cmd := &cobra.Command{
		Use:   "devices",
		Args:  rejectExtraArgs,
		Short: "List devices from the devices file and whether each one's token resolves",
		Long: strings.Trim(`
List the FortiGates the fleet, audit and policies hygiene commands will query.
Reads devices.toml ($FORTINET_DEVICES or <config dir>/devices.toml):

  [[device]]
  name = "acme-hq"
  url = "https://203.0.113.10:8443"     # /api/v2 is appended
  token_file = "~/.fgt/acme-hq"         # the REST API token, chmod 600
  insecure = true                        # accept the device's self-signed cert
  vdoms = ["root"]                       # optional; default every VDOM
  client = "Acme Corp"

Token values are never printed. Without a devices file the configured
FORTINET_BASE_URL / FORTINET_API_TOKEN device is the fleet.`, "\n"),
		Example:     "  fortinet-cli fleet devices\n  fortinet-cli fleet devices --devices ./clients.toml --json",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet devices")
			}
			targets, failures, err := loadFleet(flags, o)
			if err != nil {
				return err
			}
			rows := make([]fleetDeviceView, 0, len(targets)+len(failures))
			for _, t := range targets {
				src := ""
				row := fleetDeviceView{Name: t.Name, Client: t.Client, URL: t.apiURL, Insecure: t.Insecure, VDOMs: t.vdomList(), Ready: true}
				switch {
				case t.single:
					src = "configured credential (FORTINET_API_TOKEN / auth set-token)"
					if problem := singleDeviceProblem(flags); problem != "" {
						row.Ready, row.Problem = false, problem
					}
				case t.TokenFile != "":
					src = "token_file " + t.TokenFile
				}
				row.Token = src
				rows = append(rows, row)
			}
			for _, f := range failures {
				rows = append(rows, fleetDeviceView{Name: f.ID, VDOMs: []string{}, Problem: f.Error})
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			for _, r := range rows {
				state := "ready"
				if !r.Ready {
					state = "NOT READY: " + r.Problem
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-40s %s\n", r.Name, r.URL, state)
			}
			return nil
		},
	}
	addFleetFlags(cmd, &o)
	return cmd
}

// singleDeviceProblem explains why the no-devices-file fallback device cannot
// be queried yet ("" when it can).
func singleDeviceProblem(flags *rootFlags) string {
	cfg, err := config.Load(flags.configPath)
	if err != nil {
		return err.Error()
	}
	var missing []string
	if u, err := url.Parse(cfg.BaseURL); err != nil || u.Hostname() == "" || strings.HasSuffix(u.Hostname(), "example.com") {
		missing = append(missing, "FORTINET_BASE_URL (https://<fortigate>/api/v2) or a devices file")
	}
	if cfg.AuthHeader() == "" {
		missing = append(missing, "FORTINET_API_TOKEN (or auth set-token)")
	}
	if len(missing) == 0 {
		return ""
	}
	return "set " + strings.Join(missing, " and ")
}

// ---------- fleet backup ----------

// backupFailureErr makes a run with any failed backup exit 5 (after the report
// is printed) so a scheduled backup cannot fail silently.
func backupFailureErr(failures []fetchFailure, ok int) error {
	if len(failures) == 0 {
		return nil
	}
	return apiErr(fmt.Errorf("%d backup(s) failed, %d written", len(failures), ok))
}

type backupResult struct {
	Device string `json:"device"`
	File   string `json:"file"`
	Bytes  int    `json:"bytes"`
}

// backupConfig downloads the full-configuration backup. GET is the form in
// Fortinet's fortios_monitor_fact selector system_config_backup; builds that
// answer 404/405 to GET get the POST form (fortios_monitor backup.system.config).
func backupConfig(ctx context.Context, c *client.Client) ([]byte, error) {
	// The binary download drops the client's whole-request timeout so a large
	// configuration can stream; a device that sends headers and then stalls
	// would hang the backup forever. Bound the whole download instead.
	ctx, cancel := context.WithTimeout(ctx, 5*c.ConfiguredTimeout())
	defer cancel()
	headers := map[string]string{client.BinaryResponseHeader: "true"}
	raw, err := c.GetWithHeaders(ctx, "/monitor/system/config/backup", map[string]string{"scope": "global"}, headers)
	if err != nil && isHTTPStatus(err, 404, 405) {
		raw, _, err = c.PostWithHeaders(ctx, "/monitor/system/config/backup", map[string]any{"destination": "file", "scope": "global"}, headers)
	}
	if err != nil {
		return nil, err
	}
	if b, _, ok := client.UnwrapBinaryResponse(raw); ok {
		raw = b
	}
	if !strings.Contains(string(raw), "config ") {
		return nil, fmt.Errorf("backup response does not look like a FortiOS configuration")
	}
	return raw, nil
}

func backupDir(dir string) (string, error) {
	if dir != "" {
		return expandHome(dir), nil
	}
	data, err := cliutil.DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(data, "backups"), nil
}

func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || r == '.' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, s)
}

// deviceDirName is the backup directory for a device. A name that had to be
// sanitized gets "~<hash>": safeName never emits "~", so a sanitized name can
// never equal a clean one ("a/b" and "a_b" never share a directory).
func deviceDirName(name string) string {
	safe := safeName(name)
	if safe == name && safe != "" && safe != "." && safe != ".." {
		return safe
	}
	sum := sha256.Sum256([]byte(name))
	return safe + "~" + hex.EncodeToString(sum[:8])
}

// backupDeviceDir is the backup directory for one firewall: its name, "@",
// and a short hash of its normalized API URL. Two firewalls that share an
// inventory name (an "hq" in two clients' devices files) never share a
// directory, so fleet changes can never diff one client's configuration
// against another's. safeName never emits "@", so the separator is
// unambiguous.
func backupDeviceDir(name, apiURL string) string {
	sum := sha256.Sum256([]byte(apiURL))
	return deviceDirName(name) + "@" + hex.EncodeToString(sum[:6])
}

// backupStamp names backups; nanoseconds keep same-second runs distinct.
const backupStamp = "20060102T150405,000000000Z"

// writeNewPrivateFile creates path exclusively with 0600 permissions: it never
// overwrites an existing file and never follows a planted symlink.
func writeNewPrivateFile(path string, body []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- backup path built from the backup dir + sanitized device dir; O_EXCL refuses existing files and symlinks
	if err != nil {
		return err
	}
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return err
	}
	return f.Close()
}

func newFleetBackupCmd(flags *rootFlags) *cobra.Command {
	var o fleetOpts
	var outDir string
	cmd := &cobra.Command{
		Use:   "backup",
		Args:  rejectExtraArgs,
		Short: "Download each FortiGate's full configuration into dated 0600 files",
		Long: strings.Trim(`
Download every device's full configuration backup (scope global) and write it
to <out-dir>/<device>@<id>/<device>-<UTC timestamp>.conf with 0600 permissions,
where <id> is a short hash of the device's API URL: two firewalls that share a
name in different devices files never share a directory.
Backups contain encrypted secrets and private keys: keep the directory private.
'fleet changes' diffs these files (pass it the same --dir if you use --out-dir).
Default out-dir: <data dir>/backups. Builds that refuse the GET form get the POST
form of the same backup call; neither changes device configuration.`, "\n"),
		Example:     "  fortinet-cli fleet backup\n  fortinet-cli fleet backup --out-dir ./fortigate-backups --device acme-hq",
		Annotations: map[string]string{"pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet backup")
			}
			ctx, cancel := context.WithCancel(cmd.Context()) // --timeout bounds each request, not the fleet: see fanOut
			defer cancel()
			targets, failures, err := loadFleet(flags, o)
			if err != nil {
				return err
			}
			dir, err := backupDir(outDir)
			if err != nil {
				return err
			}
			stamp := time.Now().UTC().Format(backupStamp)
			done, fails := fanOut(ctx, targets, o.concurrency, func(ctx context.Context, t fleetTarget) (backupResult, error) {
				c, err := t.client(flags)
				if err != nil {
					return backupResult{}, err
				}
				body, err := backupConfig(ctx, c)
				if err != nil {
					return backupResult{}, err
				}
				devDir := filepath.Join(dir, backupDeviceDir(t.Name, t.apiURL))
				if err := os.MkdirAll(devDir, 0o700); err != nil {
					return backupResult{}, err
				}
				file := filepath.Join(devDir, deviceDirName(t.Name)+"-"+stamp+".conf")
				if err := writeNewPrivateFile(file, body); err != nil {
					return backupResult{}, err
				}
				return backupResult{Device: t.Name, File: file, Bytes: len(body)}, nil
			})
			failures = append(failures, fails...)
			warnFailures(cmd, failures, len(targets)+len(failures)-len(fails))
			view := struct {
				Backups       []backupResult `json:"backups"`
				FetchFailures []fetchFailure `json:"fetch_failures,omitempty"`
			}{done, failures}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), view, flags); err != nil {
					return err
				}
				return backupFailureErr(failures, len(done))
			}
			for _, b := range done {
				fmt.Fprintf(cmd.OutOrStdout(), "%-24s %8d bytes  %s\n", b.Device, b.Bytes, b.File)
			}
			for _, f := range failures {
				fmt.Fprintf(cmd.OutOrStdout(), "%-24s FAILED: %s\n", f.ID, f.Error)
			}
			return backupFailureErr(failures, len(done))
		},
	}
	addFleetFlags(cmd, &o)
	cmd.Flags().StringVar(&outDir, "out-dir", "", "Directory for backup files (default <data dir>/backups)")
	return cmd
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		if fleet, _, err := root.Find([]string{"fleet"}); err == nil && fleet != root {
			fleet.Short = "Run read-only checks across every FortiGate in your devices file"
			addNovelCommandIfAbsent(fleet, newFleetDevicesCmd(flags))
			addNovelCommandIfAbsent(fleet, newFleetBackupCmd(flags))
		}
	})
}
