// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): what the shared credential kit does not read by
// itself and this connector still gives it, where it is read (credkit-wiring):
// an undeclared SUPEROPS_* value, a proxy URL's userinfo and the fields of a
// loaded config.

package cliutil

import (
	"errors"
	"os"
	"reflect"
	"regexp"
	"strings"
	"time"

	"superops-pp-cli/internal/credkit"
)

// Hold is credkit.Hold. A variable because the kit's held set is process-wide
// with no reset: in-process tests (no credkit.Start) would check every later
// statement and file against their fixtures. The built binaries are the proof
// of the hold (cmd/superops-mcp e2e tests).
var Hold = credkit.Hold

// HoldValues holds values the connector read or minted itself. Their origins
// are the "*" rule of credwire.Declarations.
func HoldValues(values ...string) { Hold("minted:connector", values...) }

// HeldText reports whether the kit withholds text as a file record. It is for a
// record the program drops and goes on without (a kept log line, a prune); every
// other write simply goes through the kit. A clean text is written to the null
// device, so nothing but the check happens.
func HeldText(text string) bool {
	return errors.Is(credkit.WriteFile(os.DevNull, []byte(text), 0o600), credkit.ErrWithheld)
}

// HoldProcessCredentials holds what the environment and the command line carry,
// as main's second statement, before any command or tool runs.
func HoldProcessCredentials() { holdProcessCredentials(os.Environ(), os.Args) }

// nonSecretEnvVars are the SUPEROPS_* settings that never hold a credential:
// paths, switches, names and enumerations. Every other SUPEROPS_* value is a
// credential and is held whole (fail-closed: a variable a reprint adds is held
// until it is named here). SUPEROPS_BASE_URL is shown deliberately, so only
// its userinfo is held (the URL rule below).
var nonSecretEnvVars = map[string]bool{
	"SUPEROPS_BASE_URL": true, "SUPEROPS_CA_CERT": true, "SUPEROPS_CACHE_DIR": true,
	"SUPEROPS_CLI_PATH": true, "SUPEROPS_CONFIG": true, "SUPEROPS_CONFIG_DIR": true,
	"SUPEROPS_DATA_DIR": true, "SUPEROPS_DATA_SOURCE": true, "SUPEROPS_FEEDBACK_AUTO_SEND": true, "SUPEROPS_FEEDBACK_ENDPOINT": true,
	"SUPEROPS_HOME": true, "SUPEROPS_LEARN_NO_CAPTURE": true, "SUPEROPS_LEARN_SESSION": true,
	"SUPEROPS_LEARN_SURFACE": true, "SUPEROPS_MAX_RESPONSE_BYTES": true, "SUPEROPS_MCP_CHILD": true,
	"SUPEROPS_MCP_READ_ONLY": true, "SUPEROPS_MCP_TOKEN_HELPER": true, "SUPEROPS_NO_LEARN": true,
	"SUPEROPS_NO_LOCAL_DATA": true, "SUPEROPS_REGION": true, "SUPEROPS_STATE_DIR": true,
	"SUPEROPS_SUBDOMAIN": true, "SUPEROPS_TLS_SERVER_NAME": true, "SUPEROPS_USER_AGENT": true,
}

// proxyEnvVars name a URL that may be written without a scheme
// ("user:password@proxy:8080"); its userinfo is held whole, username alone
// included.
var proxyEnvVars = map[string]bool{"HTTP_PROXY": true, "HTTPS_PROXY": true, "ALL_PROXY": true, "NO_PROXY": true, "FTP_PROXY": true}

// holdProcessCredentials holds every SUPEROPS_* value not named in
// nonSecretEnvVars and the userinfo of a proxy setting. A URL in any other
// value or argument is free text, which contract section 1 does not name.
func holdProcessCredentials(env, args []string) {
	for _, kv := range env {
		name, value, _ := strings.Cut(kv, "=")
		upper := strings.ToUpper(name)
		if undeclaredSecretEnv(upper) {
			HoldValues(value)
		}
		if proxyEnvVars[upper] {
			HoldURLUserinfo(value)
		}
	}
}

func undeclaredSecretEnv(upper string) bool {
	return strings.HasPrefix(upper, "SUPEROPS_") && !nonSecretEnvVars[upper]
}

// nonSecretHeaders are the request headers that never carry a credential.
// Every other configured header value is one.
var nonSecretHeaders = []string{"CustomerSubDomain", "Content-Type", "Accept", "User-Agent"}

func nonSecretHeader(name string) bool {
	for _, h := range nonSecretHeaders {
		if strings.EqualFold(strings.TrimSpace(name), h) {
			return true
		}
	}
	return false
}

// CredentialStrings is every credential value v holds: each string reachable
// from it (fields, pointers, maps, slices) except fields named in nonSecret,
// time values, a "BaseURL" field (only its userinfo: URLUserinfoSecrets) and
// the values of a "Headers" map under a name in nonSecretHeaders. A header
// value also gives its components (headerComponents).
// Fail-closed: a field nobody named is a credential.
func CredentialStrings(v any, nonSecret ...string) []string {
	skip := map[string]bool{}
	for _, name := range nonSecret {
		skip[name] = true
	}
	var out []string
	var walk func(reflect.Value, int)
	walk = func(rv reflect.Value, depth int) {
		if !rv.IsValid() || depth > 16 {
			return
		}
		switch rv.Kind() {
		case reflect.String:
			out = append(out, rv.String())
		case reflect.Pointer, reflect.Interface:
			if !rv.IsNil() {
				walk(rv.Elem(), depth+1)
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < rv.Len(); i++ {
				walk(rv.Index(i), depth+1)
			}
		case reflect.Map:
			iter := rv.MapRange()
			for iter.Next() {
				walk(iter.Key(), depth+1)
				walk(iter.Value(), depth+1)
			}
		case reflect.Struct:
			if rv.Type() == reflect.TypeOf(time.Time{}) {
				return
			}
			for i := 0; i < rv.NumField(); i++ {
				name, field := rv.Type().Field(i).Name, rv.Field(i)
				switch {
				case skip[name]:
				case name == "BaseURL" && field.Kind() == reflect.String:
					out = append(out, URLUserinfoSecrets(field.String())...)
				case name == "Headers" && field.Kind() == reflect.Map:
					iter := field.MapRange()
					for iter.Next() {
						if !nonSecretHeader(iter.Key().String()) {
							walk(iter.Value(), depth+1)
							if iter.Value().Kind() == reflect.String {
								out = append(out, headerComponents(iter.Value().String())...)
							}
						}
					}
				default:
					walk(field, depth+1)
				}
			}
		}
	}
	walk(reflect.ValueOf(v), 0)
	return out
}

// hold registers every value a credentials file supplied (any length).
func (c *Credentials) hold() {
	HoldValues(CredentialStrings(c, "TokenExpiry")...)
}

var headerComponentRE = regexp.MustCompile(`[^\s,;=]+=([^,;]+)`)

// headerComponents is every part of a header value that can be a credential
// by itself: the value after a scheme word ("Bearer x") and every value of a
// "name=value" list ("sid=x; theme=y", `Digest username="u", response="r"`),
// unquoted. The shared funnel adds the halves of a base64 "id:secret" pair.
func headerComponents(value string) []string {
	var out []string
	if _, rest, ok := strings.Cut(strings.TrimSpace(value), " "); ok {
		out = append(out, strings.TrimSpace(rest))
	}
	for _, m := range headerComponentRE.FindAllStringSubmatch(value, -1) {
		if v := strings.Trim(strings.TrimSpace(m[1]), `"`); v != "" {
			out = append(out, v)
		}
	}
	return out
}
