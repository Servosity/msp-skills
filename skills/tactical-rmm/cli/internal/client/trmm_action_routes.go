// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): tactical-rmm handfix action-gets-need-confirm.

package client

// Tactical RMM GET routes that act (vendor-src/agents/views.py
// bulk_agent_recovery, vendor-src/core/views.py clear_cache) or dispatch a
// command to the managed agent (ping, AgentProcesses.get, get_event_log,
// browse_registry; vendor-src/services/views.py GetServices.get,
// GetEditActionService.get). The spec marks them x-pp-mutation, so their own
// commands call GetMutating. A plain Get is a read: it is refused when its
// path routes to one of these, however an id or parameter spells the path
// (Django decodes %2F before routing; dot segments may be resolved on the way).

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
)

var trmmActionRoutes = []*regexp.Regexp{
	regexp.MustCompile(`^/agents/bulkrecovery/?$`),
	regexp.MustCompile(`^/core/clearcache/?$`),
	regexp.MustCompile(`^/agents/[^/]+/ping/?$`),
	regexp.MustCompile(`^/agents/[^/]+/processes(/[^/]+)?/?$`),
	regexp.MustCompile(`^/agents/[^/]+/eventlog/[^/]+/[^/]+/?$`),
	regexp.MustCompile(`^/agents/[^/]+/registry/?$`),
	regexp.MustCompile(`^/services/[^/]+(/[^/]+)?/?$`),
}

// refuseReadOfActionRoute fails closed: a path that cannot be decoded is refused.
func refuseReadOfActionRoute(method, p string, mutationIntent bool) error {
	if mutationIntent || !strings.EqualFold(method, "GET") {
		return nil
	}
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	decoded := p
	for n := 0; n < 4; n++ {
		next, err := url.PathUnescape(decoded)
		if err != nil {
			return fmt.Errorf("refusing GET %s: the path does not decode", p)
		}
		if next == decoded {
			break
		}
		decoded = next
	}
	cleaned := path.Clean("/" + decoded)
	for _, form := range []string{decoded, cleaned, cleaned + "/"} {
		for _, re := range trmmActionRoutes {
			if re.MatchString(form) {
				return fmt.Errorf("refusing GET %s: it routes to a Tactical RMM action (%s); use that action's own command", p, form)
			}
		}
	}
	return nil
}
