// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generated). The ImmyBot GET endpoints that change state:
// the one declared list every surface consults. A GET here is a write: the
// MCP read tool refuses it, immybot_execute runs it only with confirm, the
// read-only switch refuses it, the client never answers it from or writes it
// to the response cache, and the CLI never answers it from or files it in the
// local store. The press marks a GET mutating only from its last path word,
// so it missed the action words these paths carry elsewhere (spec.json paths).
// See handfixes.json: mutating-gets-are-writes.

package cliutil

import (
	"fmt"
	"path"
	"strings"
)

var mutatingGETPaths = []string{
	// Cancels a maintenance session.
	"/api/v1/maintenance-sessions/{sessionId}/cancel",
	// Cancels the sessions a schedule started (operationId getCancelForScheduleByScheduleId).
	"/api/v1/maintenance-sessions/cancel-for-schedule/{scheduleId}",
	// The maintenance-email links: reboot the computer now, run maintenance now, postpone it.
	"/api/v1/maintenance-emails/{emailGuid}/jobs/rebootnow",
	"/api/v1/maintenance-emails/{emailGuid}/jobs/now",
	"/api/v1/maintenance-emails/{emailGuid}/jobs/postpone",
	// Launches an ephemeral agent on the computer (200 is an OpResult).
	"/api/v1/computers/{computerId}/launch-ephemeral-agent",
	// Refreshes, starts and ends the signed-in user session.
	"/auth/refresh",
	"/auth/login",
	"/auth/logout",
	// The rest leave their 200 response undocumented, so they fail closed:
	// the inbound webhook (getById beside createById, no list), an ephemeral
	// agent's session, a started language service, and a provider plugin's
	// passthrough, which can be any request.
	"/api/v1/webhooks/{id}",
	"/api/v1/ephemeral-session/{ephemeralSessionId}/{agentInstanceId}/{providerAgentId}",
	"/api/v1/scripts/language-service/{terminalId}/language",
	"/plugins/api/v1/{providerLinkId}/{catchAll}",
}

// MutatingGETPaths returns the declared list.
func MutatingGETPaths() []string { return append([]string(nil), mutatingGETPaths...) }

// IsMutatingGETPath reports whether raw (a template or a concrete path, with
// or without a query) is a declared state-changing GET. A {catchAll}
// segment takes the rest of the path.
func IsMutatingGETPath(raw string) bool {
	for _, template := range mutatingGETPaths {
		if pathMatchesTemplate(raw, template) {
			return true
		}
	}
	return false
}

// MutatingGETRedirectError refuses a redirect whose target is a declared
// state-changing GET, whatever started the chain: the vendor chose that hop,
// nobody confirmed it.
func MutatingGETRedirectError(target string) error {
	if IsMutatingGETPath(target) {
		return fmt.Errorf("refused a redirect to %s: that GET changes state and a redirect cannot confirm it", target)
	}
	return nil
}

func pathMatchesTemplate(raw, template string) bool {
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	// A server resolves dot segments and repeated slashes, so the match does too.
	got := strings.Split(strings.Trim(path.Clean("/"+raw), "/"), "/")
	want := strings.Split(strings.Trim(template, "/"), "/")
	if last := len(want) - 1; want[last] == "{catchAll}" && len(got) > len(want) {
		got = append(got[:last:last], strings.Join(got[last:], "/")) // the passthrough takes the rest of the path
	}
	if len(got) != len(want) {
		return false
	}
	for i, w := range want {
		if strings.HasPrefix(w, "{") && strings.HasSuffix(w, "}") {
			if got[i] == "" {
				return false
			}
			continue
		}
		if !strings.EqualFold(got[i], w) {
			return false
		}
	}
	return true
}
