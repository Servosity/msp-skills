// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import "fmt"

// HostTemplateLabelOK reports whether v may be substituted into the base URL's
// host: one ASCII DNS label (letters, digits, inner hyphens, 1 to 63
// characters). The host decides which server receives the credential, so a
// value that could add a host, path, port, user or encoding is refused
// (hand-fix handfixes.json host-template-single-label).
func HostTemplateLabelOK(v string) bool {
	if len(v) == 0 || len(v) > 63 || v[0] == '-' || v[len(v)-1] == '-' {
		return false
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

// checkHostTemplateValues refuses a base URL whose {placeholder} values are
// not single host labels, before anything is substituted.
func checkHostTemplateValues(baseURL string, vars map[string]string) error {
	for _, m := range templateVarPattern.FindAllStringSubmatch(baseURL, -1) {
		if v, ok := vars[m[1]]; ok && v != "" && !HostTemplateLabelOK(v) {
			return fmt.Errorf("template variable {%s} must be one host label such as eu2-cloud (letters, digits and hyphens only)", m[1])
		}
	}
	return nil
}
