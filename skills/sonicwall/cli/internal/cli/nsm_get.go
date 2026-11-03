// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// nsmPathChars is the character set NSM paths use; anything else (percent
// escapes, spaces, query strings) is rejected rather than normalised.
var nsmPathChars = regexp.MustCompile(`^[A-Za-z0-9/_.~{}-]+$`)

// nsmGetPath validates an escape-hatch path: relative to /api/manager, never an
// absolute URL, and never a MySonicWall route (the client attaches the
// MySonicWall key to those, and they must not reach the NSM host). The path is
// canonicalised (duplicate slashes and dot segments removed) and the canonical
// form is what gets sent, so the check and the request see the same path.
func nsmGetPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("path is required, e.g. /devices/summary")
	}
	if strings.Contains(p, "://") || strings.HasPrefix(p, "//") {
		return "", fmt.Errorf("path must be relative to the NSM API (e.g. /devices/summary), not a URL")
	}
	if !nsmPathChars.MatchString(p) {
		return "", fmt.Errorf("path %q has characters NSM paths do not use (percent escapes, spaces or a query string); pass query values with --param", p)
	}
	clean := path.Clean("/" + p)
	if clean == "/api/manager" || strings.HasPrefix(clean, "/api/manager/") {
		clean = path.Clean("/" + strings.TrimPrefix(clean, "/api/manager"))
	}
	lower := strings.ToLower(clean)
	if lower == "/api" || strings.HasPrefix(lower, "/api/") {
		return "", fmt.Errorf("%s looks like a MySonicWall path; nsm-get only reads NSM paths under /api/manager", clean)
	}
	if clean == "/" {
		return "", fmt.Errorf("path is required, e.g. /devices/summary")
	}
	if credentialRoute(lower) {
		return "", fmt.Errorf("%s returns or exchanges a credential (token, password, key pair, TOTP secret or auth code); nsm-get does not read credential routes", clean)
	}
	if !documentedReadRoute(clean) {
		return "", fmt.Errorf("%s is not a vendor-documented NSM or SonicOS read route that nsm-get allows (state-changing GETs such as resets, syncs and exports are refused)", clean)
	}
	return clean, nil
}

// documentedReadRoute reports whether the most specific vendor GET template
// matching clean is an allowed one. A {param} segment matches any one non-empty
// segment; a template with more literal segments wins, so an excluded static
// route (/devices/export) is never re-admitted by an allowed parameterised one
// (/devices/{serialNumber}).
func documentedReadRoute(clean string) bool {
	segs := strings.Split(strings.Trim(clean, "/"), "/")
	best, allowed := -1, false
	consider := func(routes []string, ok bool) {
		for _, route := range routes {
			if lit, match := templateMatch(route, segs); match && lit > best {
				best, allowed = lit, ok
			} else if match && lit == best && !ok {
				allowed = false // ties go to refusal
			}
		}
	}
	consider(nsmGetRoutes, true)
	consider(nsmRefusedRoutes, false)
	return best >= 0 && allowed
}

// templateMatch reports whether route matches segs, with its literal-segment count.
func templateMatch(route string, segs []string) (int, bool) {
	tmpl := strings.Split(strings.Trim(route, "/"), "/")
	if len(tmpl) != len(segs) {
		return 0, false
	}
	literal := 0
	for i, t := range tmpl {
		if strings.HasPrefix(t, "{") && strings.HasSuffix(t, "}") {
			if segs[i] == "" {
				return 0, false
			}
			continue
		}
		if t != segs[i] {
			return 0, false
		}
		literal++
	}
	return literal, true
}

// credentialRoute reports NSM / SonicOS-passthrough GET routes whose responses
// are, or are exchangeable for, credentials (from the vendor specs: /auth/tempToken,
// getNSMAuthCode, users/{id}/totp, cert-key-pair exports, k8s-dashboard/token, ...).
// ponytail: segment denylist over the vendor route vocabulary; review it when the NSM spec adds routes.
func credentialRoute(lowerPath string) bool {
	for _, seg := range strings.Split(lowerPath, "/") {
		switch {
		case seg == "auth", seg == "totp", seg == "license-wrapper",
			strings.Contains(seg, "token"), strings.Contains(seg, "password"), strings.Contains(seg, "passwd"),
			strings.Contains(seg, "authcode"), strings.Contains(seg, "key-pair"), strings.Contains(seg, "recoverycode"),
			strings.Contains(seg, "secret"), strings.Contains(seg, "credential"):
			return true
		}
	}
	return false
}

