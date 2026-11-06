// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// nerdio handfix tenant-secrets-shown-never-stored: a device's LAPS password,
// its BitLocker recovery keys and secure variable values are customer
// secrets the operator asks for by name (devices laps, devices
// bitlocker-keys, secure-variables list). They are not this connector's
// credentials, so the shared credential kit does not hold them: the command
// exists to show them, and it shows them on stdout and in MCP results.
// Every other sink withholds a value a tenant-secret answer carried in this
// process (registered here by the response guard, internal/cli/nerdio_guard.go):
// stderr (the command error filter), every file write (WriteFile and the
// writers in nerdio_tenant_sinks.go) and the local store's SQL writes
// (internal/credsql), and the MCP server refuses a child argv that repeats one.
// An MCP child's stdout does not answer the caller (the server reads it), so a
// child holds each one with the kit instead, which withholds it everywhere.

import (
	"encoding/json"
	"net/url"
	"strings"
	"sync"
)

var tenantSecretRegistry = struct {
	sync.RWMutex
	set map[string]bool
}{set: map[string]bool{}}

// ShowTenantSecrets registers the tenant secrets a response body carries:
// every string or number under a credential-named key, and under any of
// valueNames (the secure variables' "value"), at any depth.
func ShowTenantSecrets(body []byte, valueNames ...string) {
	showTenantSecrets(body, true, valueNames...)
}

// ShowTenantSecretMembers registers the secret members of a tenant-secret
// path's error answer (Codex r2 class tenant secret written to disk, fixed
// after the stop rule): what sits under a credential-named or value-named
// member, JSON or markup, never every word of a text body (an error page is
// not the secret). The guard also holds the answer's credential-named
// members, so nothing on these paths is ever both unheld and unregistered.
func ShowTenantSecretMembers(body []byte, valueNames ...string) {
	showTenantSecrets(body, false, valueNames...)
}

func showTenantSecrets(body []byte, everyWord bool, valueNames ...string) {
	named := map[string]bool{}
	for _, n := range valueNames {
		named[normalizedFieldName(n)] = true
	}
	var doc any
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()
	if dec.Decode(&doc) != nil {
		showTenantSecretsText(string(body), named, everyWord)
		return
	}
	var walk func(v any, secret bool)
	walk = func(v any, secret bool) {
		switch t := v.(type) {
		case map[string]any:
			for k, child := range t {
				walk(child, secret || SecretFieldName(k) || named[normalizedFieldName(k)])
			}
		case []any:
			for _, child := range t {
				walk(child, secret)
			}
		case string:
			if secret {
				registerTenantSecret(t)
			}
		case json.Number:
			if secret {
				registerTenantSecret(t.String())
			}
		}
	}
	walk(doc, false)
}

// showTenantSecretsText registers the tenant secrets of a body that is not
// JSON (Codex review r1, the markup family of class tenant secret written to
// disk: such a body registered nothing, so a file sink wrote it clear):
// everything under a credential-named or value-named markup element, at any
// depth (walkMarkupTree), and every credential- or value-named "Name: value"
// line and parameter. A body that names no such member is the secret itself
// (a bare text answer): every word of it is registered, so the paths that do
// not answer the caller withhold it (fail closed).
func showTenantSecretsText(s string, named map[string]bool, everyWord bool) {
	found := false
	reg := func(v string) {
		for _, f := range []string{v, strings.Trim(v, `"'`)} {
			if f = strings.TrimSpace(f); f != "" && !isMaskValue(f) {
				registerTenantSecret(f)
				found = true
			}
		}
	}
	isName := func(n string) bool { return SecretFieldName(n) || named[normalizedFieldName(n)] }
	if strings.Contains(s, "<") {
		walkMarkupTree(s, isName, reg)
	}
	for _, m := range embeddedHeader.FindAllStringSubmatch(s, -1) {
		if isName(m[1]) {
			reg(m[2])
		}
	}
	for _, m := range embeddedParam.FindAllStringSubmatch(s, -1) {
		k := m[1]
		if dk, err := url.QueryUnescape(k); err == nil {
			k = dk
		}
		if !isName(k) {
			continue
		}
		reg(m[2])
		if dv, err := url.QueryUnescape(strings.Trim(m[2], `"`)); err == nil {
			reg(dv)
		}
	}
	if !found && everyWord {
		for _, w := range strings.Fields(s) {
			reg(w)
		}
	}
}

func registerTenantSecret(v string) {
	v = strings.TrimSpace(v)
	if v == "" || isMaskValue(v) {
		return
	}
	tenantSecretRegistry.Lock()
	tenantSecretRegistry.set[v] = true
	tenantSecretRegistry.Unlock()
	if IsMCPChildEnv() {
		Hold("minted:tenant-secret", v)
	}
}

// TenantSecretEchoes reports text that repeats a registered tenant secret,
// at any length (fail closed: a very short one withholds every text that
// repeats it on the paths that do not answer the caller), as written or in
// any form Echoes reads.
func TenantSecretEchoes(text string) bool {
	tenantSecretRegistry.RLock()
	all := make([]string, 0, len(tenantSecretRegistry.set))
	for s := range tenantSecretRegistry.set {
		all = append(all, s)
	}
	tenantSecretRegistry.RUnlock()
	return len(all) > 0 && text != "" && Echoes(text, all)
}

// ArgvRepeatsSecret reports a child process's argv that repeats a tenant
// secret this process showed (nerdio handfix tenant-secrets-shown-never-stored): an
// MCP caller handed a LAPS password could otherwise pass it back as a tool
// argument to a mirrored command that writes a file (teach --notes,
// feedback). A held credential in a child argv is the kit's to refuse
// (credkit.StartCommand).
func ArgvRepeatsSecret(text string) bool {
	return TenantSecretEchoes(text)
}
