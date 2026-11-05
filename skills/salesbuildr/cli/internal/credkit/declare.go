// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Contract 7: per-connector declarations. A connector fills in exactly one
// Declarations composite literal, in its one hand-written wiring file, and
// passes it to Start as the first statement of each main. Every field is
// plain data except Files, which must be a function literal or a
// package-level function (never a method value), so a go/types checker reads
// every declaration from that literal without running anything.

import (
	"fmt"
	"net/url"
	"strings"
)

// Source names where a held value came from. The kit and the connector use
// the same spellings:
//
//	env:NAME        an environment variable (declared in SecretEnv)
//	config:KEY      a key of a credential file (its last key, as written)
//	flag:--NAME     a command-line flag (declared in CredentialFlags)
//	stdin           a credential read from stdin (StdinFlags)
//	header:NAME     a request header
//	field:NAME      a request body field (form or JSON)
//	response:NAME   a vendor response field (ResponseFields)
//	cookie:NAME     a cookie, sent or set
//	mcp:NAME        an MCP tool argument (MCPArguments)
//	query:NAME      a query value inside base_url
//	userinfo        the user or password inside base_url
//	basic-user      the user part of a Basic credential
//	minted:NAME     a token the connector mints and holds itself (Hold)
//	*               the default, in Origins only
//
// A declared source names a value's source exactly; only a header name
// ignores case.
type Source = string

// Identifier exempts one source from holding, with the reason it is not a
// credential (contract 1). A value also held through any other source stays
// held.
type Identifier struct {
	Source Source
	Reason string
}

// Origin binds the credentials of one source to the origins they may be sent
// to (contract 2(c)). Each allowed entry is an origin ("https://api.example.com",
// "https://api.example.com:8443"), a subdomain wildcard
// ("https://*.example.com", which matches any subdomain of example.com on
// port 443 and not example.com itself), or the word "base_url", meaning the
// origin of the base URL the program is configured with (BaseURL). Source
// "*" is the default for every held value whose source has no entry of its
// own; a value bound to no origin may be sent nowhere.
type Origin struct {
	Source  Source
	Allowed []string
}

// BaseURL names where the configured base URL comes from: a credential-file
// key, an environment variable that overrides it, and the default.
type BaseURL struct {
	ConfigKey string
	Env       string
	Default   string
}

// Declarations is everything a connector declares about its credentials
// (contract 7). The kit holds what these name and nothing it would have to
// guess: an environment variable is held only when SecretEnv names it.
type Declarations struct {
	// SecretEnv: every environment variable whose value is a credential.
	SecretEnv []string
	// URLEnv: other environment variables that hold a URL (a token URL, a
	// feedback endpoint). The password and query values of each are held from
	// startup, as BaseURL.Env's are. A request that carries one needs an
	// allowed origin like any other (Origins: "userinfo", "query:NAME").
	URLEnv []string
	// Files returns every file a credential may be read from, given the
	// command line (os.Args[1:]): the config file the program will read,
	// its legacy paths, and each credential store file. It runs once, in
	// Start, before any output.
	Files func(args []string) []string
	// ConfigFields: the credential keys of those files. [headers] values,
	// auth_header and the credentials inside the base URL are held whether
	// or not they are listed.
	ConfigFields []string
	// StoredRecords: the keys of stored credential records (a credential
	// store file's token, refresh token, expiry-bound secrets).
	StoredRecords []string
	// CredentialFlags: flags whose value is a credential ("--token").
	CredentialFlags []string
	// StdinFlags: flags that make the program read a credential from stdin.
	StdinFlags []string
	// CredentialHeaders: request headers that carry a credential, besides
	// Authorization, Proxy-Authorization, Cookie and every header named in a
	// [headers] table, which are always held.
	CredentialHeaders []string
	// CredentialFields: form or JSON request body fields that carry one.
	CredentialFields []string
	// ResponseFields: vendor response fields that carry one (an access
	// token, a session id), held before the response reaches any sink.
	ResponseFields []string
	// MCPArguments: MCP tool arguments that carry one, held before dispatch.
	MCPArguments []string
	// BaseURL: where the configured base URL comes from.
	BaseURL BaseURL
	// Origins: each credential's allowed origins.
	Origins []Origin
	// Identifiers: sources exempt from holding, each with its reason.
	Identifiers []Identifier
}

// validate rejects declarations the kit cannot enforce. Start fails closed on
// an error here.
func (d *Declarations) validate() error {
	for _, id := range d.Identifiers {
		if strings.TrimSpace(id.Source) == "" || strings.TrimSpace(id.Reason) == "" {
			return fmt.Errorf("credkit: identifier %q is declared without a source or a reason", id.Source)
		}
	}
	for _, o := range d.Origins {
		for _, a := range o.Allowed {
			if a == "base_url" {
				continue
			}
			if _, err := parseAllowed(a); err != nil {
				return err
			}
		}
	}
	for _, f := range d.CredentialFlags {
		if !strings.HasPrefix(f, "-") {
			return fmt.Errorf("credkit: credential flag %q must be spelled with its dashes", f)
		}
	}
	return nil
}

// allowedOrigin is one parsed Origin entry.
type allowedOrigin struct {
	scheme, host, port string
	wildcard           bool
}

func parseAllowed(s string) (allowedOrigin, error) {
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" || u.Host == "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.User != nil {
		return allowedOrigin{}, fmt.Errorf("credkit: allowed origin %q is not scheme://host[:port]", s)
	}
	host := u.Hostname()
	wild := strings.HasPrefix(host, "*.")
	if wild {
		host = host[2:]
	}
	h, ok := canonicalHost(host)
	if !ok || strings.Contains(h, "*") {
		return allowedOrigin{}, fmt.Errorf("credkit: allowed origin %q must name its host in ASCII (Punycode for an international name)", s)
	}
	return allowedOrigin{scheme: strings.ToLower(u.Scheme), host: h, port: portOf(u), wildcard: wild}, nil
}