func newNsmGetCmd(flags *rootFlags) *cobra.Command {
	var device string
	var params []string
	cmd := &cobra.Command{
		Use:   "nsm-get [path]",
		Short: "GET a vendor-documented NSM read route by path, or a SonicOS passthrough read with --device <serial>; credential and state-changing routes are refused (escape hatch)",
		Long: "Send an authenticated GET to a vendor-documented NSM API read route under /api/manager. With --device,\n" +
			"the call is a SonicOS passthrough read on that firewall (sent as the X-DEVICE-ID header), reaching the\n" +
			"SonicOS reads NSM exposes beyond the dedicated commands. Only GET routes documented in the vendor NSM and\n" +
			"SonicOS specs are allowed; routes that return credentials, and routes the vendor describes as resets,\n" +
			"syncs, exports or other actions, are refused. Responses are never cached, and values under\n" +
			"credential-shaped keys (passwords, secrets, tokens, keys) are redacted before printing.",
		Example: strings.Trim(`
  sonicwall-cli nsm-get /appVersion
  sonicwall-cli nsm-get /devices/summary --json
  sonicwall-cli nsm-get /firewall/reporting/status/system --device 2CB8ED000001
  sonicwall-cli nsm-get /alert/count --param severity=critical`, "\n"),
		// Not mcp:read-only: GET alone does not prove a vendor route has no side effect.
		Annotations: map[string]string{"pp:data-source": "live", "pp:happy-args": "path=/appVersion"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "nsm-get")
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("path is required, e.g. sonicwall-cli nsm-get /devices/summary"))
			}
			path, err := nsmGetPath(args[0])
			if err != nil {
				return usageErr(err)
			}
			query := map[string]string{}
			for _, kv := range params {
				k, v, ok := strings.Cut(kv, "=")
				if !ok || strings.TrimSpace(k) == "" {
					return usageErr(fmt.Errorf("--param must be key=value, got %q", kv))
				}
				query[strings.TrimSpace(k)] = v
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			c.NoCache = true // raw responses may carry secrets; never write them to the response cache
			var raw json.RawMessage
			if device != "" {
				raw, err = c.GetWithHeaders(cmd.Context(), path, query, map[string]string{"X-DEVICE-ID": device})
			} else {
				raw, err = c.Get(cmd.Context(), path, query)
			}
			if err != nil {
				return classifyReadError(cmd.OutOrStdout(), err, flags)
			}
			var v any
			if err := json.Unmarshal(raw, &v); err != nil {
				// Unparseable bodies cannot be redacted, so they are never echoed.
				return apiErr(fmt.Errorf("%s did not return readable JSON (%d bytes); nsm-get prints only JSON it can redact", path, len(raw)))
			}
			return printJSONFiltered(cmd.OutOrStdout(), redactCredentials(v), flags)
		},
	}
	cmd.Flags().StringVar(&device, "device", "", "Firewall serial number for a SonicOS passthrough read (sent as X-DEVICE-ID)")
	cmd.Flags().StringArrayVar(&params, "param", nil, "Query parameter as key=value (repeatable)")
	return cmd
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		addNovelCommandIfAbsent(root, newNsmGetCmd(flags))
	})
}

// credentialKey matches response field names that carry secrets in the vendor
// schemas (devicePassword, pop_pass, tempToken, authCode, private_key, psk, ...).
// ponytail: key-name heuristic; it redacts too much (e.g. tokenExpiry) rather than too little.
var keyTokenSplit = regexp.MustCompile(`[_\-\s.]+|([a-z0-9])([A-Z])`)

func credentialKey(k string) bool {
	if nsmCredentialFields[strings.ToLower(k)] {
		return true // the vendor schema documents this field as a secret
	}
	tokens := strings.Fields(strings.ToLower(keyTokenSplit.ReplaceAllString(k, "$1 $2")))
	for _, t := range tokens {
		switch t {
		case "pass", "pwd", "pw", "psk", "totp", "otp", "pin", "passcode", "key", "keys":
			return true
		}
	}
	joined := strings.Join(tokens, "")
	for _, frag := range []string{"password", "passwd", "secret", "token", "authcode", "privatekey", "sharedkey",
		"presharedkey", "recoverycode", "apikey", "credential", "passphrase"} {
		if strings.Contains(joined, frag) {
			return true
		}
	}
	return false
}

// redactCredentials replaces every non-empty scalar under a credential-shaped
// key, including scalars nested anywhere inside a credential-shaped container
// (cli.ftp.password.default, wep_key.key1.value). Containers themselves are
// never blanked wholesale, so ordinary configuration objects stay readable.
func redactCredentials(v any) any {
	return redactWalk(v, false)
}

func redactWalk(v any, inSecret bool) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			secret := inSecret || credentialKey(k)
			switch val.(type) {
			case map[string]any, []any:
				t[k] = redactWalk(val, secret)
				continue
			}
			if secret && val != nil && val != "" {
				t[k] = "[redacted]"
			}
		}
		return t
	case []any:
		for i := range t {
			switch t[i].(type) {
			case map[string]any, []any:
				t[i] = redactWalk(t[i], inSecret)
			default:
				if inSecret && t[i] != nil && t[i] != "" {
					t[i] = "[redacted]"
				}
			}
		}
		return t
	}
	return v
}
