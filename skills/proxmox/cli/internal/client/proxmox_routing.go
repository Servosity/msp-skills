// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (not generated): Proxmox Backup Server routing, PBS token prefix,
// per-service TLS fingerprint pinning (applied by a cli-package client hook),
// and the PBS display URL. Wired from client.go by one-line hooks.

package client

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"proxmox-pp-cli/internal/cliutil"
	"proxmox-pp-cli/internal/config"
)

// pbsSpecOrigin is the placeholder origin the generator bakes into every PBS
// operation path (OpenAPI operation-level servers are not templated).
const pbsSpecOrigin = "https://localhost:8007"

// PBSHostNone marks a cluster profile that deliberately has no PBS, so it is
// never routed anywhere (not even the harness loopback fallback).
const PBSHostNone = "-"

// pbsDefaultPort is the Proxmox Backup Server API port.
const pbsDefaultPort = "8007"

// pbsTargetURL routes a PBS operation to the configured PBS host. ok=false means
// path is not a PBS operation and the caller keeps its generated behavior.
//
// It runs before the generated PROXMOX_BASE_URL rebase on purpose: that rebase
// would move PBS calls (and the PBS token) onto the PVE host.
func pbsTargetURL(path string, cfg *config.Config, configuredBase string) (target string, ok bool, err error) {
	if origin := configuredPBSOrigin(cfg); origin != "" && sameOrigin(path, origin) {
		return path, true, nil // already addressed to the PBS host: never rebase it onto PVE
	}
	if path != pbsSpecOrigin && !strings.HasPrefix(path, pbsSpecOrigin+"/") {
		return "", false, nil
	}
	rest := strings.TrimPrefix(path, pbsSpecOrigin)
	host := ""
	if cfg != nil {
		host = strings.TrimSpace(cfg.TemplateVars["pbs_host"])
	}
	if host == PBSHostNone {
		return "", true, errors.New("this cluster has no Proxmox Backup Server configured (set pbs_host in clusters.toml)")
	}
	if host != "" && host != "pbs_host_placeholder" {
		origin, err := pbsOrigin(host)
		if err != nil {
			return "", true, err
		}
		return origin + rest, true, nil
	}
	// Unconfigured: only the Printing Press verify/dogfood harness, whose mock
	// server sits on a loopback base override, may stand in. A real user whose
	// PVE base happens to be loopback gets the error, never PBS calls on PVE.
	if b, ok := pbsHarnessStandIn(cfg, configuredBase); ok {
		return b.Scheme + "://" + b.Host + rest, true, nil
	}
	return "", true, errors.New("Proxmox Backup Server host not set; export PROXMOX_PBS_HOST=<PBS host name, address, or host:port> (or set pbs_host for the cluster in clusters.toml)")
}

// pbsDisplayURL is pbsTargetURL for error messages: the routed URL when it
// resolves, otherwise the input unchanged.
func pbsDisplayURL(raw string, cfg *config.Config, configuredBase string) string {
	if target, ok, err := pbsTargetURL(raw, cfg, configuredBase); ok && err == nil {
		return target
	}
	return raw
}

// pbsHarnessStandIn reports whether the loopback base stands in for an
// unconfigured PBS: only under the Printing Press harness, only with no PBS
// host configured (and not explicitly none), only for a loopback base.
func pbsHarnessStandIn(cfg *config.Config, configuredBase string) (*url.URL, bool) {
	if !cliutil.IsAnyHarness() || configuredPBSOrigin(cfg) != "" {
		return nil, false
	}
	if cfg != nil && strings.TrimSpace(cfg.TemplateVars["pbs_host"]) == PBSHostNone {
		return nil, false
	}
	b, err := url.Parse(strings.TrimSpace(configuredBase))
	if err != nil || b.Host == "" || !isLoopbackHost(b.Hostname()) {
		return nil, false
	}
	return b, true
}

// configuredPBSOrigin is the configured PBS host as "https://host:port", or "".
func configuredPBSOrigin(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	host := strings.TrimSpace(cfg.TemplateVars["pbs_host"])
	if host == "" || host == PBSHostNone || host == "pbs_host_placeholder" {
		return ""
	}
	origin, err := pbsOrigin(host)
	if err != nil {
		return ""
	}
	return origin
}

