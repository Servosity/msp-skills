// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"fmt"
	"regexp"
	"strings"
)

// Template values ({site}, {cloudId}, {workspaceId}) are spliced into the
// host and path raw. A value carrying "/", "@", "#", "?", ":" or ".." would
// move the request, Basic credentials included, to another host or endpoint
// (reachable from an agent via sync --path-context). Allow only the shapes
// Atlassian uses: subdomains and UUID-like ids.
var jsmSafeTemplateValue = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func jsmCheckTemplateVars(full string, vars map[string]string) error {
	for _, m := range templateVarPattern.FindAllString(full, -1) {
		key := m[1 : len(m)-1]
		v, ok := vars[key]
		if !ok || v == "" {
			continue // unresolved: buildURL reports the env var to set
		}
		if !jsmSafeTemplateValue.MatchString(v) || strings.Contains(v, "..") {
			name := templateVarEnvNames[key]
			if name == "" {
				name = "{" + key + "}"
			}
			return fmt.Errorf("%s value %q is not a valid identifier (letters, digits, '.', '_' or '-' only); for the site use the subdomain, e.g. acme for acme.atlassian.net", name, v)
		}
	}
	return nil
}