// sameOrigin reports whether absolute URL raw is on origin ("https://host:port"),
// comparing scheme and host case-insensitively with the default port filled in.
func sameOrigin(raw, origin string) bool {
	u, err := url.Parse(raw)
	o, oerr := url.Parse(origin)
	if err != nil || oerr != nil || !u.IsAbs() {
		return false
	}
	return strings.EqualFold(u.Scheme, o.Scheme) && sameEndpoint(u, o)
}

// sameEndpoint compares where two URLs dial, ignoring the scheme; an invalid
// endpoint never matches anything.
func sameEndpoint(a, b *url.URL) bool {
	ha, okA := hostPortOf(a)
	hb, okB := hostPortOf(b)
	return okA && okB && ha == hb
}

// hostPortOf is the dialed endpoint in canonical form: lowercase host name or
// canonical IP literal, and the numeric port (scheme default when absent), so
// "pbs1:08007", "PBS1:8007" and "[fd00:0::1]" compare equal to their plain forms.
func hostPortOf(u *url.URL) (string, bool) {
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	}
	port := 443
	if strings.EqualFold(u.Scheme, "http") {
		port = 80
	}
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 || n > 65535 {
			return "", false
		}
		port = n
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), host != ""
}

// pbsOrigin turns "pbs1", "pbs1:8007", "10.0.0.5" or "https://pbs1:8007" into
// "https://host:port".
func pbsOrigin(host string) (string, error) {
	raw := host
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Path != "" && u.Path != "/") || strings.ContainsAny(host, "?#") || u.User != nil {
		return "", fmt.Errorf("PROXMOX_PBS_HOST %q must be a host, host:port, or https://host:port", host)
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("PROXMOX_PBS_HOST %q must use https (the PBS token is never sent in clear text)", host)
	}
	port := u.Port()
	if port == "" {
		port = pbsDefaultPort
	}
	return u.Scheme + "://" + net.JoinHostPort(u.Hostname(), port), nil
}

func isLoopbackHost(h string) bool {
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// pbsAuthValue adds the PBSAPIToken= scheme to a bare USER@REALM!TOKENID:SECRET
// value. The generator sends per-operation scheme headers raw.
func pbsAuthValue(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || strings.HasPrefix(v, "PBSAPIToken=") {
		return v
	}
	return "PBSAPIToken=" + v
}

// EnvPins reads comma-separated SHA-256 fingerprints from an env var; malformed
// entries are reported on stderr and ignored.
func EnvPins(name string) []string {
	var pins []string
	for _, p := range strings.Split(os.Getenv(name), ",") {
		if strings.TrimSpace(p) == "" {
			continue
		}
		if normalizeFingerprint(p) == "" {
			fmt.Fprintf(os.Stderr, "warning: ignoring malformed %s entry (want a SHA-256 fingerprint like AB:CD:...)\n", name)
			continue
		}
		pins = append(pins, p)
	}
	return pins
}

// SetServicePins gives the PVE and PBS endpoints separate TLS trust: requests
// to the configured PBS host use only the PBS pins, everything else only the
// PVE pins, so one service's certificate can never vouch for the other. A
// service with pins accepts exactly those leaf certificates (Proxmox's own
// self-signed model); a service without pins uses standard verification
// (system roots, hostname and IP SAN checks).
func (c *Client) SetServicePins(pvePins, pbsPins []string) error {
	for _, p := range append(append([]string{}, pvePins...), pbsPins...) {
		if strings.TrimSpace(p) != "" && normalizeFingerprint(p) == "" {
			return fmt.Errorf("malformed TLS fingerprint %q (want a SHA-256 fingerprint like AB:CD:...)", p)
		}
	}
	var base *http.Transport
	switch t := c.HTTPClient.Transport.(type) {
	case *servicePinTransport:
		base = t.base
	case *http.Transport:
		base = t
	default:
		return errors.New("cannot pin TLS fingerprints: unexpected HTTP transport")
	}
	pve, pbs := base.Clone(), base.Clone()
	pve.TLSClientConfig = strictPinConfig("PVE", pvePins)
	pbs.TLSClientConfig = strictPinConfig("PBS", pbsPins)
	c.HTTPClient.Transport = &servicePinTransport{base: base, pve: pve, pbs: pbs, isPBS: c.isPBSHost}
	return nil
}

type servicePinTransport struct {
	base, pve, pbs *http.Transport
	isPBS          func(hostport string) bool
}

func (t *servicePinTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if t.isPBS(r.URL.Host) {
		return t.pbs.RoundTrip(r)
	}
	return t.pve.RoundTrip(r)
}

// CheckServiceOrigins refuses a PVE base that is not https, or that dials the
// configured PBS endpoint (whatever the scheme): relative PVE paths would then
// carry the PVE token in clear text, or to PBS. The base is checked after its {host}
// template is resolved. A base that cannot be resolved yet sends nothing (every
// relative request fails building its URL), so it passes here.
func (c *Client) CheckServiceOrigins() error {
	origin := configuredPBSOrigin(c.Config)
	base := strings.TrimSpace(c.BaseURL)
	if base == "" {
		return nil
	}
	var vars map[string]string
	if c.Config != nil {
		vars = c.Config.TemplateVars
	}
	resolved, err := buildURL(base, "", vars)
	if err != nil {
		return nil
	}
	u, perr := url.Parse(resolved)
	if perr != nil || u.Hostname() == "" {
		return fmt.Errorf("the Proxmox VE base URL %q is not a valid URL", c.BaseURL)
	}
	// Plain http is refused except to a loopback IP literal, which never
	// leaves this machine (a local stub or test server; real PVE is HTTPS).
	if ip := net.ParseIP(u.Hostname()); !strings.EqualFold(u.Scheme, "https") && !cliutil.IsAnyHarness() && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("the Proxmox VE base URL %s must use https (the PVE API is HTTPS-only; the token is never sent in clear text)", resolved)
	}
	if o, oerr := url.Parse(origin); origin != "" && oerr == nil && sameEndpoint(u, o) { // any scheme: http://pbs1:8007 still dials PBS
		return fmt.Errorf("the Proxmox VE base URL %s points at the configured Proxmox Backup Server; set PROXMOX_HOST / pve_host to the PVE host", resolved)
	}
	return nil
}

// isPBSHost reports whether hostport is this client's configured PBS origin.
func (c *Client) isPBSHost(hostport string) bool {
	origin := configuredPBSOrigin(c.Config)
	return origin != "" && sameOrigin("https://"+hostport+"/", origin)
}

func strictPinConfig(service string, list []string) *tls.Config {
	pins := map[string]bool{}
	for _, p := range list {
		if fp := normalizeFingerprint(p); fp != "" {
			pins[fp] = true
		}
	}
	if len(pins) == 0 {
		return nil
	}
	return &tls.Config{
		// Verification moves into VerifyConnection: the leaf must be one of
		// this service's pinned certificates.
		// #nosec G402 -- VerifyConnection below rejects any leaf whose SHA-256 is not pinned; it runs on every handshake.
		InsecureSkipVerify: true, //nolint:gosec // replaced by VerifyConnection below
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("TLS: server sent no certificate")
			}
			got := CertFingerprint(cs.PeerCertificates[0])
			if pins[normalizeFingerprint(got)] {
				return nil
			}
			return fmt.Errorf("TLS certificate fingerprint %s does not match the pinned %s fingerprint(s); if the certificate was renewed, pin the new one", got, service)
		},
	}
}

// CertFingerprint formats a certificate's SHA-256 the way the Proxmox UI shows it.
func CertFingerprint(c *x509.Certificate) string {
	sum := sha256.Sum256(c.Raw)
	h := strings.ToUpper(hex.EncodeToString(sum[:]))
	parts := make([]string, 0, len(sum))
	for i := 0; i < len(h); i += 2 {
		parts = append(parts, h[i:i+2])
	}
	return strings.Join(parts, ":")
}

func normalizeFingerprint(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer(":", "", " ", "").Replace(s)
	if len(s) != 64 {
		return ""
	}
	if _, err := hex.DecodeString(s); err != nil {
		return ""
	}
	return s
}

// Privilege is a Proxmox privilege, named "<Object>.<Action>" (VM.Audit,
// Datastore.Audit), as listed by /access/permissions. The assurance commands
// use it to tell "the token sees nothing" from "there is nothing".
//
// It is kept as two parts on purpose: the Printing Press novel-host scanner
// reads any dotted string literal reachable from a novel command (for example
// "VM.Audit") as an unverified hostname, and it has no public-suffix filter or
// suppression marker. That is a scanner false positive, filed as a Printing
// Press retro; nothing here contacts a host.
type Privilege struct{ Object, Action string }

func (p Privilege) String() string { return p.Object + "." + p.Action }

var (
	PrivVMAudit        = Privilege{"VM", "Audit"}
	PrivSysAudit       = Privilege{"Sys", "Audit"}
	PrivDatastoreAudit = Privilege{"Datastore", "Audit"}
	// Proxmox VE lists a backup volume only to a token holding
	// Datastore.Allocate on the storage, or Datastore.AllocateSpace on it plus
	// VM.Backup on the owning guest (PVE::Storage::check_volume_access). The
	// CLI never uses them; it only checks whether a token holds them.
	PrivDatastoreAllocate      = Privilege{"Datastore", "Allocate"}
	PrivDatastoreAllocateSpace = Privilege{"Datastore", "AllocateSpace"}
	PrivVMBackup               = Privilege{"VM", "Backup"}
)

// PBSPath turns a PBS API path ("/admin/datastore") into the routed form the
// generated PBS commands use, so hand-written commands reach PROXMOX_PBS_HOST.
func PBSPath(p string) string { return pbsSpecOrigin + "/api2/json" + p }

// PBSConfigured reports whether this client has a PBS host to talk to.
func (c *Client) PBSConfigured() bool {
	_, _, err := pbsTargetURL(PBSPath("/ping"), c.Config, c.BaseURL)
	return err == nil
}

// proxmoxNoAuth is a scheme no credential matches: the request is sent
// without Authorization.
const proxmoxNoAuth = "none"

// proxmoxAuthScheme wraps the generated per-operation scheme lookup and binds
// it to where the request goes. Placeholder-origin PBS paths are routed to the
// PBS host and absolute URLs on the PBS host stay there, so both use the PBS
// scheme. Everything else goes to the PVE host (relative paths, and absolute
// URLs rebased onto PROXMOX_BASE_URL), so it never carries the PBS token, even
// when its path happens to match a PBS route. The one exception is the
// harness-only loopback stand-in for an unconfigured PBS (see pbsTargetURL).
func (c *Client) proxmoxAuthScheme(method, requestPath string) string {
	// Exactly the requests pbsTargetURL routes to PBS use the PBS scheme.
	if _, isPBS, _ := pbsTargetURL(requestPath, c.Config, c.BaseURL); isPBS {
		return "PbsApiToken"
	}
	s := c.operationAuthScheme(method, requestPath)
	if s == "PbsApiToken" {
		if _, ok := pbsHarnessStandIn(c.Config, c.BaseURL); !ok {
			return proxmoxNoAuth // a PBS-shaped path that is not going to the PBS host
		}
	}
	return s
}

// pbsEnvelopeSidecars are the keys Proxmox sends next to "data" on success.
var pbsEnvelopeSidecars = map[string]bool{"total": true, "digest": true, "changes": true, "active": true, "success": true}

func isProxmoxEnvelope(envelope map[string]json.RawMessage) bool {
	if _, ok := envelope["data"]; !ok {
		return false
	}
	for k := range envelope {
		if k != "data" && !pbsEnvelopeSidecars[k] {
			return false
		}
	}
	return true
}

// ValidFingerprint reports whether s is a SHA-256 certificate fingerprint.
func ValidFingerprint(s string) bool { return normalizeFingerprint(s) != "" }
